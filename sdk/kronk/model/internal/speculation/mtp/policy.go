package mtp

import "time"

const (
	adaptationWindow     = 8
	lowAcceptanceWindows = 2
	lowAcceptanceEMA     = 0.55
	observationRounds    = 32
	trialRounds          = 16
)

type policyPhase uint8

const (
	policyObserving policyPhase = iota
	policyTrialTwo
	policyTrialOne
	policyLocked
)

// Policy learns one MTP draft count for the lifetime of a loaded model.
// The batch engine serializes access to the policy across execution slots.
type Policy struct {
	phase                policyPhase
	configuredDraft      int
	activeDraft          int
	pendingDecision      bool
	baselineTokens       int
	baselineElapsed      time.Duration
	draft2TPS            float64
	trialTokens          int
	trialElapsed         time.Duration
	trialCompletedRounds int
}

// RoundResult reports MTP-owned diagnostics from a completed round.
type RoundResult struct {
	Made        bool
	Draft       int
	Reason      string
	BaselineTPS float64
	Draft2TPS   float64
	Draft1TPS   float64
	Round       int
	Report      bool
}

func (p *Policy) draftCountAt(state *SlotState, maxDraft, configuredDraft int, now time.Time) int {
	p.configure(configuredDraft)

	if p.phase == policyObserving &&
		maxDraft >= p.configuredDraft &&
		state.Rounds > 0 &&
		state.Rounds%adaptationWindow == 0 {
		p.evaluateAcceptance(state)
	}

	count := min(maxDraft, p.activeDraft)
	state.RoundDraft = count
	state.RoundStarted = now
	return count
}

func (p *Policy) configure(configuredDraft int) {
	if configuredDraft <= 0 {
		return
	}
	if p.configuredDraft == configuredDraft {
		return
	}
	*p = Policy{configuredDraft: configuredDraft, activeDraft: configuredDraft}
	if configuredDraft <= 2 {
		p.phase = policyLocked
	}
}

func (p *Policy) evaluateAcceptance(state *SlotState) {
	if state.AcceptanceEMA < lowAcceptanceEMA {
		state.LowWindows++
		if state.LowWindows == 1 {
			state.PriorTokens = state.WindowTokens
			state.PriorElapsed = state.WindowElapsed
		}
	} else {
		state.LowWindows = 0
		state.PriorTokens = 0
		state.PriorElapsed = 0
	}

	if state.LowWindows >= lowAcceptanceWindows {
		p.baselineTokens = state.PriorTokens + state.WindowTokens
		p.baselineElapsed = state.PriorElapsed + state.WindowElapsed
		p.beginTrial(policyTrialTwo, 2)
		state.LowWindows = 0
		state.PriorTokens = 0
		state.PriorElapsed = 0
	} else if state.Rounds >= observationRounds && state.LowWindows == 0 {
		p.phase = policyLocked
		p.pendingDecision = true
	}

	state.WindowTokens = 0
	state.WindowElapsed = 0
}

// CompleteRound records one verified round and reports MTP-owned diagnostics.
func (p *Policy) CompleteRound(state *SlotState, emittedTokens int, completedAt time.Time) RoundResult {
	state.Rounds++
	result := RoundResult{
		Round:  state.Rounds,
		Report: state.Rounds == 1 || state.Rounds%adaptationWindow == 0,
	}
	elapsed := completedAt.Sub(state.RoundStarted)
	if emittedTokens <= 0 || elapsed <= 0 {
		return result
	}
	if p.pendingDecision {
		p.pendingDecision = false
		result.Made = true
		result.Draft = p.activeDraft
		result.Reason = "healthy-acceptance"
		return result
	}

	switch {
	case p.phase == policyObserving && state.RoundDraft == p.configuredDraft:
		state.WindowTokens += emittedTokens
		state.WindowElapsed += elapsed

	case p.phase == policyTrialTwo && state.RoundDraft == 2:
		if !p.completeTrialRound(emittedTokens, elapsed) {
			return result
		}
		p.draft2TPS = tokensPerSecond(p.trialTokens, p.trialElapsed)
		p.beginTrial(policyTrialOne, 1)

	case p.phase == policyTrialOne && state.RoundDraft == 1:
		if !p.completeTrialRound(emittedTokens, elapsed) {
			return result
		}

		baselineTPS := tokensPerSecond(p.baselineTokens, p.baselineElapsed)
		draft1TPS := tokensPerSecond(p.trialTokens, p.trialElapsed)
		p.activeDraft = p.configuredDraft
		bestTPS := baselineTPS
		if p.draft2TPS > bestTPS {
			p.activeDraft = 2
			bestTPS = p.draft2TPS
		}
		if draft1TPS > bestTPS {
			p.activeDraft = 1
		}
		p.phase = policyLocked
		result.Made = true
		result.Draft = p.activeDraft
		result.Reason = "throughput-trial"
		result.BaselineTPS = baselineTPS
		result.Draft2TPS = p.draft2TPS
		result.Draft1TPS = draft1TPS
		return result
	}

	return result
}

func (p *Policy) beginTrial(phase policyPhase, draft int) {
	p.phase = phase
	p.activeDraft = draft
	p.trialTokens = 0
	p.trialElapsed = 0
	p.trialCompletedRounds = 0
}

func (p *Policy) completeTrialRound(emittedTokens int, elapsed time.Duration) bool {
	p.trialTokens += emittedTokens
	p.trialElapsed += elapsed
	p.trialCompletedRounds++
	return p.trialCompletedRounds >= trialRounds
}

func tokensPerSecond(tokens int, elapsed time.Duration) float64 {
	if tokens <= 0 || elapsed <= 0 {
		return 0
	}
	return float64(tokens) / elapsed.Seconds()
}
