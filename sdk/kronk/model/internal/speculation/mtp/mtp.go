// Package mtp provides Multi-Token Prediction lifecycle orchestration.
package mtp

import (
	"errors"
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// Controller implements MTP speculative-decoding lifecycle orchestration.
type Controller struct {
	host Host
}

// Host exposes target-engine lifecycle and one-step llama operations to MTP.
// Candidate-loop policy remains owned by this package.
type Host interface {
	SlotCount() int
	SlotActive(slot int) bool
	MTPDisableReason(slot int) string
	MTPPrefillInput(slot int, prefill speculation.Prefill) (SyncInput, error)
	CanSpeculate(slot int) bool
	MTPDraftInput(slot int) (DraftInput, error)
	CommitMTPDraft(slot int, result DraftResult)
	CommitSpeculative(slot int, candidates []llama.Token, targetRange speculation.TargetRange) error
	TrackTargetRange(slot int, targetRange speculation.TargetRange)
	ResetTargetRange(slot int)
	HasSpeculativeRound(slot int) bool
	HasPendingFinalize(slot int) bool
	MTPSyncInput(slot int, effectiveCount int) (SyncInput, error)
	CommitMTPSync(slot int, result SyncResult)
	ProcessOrdinary(slot int, buf []byte)
	MTPVerifyInput(slot int, buf []byte) (VerifyInput, error)
	CommitMatchVerify(slot int, buf []byte, result VerifyResult)
	SpeculativeFinalizePlan(slot int) (FinalizePlan, bool)
	RollbackSpeculativeTarget(slot int, plan FinalizePlan) (bool, error)
	RollbackMTPDraft(slot int, plan FinalizePlan) error
	DisableMTP(slot int, reason string, accepted int) error
	CompleteSpeculativeFinalize(slot int, buf []byte, plan FinalizePlan, hybridRestore bool)
	Fail(slot int, err error)
}

// New constructs an MTP speculation controller.
func New(host Host) *Controller {
	return &Controller{host: host}
}

func (*Controller) Mode() speculation.Mode { return speculation.ModeMTP }
func (*Controller) Enabled() bool          { return true }

// StartRequest retains MTP's cached native prefix; the host resets slot state.
func (*Controller) StartRequest(int) error { return nil }

// DisableReason reports the selected slot's request-local fallback status.
func (c *Controller) DisableReason(slot int) string { return c.host.MTPDisableReason(slot) }

// AfterPrefill synchronizes a direct target contribution without emitting tokens.
// A failed draft handoff disables this request; target or cleanup failures remain
// fatal to the caller.
func (c *Controller) AfterPrefill(slot int, prefill speculation.Prefill) error {
	input, err := c.host.MTPPrefillInput(slot, prefill)
	if err == nil {
		var result SyncResult
		result, err = Synchronize(input)
		if err == nil {
			c.host.CommitMTPSync(slot, result)
			return nil
		}
	}
	if disableErr := c.host.DisableMTP(slot, "sync-error", 0); disableErr != nil {
		return fmt.Errorf("disabling MTP after prefill sync failure: %w", errors.Join(err, disableErr))
	}
	return nil
}

func (c *Controller) BeginBatch() {
	for slot := range c.host.SlotCount() {
		c.host.ResetTargetRange(slot)
	}
}

func (*Controller) Prepare() {}

func (c *Controller) PlanGeneration(slot int) (speculation.Generation, error) {
	if !c.host.CanSpeculate(slot) {
		return speculation.Generation{}, nil
	}
	input, err := c.host.MTPDraftInput(slot)
	if err != nil {
		return speculation.Generation{}, err
	}
	result, err := Generate(input)
	if err != nil {
		return speculation.Generation{}, fmt.Errorf("generating MTP draft tokens: %w", err)
	}
	// DecodeStep may disable MTP after a native draft failure. Discard any
	// candidates produced before that failure instead of committing state from
	// a draft context that has been cleared or may be poisoned.
	if !c.host.CanSpeculate(slot) {
		return speculation.Generation{}, nil
	}
	c.host.CommitMTPDraft(slot, result)
	return speculation.Generation{Candidates: result.Candidates, Mode: "mtp"}, nil
}

func (c *Controller) CommitGeneration(slot int, candidates []llama.Token, targetRange speculation.TargetRange) error {
	if err := c.host.CommitSpeculative(slot, candidates, targetRange); err != nil {
		return err
	}
	c.host.TrackTargetRange(slot, targetRange)
	return nil
}

func (c *Controller) TargetRowsStaged(slot int, targetRange speculation.TargetRange) {
	c.host.TrackTargetRange(slot, targetRange)
}

func (c *Controller) AfterTargetDecode(buf []byte) {
	for slot := range c.host.SlotCount() {
		if !c.host.SlotActive(slot) || c.host.HasSpeculativeRound(slot) {
			continue
		}
		input, err := c.host.MTPSyncInput(slot, 0)
		if err != nil {
			if disableErr := c.host.DisableMTP(slot, "sync-error", 0); disableErr != nil {
				c.host.Fail(slot, disableErr)
				continue
			}
			c.host.ProcessOrdinary(slot, buf)
			continue
		}
		result, err := Synchronize(input)
		if err != nil {
			if disableErr := c.host.DisableMTP(slot, "sync-error", 0); disableErr != nil {
				c.host.Fail(slot, disableErr)
				continue
			}
			c.host.ProcessOrdinary(slot, buf)
			continue
		}
		c.host.CommitMTPSync(slot, result)
		c.host.ProcessOrdinary(slot, buf)
	}
	for slot := range c.host.SlotCount() {
		if c.host.SlotActive(slot) && c.host.HasSpeculativeRound(slot) {
			input, err := c.host.MTPVerifyInput(slot, buf)
			if err != nil {
				c.host.Fail(slot, fmt.Errorf("MTP verify: %w", err))
				continue
			}
			c.host.CommitMatchVerify(slot, buf, Verify(input))
		}
	}
	for slot := range c.host.SlotCount() {
		if c.host.SlotActive(slot) && c.host.HasPendingFinalize(slot) {
			c.finalize(slot, buf)
		}
	}
}

func (c *Controller) finalize(slot int, buf []byte) {
	plan, ok := c.host.SpeculativeFinalizePlan(slot)
	if !ok {
		return
	}
	hybridRestore, err := c.host.RollbackSpeculativeTarget(slot, plan)
	if err != nil {
		c.host.Fail(slot, err)
		return
	}
	if err := c.host.RollbackMTPDraft(slot, plan); err != nil {
		c.host.Fail(slot, err)
		return
	}

	input, err := c.host.MTPSyncInput(slot, 1+plan.Accepted)
	if err == nil {
		var result SyncResult
		result, err = Synchronize(input)
		if err == nil {
			c.host.CommitMTPSync(slot, result)
		}
	}
	if err != nil {
		if disableErr := c.host.DisableMTP(slot, "sync-error", plan.Accepted); disableErr != nil {
			c.host.Fail(slot, disableErr)
			return
		}
	}
	c.host.CompleteSpeculativeFinalize(slot, buf, plan, hybridRestore)
}
