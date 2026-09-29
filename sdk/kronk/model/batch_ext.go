package model

import (
	"fmt"

	"github.com/hybridgroup/yzma/pkg/llama"
)

type extendedBatchOutput uint8

const (
	extendedBatchOutputNone extendedBatchOutput = iota
	extendedBatchOutputLogits
	extendedBatchOutputEmbeddings
)

type extendedBatchEntry struct {
	token            llama.Token
	hasToken         bool
	embedding        []float32
	embdWidth        int
	positions        [4]llama.Pos
	positionCount    int
	sequenceID       llama.SeqId
	extraSequenceIDs []llama.SeqId
	output           extendedBatchOutput
}

// extendedBatch owns one context-bound llama BatchExt and the logical entries
// used to render it. Keeping the logical entries in Go lets the generation
// scheduler inspect indices and tokens and roll back partially staged slot
// contributions before the native batch is processed.
type extendedBatch struct {
	ctx      llama.Context
	native   llama.BatchExt
	capacity int
	entries  []extendedBatchEntry
}

func newExtendedBatch(ctx llama.Context) (*extendedBatch, error) {
	return newExtendedBatchCapacity(ctx, int(llama.NBatch(ctx)))
}

func newExtendedBatchCapacity(ctx llama.Context, entryCapacity int) (*extendedBatch, error) {
	if ctx == 0 {
		return nil, fmt.Errorf("new extended batch: invalid context")
	}

	native, err := llama.BatchExtInit(ctx)
	if err != nil {
		return nil, fmt.Errorf("new extended batch: %w", err)
	}

	capacity := int(llama.NBatch(ctx))
	entryCapacity = min(max(entryCapacity, 0), capacity)
	return &extendedBatch{
		ctx:      ctx,
		native:   native,
		capacity: capacity,
		entries:  make([]extendedBatchEntry, 0, entryCapacity),
	}, nil
}

func (b *extendedBatch) free() error {
	if b == nil || b.native == 0 {
		return nil
	}

	err := llama.BatchExtFree(b.native)
	b.native = 0
	b.ctx = 0
	b.entries = nil

	return err
}

func (b *extendedBatch) clear() {
	if b == nil {
		return
	}
	clear(b.entries)
	b.entries = b.entries[:0]
}

func (b *extendedBatch) len() int {
	if b == nil {
		return 0
	}

	return len(b.entries)
}

func (b *extendedBatch) truncate(length int) error {
	if b == nil {
		return fmt.Errorf("truncate extended batch: invalid batch")
	}
	if length < 0 || length > len(b.entries) {
		return fmt.Errorf("truncate extended batch: length %d outside [0, %d]", length, len(b.entries))
	}

	clear(b.entries[length:])
	b.entries = b.entries[:length]

	return nil
}

func (b *extendedBatch) addToken(token llama.Token, position llama.Pos, sequenceIDs []llama.SeqId, output extendedBatchOutput) (int32, error) {
	return b.addTokenPositions(token, []llama.Pos{position}, sequenceIDs, output)
}

func (b *extendedBatch) addTokenPositions(token llama.Token, positions []llama.Pos, sequenceIDs []llama.SeqId, output extendedBatchOutput) (int32, error) {
	if len(positions) == 0 || len(positions) > 4 {
		return -1, fmt.Errorf("add extended batch token: got %d positions, want 1 to 4", len(positions))
	}

	entry := extendedBatchEntry{
		token:         token,
		hasToken:      true,
		positionCount: len(positions),
		output:        output,
	}
	copy(entry.positions[:], positions)

	return b.addEntry(entry, sequenceIDs)
}

func (b *extendedBatch) addTokenEmbedding(token llama.Token, embedding []float32, embdWidth int, position llama.Pos, sequenceIDs []llama.SeqId, output extendedBatchOutput) (int32, error) {
	if embdWidth <= 0 || len(embedding) != embdWidth {
		return -1, fmt.Errorf("add token embedding: got %d values, want one row of %d", len(embedding), embdWidth)
	}

	return b.addEntry(extendedBatchEntry{
		token:         token,
		hasToken:      true,
		embedding:     embedding,
		embdWidth:     embdWidth,
		positions:     [4]llama.Pos{position},
		positionCount: 1,
		output:        output,
	}, sequenceIDs)
}

func (b *extendedBatch) addEmbedding(embedding []float32, embdWidth int, positions []llama.Pos, sequenceIDs []llama.SeqId, output extendedBatchOutput) (int32, error) {
	if len(positions) == 0 || len(positions) > 4 {
		return -1, fmt.Errorf("add extended batch entry: got %d positions, want 1 to 4", len(positions))
	}
	if embdWidth <= 0 || len(embedding) != embdWidth {
		return -1, fmt.Errorf("add embedding: got %d values, want one row of %d", len(embedding), embdWidth)
	}

	entry := extendedBatchEntry{
		embedding:     embedding,
		embdWidth:     embdWidth,
		positionCount: len(positions),
		output:        output,
	}
	copy(entry.positions[:], positions)

	return b.addEntry(entry, sequenceIDs)
}

func (b *extendedBatch) addEntry(entry extendedBatchEntry, sequenceIDs []llama.SeqId) (int32, error) {
	if b == nil {
		return -1, fmt.Errorf("add extended batch entry: invalid batch")
	}
	if b.capacity <= 0 || len(b.entries) >= b.capacity {
		return -1, fmt.Errorf("add extended batch entry: batch is full (%d entries)", len(b.entries))
	}
	if len(sequenceIDs) == 0 {
		return -1, fmt.Errorf("add extended batch entry: no sequence ID")
	}
	if entry.positionCount == 0 || entry.positionCount > len(entry.positions) {
		return -1, fmt.Errorf("add extended batch entry: got %d positions, want 1 to 4", entry.positionCount)
	}
	if entry.output > extendedBatchOutputEmbeddings {
		return -1, fmt.Errorf("add extended batch entry: invalid output type %d", entry.output)
	}
	if len(entry.embedding) > 0 {
		if entry.embdWidth <= 0 || len(entry.embedding)%entry.embdWidth != 0 {
			return -1, fmt.Errorf("add extended batch entry: %d embedding values in rows of %d", len(entry.embedding), entry.embdWidth)
		}
	} else if !entry.hasToken {
		return -1, fmt.Errorf("add extended batch entry: entry has no token or embedding")
	}

	entry.embedding = append([]float32(nil), entry.embedding...)
	entry.sequenceID = sequenceIDs[0]
	entry.extraSequenceIDs = append([]llama.SeqId(nil), sequenceIDs[1:]...)

	idx := int32(len(b.entries))
	b.entries = append(b.entries, entry)

	return idx, nil
}

func (b *extendedBatch) tokens(start, count int) ([]llama.Token, bool) {
	if b == nil || start < 0 || count <= 0 || start+count > len(b.entries) {
		return nil, false
	}

	tokens := make([]llama.Token, count)
	for i, entry := range b.entries[start : start+count] {
		if !entry.hasToken {
			return nil, false
		}
		tokens[i] = entry.token
	}

	return tokens, true
}

func (b *extendedBatch) process(typ llama.ProcessType) (int32, error) {
	if err := b.render(); err != nil {
		return 0, err
	}

	return llama.Process(b.ctx, typ, b.native)
}

func (b *extendedBatch) render() (retErr error) {
	if b == nil || b.ctx == 0 || b.native == 0 {
		return fmt.Errorf("render extended batch: invalid batch")
	}
	if err := llama.BatchExtClear(b.native); err != nil {
		return fmt.Errorf("render extended batch: clear: %w", err)
	}
	defer func() {
		if retErr != nil {
			_ = llama.BatchExtClear(b.native)
		}
	}()

	for wantIdx, entry := range b.entries {
		idx, err := b.renderEntry(entry)
		if err != nil {
			return fmt.Errorf("render extended batch entry %d: %w", wantIdx, err)
		}
		if idx != int32(wantIdx) {
			return fmt.Errorf("render extended batch entry %d: native index %d", wantIdx, idx)
		}
	}

	return nil
}

func (b *extendedBatch) renderEntry(entry extendedBatchEntry) (int32, error) {
	var (
		idx int32
		err error
	)

	switch {
	case entry.hasToken:
		idx, err = llama.BatchExtAddToken(b.native, entry.sequenceID, entry.token)
	case len(entry.embedding) > 0:
		idx, err = llama.BatchExtAddEmbd(b.native, entry.sequenceID, entry.embedding, entry.embdWidth)
	default:
		return -1, fmt.Errorf("entry has no token or embedding")
	}
	if err != nil {
		return idx, err
	}

	for _, sequenceID := range entry.extraSequenceIDs {
		if err := llama.BatchExtAddSeq(b.native, idx, sequenceID); err != nil {
			return idx, err
		}
	}
	if entry.hasToken && len(entry.embedding) > 0 {
		if err := llama.BatchExtSetEmbdToken(b.native, idx, entry.embedding, entry.embdWidth); err != nil {
			return idx, err
		}
	}
	if err := llama.BatchExtSetPos(b.native, idx, entry.positions[:entry.positionCount]...); err != nil {
		return idx, err
	}

	switch entry.output {
	case extendedBatchOutputNone:
	case extendedBatchOutputLogits:
		if err := llama.BatchExtSetOutputLogits(b.native, idx, true); err != nil {
			return idx, err
		}
	case extendedBatchOutputEmbeddings:
		if err := llama.BatchExtSetOutputEmbd(b.native, idx, true); err != nil {
			return idx, err
		}
	}

	return idx, nil
}
