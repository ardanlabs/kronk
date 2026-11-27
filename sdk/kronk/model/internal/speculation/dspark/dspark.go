package dspark

import (
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// Host supplies target batch synchronization without exposing slots or engine
// state. Target output processing remains owned by the model engine.
type Host interface {
	speculation.Host
	ResetDSparkSequence(slot int) error
	WithDSparkTarget(consume func(SyncInput) []SyncResult)
	DSparkGenerationInput(slot int) GenerationInput
	CommitSpeculative(slot int, candidates []llama.Token, targetRange speculation.TargetRange) error
	HasSpeculativeRound(slot int) bool
	HasPendingFinalize(slot int) bool
	MatchVerifyInput(slot int, buf []byte) speculation.MatchInput
	CommitMatchVerify(slot int, buf []byte, result speculation.MatchResult)
	DistributionVerifyInput(slot int, buf []byte, distributions [][]llama.DraftCandidate) speculation.DistributionInput
	CommitDistributionVerify(slot int, buf []byte, result speculation.DistributionResult)
	SpeculativeFinalizePlan(slot int) (speculation.FinalizePlan, bool)
	RollbackSpeculativeTarget(slot int, plan speculation.FinalizePlan) (bool, error)
	CompleteSpeculativeFinalize(slot int, buf []byte, plan speculation.FinalizePlan, restored bool)
	Fail(slot int, err error)
}

// GenerationInput borrows proposal resources and describes the request's limits.
// The algorithm owns support policy; the host owns slot and parameter adaptation.
type GenerationInput struct {
	Temperature        float32
	SamplingCompatible bool
	HasMedia           bool
	MaxDraft           int
	Generate           func(count int) (Proposal, error)
}

// New constructs a block controller with request-local synchronization.
func New(host Host) *Controller {
	c := Controller{host: host, reasons: make([]string, host.SlotCount()), proposals: make([]Proposal, host.SlotCount())}
	return &c
}

// Controller owns proposal policy and the accepted-feature handoff. Target
// sampling, output processing, and rollback stay behind the shared host contract.
type Controller struct {
	host      Host
	reasons   []string
	proposals []Proposal
}

func (*Controller) Mode() speculation.Mode                        { return speculation.ModeDSpark }
func (*Controller) Enabled() bool                                 { return true }
func (*Controller) BeginBatch()                                   {}
func (*Controller) Prepare()                                      {}
func (*Controller) TargetRowsStaged(int, speculation.TargetRange) {}

// StartRequest invalidates this sequence's native prefix and Markov state.
func (c *Controller) StartRequest(slot int) error {
	c.reasons[slot] = ""
	c.proposals[slot] = Proposal{}
	return c.host.ResetDSparkSequence(slot)
}

// PlanGeneration proposes only supported text requests. Unsupported
// requests keep the authoritative target sampler and expose a fallback reason.
func (c *Controller) PlanGeneration(slot int) (speculation.Generation, error) {
	if c.reasons[slot] != "" {
		return speculation.Generation{}, nil
	}
	input := c.host.DSparkGenerationInput(slot)
	switch {
	case input.HasMedia:
		c.reasons[slot] = "unsupported-media"
	case !input.SamplingCompatible:
		c.reasons[slot] = "unsupported-sampling"
	}
	if c.reasons[slot] != "" || input.MaxDraft <= 0 {
		return speculation.Generation{}, nil
	}
	proposal, err := input.Generate(input.MaxDraft)
	if err != nil {
		// Cleanup can also fail; do not conceal uncertain native state behind
		// a target-only fallback. Fail this request and reset before reuse.
		return speculation.Generation{}, fmt.Errorf("generating DSpark block: %w", err)
	}
	c.proposals[slot] = proposal
	return speculation.Generation{Candidates: proposal.Candidates, Mode: "dspark"}, nil
}

func (c *Controller) CommitGeneration(slot int, candidates []llama.Token, targetRange speculation.TargetRange) error {
	return c.host.CommitSpeculative(slot, candidates, targetRange)
}

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
			return c.record(SynchronizeTarget(input))
		}
		return nil
	})
	return nil
}

// DisableReason reports why this request uses target-only generation.
func (c *Controller) DisableReason(slot int) string { return c.reasons[slot] }

func (c *Controller) record(results []SyncResult) []SyncResult {
	for _, result := range results {
		if result.Err != nil {
			c.reasons[result.Slot] = "accepted-prefix-unavailable"
		}
	}
	return results
}

// AfterTargetDecode copies and injects the target features before processing
// output can finish a slot and clear its native sequence.
func (c *Controller) AfterTargetDecode(buf []byte) {
	var captured SyncInput
	var captureErr error
	c.host.WithDSparkTarget(func(input SyncInput) []SyncResult {
		captured = input
		if input.RowCount == 0 {
			return nil
		}
		// Copy the entire shared tray BEFORE any slot can roll back/redecode
		// the target. Speculative suffixes must not reach sidecar accepted KV.
		captureErr = input.Resources.Capture(input.RowCount)
		ordinary := input
		ordinary.Sequences = nil
		for _, seq := range input.Sequences {
			if !c.host.HasSpeculativeRound(seq.Slot) {
				ordinary.Sequences = append(ordinary.Sequences, seq)
			}
		}
		return c.record(synchronizeCaptured(ordinary, captureErr))
	})
	for slot := range c.host.SlotCount() {
		if !c.host.SlotActive(slot) {
			continue
		}
		if !c.host.HasSpeculativeRound(slot) {
			c.host.ProcessOrdinary(slot, buf)
			continue
		}
		proposal := c.proposals[slot]
		if proposal.Distributions != nil {
			input := c.host.DistributionVerifyInput(slot, buf, proposal.Distributions)
			c.host.CommitDistributionVerify(slot, buf, speculation.VerifyDistribution(input))
		} else {
			input := c.host.MatchVerifyInput(slot, buf)
			c.host.CommitMatchVerify(slot, buf, speculation.VerifyMatches(input))
		}
		c.proposals[slot] = Proposal{}
	}
	// All target samples and bonus logprobs have now been consumed. Native
	// recovery may invalidate the target tray, but not captured feature copies.
	for _, seq := range captured.Sequences {
		if !c.host.SlotActive(seq.Slot) || !c.host.HasPendingFinalize(seq.Slot) {
			continue
		}
		plan, ok := c.host.SpeculativeFinalizePlan(seq.Slot)
		if !ok {
			continue
		}
		restored, err := c.host.RollbackSpeculativeTarget(seq.Slot, plan)
		if err != nil {
			c.host.Fail(seq.Slot, err)
			continue
		}
		seq.Rows = seq.Rows[:1+plan.Accepted]
		accepted := captured
		accepted.Sequences = []SequenceInput{seq}
		c.record(synchronizeCaptured(accepted, captureErr))
		c.host.CompleteSpeculativeFinalize(seq.Slot, buf, plan, restored)
	}
}

var _ speculation.Controller = (*Controller)(nil)
