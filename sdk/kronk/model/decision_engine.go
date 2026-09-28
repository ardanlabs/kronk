package model

import (
	"context"
	"errors"
	"fmt"
	"slices"

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
	lctx      llama.Context
	mem       llama.Memory
	nVocab    int
	nCtx      int
	ubatch    int
	cached    []llama.Token
	admission chan struct{}
}

func decisionContextParams(base llama.ContextParams, cfg Config) llama.ContextParams {
	params := base
	params.NCtx = uint32(cfg.ContextWindow())
	params.NBatch = params.NCtx
	params.NUbatch = min(uint32(cfg.EffectiveNUBatch()), params.NCtx)
	params.NSeqMax = 2
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

	if err := m.applyAdapters(lctx); err != nil {
		llama.Free(lctx)
		return fmt.Errorf("init-decision-runtime: %w", err)
	}

	m.ctxParams = params
	m.lctx = lctx
	m.mem = mem
	m.decision = &decisionEngine{
		lctx:      lctx,
		mem:       mem,
		nVocab:    int(llama.VocabNTokens(m.vocab)),
		nCtx:      int(params.NCtx),
		ubatch:    int(params.NUbatch),
		admission: make(chan struct{}, 1),
	}

	return nil
}

func (e *decisionEngine) run(ctx context.Context, work []decisionWork) ([][][]float32, error) {
	if len(work) == 0 {
		return nil, nil
	}
	if err := e.validate(work); err != nil {
		return nil, err
	}

	select {
	case e.admission <- struct{}{}:
		defer func() { <-e.admission }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	result, err := e.exact(ctx, work)
	if err != nil {
		return nil, errors.Join(err, e.clear())
	}

	return result, nil
}

func (e *decisionEngine) validate(work []decisionWork) error {
	for i, item := range work {
		if len(item.tokens) == 0 {
			return fmt.Errorf("decision work[%d] has no tokens", i)
		}

		if len(item.tokens) > e.nCtx {
			return fmt.Errorf("decision work[%d] has %d tokens, context window is %d", i, len(item.tokens), e.nCtx)
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

func decisionSharedLen(work []decisionWork) int {
	shared := work[0].prefixLen
	for _, item := range work[1:] {
		shared = min(shared, item.prefixLen)
		for i := range shared {
			if item.tokens[i] != work[0].tokens[i] {
				shared = i
				break
			}
		}
	}

	return shared
}

func (e *decisionEngine) exact(ctx context.Context, work []decisionWork) ([][][]float32, error) {
	shared := decisionSharedLen(work) / e.ubatch * e.ubatch
	if shared == 0 {
		return e.separate(ctx, work)
	}

	if err := e.keepPrefix(ctx, work[0].tokens[:shared]); err != nil {
		return nil, err
	}

	result := make([][][]float32, len(work))
	for i, item := range work {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if _, err := llama.MemorySeqRm(e.mem, 1, -1, -1); err != nil {
			return nil, fmt.Errorf("decision remove sequence 1: %w", err)
		}

		if err := llama.MemorySeqCp(e.mem, 0, 1, -1, -1); err != nil {
			return nil, fmt.Errorf("decision copy shared prefix: %w", err)
		}

		logits, err := e.decode(decisionPart{
			tokens:   item.tokens[shared:],
			position: shared,
			sequence: 1,
			readouts: item.readouts,
		})

		if _, removeErr := llama.MemorySeqRm(e.mem, 1, -1, -1); err == nil && removeErr != nil {
			err = fmt.Errorf("decision remove completed sequence 1: %w", removeErr)
		}

		if err != nil {
			return nil, err
		}

		result[i] = logits[0]
	}

	return result, nil
}

func (e *decisionEngine) separate(ctx context.Context, work []decisionWork) ([][][]float32, error) {
	result := make([][][]float32, len(work))
	for i, item := range work {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if err := e.clear(); err != nil {
			return nil, err
		}

		logits, err := e.decode(decisionPart{tokens: item.tokens, readouts: item.readouts})
		if err != nil {
			return nil, err
		}

		result[i] = logits[0]
	}
	return result, e.clear()
}

func (e *decisionEngine) keepPrefix(ctx context.Context, prefix []llama.Token) error {
	if slices.Equal(e.cached, prefix) {
		return nil
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if err := e.clear(); err != nil {
		return err
	}

	if _, err := e.decode(decisionPart{tokens: prefix}); err != nil {
		return err
	}

	e.cached = slices.Clone(prefix)

	return nil
}

func (e *decisionEngine) clear() error {
	e.cached = nil
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

func (e *decisionEngine) decode(parts ...decisionPart) ([][][]float32, error) {
	var total int
	for _, part := range parts {
		total += len(part.tokens)
	}

	if total == 0 {
		return make([][][]float32, len(parts)), nil
	}

	batch := llama.BatchInit(int32(total), 0, 1)
	defer llama.BatchFree(batch)

	indices := make([][]int32, len(parts))
	for partIndex, part := range parts {
		readoutAt := make(map[int]bool, len(part.readouts))

		for _, readout := range part.readouts {
			readoutAt[readout.position] = true
		}

		for i, token := range part.tokens {
			position := part.position + i
			if readoutAt[position] {
				indices[partIndex] = append(indices[partIndex], batch.NTokens)
			}
			if err := batch.Add(token, llama.Pos(position), []llama.SeqId{part.sequence}, readoutAt[position]); err != nil {
				return nil, fmt.Errorf("decision add token to batch: %w", err)
			}
		}
	}

	code, err := llama.Decode(e.lctx, batch)
	if err != nil {
		return nil, fmt.Errorf("decision decode: %w", err)
	}

	if code != 0 {
		return nil, fmt.Errorf("decision decode returned %d", code)
	}

	result := make([][][]float32, len(parts))
	for partIndex, part := range parts {
		if len(indices[partIndex]) != len(part.readouts) {
			return nil, fmt.Errorf("decision produced %d readout rows, want %d", len(indices[partIndex]), len(part.readouts))
		}

		result[partIndex] = make([][]float32, len(part.readouts))

		for readoutIndex, batchIndex := range indices[partIndex] {
			allLogits, err := llama.GetLogitsIth(e.lctx, batchIndex, e.nVocab)
			if err != nil {
				return nil, fmt.Errorf("decision get logits at batch index %d: %w", batchIndex, err)
			}

			if allLogits == nil {
				return nil, fmt.Errorf("decision has no logits at batch index %d", batchIndex)
			}

			candidates := part.readouts[readoutIndex].candidates
			result[partIndex][readoutIndex] = make([]float32, len(candidates))

			for i, token := range candidates {
				result[partIndex][readoutIndex][i] = allLogits[token]
			}
		}
	}

	return result, nil
}
