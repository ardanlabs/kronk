package model

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"

	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
)

var errSpeechEngineStopped = errors.New("speech engine stopped")

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

func (e *speechEngine) start(m *Model) {
	e.model = m
	e.maxFrames = m.cfg.SpeechMaxFrames()
	e.requestQ = make(chan *speechJob, max(len(e.slots)*m.cfg.QueueDepth(), 1))
	e.stopCh = make(chan struct{})
	e.doneCh = make(chan struct{})
	e.accepting = true
	go e.processLoop()
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

func (e *speechEngine) run(ctx context.Context, req SpeechRequest) speechResult {
	if err := ctx.Err(); err != nil {
		return speechResult{err: err}
	}
	job := speechJob{ctx: ctx, request: req, resultCh: make(chan speechResult, 1)}
	e.submitMu.Lock()
	if !e.accepting {
		err := e.stopErr
		e.submitMu.Unlock()
		return speechResult{err: err}
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
		return speechResult{err: err}
	}
	var result speechResult
	select {
	case result = <-job.resultCh:
	case <-ctx.Done():
		// Claim an unstarted job solely to prevent the owner starting it.
		// Running work must acknowledge cleanup before releasing admission.
		if job.claimed.CompareAndSwap(false, true) {
			return speechResult{err: ctx.Err()}
		}
		result = <-job.resultCh
	}
	if err := ctx.Err(); err != nil {
		result.response = SpeechResponse{}
		result.err = err
	}
	return result
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

func (e *speechEngine) prepare(slot *speechSlot) error {
	req := slot.job.request
	tokens := llama.Tokenize(e.model.vocab, req.Input, false, true)
	slot.job.promptTokens = len(tokens)
	if len(tokens) == 0 || len(tokens)+1+e.maxFrames > speechContextWindow {
		return fmt.Errorf("%w: speech text exceeds the context budget", ErrInvalidRequest)
	}
	input := slot.input
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
	input.SetPrompt(req.Input)
	if err := mtmd.HelperGenAudioSetInput(slot.helper, &input); err != nil {
		return fmt.Errorf("prepare speech input: %w", err)
	}
	llama.SamplerReset(slot.sampler)
	slot.started, slot.prefilling = true, true
	return nil
}

func (e *speechEngine) step(slot *speechSlot) (SpeechResponse, bool, bool, error) {
	if !slot.started {
		if err := e.prepare(slot); err != nil {
			return SpeechResponse{}, false, !errors.Is(err, ErrInvalidRequest), err
		}
	}
	if err := slot.job.ctx.Err(); err != nil {
		return SpeechResponse{}, false, false, err
	}
	if slot.prefilling {
		remaining, err := mtmd.HelperGenAudioStepPrompt(slot.helper, speechBatchSize)
		if err != nil {
			return SpeechResponse{}, false, true, fmt.Errorf("speech prompt decoding: %w", err)
		}
		pos, err := llama.MemorySeqPosMax(e.model.mem, slot.input.SeqID)
		if err != nil {
			return SpeechResponse{}, false, true, err
		}
		// The first native batch fits independently. Its remaining count lets
		// us include actual voice conditioning before decoding any more rows.
		if int(pos)+1+int(remaining)+e.maxFrames > speechContextWindow {
			return SpeechResponse{}, false, false, fmt.Errorf("%w: speech text and voice conditioning exceed the context budget", ErrInvalidRequest)
		}
		if remaining > 0 {
			return SpeechResponse{}, false, false, nil
		}
		hidden, err := llama.GetEmbeddingsIth(e.model.lctx, -1, llama.ModelNEmbdOut(e.model.model))
		if err != nil {
			return SpeechResponse{}, false, true, err
		}
		// The shared context's output is overwritten by the next slot.
		slot.hidden = slices.Clone(hidden)
		slot.prefilling = false
		e.sample(slot)
		e.model.log(slot.job.ctx, "speech", "status", "prefill-complete", "sequence", slot.input.SeqID, "prompt_tokens", slot.job.promptTokens)
		return SpeechResponse{}, false, false, nil
	}

	// Permit EOS at the exact cap, but never decode another frame past it.
	if slot.frames >= e.maxFrames && !llama.VocabIsEOG(e.model.vocab, slot.token) {
		return SpeechResponse{}, false, false, ErrSpeechLimit
	}
	next, stop, err := mtmd.HelperGenAudioStepGen(slot.helper, slot.token, slot.hidden)
	if err != nil {
		return SpeechResponse{}, false, true, err
	}
	if stop {
		if err := slot.job.ctx.Err(); err != nil {
			return SpeechResponse{}, false, false, err
		}
		output, err := mtmd.HelperGenAudioGetOutput(slot.helper)
		if err != nil {
			return SpeechResponse{}, false, true, err
		}
		data, rate, samples := output.Data, output.SampleRate, output.NSamples
		if len(data) < 44 || samples <= 0 || rate <= 0 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" ||
			binary.LittleEndian.Uint16(data[20:22]) != 1 || binary.LittleEndian.Uint16(data[22:24]) != 1 ||
			binary.LittleEndian.Uint32(data[24:28]) != uint32(rate) || binary.LittleEndian.Uint16(data[34:36]) != 16 ||
			samples != int64((len(data)-44)/2) || int64(binary.LittleEndian.Uint32(data[40:44])) != samples*2 {
			return SpeechResponse{}, false, true, errors.New("speech decoder returned invalid PCM16 mono WAV")
		}
		return SpeechResponse{Audio: data, SampleRate: rate, Samples: samples}, true, false, nil
	}
	if next == nil {
		return SpeechResponse{}, false, true, errors.New("speech generation ended without EOS")
	}
	slot.frames++
	slot.hidden = next
	e.sample(slot)
	return SpeechResponse{}, false, false, nil
}

func (e *speechEngine) sample(slot *speechSlot) {
	// Capture the token before another slot replaces the shared logits.
	slot.token = llama.SamplerSample(slot.sampler, e.model.lctx, -1)
	llama.SamplerAccept(slot.sampler, slot.token)
}

func (e *speechEngine) finish(slot *speechSlot, response SpeechResponse, err error) {
	job := slot.job
	mtmd.HelperGenAudioReset(slot.helper)
	_, cleanupErr := llama.MemorySeqRm(e.model.mem, slot.input.SeqID, 0, -1)
	if cleanupErr != nil {
		e.shutdown(cleanupErr)
		err = errors.Join(err, cleanupErr)
	}
	if slot.voice != 0 {
		mtmd.BitmapFree(slot.voice)
	}
	*slot = speechSlot{helper: slot.helper, input: slot.input, sampler: slot.sampler}
	if cancelErr := job.ctx.Err(); cancelErr != nil {
		err = cancelErr
	}
	if err != nil {
		response = SpeechResponse{}
	}
	job.resultCh <- speechResult{response: response, promptTokens: job.promptTokens, err: err}
}
