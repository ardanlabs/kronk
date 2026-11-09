// This example shows how to generate speech using the raw yzma llama and MTMD
// APIs. It loads Chatterbox Turbo once, then tests one, two, and four active
// requests. Each configuration shares one backbone context and audio decoder.
//
// Each request owns its helper, sampler, pending token, and hidden state. One
// owner alternates native steps, sampling immediately after each decode before
// another sequence overwrites the context's logits. Native calls are serialized;
// this proves overlapping request lifetimes, not combined decode batches.
//
// Serial runs provide reference audio and token streams. The example checks
// interleaved requests, reversed scheduling, and replacement of an unfinished
// request while its neighbor continues. Successful output must match the serial
// reference byte for byte. It writes speech-0.wav through speech-3.wav in examples/.
// Chatterbox Turbo's waveform generation is one-shot, not incremental playback.
// Reaching the frame limit returns an error rather than writing truncated audio.
// A separate diagnostic deliberately flushes capped speech to measure the full
// 512-frame decoder workspace; that incomplete audio is never written to disk.
//
// Checkpoints report process RSS (using ps) and Go heap; a 10 ms sampler also
// records backend device usage. All sizes are bytes. Sampled device peaks are
// lower bounds; use /usr/bin/time -l on macOS for the kernel's process RSS peak.
// Metal reports this process's allocated GPU buffers, which overlap process
// RAM: do not add RSS and Metal usage together.
// Other backends can report device-wide usage rather than process allocations.
// Each context reserves 2048 tokens per slot, with separate KV streams and one
// shared compute context. Unified KV passed simple interleaving but produced
// small logit differences after four-slot replacement on the tested Metal build;
// separate streams passed the stricter byte-for-byte replacement check too.
// These are measurements, not a portable VRAM formula or a throughput benchmark.
//
// Download the model and decoder first:
// $ kronk model pull https://huggingface.co/ggml-org/chatterbox-turbo-GGUF/resolve/main/chatterbox-turbo-Q8_0.gguf --proj https://huggingface.co/ggml-org/chatterbox-turbo-GGUF/resolve/main/mmproj-chatterbox-turbo-F16.gguf --local
// Kronk names the stored F16 projector after the backbone's Q8_0 suffix.
//
// Run the example like this from the root of the project:
// $ make example-yzma-step10
// Play the results on macOS:
// $ afplay examples/speech-0.wav
// $ afplay examples/speech-1.wav

package main

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
	"github.com/hybridgroup/yzma/pkg/utils"
)

const (
	maxFrames      = 512
	contextPerSlot = 2048
)

var errFrameLimit = errors.New("speech frame limit reached")

type speechSlot struct {
	lctx    llama.Context
	gen     mtmd.HelperGenAudio
	input   mtmd.HelperGenAudioInput
	sampler llama.Sampler
	nEmbd   int32
	token   llama.Token
	hidden  *float32
	tokens  []llama.Token
	frames  int
	done    bool
	output  speechResult
	memory  *memoryMonitor
}

type speechResult struct {
	audio   []byte
	tokens  []llama.Token
	rate    int32
	samples int64
}

func main() {
	if err := run(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}

func run() error {
	if err := initYzma(); err != nil {
		return fmt.Errorf("unable to init yzma: %w", err)
	}
	defer llama.Close()

	memory := newMemoryMonitor()
	memory.mark("baseline")
	err := runExperiments(memory)
	runtime.GC()
	memory.mark("all-native-resources-freed")
	return errors.Join(err, memory.close())
}

func runExperiments(memory *memoryMonitor) error {
	// =========================================================================
	// Load the speech backbone and matching audio decoder once.

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("unable to get home dir: %w", err)
	}
	modelFile := filepath.Join(home, ".kronk/models/ggml-org/chatterbox-turbo-GGUF/chatterbox-turbo-Q8_0.gguf")
	projFile := filepath.Join(home, ".kronk/models/ggml-org/chatterbox-turbo-GGUF/mmproj-chatterbox-turbo-Q8_0.gguf")
	fmt.Println("Loading model:", modelFile)

	mdl, err := llama.ModelLoadFromFile(modelFile, llama.ModelDefaultParams())
	if err != nil {
		return err
	}
	defer llama.ModelFree(mdl)
	memory.mark("backbone-loaded")
	cacheBytes := int64(4) * int64(llama.ModelNEmbd(mdl)) * int64(llama.VocabNTokens(llama.ModelGetVocab(mdl)))
	fmt.Printf("Backbone weights=%d bytes; native F32 embedding copy=%d bytes PER HELPER\n", llama.ModelSize(mdl), cacheBytes)
	for _, nSlots := range []int{1, 2, 4} {
		fmt.Printf("\nCASE slots=%d aggregate_context=%d batch=512 max_frames=%d\n", nSlots, contextPerSlot*nSlots, maxFrames)
		if err := runCase(mdl, projFile, nSlots, memory); err != nil {
			return fmt.Errorf("slots %d: %w", nSlots, err)
		}
		runtime.GC()
		memory.mark(fmt.Sprintf("slots-%d-contexts-and-helpers-freed", nSlots))
	}
	return nil
}

func runCase(mdl llama.Model, projFile string, nSlots int, memory *memoryMonitor) error {
	cparams := llama.ContextDefaultParams()
	cparams.NCtx, cparams.NBatch, cparams.NUbatch, cparams.NSeqMax = uint32(contextPerSlot*nSlots), 512, 512, uint32(nSlots)
	cparams.KVUnified = 0  // Stable per-sequence placement when another slot resets.
	cparams.Embeddings = 1 // The native helper reads per-token backbone embeddings.
	lctx, err := llama.InitFromModel(mdl, cparams)
	if err != nil {
		return err
	}
	defer llama.Free(lctx)
	memory.mark("backbone-context-created")

	mctx, err := mtmd.InitFromFile(projFile, mdl, mtmd.ContextParamsDefault())
	if err != nil {
		return err
	}
	defer mtmd.Free(mctx)
	memory.mark("decoder-loaded")
	caps, err := mtmd.HelperGetCapabilities(mdl, mctx)
	if err != nil {
		return err
	}
	if !caps.AudioGen {
		return errors.New("model and projector do not support audio generation")
	}

	// =========================================================================
	// Share model and decoder weights, but keep each sequence's request state.

	voice, err := mtmd.GenAudioGetVoice(mctx, "default")
	if err != nil {
		return err
	}
	defer mtmd.BitmapFree(voice)

	slots := make([]*speechSlot, nSlots)
	for i := range slots {
		slots[i], err = newSpeechSlot(mdl, lctx, mctx, voice, llama.SeqId(i))
		if err != nil {
			return err
		}
		slots[i].memory = memory
		defer slots[i].close()
	}
	memory.mark("helpers-created-before-lazy-caches")

	prompts := []string{
		"I played column four. Your turn!",
		"You won the game. Well played!",
		"Watch out for the diagonal. You can still block it in column three.",
		"Welcome to Connect Four. We take turns dropping a piece into one of the seven columns. Try to connect four pieces across, down, or diagonally. Think carefully about your next move, and remember to watch what your opponent is planning. Good luck, and have fun!",
		"My turn.",
	}

	// Touch every helper's lazy cache before serial references can warm only a
	// subset. Each helper retains its own F32 copy of the token embedding table.
	for i, slot := range slots {
		if err := slot.start(prompts[i]); err != nil {
			return err
		}
		memory.mark(fmt.Sprintf("helper-%d-first-prefill", i))
	}

	// =========================================================================
	// Generate independent references before admitting overlapping requests.

	serial := make([]speechResult, len(prompts))
	for i, prompt := range prompts {
		slot := slots[i%len(slots)]
		if err := slot.start(prompt); err != nil {
			return err
		}
		if _, err := generateSpeech(slot); err != nil {
			return err
		}
		serial[i] = slot.output
		fmt.Printf("Serial %d: %q, %d frames, %.2fs audio\n", i, prompt, slot.frames, float64(serial[i].samples)/float64(serial[i].rate))
	}
	memory.mark("serial-references-completed")
	if bytes.Equal(serial[0].audio, serial[1].audio) {
		return errors.New("different prompts returned identical audio")
	}

	// =========================================================================
	// Keep all requests live and alternate their decode steps in both orders.

	for round := range 2 {
		for i, slot := range slots {
			if err := slot.start(prompts[i]); err != nil {
				return err
			}
		}
		order := slices.Clone(slots)
		if round == 1 {
			slices.Reverse(order)
		}
		start := time.Now()
		overlapped, err := generateSpeech(order...)
		if err != nil {
			return err
		}
		if nSlots > 1 && overlapped == 0 {
			return errors.New("requests never overlapped")
		}
		for i, slot := range slots {
			result := slot.output
			if err := compareSpeech(serial[i], result); err != nil {
				return fmt.Errorf("interleaved round %d sequence %d: %w", round, i, err)
			}
			if err := os.WriteFile(fmt.Sprintf("speech-%d.wav", i), result.audio, 0644); err != nil {
				return err
			}
		}
		fmt.Printf("Interleaved round %d: %d overlapping rounds, all %d outputs match serial, %s\n", round, overlapped, nSlots, time.Since(start).Round(time.Millisecond))
		memory.mark(fmt.Sprintf("interleaved-round-%d", round))
	}

	// =========================================================================
	// Replace one unfinished request without clearing its neighbor's sequence.

	for i, slot := range slots {
		if err := slot.start(prompts[i]); err != nil {
			return err
		}
	}
	for range 7 {
		for _, slot := range slots {
			if err := slot.step(); err != nil {
				return err
			}
			if slot.done {
				return errors.New("request finished before the replacement check")
			}
		}
	}
	if err := slots[0].start(prompts[4]); err != nil {
		return err
	}
	if _, err := generateSpeech(slots[:]...); err != nil {
		return err
	}
	for i := range slots {
		baseline := i
		if i == 0 {
			baseline = 4
		}
		if err := compareSpeech(serial[baseline], slots[i].output); err != nil {
			return fmt.Errorf("replacement sequence %d: %w", i, err)
		}
	}
	fmt.Println("Replacement: new request and all continuing neighbors match serial")
	memory.mark("replacement-completed")

	// A ceiling probe, NOT a successful speech request: stop at the frame cap
	// and explicitly flush the partial codes solely to size decoder workspace.
	// Keep every slot's full-length output resident to expose per-slot RAM.
	longPrompt := strings.Repeat("We are playing Connect Four together. Take your time to look at the board before choosing a column. You can win by connecting four pieces in a row, but you also need to block your opponent. Think about the center of the board and watch for diagonal threats. ", 3)
	for _, slot := range slots {
		if err := slot.start(longPrompt); err != nil {
			return err
		}
	}
	memory.mark("ceiling-probe-prefilled")
	for frame := range maxFrames {
		for _, slot := range slots {
			if err := slot.step(); err != nil {
				return err
			}
			if slot.done {
				return fmt.Errorf("ceiling probe ended early at %d frames", slot.frames)
			}
		}
		if frame == 127 || frame == 255 || frame == maxFrames-1 {
			memory.mark(fmt.Sprintf("ceiling-probe-%d-frames-per-slot", frame+1))
		}
	}
	for _, slot := range slots {
		if err := slot.step(); !errors.Is(err, errFrameLimit) {
			return fmt.Errorf("frame limit: got %v, want %w", err, errFrameLimit)
		}
		memory.mark(fmt.Sprintf("seq-%d-before-ceiling-flush", slot.input.SeqID))
		if err := mtmd.HelperGenAudioFlush(slot.gen); err != nil {
			return err
		}
		memory.mark(fmt.Sprintf("seq-%d-after-ceiling-flush", slot.input.SeqID))
		if err := slot.collect(); err != nil {
			return err
		}
		fmt.Printf("Diagnostic ONLY: seq=%d frames=%d audio=%.2fs bytes=%d; capped output not saved\n", slot.input.SeqID, slot.frames, float64(slot.output.samples)/float64(slot.output.rate), len(slot.output.audio))
	}
	memory.mark("ceiling-probe-all-audio-resident")
	// Full physical prefill batches can coexist with retained decoder arenas.
	// Measure that combination too, not just the separate phase maxima. This
	// longer input is a prefill-only allocation probe, not a speech-quality test.
	for _, slot := range slots {
		if err := slot.start(strings.Repeat(longPrompt, 4)); err != nil {
			return err
		}
	}
	memory.mark("ceiling-probe-warm-long-prefill")
	// Reuse after the largest buffers were allocated, checking both correctness
	// and the retained memory that a pooled idle model must continue reserving.
	for round := range 2 {
		for i, slot := range slots {
			if err := slot.start(prompts[i]); err != nil {
				return err
			}
		}
		if _, err := generateSpeech(slots...); err != nil {
			return err
		}
		for i, slot := range slots {
			if err := compareSpeech(serial[i], slot.output); err != nil {
				return fmt.Errorf("post-ceiling reuse %d sequence %d: %w", round, i, err)
			}
		}
		memory.mark(fmt.Sprintf("post-ceiling-reuse-%d", round))
	}
	fmt.Printf("PASS: slots=%d, 1 shared backbone context and decoder, serial equality, replacement, frame cap, and warm reuse\n", nSlots)
	return nil
}

func newSpeechSlot(mdl llama.Model, lctx llama.Context, mctx mtmd.Context, voice mtmd.Bitmap, seqID llama.SeqId) (*speechSlot, error) {
	input, err := mtmd.HelperGenAudioInputDefault()
	if err != nil {
		return nil, err
	}
	input.SeqID = seqID
	input.SpeakerRef = voice
	input.OutType = mtmd.GenAudioOutputWAV
	input.Seed, input.Temp = 42, 0.8
	gen, err := mtmd.HelperGenAudioInit(lctx, mctx)
	if err != nil {
		return nil, err
	}

	sampler := llama.SamplerChainInit(llama.SamplerChainDefaultParams())
	vocab := llama.ModelGetVocab(mdl)
	// Honor the model's suppressed tokens before sampling speech codes.
	var biases []llama.LogitBias
	for _, token := range llama.VocabGetSuppressTokens(vocab) {
		biases = append(biases, llama.LogitBias{Token: token, Bias: float32(math.Inf(-1))})
	}
	if len(biases) > 0 {
		llama.SamplerChainAdd(sampler, llama.SamplerInitLogitBias(llama.VocabNTokens(vocab), int32(len(biases)), unsafe.SliceData(biases)))
	}
	llama.SamplerChainAdd(sampler, llama.SamplerInitPenalties(llama.VocabNTokens(vocab), 4096, 1.05, 0, 0))
	llama.SamplerChainAdd(sampler, llama.SamplerInitTopK(input.TopK))
	llama.SamplerChainAdd(sampler, llama.SamplerInitTemp(input.Temp))
	llama.SamplerChainAdd(sampler, llama.SamplerInitDist(input.Seed))

	slot := speechSlot{
		lctx:    lctx,
		gen:     gen,
		input:   input,
		sampler: sampler,
		nEmbd:   llama.ModelNEmbdOut(mdl),
	}
	return &slot, nil
}

func (s *speechSlot) close() {
	llama.SamplerFree(s.sampler)
	mtmd.HelperGenAudioFree(s.gen)
}

func (s *speechSlot) start(prompt string) error {
	input := s.input
	var err error
	input.Prompt, err = utils.BytePtrFromString(prompt)
	if err != nil {
		return err
	}
	input.PromptLen = uint64(len(prompt))
	// SetInput resets this helper and removes only input.SeqID's KV entries.
	if err := mtmd.HelperGenAudioSetInput(s.gen, input); err != nil {
		return err
	}
	llama.SamplerReset(s.sampler)
	s.frames, s.done, s.hidden, s.tokens = 0, false, nil, nil
	s.output = speechResult{}
	batches := 0
	for {
		remaining := mtmd.HelperGenAudioStepPrompt(s.gen, 512)
		if remaining < 0 {
			return fmt.Errorf("sequence %d prompt processing failed: %d", input.SeqID, remaining)
		}
		batches++
		if remaining == 0 {
			break
		}
	}
	if batches > 1 {
		fmt.Printf("Prefill seq=%d text_bytes=%d batches=%d (capacity 512)\n", input.SeqID, len(prompt), batches)
	}
	hidden, err := llama.GetEmbeddingsIth(s.lctx, -1, s.nEmbd)
	if err != nil {
		return err
	}
	// Prefill embeddings are borrowed from the shared context, so keep a copy.
	s.hidden = unsafe.SliceData(slices.Clone(hidden))
	s.sample()
	return nil
}

func (s *speechSlot) sample() {
	// Capture the next token before another sequence replaces the last logits.
	s.token = llama.SamplerSample(s.sampler, s.lctx, -1)
	llama.SamplerAccept(s.sampler, s.token)
}

func (s *speechSlot) step() error {
	if s.frames >= maxFrames {
		return fmt.Errorf("sequence %d reached %d frames: %w", s.input.SeqID, maxFrames, errFrameLimit)
	}
	s.tokens = append(s.tokens, s.token)
	next, stop, err := mtmd.HelperGenAudioStepGen(s.gen, s.token, s.hidden)
	if err != nil {
		return err
	}
	s.done = stop
	if next != nil {
		s.frames++
		// This pointer belongs to this helper, not the shared context.
		s.hidden = next
	}
	if stop {
		return nil
	}
	if next == nil {
		return errors.New("speech generation ended without an end-of-speech marker")
	}
	s.sample()
	return nil
}

func generateSpeech(slots ...*speechSlot) (int, error) {
	overlapped := 0
	for {
		active := 0
		for _, slot := range slots {
			if slot.done {
				continue
			}
			active++
			if err := slot.step(); err != nil {
				return overlapped, err
			}
			if slot.done {
				// Vocode now, while the other sequence can still be decoding.
				if err := slot.collect(); err != nil {
					return overlapped, err
				}
			}
		}
		if active == 0 {
			return overlapped, nil
		}
		if active > 1 {
			overlapped++
		}
	}
}

func (s *speechSlot) collect() error {
	s.memory.mark(fmt.Sprintf("seq-%d-before-vocode-frames-%d", s.input.SeqID, s.frames))
	data, rate, samples, err := mtmd.HelperGenAudioGetOutput(s.gen)
	if err != nil {
		return err
	}
	if rate <= 0 || samples <= 0 || len(data) < 44 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return errors.New("speech generation returned invalid WAV audio")
	}
	s.output = speechResult{audio: data, tokens: slices.Clone(s.tokens), rate: rate, samples: samples}
	s.memory.mark(fmt.Sprintf("seq-%d-after-vocode-frames-%d", s.input.SeqID, s.frames))
	return nil
}

func compareSpeech(want, got speechResult) error {
	for i := range min(len(want.tokens), len(got.tokens)) {
		if want.tokens[i] != got.tokens[i] {
			return fmt.Errorf("sampled token %d: got %d, want %d (lengths %d/%d)", i, got.tokens[i], want.tokens[i], len(got.tokens), len(want.tokens))
		}
	}
	if len(want.tokens) != len(got.tokens) {
		return fmt.Errorf("sampled token count: got %d, want %d", len(got.tokens), len(want.tokens))
	}
	if want.rate != got.rate || want.samples != got.samples || !bytes.Equal(want.audio, got.audio) {
		return errors.New("audio differs from serial reference")
	}
	return nil
}

// memoryMonitor observes the native allocations that Go heap statistics omit.
// The background sampler queries device properties only, never model/context
// state. It is stopped before unloading the native libraries. ps is required.
type memoryMonitor struct {
	mu      sync.Mutex
	devices []llama.GGMLBackendDevice
	names   []string
	current []uint64
	peak    []uint64
	err     error
	stop    chan struct{}
	done    chan struct{}
}

func newMemoryMonitor() *memoryMonitor {
	m := memoryMonitor{stop: make(chan struct{}), done: make(chan struct{})}
	for i := range llama.GGMLBackendDeviceCount() {
		dev := llama.GGMLBackendDeviceGet(i)
		typ := llama.GGMLBackendDevType(dev)
		if typ == llama.GGMLBackendDeviceTypeGPU || typ == llama.GGMLBackendDeviceTypeIGPU {
			m.devices = append(m.devices, dev)
			m.names = append(m.names, llama.GGMLBackendDeviceName(dev))
		}
	}
	m.current = make([]uint64, len(m.devices))
	m.peak = make([]uint64, len(m.devices))
	m.sample()
	go func() {
		defer close(m.done)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				m.mu.Lock()
				m.sample()
				m.mu.Unlock()
			case <-m.stop:
				return
			}
		}
	}()
	return &m
}

// sample runs under mu once the background sampler has started. Peak values
// are independent maxima since the last checkpoint, not a simultaneous sum.
func (m *memoryMonitor) sample() {
	for i, dev := range m.devices {
		free, total := llama.GGMLBackendDeviceMemory(dev)
		m.current[i] = total - min(free, total)
		m.peak[i] = max(m.peak[i], m.current[i])
	}
}

func (m *memoryMonitor) mark(stage string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return
	}
	// Forking ps while decoding a large model is expensive on macOS. Read
	// process RSS only at checkpoints; sample device counters independently.
	data, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(os.Getpid())).Output()
	if err != nil {
		m.err = fmt.Errorf("reading process RSS with ps: %w", err)
		return
	}
	kib, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		m.err = fmt.Errorf("parsing process RSS: %w", err)
		return
	}
	m.sample()
	// ReadMemStats stops the Go world; do it only between native operations,
	// not from the sampler while a decode is executing through FFI.
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	fmt.Printf("MEM %s rss=%d go_heap=%d", stage, kib*1024, stats.HeapAlloc)
	for i, name := range m.names {
		fmt.Printf(" device=%q used=%d peak_used=%d", name, m.current[i], m.peak[i])
		m.peak[i] = m.current[i]
	}
	fmt.Println()
}

func (m *memoryMonitor) close() error {
	close(m.stop)
	<-m.done
	return m.err
}

func initYzma() error {
	libPath := libs.Path("")

	if err := llama.Load(libPath); err != nil {
		return fmt.Errorf("unable to load library: %w", err)
	}
	if err := mtmd.Load(libPath); err != nil {
		return fmt.Errorf("unable to load MTMD library: %w", err)
	}
	llama.LogSet(llama.LogSilent())
	mtmd.LogSet(llama.LogSilent())

	if err := llama.Init(); err != nil {
		return fmt.Errorf("unable to initialize llama: %w", err)
	}

	return nil
}
