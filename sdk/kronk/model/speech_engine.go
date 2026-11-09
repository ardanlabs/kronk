package model

import (
	"context"
	"errors"
	"fmt"
	"math"
	"runtime"
	"slices"
	"sync"
	"unsafe"

	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
)

const (
	speechContextWindow = 2048
	speechBatchSize     = 512
)

// speechEngine owns per-sequence pipelines and their shared default voice.
// Model owns the backbone context and decoder they borrow. All native work on
// those shared contexts must be serialized; helpers are not independent workers.
type speechEngine struct {
	voice     mtmd.Bitmap
	slots     []speechSlot
	model     *Model
	maxFrames int // Immutable load-time resource limit.
	requestQ  chan *speechJob
	stopCh    chan struct{}
	doneCh    chan struct{}
	submitMu  sync.Mutex
	submitWG  sync.WaitGroup
	accepting bool
	stopErr   error
}

type speechSlot struct {
	helper        MTMDHelperGenAudio
	input         MTMDHelperGenAudioInput
	sampler       llama.Sampler
	job           *speechJob
	started       bool
	prefilling    bool
	frames        int
	token         llama.Token
	hidden        *float32
	prefillHidden []float32
	voice         mtmd.Bitmap
}

// adjustSpeechConfig resolves only the verified speech resource envelope. It
// deliberately does not apply generation defaults, IMC, or sampling parameters.
func adjustSpeechConfig(cfg Config) (Config, error) {
	switch {
	case cfg.ContextWindow() != 0 && cfg.ContextWindow() != speechContextWindow:
		return cfg, fmt.Errorf("%w: speech context window must be %d per sequence", ErrInvalidRequest, speechContextWindow)
	case cfg.PtrPrefillBatchSize != nil && cfg.PrefillBatchSize() != speechBatchSize:
		return cfg, fmt.Errorf("%w: speech prefill batch size must be %d", ErrInvalidRequest, speechBatchSize)
	case cfg.PtrPhysicalBatchSize != nil && cfg.PhysicalBatchSize() != speechBatchSize:
		return cfg, fmt.Errorf("%w: speech physical batch size must be %d", ErrInvalidRequest, speechBatchSize)
	case cfg.KVCacheMode() != KVCachePartitioned:
		return cfg, fmt.Errorf("%w: speech requires partitioned KV cache", ErrInvalidRequest)
	case cfg.IncrementalCache() || cfg.IMCSessionCapacity() > 0:
		return cfg, fmt.Errorf("%w: speech does not support IMC", ErrInvalidRequest)
	case cfg.PtrDraftModel != nil || cfg.MTPDrafterFile != "" ||
		(cfg.SpeculationMode() != SpeculationAuto && cfg.SpeculationMode() != SpeculationDisabled):
		return cfg, fmt.Errorf("%w: speech does not support speculative decoding", ErrInvalidRequest)
	case len(cfg.Adapters) > 0:
		return cfg, fmt.Errorf("%w: speech does not support adapters", ErrInvalidRequest)
	}

	for _, cacheType := range []GGMLType{cfg.CacheTypeK, cfg.CacheTypeV} {
		if !cacheType.IsZero() && cacheType != GGMLTypeAuto && cacheType != GGMLTypeF16 {
			return cfg, fmt.Errorf("%w: speech requires F16 KV cache, got %s", ErrInvalidRequest, cacheType)
		}
	}

	nSlots := cfg.NSeqMax()
	if nSlots <= 0 {
		nSlots = defNSeqMax
	}

	if nSlots > math.MaxUint32/speechContextWindow {
		return cfg, fmt.Errorf("%w: speech aggregate context exceeds native capacity", ErrInvalidRequest)
	}

	cfg = adjustQueueDepth(cfg)
	cfg = adjustAdmissionTimeout(cfg)
	cfg = adjustThreads(cfg)
	cfg.PtrNSeqMax = new(nSlots)
	cfg.PtrContextWindow = new(speechContextWindow)
	cfg.PtrPrefillBatchSize = new(speechBatchSize)
	cfg.PtrPhysicalBatchSize = new(speechBatchSize)
	cfg.PtrIncrementalCache = new(false)
	cfg.PtrSpeech = &SpeechConfig{MaxFrames: cfg.SpeechMaxFrames()}
	cfg.CacheTypeK, cfg.CacheTypeV = GGMLTypeF16, GGMLTypeF16
	cfg.Speculation = SpeculationDisabled
	cfg.nBatch, cfg.nUBatch = speechBatchSize, speechBatchSize

	return cfg, nil
}

func initSpeechRuntime(ctx context.Context, m *Model) (err error) {
	cfg, err := adjustSpeechConfig(m.cfg)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	input, err := MTMDHelperGenAudioInputDefault()
	if err != nil {
		return fmt.Errorf("speech input defaults: %w: %w", ErrSpeechUnsupported, err)
	}
	input.OutType = MTMDGenAudioOutputWAV
	input.Stream = false
	input.Seed, input.Temp = 42, 0.8

	params := modelCtxParams(cfg, m.modelInfo)
	params.Embeddings = 1
	params.PoolingType = llama.PoolingTypeNone
	// Preserve the native helper's per-token embedding/output capacity rather
	// than inheriting the chat engine's last-logit-only reservation.
	params.NOutputsMax, params.NOutputsMaxPerSeq = 0, 0
	lctx, err := llama.InitFromModel(m.model, params)
	if err != nil {
		return fmt.Errorf("init speech context: %w", err)
	}
	engine := speechEngine{}
	defer func() {
		if err != nil {
			engine.free()
			err = errors.Join(err, llama.Free(lctx))
		}
	}()

	mem, err := llama.GetMemory(lctx)
	if err != nil {
		return fmt.Errorf("speech context memory: %w", err)
	}
	if got := llama.NCtxSeq(lctx); got != speechContextWindow {
		return fmt.Errorf("speech context per sequence: got %d, want %d", got, speechContextWindow)
	}
	engine.voice, err = MTMDGenAudioGetVoice(m.mtmdMetaCtx, "default")
	if err != nil {
		return fmt.Errorf("speech default voice: %w: %w", ErrSpeechUnsupported, err)
	}
	input.SpeakerRef = engine.voice
	vocab := llama.ModelGetVocab(m.model)
	suppressed := copySuppressTokens(vocab)
	var biases []llama.LogitBias
	for _, token := range suppressed {
		biases = append(biases, llama.LogitBias{Token: token, Bias: float32(math.Inf(-1))})
	}

	for i := range cfg.NSeqMax() {
		if err := ctx.Err(); err != nil {
			return err
		}
		helper, err := MTMDHelperGenAudioInit(lctx, m.mtmdMetaCtx)
		if err != nil {
			return fmt.Errorf("init speech sequence %d: %w: %w", i, ErrSpeechUnsupported, err)
		}
		slot := speechSlot{helper: helper, input: input}
		slot.input.SeqID = llama.SeqId(i)
		slot.sampler = llama.SamplerChainInit(llama.SamplerChainDefaultParams())
		if len(biases) > 0 {
			llama.SamplerChainAdd(slot.sampler, llama.SamplerInitLogitBias(llama.VocabNTokens(vocab), int32(len(biases)), unsafe.SliceData(biases)))
		}
		llama.SamplerChainAdd(slot.sampler, llama.SamplerInitPenalties(llama.VocabNTokens(vocab), 4096, 1.05, 0, 0))
		llama.SamplerChainAdd(slot.sampler, llama.SamplerInitTopK(slot.input.TopK))
		llama.SamplerChainAdd(slot.sampler, llama.SamplerInitTemp(slot.input.Temp))
		llama.SamplerChainAdd(slot.sampler, llama.SamplerInitDist(slot.input.Seed))
		engine.slots = append(engine.slots, slot)
		m.log(ctx, "speech-runtime", "status", "slot-ready", "slot", i)
	}

	runtime.KeepAlive(biases)
	if err := ctx.Err(); err != nil {
		return err
	}

	m.cfg = cfg
	m.ctxParams = params
	m.lctx, m.mem = lctx, mem
	m.vocab, m.suppressTokens = vocab, suppressed
	m.addBOSToken = llama.VocabGetAddBOS(vocab)
	m.paramsResolved = true
	m.speech = &engine
	m.log(ctx, "MODEL-INFO", "values", m.modelInfo.String())
	m.log(ctx, "MODEL-CONFIG", "values", cfg.String())
	engine.start(m)

	return nil
}

func (e *speechEngine) free() {
	for _, v := range slices.Backward(e.slots) {
		llama.SamplerFree(v.sampler)
		MTMDHelperGenAudioFree(v.helper)
	}
	e.slots = nil
	if e.voice != 0 {
		mtmd.BitmapFree(e.voice)
		e.voice = 0
	}
}
