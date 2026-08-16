// Package classic provides separate-GGUF speculative decoding.
package classic

import (
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// Controller implements the classic speculative-decoding lifecycle.
type Controller struct {
	host Host
}

// Host exposes target, draft, and output operations to the classic algorithm.
type Host interface {
	SlotCount() int
	SlotActive(slot int) bool
	SlotNeedsDraftPrefill(slot int) bool
	PrefillDraft(slot int) error
	CanSpeculate(slot int) bool
	ClassicGenerationInput(slot int) (GenerationInput, error)
	CommitClassicDraft(slot int, result GenerationResult)
	CommitSpeculative(slot int, candidates []llama.Token, targetRange speculation.TargetRange) error
	HasSpeculativeRound(slot int) bool
	HasPendingFinalize(slot int) bool
	ProcessOrdinary(slot int, buf []byte)
	ClassicVerifyInput(slot int, buf []byte) (VerifyInput, error)
	CommitClassicVerify(slot int, buf []byte, result VerifyResult)
	ClassicFinalizePlan(slot int) (FinalizePlan, bool)
	RollbackClassicTarget(slot int, plan FinalizePlan) (bool, error)
	RollbackClassicDraft(slot int, plan FinalizePlan) error
	CompleteClassicFinalize(slot int, buf []byte, plan FinalizePlan, hybridRestore bool)
	Fail(slot int, err error)
}

// New constructs a classic speculation controller.
func New(host Host) *Controller {
	c := Controller{host: host}
	return &c
}

func (*Controller) Mode() speculation.Mode { return speculation.ModeClassic }
func (*Controller) Enabled() bool          { return true }
func (*Controller) BeginBatch()            {}

func (c *Controller) Prepare() {
	for slot := range c.host.SlotCount() {
		if !c.host.SlotActive(slot) || !c.host.SlotNeedsDraftPrefill(slot) {
			continue
		}
		if err := c.host.PrefillDraft(slot); err != nil {
			c.host.Fail(slot, err)
		}
	}
}

func (c *Controller) PlanGeneration(slot int) (speculation.Generation, error) {
	if !c.host.CanSpeculate(slot) {
		return speculation.Generation{}, nil
	}
	input, err := c.host.ClassicGenerationInput(slot)
	if err != nil {
		return speculation.Generation{}, err
	}
	result, err := Generate(input)
	if err != nil {
		return speculation.Generation{}, fmt.Errorf("generating classic draft tokens: %w", err)
	}
	c.host.CommitClassicDraft(slot, result)
	return speculation.Generation{Candidates: result.Candidates, Mode: "classic"}, nil
}

func (c *Controller) CommitGeneration(slot int, candidates []llama.Token, targetRange speculation.TargetRange) error {
	return c.host.CommitSpeculative(slot, candidates, targetRange)
}

func (*Controller) TargetRowsStaged(int, speculation.TargetRange) {}

func (c *Controller) AfterTargetDecode(buf []byte) {
	for slot := range c.host.SlotCount() {
		if c.host.SlotActive(slot) && !c.host.HasSpeculativeRound(slot) {
			c.host.ProcessOrdinary(slot, buf)
		}
	}
	for slot := range c.host.SlotCount() {
		if !c.host.SlotActive(slot) || !c.host.HasSpeculativeRound(slot) {
			continue
		}
		input, err := c.host.ClassicVerifyInput(slot, buf)
		if err != nil {
			c.host.Fail(slot, err)
			continue
		}
		c.host.CommitClassicVerify(slot, buf, Verify(input))
	}
	for slot := range c.host.SlotCount() {
		if c.host.SlotActive(slot) && c.host.HasPendingFinalize(slot) {
			c.finalize(slot, buf)
		}
	}
}

func (c *Controller) finalize(slot int, buf []byte) {
	plan, ok := c.host.ClassicFinalizePlan(slot)
	if !ok {
		return
	}
	hybridRestore, err := c.host.RollbackClassicTarget(slot, plan)
	if err != nil {
		c.host.Fail(slot, err)
		return
	}
	if err := c.host.RollbackClassicDraft(slot, plan); err != nil {
		c.host.Fail(slot, err)
		return
	}
	c.host.CompleteClassicFinalize(slot, buf, plan, hybridRestore)
}
