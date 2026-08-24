package model

import (
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"
	classicengine "github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation/classic"
	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation/mtp"
	"github.com/hybridgroup/yzma/pkg/llama"
)

func newSpeculationController(e *batchEngine) speculation.Controller {
	plan := e.model.cfg.speculationPlan
	if !plan.Active() {
		return speculation.NewDisabled(e)
	}

	switch plan.Source {
	case speculationSourceClassic:
		return classicengine.New(e)
	case speculationSourceMTPCompanion, speculationSourceMTPEmbedded:
		return mtp.New(e)
	}
	return speculation.NewDisabled(e)
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
	return e.model.draft != nil && !s.draftPrefillNeeded && s.draftNPast > 0 && (!e.model.cfg.speculationPlan.MTP() || !s.mtp.Disabled)
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
	if _, shared := e.model.draft.(*sharedMTPDrafter); shared && len(result.Hidden) == e.model.draft.core().mtp.EmbeddingSize() {
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
	if needsTargetSpecSnapshot(e.model.modelInfo.Type, e.model.ctxParams.NRsSeq, len(candidates)) {
		if err := e.captureTargetSpecSnapshot(s); err != nil {
			e.model.log(s.job.ctx, "speculative", "status", "snapshot-error", "slot", s.id, "err", err)
			s.specSnapshot = s.specSnapshot[:0]
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

	draft := e.model.draft.core()
	count := int(s.mtp.TargetRange.Count)
	if effectiveCount > 0 {
		count = effectiveCount
	}
	start := int(s.mtp.TargetRange.Start)
	var hiddenRows []float32
	nEmbd := draft.mtp.EmbeddingSize()
	if len(s.mtp.VerifyHidden) >= count*nEmbd {
		hiddenRows = s.mtp.VerifyHidden[:count*nEmbd]
	} else {
		totalRows := int(e.batch.NTokens)
		hidden := GetEmbeddingsPreNorm(e.model.lctx, totalRows, nEmbd)
		if hidden == nil || start < 0 || start+count > totalRows {
			return mtp.SyncInput{}, fmt.Errorf("target pre-norm rows unavailable for range [%d..%d)", start, start+count)
		}
		hiddenRows = hidden[start*nEmbd : (start+count)*nEmbd]
	}
	tokens := batchTokensAt(e.batch, start, count)
	if tokens == nil {
		return mtp.SyncInput{}, fmt.Errorf("target tokens unavailable for range [%d..%d)", start, start+count)
	}
	_, shared := e.model.draft.(*sharedMTPDrafter)

	return mtp.SyncInput{
		Tokens:        tokens,
		HiddenRows:    hiddenRows,
		PendingHidden: s.mtp.PendingHidden,
		HiddenScratch: draft.mtp.MirrorHidden,
		BasePosition:  s.mtp.TargetRange.BasePos,
		EmbeddingSize: nEmbd,
		ChunkSize:     draft.mtp.MirrorCapacity(),
		SharedKV:      shared,
		DecodeOwnChunk: func(tokens []llama.Token, basePosition llama.Pos, hiddenRows []float32, finalChunk bool) error {
			mirror := draft.mtp.MirrorBatch
			mirror.NTokens = 0
			for i, token := range tokens {
				if err := mirror.Add(token, basePosition+llama.Pos(i), s.seqIDs, finalChunk && i == len(tokens)-1); err != nil {
					return fmt.Errorf("adding MTP mirror token: %w", err)
				}
			}
			copy(draft.mtp.MirrorHidden, hiddenRows)
			ret, err := llama.Decode(draft.lctx, mirror)
			if err != nil || ret != 0 {
				return decodeError(ret, err)
			}
			llama.Synchronize(draft.lctx)
			return nil
		},
	}, nil
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

func (e *batchEngine) syncMTPCacheRows(s *slot, tokens []llama.Token, hiddenRows []float32, basePosition llama.Pos) error {
	draft := e.model.draft.core()
	nEmbd := draft.mtp.EmbeddingSize()
	_, shared := e.model.draft.(*sharedMTPDrafter)
	input := mtp.SyncInput{
		Tokens:        tokens,
		HiddenRows:    hiddenRows,
		PendingHidden: s.mtp.PendingHidden,
		HiddenScratch: draft.mtp.MirrorHidden,
		BasePosition:  basePosition,
		EmbeddingSize: nEmbd,
		ChunkSize:     draft.mtp.MirrorCapacity(),
		SharedKV:      shared,
		DecodeOwnChunk: func(tokens []llama.Token, basePosition llama.Pos, hiddenRows []float32, finalChunk bool) error {
			mirror := draft.mtp.MirrorBatch
			mirror.NTokens = 0
			for i, token := range tokens {
				if err := mirror.Add(token, basePosition+llama.Pos(i), s.seqIDs, finalChunk && i == len(tokens)-1); err != nil {
					return fmt.Errorf("adding MTP cache mirror token: %w", err)
				}
			}
			copy(draft.mtp.MirrorHidden, hiddenRows)
			ret, err := llama.Decode(draft.lctx, mirror)
			if err != nil || ret != 0 {
				return decodeError(ret, err)
			}
			llama.Synchronize(draft.lctx)
			return nil
		},
	}
	result, err := mtp.Synchronize(input)
	if err != nil {
		return err
	}
	s.mtp.PendingHidden = result.PendingHidden
	s.draftNPast = result.Position
	return nil
}

func (e *batchEngine) ProcessOrdinary(slotID int, buf []byte) {
	s := e.slots[slotID]
	if s.iBatch >= 0 {
		e.processSlotToken(s, buf)
	}
}

func (e *batchEngine) ClassicVerifyInput(slotID int, buf []byte) (classicengine.VerifyInput, error) {
	s := e.slots[slotID]
	draft := e.model.draft.core()
	nVocab := int(llama.VocabNTokens(e.model.vocab))
	greedy := s.job.params.Temperature == 0
	s.specPendingOriginalSampled = s.sampled
	distributions := s.classic.DraftDistributions
	s.classic.DraftDistributions = nil

	return classicengine.VerifyInput{
		State:         &s.classic,
		Candidates:    s.specDraftTokens,
		Distributions: distributions,
		Greedy:        greedy,
		Target: func(index int) classicengine.Target {
			row := s.specBaseBatch + int32(index)
			if s.grammarSampler != nil && s.reasonFlag == 0 {
				return classicengine.Target{Token: e.sampleSlotToken(s, row), SamplerAccepted: true}
			}
			logits, err := llama.GetLogitsIth(e.model.lctx, row, nVocab)
			if err != nil {
				return classicengine.Target{Token: e.sampleSlotToken(s, row), SamplerAccepted: true}
			}
			if greedy {
				maskSuppressTokenLogits(logits, e.model.suppressTokens)
				return classicengine.Target{Token: classicengine.GreedyToken(logits)}
			}
			draft.sortIndices = applySamplerFilters(logits, draft.targetProbs, e.model.suppressTokens,
				s.job.params.Temperature, s.job.params.TopP, s.job.params.MinP, s.job.params.TopK,
				draft.sortIndices, &draft.filterBuf)
			return classicengine.Target{Probabilities: draft.targetProbs}
		},
		Accept: func(index int, token llama.Token, samplerAccepted bool) bool {
			s.specAcceptedTotal++
			s.nPast = specAcceptedNPast(s.specBasePast, index+1)
			e.handleSpeculativeToken(s, token, s.specBaseBatch+int32(index), buf, samplerAccepted, false, nil)
			return s.active
		},
		Random:   s.specRNG.Float64,
		Random32: s.specRNG.Float32,
	}, nil
}

func (e *batchEngine) CommitClassicVerify(slotID int, buf []byte, result classicengine.VerifyResult) {
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

func (e *batchEngine) ClassicFinalizePlan(slotID int) (classicengine.FinalizePlan, bool) {
	s := e.slots[slotID]
	if !s.specPendingFinalize {
		return classicengine.FinalizePlan{}, false
	}
	return classicengine.FinalizePlan{Accepted: s.specPendingAccepted, Drafted: len(s.specDraftTokens)}, true
}

func (e *batchEngine) RollbackClassicTarget(slotID int, plan classicengine.FinalizePlan) (bool, error) {
	s := e.slots[slotID]
	rollbackFrom := s.specBasePast + llama.Pos(1+plan.Accepted)
	rollbackTo := s.specBasePast + llama.Pos(1+plan.Drafted)
	hybridRestore := e.model.modelInfo.Type == ModelTypeHybrid && len(s.specSnapshot) > 0 && rollbackFrom < rollbackTo
	if hybridRestore {
		if err := e.restoreTargetSpecSnapshot(s, s.specBasePast, s.specPendingOriginalSampled, s.specDraftTokens, plan.Accepted); err != nil {
			return false, fmt.Errorf("restoring target state after speculative rejection: %w", err)
		}
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
	}
	return false, nil
}

func (e *batchEngine) RollbackClassicDraft(slotID int, plan classicengine.FinalizePlan) error {
	s := e.slots[slotID]
	return e.rollbackDraft(s.job.ctx, s, plan.Accepted, plan.Drafted)
}

func (e *batchEngine) CompleteClassicFinalize(slotID int, buf []byte, plan classicengine.FinalizePlan, hybridRestore bool) {
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
	s.classic.Rounds++
	if s.classic.Rounds == 1 || s.classic.Rounds%32 == 0 {
		e.model.log(s.job.ctx, "speculative", "status", "verify-done",
			"slot", s.id, "round", s.classic.Rounds, "accepted", plan.Accepted, "nDraft", plan.Drafted,
			"target_nPast", s.nPast, "draft_nPast", s.draftNPast,
			"acc_ema", fmt.Sprintf("%.2f", s.classic.AcceptanceEMA))
	}

	bonusBatch := baseBatch + int32(plan.Accepted)
	if hybridRestore {
		bonusBatch = int32(plan.Accepted)
	}
	e.handleSpeculativeToken(s, bonusToken, bonusBatch, buf, bonusSamplerAccepted, bonusLogprobReady, bonusLogprob)
	if s.active {
		s.iBatch = -1
	}
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
	s.specPendingOriginalSampled = s.sampled

	return mtp.VerifyInput{
		State:      &s.mtp,
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
	}, nil
}

func (e *batchEngine) CommitMTPVerify(slotID int, buf []byte, result mtp.VerifyResult) {
	s := e.slots[slotID]
	if !result.Complete || !s.active {
		return
	}

	var bonusLogprob *ContentLogprob
	bonusLogprobReady := false
	if s.job.params.Logprobs {
		bonusLogprobReady = true
		bonusLogprob, _ = extractLogprobs(e.model.lctx, e.model.vocab, e.model.suppressTokens, result.Bonus,
			s.specBaseBatch+int32(result.Accepted), s.job.params.TopLogprobs, buf)
	}
	s.specPendingAccepted = result.Accepted
	s.specPendingBonusToken = result.Bonus
	s.specPendingSamplerAccepted = true
	s.specPendingLogprobReady = bonusLogprobReady
	s.specPendingLogprob = bonusLogprob
	s.specPendingFinalize = true
}

func (e *batchEngine) MTPFinalizePlan(slotID int) (mtp.FinalizePlan, bool) {
	s := e.slots[slotID]
	if !s.specPendingFinalize {
		return mtp.FinalizePlan{}, false
	}
	return mtp.FinalizePlan{Accepted: s.specPendingAccepted, Drafted: len(s.specDraftTokens)}, true
}

func (e *batchEngine) RollbackMTPTarget(slotID int, plan mtp.FinalizePlan) (bool, error) {
	s := e.slots[slotID]
	rollbackFrom := s.specBasePast + llama.Pos(1+plan.Accepted)
	rollbackTo := s.specBasePast + llama.Pos(1+plan.Drafted)
	hybridRestore := e.model.modelInfo.Type == ModelTypeHybrid && len(s.specSnapshot) > 0 && rollbackFrom < rollbackTo
	if hybridRestore {
		if err := e.restoreTargetSpecSnapshot(s, s.specBasePast, s.specPendingOriginalSampled, s.specDraftTokens, plan.Accepted); err != nil {
			return false, fmt.Errorf("restoring target state after speculative rejection: %w", err)
		}
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
	}
	return false, nil
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

func (e *batchEngine) CompleteMTPFinalize(slotID int, buf []byte, plan mtp.FinalizePlan, hybridRestore bool) {
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
	round := e.model.draft.core().mtpPolicy.CompleteRound(
		&s.mtp,
		plan.Accepted+1,
		time.Now(),
	)
	if round.Made {
		args := []any{"status", "draft-policy-decided",
			"mode", "mtp", "selected_nDraft", round.Draft,
			"configured_nDraft", e.model.draft.core().nDraft, "reason", round.Reason}
		if round.Reason == "throughput-trial" {
			args = append(args,
				"baseline_tps", fmt.Sprintf("%.2f", round.BaselineTPS),
				"draft_2_tps", fmt.Sprintf("%.2f", round.Draft2TPS),
				"draft_1_tps", fmt.Sprintf("%.2f", round.Draft1TPS))
		}
		e.model.log(s.job.ctx, "speculative", args...)
	}
	if round.Report {
		e.model.log(s.job.ctx, "speculative", "status", "verify-done",
			"mode", "mtp", "slot", s.id, "round", round.Round,
			"accepted", plan.Accepted, "nDraft", plan.Drafted,
			"target_nPast", s.nPast, "draft_nPast", s.draftNPast,
			"acc_ema", fmt.Sprintf("%.2f", s.mtp.AcceptanceEMA))
	}

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
