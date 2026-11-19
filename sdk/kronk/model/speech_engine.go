package model

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/ardanlabs/kronk/sdk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/speech"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile/arch"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
)

var errSpeechEngineStopped = errors.New("speech engine stopped")

// speechModel owns model-specific conditioning without depending on the public
// request types or shared engine state. Its errors reject the current request.
type speechModel interface {
	Validate(language, referenceText string) (string, error)
	SupportsVoice(name string) bool
	Prepare(ctx context.Context, vocab llama.Vocab, prompt speech.Prompt, input *mtmd.HelperGenAudioInput) (int, error)
	NewExecution(ctx context.Context, log applog.Logger, cfg speech.ExecutionConfig) (speech.Execution, error)
}

// speechEngine owns scheduling, jobs, and the shared default voice. Model owns
// the backbone context and decoder borrowed by the sequence implementations.
// All native work on those shared contexts remains serialized by the engine.
type speechEngine struct {
	adapter   speechModel
	voice     mtmd.Bitmap
	voiceName string
	voices    []string // Immutable supported presets, including the default alias.
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
	execution speech.Execution
	input     mtmd.HelperGenAudioInput
	job       *speechJob
	started   bool
	voice     mtmd.Bitmap
}

type speechResult struct {
	response     SpeechResponse
	promptTokens int
	err          error
}

type speechJob struct {
	ctx          context.Context
	request      SpeechRequest
	promptTokens int
	resultCh     chan speechResult
	claimed      atomic.Bool
}

func initSpeechRuntime(ctx context.Context, m *Model) (err error) {
	// Speech purpose and a generic backbone architecture do not identify an
	// execution protocol. Use the same tensor-aware family resolution as planning.
	data, err := gguf.ReadHeaderBytes(m.cfg.ModelFiles[0])
	if err != nil {
		return fmt.Errorf("read speech model header: %w", err)
	}
	metadata, tensors, err := gguf.ParseHeaderAndTensors(data, 0)
	if err != nil {
		return fmt.Errorf("parse speech model header: %w", err)
	}
	profile := arch.ResolveWithTensors(metadata, tensors)

	cfg, err := ResolveSpeechConfig(m.cfg)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	input, err := mtmd.HelperGenAudioInputDefault()
	if err != nil {
		return fmt.Errorf("speech input defaults: %w: %w", ErrSpeechUnsupported, err)
	}
	input.OutType = mtmd.HelperGenAudioOutTypeWAV
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
	if got := llama.NCtxSeq(lctx); got != speech.AutoregressiveContextWindow {
		return fmt.Errorf("speech context per sequence: got %d, want %d", got, speech.AutoregressiveContextWindow)
	}
	voiceName, err := mtmd.GenAudioVoiceName(m.mtmdMetaCtx, 0)
	if err != nil {
		return fmt.Errorf("speech default voice: %w: %w", ErrSpeechUnsupported, err)
	}
	if voiceName != "" {
		engine.voice, err = mtmd.GenAudioGetVoice(m.mtmdMetaCtx, voiceName)
		if err != nil {
			return fmt.Errorf("speech default voice: %w: %w", ErrSpeechUnsupported, err)
		}
		if engine.voice == 0 {
			return fmt.Errorf("speech default voice %q: %w", voiceName, ErrSpeechUnsupported)
		}
	}
	input.SpeakerRef = engine.voice
	engine.voiceName = voiceName
	vocab := llama.ModelGetVocab(m.model)
	suppressed := copySuppressTokens(vocab)

	// Model and protocol selection happens only at composition. Scheduling and
	// request preparation use their contracts without model-family branches.
	switch profile.ComputeKind {
	case modelprofile.ComputeQwen3TTSBackbone:
		engine.adapter = &speech.Qwen3TTS{}
	case modelprofile.ComputeChatterboxTurboBackbone:
		engine.adapter = &speech.ChatterboxTurbo{}
	case modelprofile.ComputeKokoroBackbone:
		engine.adapter, err = speech.NewKokoro(m.model, cfg.SpeechMaxFrames(), modelMetaArrayCount, modelMetaArrayString)
	case modelprofile.ComputeTinyVocaBackbone:
		engine.adapter, err = speech.NewTinyVoca(m.model, m.mtmdMetaCtx, cfg.SpeechMaxFrames(), modelMetaArrayCount, modelMetaArrayString, genAudioG2P)
	default:
		return fmt.Errorf("%w: no speech runtime for architecture %q", ErrSpeechUnsupported, m.modelInfo.Architecture)
	}
	if err != nil {
		kind := ErrSpeechUnsupported
		if errors.Is(err, speech.ErrInvalidConfig) {
			kind = ErrInvalidRequest
		}
		return fmt.Errorf("init speech model: %w: %w", kind, err)
	}

	engine.voices = []string{"default"}
	for i := int32(0); ; i++ {
		name, err := mtmd.GenAudioVoiceName(m.mtmdMetaCtx, i)
		if err != nil {
			return fmt.Errorf("speech preset voices: %w: %w", ErrSpeechUnsupported, err)
		}
		if name == "" {
			break
		}
		if name != "default" && engine.adapter.SupportsVoice(name) {
			engine.voices = append(engine.voices, name)
		}
	}

	for i := range cfg.NSeqMax() {
		if err := ctx.Err(); err != nil {
			return err
		}
		slot := speechSlot{input: input}
		slot.input.SeqID = llama.SeqId(i)
		execution, err := engine.adapter.NewExecution(ctx, m.log, speech.ExecutionConfig{
			Model:          m.model,
			Context:        lctx,
			Memory:         mem,
			Decoder:        m.mtmdMetaCtx,
			Input:          slot.input,
			SuppressTokens: suppressed,
			MaxFrames:      cfg.SpeechMaxFrames(),
		})
		if err != nil {
			return fmt.Errorf("init speech sequence %d: %w: %w", i, ErrSpeechUnsupported, err)
		}
		slot.execution = execution
		engine.slots = append(engine.slots, slot)
		m.log(ctx, "speech-runtime", "status", "slot-ready", "slot", i)
	}

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

	engine.model = m
	engine.maxFrames = cfg.SpeechMaxFrames()
	engine.requestQ = make(chan *speechJob, max(len(engine.slots)*cfg.QueueDepth(), 1))
	engine.stopCh = make(chan struct{})
	engine.doneCh = make(chan struct{})
	engine.accepting = true

	go engine.processLoop()

	return nil
}

func (e *speechEngine) prepare(slot *speechSlot) error {
	req := slot.job.request
	if req.Voice != "" && !slices.Contains(e.voices, req.Voice) {
		return fmt.Errorf("%w: unknown speech voice %q; supported voices: %q", ErrInvalidRequest, req.Voice, e.voices)
	}
	input := slot.input
	voiceName := req.Voice
	if voiceName == "" || voiceName == "default" {
		voiceName = e.voiceName
	}
	prompt := speech.Prompt{
		Text: req.Input, Language: req.Language, Voice: voiceName,
		ReferenceText: req.Reference.Text, MaxFrames: e.maxFrames,
	}
	promptTokens, err := e.adapter.Prepare(slot.job.ctx, e.model.vocab, prompt, &input)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidRequest, err)
	}
	slot.job.promptTokens = promptTokens
	if len(req.Reference.Audio) > 0 {
		if !mtmd.SupportAudio(e.model.mtmdMetaCtx) {
			return fmt.Errorf("%w: speech decoder has no reference audio encoder", ErrInvalidRequest)
		}
		voice, err := newMediaBitmap(e.model.mtmdMetaCtx, req.Reference.Audio)
		if err != nil {
			return fmt.Errorf("%w: speech reference: %w", ErrInvalidRequest, err)
		}
		slot.voice = voice
		input.SpeakerRef = voice
	}
	if req.Voice != "" && req.Voice != "default" {
		voice, err := mtmd.GenAudioGetVoice(e.model.mtmdMetaCtx, req.Voice)
		if err != nil {
			return fmt.Errorf("%w: speech voice: %w", ErrInvalidRequest, err)
		}
		if voice == 0 {
			return fmt.Errorf("%w: unknown speech voice %q", ErrInvalidRequest, req.Voice)
		}
		slot.voice = voice
		input.SpeakerRef = voice
	}
	if err := slot.job.ctx.Err(); err != nil {
		return err
	}
	if err := slot.execution.Start(&input); err != nil {
		// Reference preprocessing can reject an otherwise valid WAV (for
		// example, a clip too short for the speaker encoder). No backbone
		// rows have been decoded; normal helper/sequence cleanup permits reuse.
		if len(req.Reference.Audio) > 0 {
			return fmt.Errorf("%w: preparing speech reference: %w", ErrInvalidRequest, err)
		}
		return fmt.Errorf("prepare speech input: %w", err)
	}
	slot.started = true
	return nil
}

func (e *speechEngine) step(slot *speechSlot) (SpeechResponse, bool, bool, error) {
	if !slot.started {
		if err := e.prepare(slot); err != nil {
			fatal := !errors.Is(err, ErrInvalidRequest) && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
			return SpeechResponse{}, false, fatal, err
		}
	}
	output, done, fatal, err := slot.execution.Step(slot.job.ctx, slot.job.promptTokens)
	switch {
	case errors.Is(err, speech.ErrContextLimit):
		err = fmt.Errorf("%w: %w", ErrInvalidRequest, err)
	case errors.Is(err, speech.ErrFrameLimit):
		err = ErrSpeechLimit
	}
	response := SpeechResponse{
		Audio:      output.Audio,
		SampleRate: output.SampleRate,
		Samples:    output.Samples,
		Stereo:     output.Stereo,
	}
	return response, done, fatal, err
}

func (e *speechEngine) shutdown(err error) {
	e.submitMu.Lock()
	defer e.submitMu.Unlock()
	if e.accepting {
		e.accepting = false
		e.stopErr = err
		close(e.stopCh)
	}
}

func (e *speechEngine) stoppedErr() error {
	e.submitMu.Lock()
	defer e.submitMu.Unlock()
	return e.stopErr
}

func (e *speechEngine) stop(ctx context.Context) error {
	e.shutdown(errSpeechEngineStopped)
	select {
	case <-e.doneCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (e *speechEngine) run(ctx context.Context, req SpeechRequest) (SpeechResponse, int, error) {
	if err := ctx.Err(); err != nil {
		return SpeechResponse{}, 0, err
	}
	job := speechJob{ctx: ctx, request: req, resultCh: make(chan speechResult, 1)}
	e.submitMu.Lock()
	if !e.accepting {
		err := e.stopErr
		e.submitMu.Unlock()
		return SpeechResponse{}, 0, err
	}
	e.submitWG.Add(1)
	e.submitMu.Unlock()

	var err error
	select {
	case e.requestQ <- &job:
	case <-ctx.Done():
		err = ctx.Err()
	case <-e.stopCh:
		err = e.stoppedErr()
	}
	e.submitWG.Done()
	if err != nil {
		return SpeechResponse{}, 0, err
	}
	var result speechResult
	select {
	case result = <-job.resultCh:
	case <-ctx.Done():
		// Claim an unstarted job solely to prevent the owner starting it.
		// Running work must acknowledge cleanup before releasing admission.
		if job.claimed.CompareAndSwap(false, true) {
			return SpeechResponse{}, 0, ctx.Err()
		}
		result = <-job.resultCh
	}
	if err := ctx.Err(); err != nil {
		result.response = SpeechResponse{}
		result.err = err
	}
	return result.response, result.promptTokens, result.err
}

// processLoop gives every active slot one bounded prefill or generation step
// per round. Only this goroutine touches native inference state after loading.
func (e *speechEngine) processLoop() {
	defer close(e.doneCh)
	for {
		select {
		case <-e.stopCh:
			e.submitWG.Wait()
			err := e.stoppedErr()
			for i := range e.slots {
				if e.slots[i].job != nil {
					e.finish(&e.slots[i], SpeechResponse{}, err)
				}
			}
			for {
				select {
				case job := <-e.requestQ:
					job.resultCh <- speechResult{err: err}
				default:
					return
				}
			}
		default:
		}

		active := false
		for i := range e.slots {
			slot := &e.slots[i]
			if slot.job == nil {
				select {
				case slot.job = <-e.requestQ:
					if !slot.job.claimed.CompareAndSwap(false, true) {
						slot.job = nil
						active = true // Revisit the queue without going idle.
						continue
					}
				default:
					continue
				}
			}
			active = true
			select {
			case <-e.stopCh:
				continue
			default:
			}
			if err := slot.job.ctx.Err(); err != nil {
				e.finish(slot, SpeechResponse{}, err)
				continue
			}
			response, done, fatal, err := e.step(slot)
			if fatal {
				e.shutdown(err)
			}
			if done || err != nil {
				e.finish(slot, response, err)
			}
		}
		if !active {
			select {
			case e.slots[0].job = <-e.requestQ:
				if !e.slots[0].job.claimed.CompareAndSwap(false, true) {
					e.slots[0].job = nil
				}
			case <-e.stopCh:
			}
		}
	}
}

func (e *speechEngine) finish(slot *speechSlot, response SpeechResponse, err error) {
	job := slot.job
	if cleanupErr := slot.execution.Reset(); cleanupErr != nil {
		e.shutdown(cleanupErr)
		err = errors.Join(err, cleanupErr)
	}
	if slot.voice != 0 {
		mtmd.BitmapFree(slot.voice)
	}
	*slot = speechSlot{execution: slot.execution, input: slot.input}
	if cancelErr := job.ctx.Err(); cancelErr != nil {
		err = cancelErr
	}
	if err != nil {
		response = SpeechResponse{}
	}
	job.resultCh <- speechResult{response: response, promptTokens: job.promptTokens, err: err}
}

func (e *speechEngine) free() {
	for _, v := range slices.Backward(e.slots) {
		v.execution.Free()
	}
	e.slots = nil
	if e.voice != 0 {
		mtmd.BitmapFree(e.voice)
		e.voice = 0
	}
}
