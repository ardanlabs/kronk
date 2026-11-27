package model

import (
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"
	classicengine "github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation/classic"
	dsparkengine "github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation/dspark"
	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation/mtp"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
	"github.com/hybridgroup/yzma/pkg/llama"
)

func newSpeculationController(e *batchEngine) speculation.Controller {
	plan := e.model.cfg.speculationPlan
	switch plan.Source {
	case speculationSourceClassic:
		if plan.Active() {
			return classicengine.New(e)
		}
	case speculationSourceMTP:
		if plan.Active() {
			return mtp.New(e)
		}
	case speculation.SourceDSpark:
		if e.model.draft != nil {
			return dsparkengine.New(e)
		}
	}
	return speculation.NewDisabled(e)
}

func (e *batchEngine) afterDraftPrefill(s *slot, batch *extendedBatch, start int, nextPosition llama.Pos) error {
	e.decodedPrefill = batch
	defer func() { e.decodedPrefill = nil }()
	count := int32(batch.len() - start)
	return e.speculation.AfterPrefill(s.id, speculation.Prefill{
		Start: int32(start), Count: count, NextPosition: nextPosition,
	})
}

func (e *batchEngine) SlotCount() int {
	return len(e.slots)
}

func (e *batchEngine) SlotActive(slotID int) bool {
	return e.slots[slotID].active
}

func (e *batchEngine) SlotNeedsDraftPrefill(slotID int) bool {
	s := e.slots[slotID]
	return s.prefillDone && s.draftPrefillNeeded
}

func (e *batchEngine) PrefillDraft(slotID int) error {
	s := e.slots[slotID]
	return e.prefillDraft(s.job.ctx, s)
}

func (e *batchEngine) CanSpeculate(slotID int) bool {
	s := e.slots[slotID]
	if e.model.cfg.speculationPlan.Source == speculationSourceClassic && !independentDraftSamplingSupported(s.job.params) {
		return false
	}
	return e.model.draft != nil && !s.draftPrefillNeeded && s.draftNPast > 0 && (!e.model.cfg.speculationPlan.MTP() || !s.mtp.Disabled)
}

func independentDraftSamplingSupported(p Params) bool {
	return p.Grammar == "" &&
		p.AdaptivePTarget == 0 &&
		p.DryMultiplier == 0 &&
		p.FrequencyPenalty == 0 &&
		p.PresencePenalty == 0 &&
		p.RepeatPenalty == DefRepeatPenalty &&
		p.XtcProbability == 0
}

func (e *batchEngine) ClassicGenerationInput(slotID int) (classicengine.GenerationInput, error) {
	if _, ok := e.model.draft.(*classicDrafter); !ok {
		return classicengine.GenerationInput{}, fmt.Errorf("classic controller requires a classic drafter")
	}
	s := e.slots[slotID]
	draft := e.model.draft.core()
	return classicengine.GenerationInput{
		State:    &s.classic,
		MaxDraft: e.maxDraftForSlot(s, draft.nDraft),
		Generate: func(count int) (classicengine.GenerationResult, error) {
			return e.generateClassicDraft(s, count)
		},
	}, nil
}

func (e *batchEngine) CommitClassicDraft(slotID int, result classicengine.GenerationResult) {
	s := e.slots[slotID]
	s.draftTokensBuf = result.Candidates
	s.classic.DraftDistributions = result.Distributions
	s.specDraftedTotal += len(result.Candidates)
}

func (e *batchEngine) MTPDraftInput(slotID int) (mtp.DraftInput, error) {
	return e.mtpDraftInput(e.slots[slotID])
}

func (e *batchEngine) CommitMTPDraft(slotID int, result mtp.DraftResult) {
	s := e.slots[slotID]
	s.draftTokensBuf = result.Candidates
	s.mtp.DraftHidden = result.Hidden
	s.draftNPast = result.Position
	if mtpUsesSharedKV(e.model.draft) && len(result.Hidden) == mtpResources(e.model.draft).EmbeddingSize() {
		s.mtp.PendingHidden = append(s.mtp.PendingHidden[:0], result.Hidden...)
	}
	s.specDraftedTotal += len(result.Candidates)
}

func (e *batchEngine) CommitSpeculative(slotID int, candidates []llama.Token, targetRange speculation.TargetRange) error {
	s := e.slots[slotID]
	s.specBasePast = targetRange.BasePos
	s.specBaseBatch = targetRange.Start
	s.specDraftTokens = candidates

	s.specSnapshot = s.specSnapshot[:0]
	if needsTargetSpecSnapshot(e.model.modelInfo.profile.MemorySemantics, e.model.ctxParams.NRsSeq, len(candidates)) {
		if err := e.captureTargetSpecSnapshot(s); err != nil {
			e.model.log(s.job.ctx, "speculative", "status", "snapshot-error", "slot", s.id, "err", err)
			s.specSnapshot = s.specSnapshot[:0]
			return fmt.Errorf("capture target state for speculative decoding: %w", err)
		}
	}

	return nil
}

func (e *batchEngine) TrackTargetRange(slotID int, targetRange speculation.TargetRange) {
	s := e.slots[slotID]
	s.mtp.TrackTargetRange(targetRange)
}

func (e *batchEngine) ResetTargetRange(slotID int) {
	s := e.slots[slotID]
	s.mtp.ClearTargetRange()
}

func (e *batchEngine) HasSpeculativeRound(slotID int) bool {
	return e.slots[slotID].specDraftTokens != nil
}

func (e *batchEngine) HasPendingFinalize(slotID int) bool {
	return e.slots[slotID].specPendingFinalize
}

func (e *batchEngine) MTPSyncInput(slotID int, effectiveCount int) (mtp.SyncInput, error) {
	s := e.slots[slotID]
	if !s.mtp.HasTargetRows || s.mtp.Disabled {
		return mtp.SyncInput{}, nil
	}

	resources := mtpResources(e.model.draft)
	count := int(s.mtp.TargetRange.Count)
	if effectiveCount > 0 {
		count = effectiveCount
	}
	start := int(s.mtp.TargetRange.Start)
	totalRows := e.batch.len()
	if start < 0 || count <= 0 || start+count > totalRows {
		return mtp.SyncInput{}, fmt.Errorf("target rows unavailable for range [%d..%d)", start, start+count)
	}
	var hiddenRows []float32
	nEmbd := resources.EmbeddingSize()
	if len(s.mtp.VerifyHidden) >= count*nEmbd {
		hiddenRows = s.mtp.VerifyHidden[:count*nEmbd]
	} else if !resources.DeviceRows() {
		hidden := resources.TargetHidden(totalRows)
		if hidden == nil {
			return mtp.SyncInput{}, fmt.Errorf("target pre-norm rows unavailable for range [%d..%d)", start, start+count)
		}
		hiddenRows = hidden[start*nEmbd : (start+count)*nEmbd]
	}
	return e.mtpSyncInput(s, e.batch.entries[start:start+count], hiddenRows, s.mtp.TargetRange.BasePos+llama.Pos(count), start)
}

func (e *batchEngine) CommitMTPSync(slotID int, result mtp.SyncResult) {
	if !result.Active {
		return
	}
	s := e.slots[slotID]
	s.mtp.PendingHidden = result.PendingHidden
	s.draftNPast = result.Position
	s.mtp.VerifyHidden = s.mtp.VerifyHidden[:0]
	s.mtp.ClearTargetRange()
}

func (e *batchEngine) ProcessOrdinary(slotID int, buf []byte) {
	s := e.slots[slotID]
	if s.iBatch >= 0 {
		e.processSlotToken(s, buf)
	}
}

func (e *batchEngine) ClassicVerifyInput(slotID int, buf []byte) (classicengine.VerifyInput, error) {
	s := e.slots[slotID]
	distributions := s.classic.DraftDistributions
	s.classic.DraftDistributions = nil
	input := e.DistributionVerifyInput(slotID, buf, distributions)
	return classicengine.VerifyInput{
		State: &s.classic, Candidates: input.Candidates, Distributions: input.Distributions,
		Greedy: input.Greedy, Target: input.Target, Accept: input.Accept,
		Random: input.Random, Random32: input.Random32,
	}, nil
}

// DistributionVerifyInput adapts target distributions and output processing
// without selecting an algorithm's proposal law or acceptance policy.
func (e *batchEngine) DistributionVerifyInput(slotID int, buf []byte, distributions [][]llama.DraftCandidate) speculation.DistributionInput {
	s := e.slots[slotID]
	draft := e.model.draft.core()
	nVocab := int(llama.VocabNTokens(e.model.vocab))
	greedy := s.job.params.Temperature == 0
	s.specPendingOriginalSampled = s.sampled
	if len(draft.targetProbs) != nVocab {
		draft.targetProbs = make([]float32, nVocab)
	}

	return speculation.DistributionInput{
		Candidates:    s.specDraftTokens,
		Distributions: distributions,
		Greedy:        greedy,
		Target: func(index int) speculation.DistributionTarget {
			row := s.specBaseBatch + int32(index)
			if s.grammarSampler != nil && s.reasonFlag == 0 {
				return speculation.DistributionTarget{Token: e.sampleSlotToken(s, row), SamplerAccepted: true}
			}
			logits, err := llama.GetLogitsIth(e.model.lctx, row, nVocab)
			if err != nil {
				return speculation.DistributionTarget{Token: e.sampleSlotToken(s, row), SamplerAccepted: true}
			}
			if greedy {
				maskSuppressTokenLogits(logits, e.model.suppressTokens)
				return speculation.DistributionTarget{Token: classicengine.GreedyToken(logits)}
			}
			draft.sortIndices = applySamplerFilters(logits, draft.targetProbs, e.model.suppressTokens,
				s.job.params.Temperature, s.job.params.TopP, s.job.params.MinP, s.job.params.TopK,
				draft.sortIndices, &draft.filterBuf)
			return speculation.DistributionTarget{Probabilities: draft.targetProbs}
		},
		Accept: func(index int, token llama.Token, samplerAccepted bool) bool {
			s.specAcceptedTotal++
			s.nPast = specAcceptedNPast(s.specBasePast, index+1)
			e.handleSpeculativeToken(s, token, s.specBaseBatch+int32(index), buf, samplerAccepted, false, nil)
			return s.active
		},
		Random:   s.specRNG.Float64,
		Random32: s.specRNG.Float32,
	}
}

func (e *batchEngine) CommitDistributionVerify(slotID int, buf []byte, result speculation.DistributionResult) {
	s := e.slots[slotID]
	if !result.Complete || !s.active {
		return
	}

	var bonusLogprob *ContentLogprob
	bonusLogprobReady := false
	if s.job.params.Logprobs {
		bonusLogprobReady = true
		var err error
		bonusLogprob, err = extractLogprobs(e.model.lctx, e.model.vocab, e.model.suppressTokens, result.Bonus,
			s.specBaseBatch+int32(result.Accepted), s.job.params.TopLogprobs, buf)
		if err != nil {
			e.model.log(s.job.ctx, "batch-engine", "status", "logprobs-error", "slot", s.id, "error", err.Error())
		}
	}
	s.specPendingAccepted = result.Accepted
	s.specPendingBonusToken = result.Bonus
	s.specPendingSamplerAccepted = result.SamplerAccepted
	s.specPendingLogprobReady = bonusLogprobReady
	s.specPendingLogprob = bonusLogprob
	s.specPendingFinalize = true
}

func (e *batchEngine) RollbackClassicDraft(slotID int, plan classicengine.FinalizePlan) error {
	s := e.slots[slotID]
	return e.rollbackDraft(s.job.ctx, s, plan.Accepted, plan.Drafted)
}

func (e *batchEngine) CompleteClassicFinalize(slotID int, buf []byte, plan classicengine.FinalizePlan, hybridRestore bool) {
	s := e.slots[slotID]
	s.classic.Rounds++
	if s.classic.Rounds == 1 || s.classic.Rounds%32 == 0 {
		e.model.log(s.job.ctx, "speculative", "status", "verify-done",
			"slot", s.id, "round", s.classic.Rounds, "accepted", plan.Accepted, "nDraft", plan.Drafted,
			"target_nPast", specAcceptedNPast(s.specBasePast, plan.Accepted), "draft_nPast", s.draftNPast,
			"acc_ema", fmt.Sprintf("%.2f", s.classic.AcceptanceEMA))
	}

	e.CompleteSpeculativeFinalize(slotID, buf, plan, hybridRestore)
}

func (e *batchEngine) MTPVerifyInput(slotID int, buf []byte) (mtp.VerifyInput, error) {
	s := e.slots[slotID]
	nDraft := len(s.specDraftTokens)
	if s.mtp.HasTargetRows && !s.mtp.Disabled {
		if err := e.captureVerifyPreNorm(s, 1+nDraft); err != nil {
			s.mtp.VerifyHidden = s.mtp.VerifyHidden[:0]
			s.mtp.Disable("verify-prenorm-capture")
		}
	}
	return e.MatchVerifyInput(slotID, buf), nil
}

// MatchVerifyInput exposes target sampling/output without algorithm state.
func (e *batchEngine) MatchVerifyInput(slotID int, buf []byte) speculation.MatchInput {
	s := e.slots[slotID]
	s.specPendingOriginalSampled = s.sampled

	return speculation.MatchInput{
		Candidates: s.specDraftTokens,
		Sample: func(index int) llama.Token {
			row := s.specBaseBatch + int32(index)
			return e.sampleSlotToken(s, row)
		},
		Accept: func(index int, token llama.Token) bool {
			s.specAcceptedTotal++
			s.nPast = specAcceptedNPast(s.specBasePast, index+1)
			e.handleSpeculativeToken(s, token, s.specBaseBatch+int32(index), buf, true, false, nil)
			return s.active
		},
	}
}

func (e *batchEngine) CommitMatchVerify(slotID int, buf []byte, result speculation.MatchResult) {
	s := e.slots[slotID]
	if !result.Complete || !s.active {
		return
	}

	var bonusLogprob *ContentLogprob
	bonusLogprobReady := false
	if s.job.params.Logprobs {
		bonusLogprobReady = true
		var err error
		bonusLogprob, err = extractLogprobs(e.model.lctx, e.model.vocab, e.model.suppressTokens, result.Bonus,
			s.specBaseBatch+int32(result.Accepted), s.job.params.TopLogprobs, buf)
		if err != nil {
			e.model.log(s.job.ctx, "batch-engine", "status", "logprobs-error", "slot", s.id, "error", err.Error())
		}
	}
	s.specPendingAccepted = result.Accepted
	s.specPendingBonusToken = result.Bonus
	s.specPendingSamplerAccepted = true
	s.specPendingLogprobReady = bonusLogprobReady
	s.specPendingLogprob = bonusLogprob
	s.specPendingFinalize = true
}

func (e *batchEngine) SpeculativeFinalizePlan(slotID int) (speculation.FinalizePlan, bool) {
	s := e.slots[slotID]
	if !s.specPendingFinalize {
		return speculation.FinalizePlan{}, false
	}
	return speculation.FinalizePlan{Accepted: s.specPendingAccepted, Drafted: len(s.specDraftTokens)}, true
}

func (e *batchEngine) RollbackSpeculativeTarget(slotID int, plan speculation.FinalizePlan) (bool, error) {
	s := e.slots[slotID]
	rollbackFrom := s.specBasePast + llama.Pos(1+plan.Accepted)
	rollbackTo := s.specBasePast + llama.Pos(1+plan.Drafted)
	recurrentRestore := e.model.modelInfo.profile.MemorySemantics == modelprofile.MemoryRecurrent && len(s.specSnapshot) > 0 && rollbackFrom < rollbackTo
	if recurrentRestore {
		if err := e.restoreTargetSpecSnapshot(s, s.specBasePast, s.specPendingOriginalSampled, s.specDraftTokens, plan.Accepted); err != nil {
			return false, fmt.Errorf("restoring target state after speculative rejection: %w", err)
		}
		e.recordTargetSpecRollback(s, true)
		return true, nil
	}
	if rollbackFrom < rollbackTo {
		e.model.decodeMu.Lock()
		removed, err := llama.MemorySeqRm(e.model.mem, s.seqID, rollbackFrom, rollbackTo)
		e.model.decodeMu.Unlock()
		if err != nil {
			return false, fmt.Errorf("removing rejected target draft positions: %w", err)
		}
		if !removed {
			return false, fmt.Errorf("removing rejected target draft positions for seq %d", s.seqID)
		}
		e.recordTargetSpecRollback(s, false)
	}
	return false, nil
}

// recordTargetSpecRollback counts completed native recovery, not just rejected
// verification results. Failed restore/removal operations must not be counted.
func (e *batchEngine) recordTargetSpecRollback(s *slot, snapshot bool) {
	strategy := "attention-suffix"
	switch {
	case snapshot:
		strategy = "recurrent-snapshot-replay"
	case e.model.modelInfo.profile.MemorySemantics == modelprofile.MemoryRecurrent:
		strategy = "recurrent-native"
	}
	if s.specTargetRollbacks == nil {
		s.specTargetRollbacks = make(map[string]uint64)
	}
	if e.specTargetRollbacks == nil {
		e.specTargetRollbacks = make(map[string]uint64)
	}
	s.specTargetRollbacks[strategy]++
	e.specTargetRollbacks[strategy]++
}

func (e *batchEngine) RollbackMTPDraft(slotID int, plan mtp.FinalizePlan) error {
	s := e.slots[slotID]
	ext, ownDraftKV := e.model.draft.(draftKVExternalizer)
	if !ownDraftKV {
		return nil
	}
	draft := ext.core()
	draftBasePast := s.draftNPast - llama.Pos(plan.Drafted)
	if draftBasePast < s.draftNPast {
		removed, err := llama.MemorySeqRm(draft.mem, s.seqID, draftBasePast, s.draftNPast)
		if err != nil {
			return fmt.Errorf("removing MTP draft positions: %w", err)
		}
		if !removed {
			return fmt.Errorf("removing MTP draft positions for seq %d", s.seqID)
		}
	}
	s.draftNPast = draftBasePast
	return nil
}

func (e *batchEngine) DisableMTP(slotID int, reason string, accepted int) error {
	s := e.slots[slotID]
	return e.disableMTPForRequestSpec(s.job.ctx, s, reason, accepted)
}

func (e *batchEngine) CompleteSpeculativeFinalize(slotID int, buf []byte, plan speculation.FinalizePlan, hybridRestore bool) {
	s := e.slots[slotID]
	bonusToken := s.specPendingBonusToken
	bonusSamplerAccepted := s.specPendingSamplerAccepted
	bonusLogprobReady := s.specPendingLogprobReady
	bonusLogprob := s.specPendingLogprob
	baseBatch := s.specBaseBatch
	basePast := s.specBasePast

	s.specPendingFinalize = false
	s.specPendingAccepted = 0
	s.specPendingBonusToken = 0
	s.specPendingOriginalSampled = 0
	s.specPendingSamplerAccepted = false
	s.specPendingLogprobReady = false
	s.specPendingLogprob = nil
	s.specDraftTokens = nil
	s.nPast = specAcceptedNPast(basePast, plan.Accepted)

	bonusBatch := baseBatch + int32(plan.Accepted)
	if hybridRestore {
		bonusBatch = int32(plan.Accepted)
	}
	e.handleSpeculativeToken(s, bonusToken, bonusBatch, buf, bonusSamplerAccepted, bonusLogprobReady, bonusLogprob)
	if s.active {
		s.iBatch = -1
	}
}

func (e *batchEngine) Fail(slotID int, err error) {
	e.finishSlot(e.slots[slotID], err)
}

var _ speculation.Host = (*batchEngine)(nil)
var _ classicengine.Host = (*batchEngine)(nil)
var _ mtp.Host = (*batchEngine)(nil)
