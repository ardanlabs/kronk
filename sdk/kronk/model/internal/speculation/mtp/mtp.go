// Package mtp provides Multi-Token Prediction lifecycle orchestration.
package mtp

import (
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
	CommitMTPVerify(slot int, buf []byte, result VerifyResult)
	MTPFinalizePlan(slot int) (FinalizePlan, bool)
	RollbackMTPTarget(slot int, plan FinalizePlan) (bool, error)
	RollbackMTPDraft(slot int, plan FinalizePlan) error
	DisableMTP(slot int, reason string, accepted int) error
	CompleteMTPFinalize(slot int, buf []byte, plan FinalizePlan, hybridRestore bool)
	Fail(slot int, err error)
}

// New constructs an MTP speculation controller.
func New(host Host) *Controller {
	return &Controller{host: host}
}

func (*Controller) Mode() speculation.Mode { return speculation.ModeMTP }
func (*Controller) Enabled() bool          { return true }

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
			c.host.Fail(slot, fmt.Errorf("MTP sync: %w", err))
			continue
		}
		result, err := Synchronize(input)
		if err != nil {
			c.host.Fail(slot, fmt.Errorf("MTP sync: %w", err))
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
			c.host.CommitMTPVerify(slot, buf, Verify(input))
		}
	}
	for slot := range c.host.SlotCount() {
		if c.host.SlotActive(slot) && c.host.HasPendingFinalize(slot) {
			c.finalize(slot, buf)
		}
	}
}

func (c *Controller) finalize(slot int, buf []byte) {
	plan, ok := c.host.MTPFinalizePlan(slot)
	if !ok {
		return
	}
	hybridRestore, err := c.host.RollbackMTPTarget(slot, plan)
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
	c.host.CompleteMTPFinalize(slot, buf, plan, hybridRestore)
}
