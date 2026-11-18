package speech

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"runtime"
	"slices"
	"unsafe"

	"github.com/ardanlabs/kronk/sdk/applog"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
)

// AutoregressiveContextWindow is the backbone capacity of each speech sequence.
const AutoregressiveContextWindow = 2048

// AutoregressiveBatchSize bounds each prefill step and its native batch capacity.
const AutoregressiveBatchSize = 512

// ErrContextLimit indicates that text and conditioning exceed the prompt budget.
var ErrContextLimit = errors.New("speech text and voice conditioning exceed the context budget")

// ErrFrameLimit indicates that generation did not reach EOS within its frame cap.
var ErrFrameLimit = errors.New("speech frame limit reached")

// Output contains caller-owned WAV audio and its per-channel sample metadata.
type Output struct {
	Audio      []byte
	SampleRate int
	Samples    int
	Stereo     bool
}

// AutoregressiveConfig supplies borrowed native handles and immutable sequence
// settings. The handles and default speaker reference must outlive the protocol.
type AutoregressiveConfig struct {
	Model          llama.Model
	Context        llama.Context
	Memory         llama.Memory
	Decoder        mtmd.Context
	Input          mtmd.HelperGenAudioInput
	SuppressTokens []llama.Token
	MaxFrames      int
}

// Autoregressive owns one sequence's helper, sampler, and generation state for
// the Qwen3-TTS/Chatterbox protocol. Calls across sequences sharing native handles
// must be serialized by the scheduler. It does not own jobs or public API types.
type Autoregressive struct {
	log         applog.Logger
	lctx        llama.Context
	mem         llama.Memory
	vocab       llama.Vocab
	outputWidth int32
	sequence    llama.SeqId
	helper      mtmd.HelperGenAudio
	sampler     llama.Sampler
	maxFrames   int
	prefilling  bool
	frames      int
	token       llama.Token
	hidden      []float32
}

// NewAutoregressive creates a sequence pipeline. Free releases its owned native
// resources; the supplied backbone and decoder remain owned by the caller.
func NewAutoregressive(ctx context.Context, log applog.Logger, cfg AutoregressiveConfig) (*Autoregressive, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	helper, err := mtmd.HelperGenAudioInit(cfg.Context, cfg.Decoder)
	if err != nil {
		return nil, err
	}

	vocab := llama.ModelGetVocab(cfg.Model)
	sampler := llama.SamplerChainInit(llama.SamplerChainDefaultParams())
	var biases []llama.LogitBias
	for _, token := range cfg.SuppressTokens {
		biases = append(biases, llama.LogitBias{Token: token, Bias: float32(math.Inf(-1))})
	}
	if len(biases) > 0 {
		llama.SamplerChainAdd(sampler, llama.SamplerInitLogitBias(llama.VocabNTokens(vocab), int32(len(biases)), unsafe.SliceData(biases)))
	}
	llama.SamplerChainAdd(sampler, llama.SamplerInitPenalties(llama.VocabNTokens(vocab), 4096, 1.05, 0, 0))
	llama.SamplerChainAdd(sampler, llama.SamplerInitTopK(cfg.Input.TopK))
	llama.SamplerChainAdd(sampler, llama.SamplerInitTemp(cfg.Input.Temp))
	llama.SamplerChainAdd(sampler, llama.SamplerInitDist(cfg.Input.Seed))
	runtime.KeepAlive(biases)

	a := Autoregressive{
		log:         log,
		lctx:        cfg.Context,
		mem:         cfg.Memory,
		vocab:       vocab,
		outputWidth: llama.ModelNEmbdOut(cfg.Model),
		sequence:    cfg.Input.SeqID,
		helper:      helper,
		sampler:     sampler,
		maxFrames:   cfg.MaxFrames,
	}
	return &a, nil
}

// Start prepares native conditioning and starts prefill. The scheduler must reset
// the sequence after every request, including one whose preparation fails.
func (a *Autoregressive) Start(input *mtmd.HelperGenAudioInput) error {
	if err := mtmd.HelperGenAudioSetInput(a.helper, input); err != nil {
		return err
	}
	llama.SamplerReset(a.sampler)
	a.prefilling = true
	return nil
}

// Step advances one bounded prefill or generation step. The booleans indicate
// completion and a fatal native failure, respectively. Request-local limits and
// cancellation leave the other sequences usable after this sequence is reset.
func (a *Autoregressive) Step(ctx context.Context, promptTokens int) (Output, bool, bool, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, false, false, err
	}
	if a.prefilling {
		remaining, err := mtmd.HelperGenAudioStepPrompt(a.helper, AutoregressiveBatchSize)
		if err != nil {
			return Output{}, false, true, fmt.Errorf("speech prompt decoding: %w", err)
		}
		pos, err := llama.MemorySeqPosMax(a.mem, a.sequence)
		if err != nil {
			return Output{}, false, true, err
		}
		// The first native batch fits independently. Its remaining count lets
		// us include actual voice conditioning before decoding any more rows.
		if int(pos)+1+int(remaining)+a.maxFrames > AutoregressiveContextWindow {
			return Output{}, false, false, ErrContextLimit
		}
		if remaining > 0 {
			return Output{}, false, false, nil
		}
		hidden, err := llama.GetEmbeddingsIth(a.lctx, -1, a.outputWidth)
		if err != nil {
			return Output{}, false, true, err
		}
		// The shared context's output is overwritten by the next sequence.
		a.hidden = slices.Clone(hidden)
		a.prefilling = false
		a.sample()
		a.log(ctx, "speech", "status", "prefill-complete", "sequence", a.sequence, "prompt_tokens", promptTokens)
		return Output{}, false, false, nil
	}

	// Permit EOS at the exact cap, but never decode another frame past it.
	if a.frames >= a.maxFrames && !llama.VocabIsEOG(a.vocab, a.token) {
		return Output{}, false, false, ErrFrameLimit
	}
	next, stop, err := mtmd.HelperGenAudioStepGen(a.helper, a.token, a.hidden)
	if err != nil {
		return Output{}, false, true, err
	}
	if stop {
		if err := ctx.Err(); err != nil {
			return Output{}, false, false, err
		}
		output, err := mtmd.HelperGenAudioGetOutput(a.helper)
		if err != nil {
			return Output{}, false, true, err
		}
		data, rate, samples := output.Data, output.SampleRate, output.NSamples
		if len(data) < 44 || samples <= 0 || rate <= 0 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" ||
			binary.LittleEndian.Uint16(data[20:22]) != 1 || binary.LittleEndian.Uint16(data[22:24]) != 1 ||
			binary.LittleEndian.Uint32(data[24:28]) != uint32(rate) || binary.LittleEndian.Uint16(data[34:36]) != 16 ||
			samples != int64((len(data)-44)/2) || int64(binary.LittleEndian.Uint32(data[40:44])) != samples*2 {
			return Output{}, false, true, errors.New("speech decoder returned invalid PCM16 mono WAV")
		}
		return Output{Audio: data, SampleRate: int(rate), Samples: int(samples)}, true, false, nil
	}
	if next == nil {
		return Output{}, false, true, errors.New("speech generation ended without EOS")
	}

	a.frames++
	a.hidden = next
	a.sample()
	return Output{}, false, false, nil
}

func (a *Autoregressive) sample() {
	// Capture the token before another sequence replaces the shared logits.
	a.token = llama.SamplerSample(a.sampler, a.lctx, -1)
	llama.SamplerAccept(a.sampler, a.token)
}

// Reset clears only this sequence's native state and per-request buffers.
func (a *Autoregressive) Reset() error {
	mtmd.HelperGenAudioReset(a.helper)
	_, err := llama.MemorySeqRm(a.mem, a.sequence, 0, -1)
	a.prefilling = false
	a.frames, a.token = 0, 0
	a.hidden = nil
	return err
}

// Free releases the sampler and helper without freeing borrowed native handles.
// The scheduler must finish active work before calling Free.
func (a *Autoregressive) Free() {
	llama.SamplerFree(a.sampler)
	mtmd.HelperGenAudioFree(a.helper)
	a.sampler, a.helper = 0, 0
	a.hidden = nil
}
