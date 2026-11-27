package dspark

import (
	"errors"
	"fmt"
	"math"

	yzmaspec "github.com/hybridgroup/yzma/exp/speculative"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// Resources owns the sidecar model, independent context/KV, injection batch,
// copied target features, and position-tagged Markov states. Target handles and
// the embedding/output tensors linked through CtxOther are borrowed. Exported
// native handles are borrowed views; Free is their only cleanup owner.
type Resources struct {
	Model      llama.Model
	Context    llama.Context
	Memory     llama.Memory
	target     llama.Context
	batch      llama.BatchExt
	layers     []int32
	width      int
	chunk      int
	features   []float32
	rows       int
	sequences  []sequence
	blockState []float32
	mask       llama.Token
	proposals  int
}

type sequence struct {
	next      llama.Pos
	positions []llama.Pos
	states    []float32
	disabled  bool
}

func newSequence(block, width int) sequence {
	s := sequence{positions: make([]llama.Pos, block+1), states: make([]float32, (block+1)*width)}
	s.reset()
	return s
}

func (s *sequence) reset() {
	s.next = 0
	s.disabled = false
	for i := range s.positions {
		s.positions[i] = -1
	}
	clear(s.states)
}

func (s *sequence) store(pos llama.Pos, state []float32) {
	index := int(pos) % len(s.positions)
	copy(s.states[index*len(state):], state)
	s.positions[index] = pos
	s.next = pos + 1
}

// ContextState returns the copied Markov state at an accepted position, or nil
// when it is absent or its ring entry has been reused. The slice is borrowed
// until this sequence is reset or synchronized again.
func (r *Resources) ContextState(seq llama.SeqId, pos llama.Pos) []float32 {
	if seq < 0 || int(seq) >= len(r.sequences) || pos < 0 {
		return nil
	}
	s := &r.sequences[seq]
	index := int(pos) % len(s.positions)
	if s.positions[index] != pos || pos >= s.next {
		return nil
	}
	return s.states[index*r.width : (index+1)*r.width]
}

// NextPosition returns the first position not yet injected for a sequence.
func (r *Resources) NextPosition(seq llama.SeqId) llama.Pos {
	return r.sequences[seq].next
}

// CheckpointState returns the borrowed Markov vector paired with the exact
// accepted prefix. Earlier ring entries are unnecessary: subsequent proposals
// start at position and synchronization stores each newly accepted position.
func (r *Resources) CheckpointState(seq llama.SeqId, position llama.Pos) []float32 {
	s := &r.sequences[seq]
	if s.disabled || s.next != position {
		return nil
	}
	return r.ContextState(seq, position-1)
}

// RestoreCheckpoint restores the Markov vector only when the independent native
// KV ends at the same accepted boundary. State is copied into this sequence's
// ring; no ring entry or disabled flag from its previous occupant survives.
func (r *Resources) RestoreCheckpoint(seq llama.SeqId, position llama.Pos, state []float32) bool {
	if position <= 0 || len(state) != r.width {
		return false
	}
	for _, value := range state {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return false
		}
	}
	last, err := llama.MemorySeqPosMax(r.Memory, seq)
	if err != nil || last != position-1 {
		return false
	}
	r.ClearCheckpoint(seq)
	r.sequences[seq].store(position-1, state)
	return true
}

// ClearCheckpoint invalidates the host state for a sequence whose native memory
// has already been removed. It does not touch other sequences or target memory.
func (r *Resources) ClearCheckpoint(seq llama.SeqId) {
	r.sequences[seq].reset()
}

// Reset removes only the selected sidecar sequence and invalidates its Markov
// states. It never clears shared target tensors or target KV.
func (r *Resources) Reset(seq llama.SeqId) error {
	removed, err := llama.MemorySeqRm(r.Memory, seq, -1, -1)
	if err != nil {
		return fmt.Errorf("DSpark reset sequence %d: %w", seq, err)
	}
	if !removed {
		return fmt.Errorf("DSpark reset sequence %d: native memory removal failed", seq)
	}
	r.ClearCheckpoint(seq)
	return nil
}

// Free unregisters target extraction before releasing owned sidecar resources.
// It is safe after a partial load and never frees the borrowed target.
func (r *Resources) Free() error {
	var errs []error
	if r.target != 0 {
		llama.Synchronize(r.target)
		for _, layer := range r.layers {
			errs = append(errs, llama.SetEmbeddingsLayerInp(r.target, uint32(layer), false))
		}
	}
	if r.batch != 0 {
		errs = append(errs, llama.BatchExtFree(r.batch))
	}
	if r.Context != 0 {
		errs = append(errs, llama.Free(r.Context))
	}
	if r.Model != 0 {
		errs = append(errs, llama.ModelFree(r.Model))
	}
	*r = Resources{}
	return errors.Join(errs...)
}

// FeatureRow maps one actual target batch row to an accepted sequence position.
// Interleaved sequences need not occupy contiguous ranges in the target batch.
type FeatureRow struct {
	Row      int
	Sequence llama.SeqId
	Position llama.Pos
}

// Capture copies every requested layer in metadata order before another target
// decode can overwrite native outputs. The caller holds the target decode lock.
func (r *Resources) Capture(rows int) error {
	r.rows = 0
	if rows <= 0 || rows*len(r.layers)*r.width > len(r.features) {
		return fmt.Errorf("DSpark capture: invalid target row count %d", rows)
	}
	llama.Synchronize(r.target)
	for k, layer := range r.layers {
		values, err := llama.GetEmbeddingsLayerInp(r.target, uint32(layer), rows, r.width)
		if err != nil {
			return fmt.Errorf("DSpark capture layer %d: %w", layer, err)
		}
		if len(values) != rows*r.width {
			return fmt.Errorf("DSpark capture layer %d: missing %d rows", layer, rows)
		}
		copyLayer(r.features, values, rows, r.width, len(r.layers), k)
	}
	r.rows = rows
	return nil
}

func copyLayer(out, values []float32, rows, width, layers, layer int) {
	for row := range rows {
		copy(out[(row*layers+layer)*width:], values[row*width:(row+1)*width])
	}
}

// Synchronize injects copied accepted features, preserving their actual batch
// indices and sequence positions. It retains each chunk's Markov states before
// the next sidecar decode overwrites native NextN output. Missing cached prefixes
// are errors, not permission to treat a suffix as a complete draft context.
func (r *Resources) Synchronize(rows []FeatureRow) error {
	if len(rows) == 0 {
		return nil
	}
	seq := rows[0].Sequence
	if seq < 0 || int(seq) >= len(r.sequences) {
		return fmt.Errorf("DSpark sync: invalid sequence %d", seq)
	}
	s := &r.sequences[seq]
	for i, row := range rows {
		if row.Sequence != seq || row.Row < 0 || row.Row >= r.rows || row.Position != s.next+llama.Pos(i) {
			return fmt.Errorf("DSpark sync: row %d sequence %d position %d does not extend sequence %d at %d", row.Row, row.Sequence, row.Position, seq, s.next+llama.Pos(i))
		}
	}
	width := len(r.layers) * r.width
	for start := 0; start < len(rows); start += r.chunk {
		chunk := rows[start:min(start+r.chunk, len(rows))]
		if err := llama.BatchExtClear(r.batch); err != nil {
			return fmt.Errorf("DSpark injection batch: %w", err)
		}
		for _, row := range chunk {
			values := r.features[row.Row*width : (row.Row+1)*width]
			idx, err := llama.BatchExtAddEmbd(r.batch, seq, values, width)
			if err != nil {
				return fmt.Errorf("DSpark feature row %d: %w", row.Row, err)
			}
			if err := llama.BatchExtSetPos(r.batch, idx, row.Position, row.Position, row.Position, 0); err != nil {
				return fmt.Errorf("DSpark feature position %d: %w", row.Position, err)
			}
		}
		code, err := llama.Process(r.Context, llama.ProcessTypeDecode, r.batch)
		if err != nil {
			return fmt.Errorf("DSpark injection decode: %w", err)
		}
		if code != 0 {
			return fmt.Errorf("DSpark injection decode returned %d", code)
		}
		llama.Synchronize(r.Context)
		states := yzmaspec.GetEmbeddingsNextN(r.Context, len(chunk), r.width)
		if len(states) != len(chunk)*r.width {
			return fmt.Errorf("DSpark injection: missing Markov states")
		}
		for i, row := range chunk {
			s.store(row.Position, states[i*r.width:(i+1)*r.width])
		}
	}
	return nil
}
