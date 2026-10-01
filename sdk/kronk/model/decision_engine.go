package model

import (
	"context"
	"errors"
	"fmt"

	"github.com/hybridgroup/yzma/pkg/llama"
)

const decisionMaxReadouts = 255

type decisionReadout struct {
	position   int
	candidates []llama.Token
}

type decisionWork struct {
	tokens    []llama.Token
	prefixLen int
	readouts  []decisionReadout
}

// decisionEngine performs only model execution. Protocol implementations own
// prompting, candidate selection, calibration, and multi-pass decisions.
type decisionEngine struct {
	lctx          llama.Context
	mem           llama.Memory
	batch         *extendedBatch
	nVocab        int
	contextWindow int
	maxTokens     int
	maxSequences  int
	maxOutputs    int
	scheduler     *decisionScheduler
}

func decisionContextParams(base llama.ContextParams, cfg Config) llama.ContextParams {
	params := base
	nSeqMax := max(cfg.NSeqMax(), 1)
	params.NCtx = uint32(cfg.ContextWindow() * nSeqMax)
	params.NBatch = uint32(cfg.ContextWindow())
	params.NUbatch = min(uint32(cfg.EffectiveNUBatch()), params.NBatch)
	params.NSeqMax = uint32(nSeqMax)
	params.NOutputsMax = decisionMaxReadouts
	params.NOutputsMaxPerSeq = decisionMaxReadouts
	params.KVUnified = 1
	params.NoPerf = 1
	return params
}

func initDecisionRuntime(m *Model) error {
	params := decisionContextParams(m.ctxParams, m.cfg)
	lctx, err := llama.InitFromModel(m.model, params)
	if err != nil {
		return fmt.Errorf("init-decision-runtime: init context: %w", err)
	}

	mem, err := llama.GetMemory(lctx)
	if err != nil {
		llama.Free(lctx)
		return fmt.Errorf("init-decision-runtime: get memory: %w", err)
	}

	if err := llama.MemoryClear(mem, true); err != nil {
		llama.Free(lctx)
		return fmt.Errorf("init-decision-runtime: clear memory: %w", err)
	}

	batch, err := newExtendedBatch(lctx)
	if err != nil {
		llama.Free(lctx)
		return fmt.Errorf("init-decision-runtime: create batch: %w", err)
	}

	if err := m.applyAdapters(lctx); err != nil {
		batchErr := batch.free()
		llama.Free(lctx)
		return errors.Join(fmt.Errorf("init-decision-runtime: %w", err), batchErr)
	}

	m.ctxParams = params
	m.lctx = lctx
	m.mem = mem

	engine := decisionEngine{
		lctx:          lctx,
		mem:           mem,
		batch:         batch,
		nVocab:        int(llama.VocabNTokens(m.vocab)),
		contextWindow: m.cfg.ContextWindow(),
		maxTokens:     int(params.NBatch),
		maxSequences:  int(params.NSeqMax),
		maxOutputs:    int(params.NOutputsMax),
	}
	engine.scheduler = newDecisionScheduler(&engine, m.cfg.QueueDepth())
	engine.scheduler.start()
	m.decision = &engine

	return nil
}

func (e *decisionEngine) run(ctx context.Context, work []decisionWork) ([][][]float32, error) {
	if len(work) == 0 {
		return nil, nil
	}
	if err := e.validate(work); err != nil {
		return nil, err
	}

	return e.scheduler.run(ctx, work)
}

func (e *decisionEngine) validate(work []decisionWork) error {
	for i, item := range work {
		if len(item.tokens) == 0 {
			return fmt.Errorf("decision work[%d] has no tokens", i)
		}

		if len(item.tokens) > e.contextWindow {
			return fmt.Errorf("decision work[%d] has %d tokens, context window is %d", i, len(item.tokens), e.contextWindow)
		}

		if item.prefixLen < 0 || item.prefixLen > len(item.tokens) {
			return fmt.Errorf("decision work[%d] has invalid prefix length %d", i, item.prefixLen)
		}

		if len(item.readouts) == 0 || len(item.readouts) > decisionMaxReadouts {
			return fmt.Errorf("decision work[%d] needs 1 to %d readouts, got %d", i, decisionMaxReadouts, len(item.readouts))
		}

		for j, readout := range item.readouts {
			if j > 0 && readout.position <= item.readouts[j-1].position {
				return fmt.Errorf("decision work[%d] readout positions are not strictly increasing", i)
			}

			if readout.position < item.prefixLen || readout.position >= len(item.tokens) {
				return fmt.Errorf("decision work[%d] readout[%d] position %d is outside [%d,%d)", i, j, readout.position, item.prefixLen, len(item.tokens))
			}

			if len(readout.candidates) == 0 {
				return fmt.Errorf("decision work[%d] readout[%d] has no candidates", i, j)
			}

			for _, token := range readout.candidates {
				if token < 0 || int(token) >= e.nVocab {
					return fmt.Errorf("decision work[%d] readout[%d] has invalid candidate token %d", i, j, token)
				}
			}
		}
	}
	return nil
}

func (e *decisionEngine) clear() error {
	if err := llama.MemoryClear(e.mem, true); err != nil {
		return fmt.Errorf("decision clear memory: %w", err)
	}

	return nil
}

type decisionPart struct {
	tokens   []llama.Token
	position int
	sequence llama.SeqId
	readouts []decisionReadout
}

func (e *decisionEngine) decode(parts ...decisionPart) ([][][]float32, bool, error) {
	var total int
	for _, part := range parts {
		total += len(part.tokens)
	}

	if total == 0 {
		return make([][][]float32, len(parts)), false, nil
	}

	indices, err := stageDecisionParts(e.batch, parts)
	if err != nil {
		return nil, true, fmt.Errorf("decision stage batch: %w", err)
	}

	code, err := e.batch.process(llama.ProcessTypeDecode)
	if err != nil {
		return nil, true, fmt.Errorf("decision process: %w", err)
	}

	if code != 0 {
		return nil, code < 0, fmt.Errorf("decision process returned %d", code)
	}

	result := make([][][]float32, len(parts))
	for partIndex, part := range parts {
		if len(indices[partIndex]) != len(part.readouts) {
			return nil, true, fmt.Errorf("decision produced %d readout rows, want %d", len(indices[partIndex]), len(part.readouts))
		}

		result[partIndex] = make([][]float32, len(part.readouts))

		for readoutIndex, batchIndex := range indices[partIndex] {
			allLogits, err := llama.GetLogitsIth(e.lctx, batchIndex, e.nVocab)
			if err != nil {
				return nil, true, fmt.Errorf("decision get logits at batch index %d: %w", batchIndex, err)
			}

			if allLogits == nil {
				return nil, true, fmt.Errorf("decision has no logits at batch index %d", batchIndex)
			}

			candidates := part.readouts[readoutIndex].candidates
			result[partIndex][readoutIndex] = make([]float32, len(candidates))

			for i, token := range candidates {
				result[partIndex][readoutIndex][i] = allLogits[token]
			}
		}
	}

	return result, false, nil
}

func stageDecisionParts(batch *extendedBatch, parts []decisionPart) ([][]int32, error) {
	batch.clear()

	indices := make([][]int32, len(parts))
	for partIndex, part := range parts {
		readoutAt := make(map[int]bool, len(part.readouts))
		for _, readout := range part.readouts {
			readoutAt[readout.position] = true
		}

		sequenceIDs := []llama.SeqId{part.sequence}
		for i, token := range part.tokens {
			position := part.position + i
			output := extendedBatchOutputNone
			if readoutAt[position] {
				output = extendedBatchOutputLogits
			}

			idx, err := batch.addToken(token, llama.Pos(position), sequenceIDs, output)
			if err != nil {
				return nil, fmt.Errorf("add part[%d] token at position %d: %w", partIndex, position, err)
			}
			if output == extendedBatchOutputLogits {
				indices[partIndex] = append(indices[partIndex], idx)
			}
		}
	}

	return indices, nil
}

func (e *decisionEngine) evaluate(entries []decisionScheduledEntry) ([][][]float32, bool, error) {
	parts := make([]decisionPart, len(entries))
	for i, entry := range entries {
		parts[i] = decisionPart{
			tokens:   entry.work.tokens,
			sequence: llama.SeqId(i),
			readouts: entry.work.readouts,
		}
	}

	outputs, fatal, err := e.decode(parts...)
	if err != nil {
		clearErr := e.clear()
		return nil, fatal || clearErr != nil, errors.Join(err, clearErr)
	}

	for i := range entries {
		if _, err := llama.MemorySeqRm(e.mem, llama.SeqId(i), -1, -1); err != nil {
			return nil, true, errors.Join(fmt.Errorf("decision remove sequence %d: %w", i, err), e.clear())
		}
	}

	return outputs, false, nil
}
