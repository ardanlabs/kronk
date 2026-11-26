package dspark

import "github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"

// Host supplies target batch synchronization without exposing slots or engine
// state. Target output processing remains owned by the model engine.
type Host interface {
	speculation.Host
	ResetDSparkSequence(slot int) error
	WithDSparkTarget(consume func(SyncInput) []SyncResult)
}

// New constructs a feature-synchronization-only controller. The embedded
// target-only controller deliberately generates no candidates until DSpark
// verification is implemented; Enabled remains false.
func New(host Host) *Controller {
	c := Controller{Controller: speculation.NewDisabled(host), host: host}
	return &c
}

// Controller synchronizes accepted target rows before ordinary output handling.
type Controller struct {
	speculation.Controller
	host Host
}

// Mode identifies the selected algorithm even while proposal execution is gated.
func (*Controller) Mode() speculation.Mode { return speculation.ModeDSpark }

// StartRequest invalidates this sequence's native prefix and Markov state.
func (c *Controller) StartRequest(slot int) error { return c.host.ResetDSparkSequence(slot) }

// AfterPrefill consumes the host's borrowed prefill batch without emitting tokens.
func (c *Controller) AfterPrefill(slot int, prefill speculation.Prefill) error {
	c.host.WithDSparkTarget(func(input SyncInput) []SyncResult {
		for _, seq := range input.Sequences {
			if seq.Slot != slot {
				continue
			}
			rows := seq.Rows[:0]
			for _, row := range seq.Rows {
				if row.Row >= int(prefill.Start) && row.Row < int(prefill.Start+prefill.Count) {
					rows = append(rows, row)
				}
			}
			seq.Rows = rows
			input.Sequences = []SequenceInput{seq}
			return SynchronizeTarget(input)
		}
		return nil
	})
	return nil
}

// DisableReason identifies the phase's intentionally unavailable proposal path.
func (*Controller) DisableReason(int) string { return "dspark-runtime-unavailable" }

// AfterTargetDecode copies and injects the target features before processing
// output can finish a slot and clear its native sequence.
func (c *Controller) AfterTargetDecode(buf []byte) {
	c.host.WithDSparkTarget(SynchronizeTarget)
	c.Controller.AfterTargetDecode(buf)
}

var _ speculation.Controller = (*Controller)(nil)
