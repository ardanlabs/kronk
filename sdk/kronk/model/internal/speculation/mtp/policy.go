package mtp

import "time"

const (
	acceptanceWindowRounds = 8
	lowAcceptanceWindows   = 3
	lowAcceptanceEMA       = 0.55
	observationRounds      = 32
	trialBlockRounds       = 8
	trialRoundsPerDraft    = 32
	trialCycles            = trialRoundsPerDraft / trialBlockRounds
	minimumThroughputGain  = 0.05
)

type policyPhase uint8

const (
	policyObserving policyPhase = iota
	policyTrialConfigured
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
	draft2Tokens         int
	draft2Elapsed        time.Duration
	draft1Tokens         int
	draft1Elapsed        time.Duration
	trialBlockCompleted  int
	trialCyclesCompleted int
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

// PolicySnapshot reports the current model-level MTP draft policy.
type PolicySnapshot struct {
	ActiveDraft int
	State       string
}

// Snapshot returns the active draft count and lifecycle state. The configured
// count is used before the first request initializes the policy.
func (p *Policy) Snapshot(configuredDraft int) PolicySnapshot {
	activeDraft := p.activeDraft
	phase := p.phase
	if p.configuredDraft == 0 {
		activeDraft = configuredDraft
		if configuredDraft <= 2 {
			phase = policyLocked
		}
	}

	state := "observing"
	switch phase {
	case policyTrialConfigured, policyTrialTwo, policyTrialOne:
		state = "calibrating"
	case policyLocked:
		state = "locked"
	}

	return PolicySnapshot{
		ActiveDraft: activeDraft,
		State:       state,
	}
}

func (p *Policy) draftCountAt(state *SlotState, maxDraft, configuredDraft int, now time.Time) int {
	p.configure(configuredDraft)

	if p.phase == policyObserving &&
		maxDraft >= p.configuredDraft &&
		state.Rounds > 0 &&
		state.Rounds%acceptanceWindowRounds == 0 {
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
	} else {
		state.LowWindows = 0
	}

	if state.LowWindows >= lowAcceptanceWindows {
		p.beginTrials()
		state.LowWindows = 0
	} else if state.Rounds >= observationRounds && state.LowWindows == 0 {
		p.phase = policyLocked
		p.pendingDecision = true
	}
}

// CompleteRound records one verified round and reports MTP-owned diagnostics.
func (p *Policy) CompleteRound(state *SlotState, emittedTokens int, completedAt time.Time) RoundResult {
	state.Rounds++
	result := RoundResult{
		Round:  state.Rounds,
		Report: state.Rounds == 1 || state.Rounds%acceptanceWindowRounds == 0,
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
	case p.phase == policyTrialConfigured && state.RoundDraft == p.configuredDraft,
		p.phase == policyTrialTwo && state.RoundDraft == 2,
		p.phase == policyTrialOne && state.RoundDraft == 1:
		if !p.recordTrialRound(emittedTokens, elapsed) {
			return result
		}

		baselineTPS := tokensPerSecond(p.baselineTokens, p.baselineElapsed)
		draft2TPS := tokensPerSecond(p.draft2Tokens, p.draft2Elapsed)
		draft1TPS := tokensPerSecond(p.draft1Tokens, p.draft1Elapsed)
		p.activeDraft = p.configuredDraft
		bestLowerDraft := 2
		bestLowerTPS := draft2TPS
		if draft1TPS > bestLowerTPS {
			bestLowerDraft = 1
			bestLowerTPS = draft1TPS
		}
		if bestLowerTPS > baselineTPS*(1+minimumThroughputGain) {
			p.activeDraft = bestLowerDraft
		}
		p.phase = policyLocked
		result.Made = true
		result.Draft = p.activeDraft
		result.Reason = "throughput-trial"
		result.BaselineTPS = baselineTPS
		result.Draft2TPS = draft2TPS
		result.Draft1TPS = draft1TPS
		return result
	}

	return result
}

func (p *Policy) beginTrials() {
	p.phase = policyTrialConfigured
	p.activeDraft = p.configuredDraft
	p.baselineTokens = 0
	p.baselineElapsed = 0
	p.draft2Tokens = 0
	p.draft2Elapsed = 0
	p.draft1Tokens = 0
	p.draft1Elapsed = 0
	p.trialBlockCompleted = 0
	p.trialCyclesCompleted = 0
}

func (p *Policy) recordTrialRound(emittedTokens int, elapsed time.Duration) bool {
	switch p.phase {
	case policyTrialConfigured:
		p.baselineTokens += emittedTokens
		p.baselineElapsed += elapsed
	case policyTrialTwo:
		p.draft2Tokens += emittedTokens
		p.draft2Elapsed += elapsed
	case policyTrialOne:
		p.draft1Tokens += emittedTokens
		p.draft1Elapsed += elapsed
	}

	p.trialBlockCompleted++
	if p.trialBlockCompleted < trialBlockRounds {
		return false
	}
	p.trialBlockCompleted = 0

	switch p.phase {
	case policyTrialConfigured:
		p.phase = policyTrialTwo
		p.activeDraft = 2
	case policyTrialTwo:
		p.phase = policyTrialOne
		p.activeDraft = 1
	case policyTrialOne:
		p.trialCyclesCompleted++
		if p.trialCyclesCompleted >= trialCycles {
			return true
		}
		p.phase = policyTrialConfigured
		p.activeDraft = p.configuredDraft
	}

	return false
}

func tokensPerSecond(tokens int, elapsed time.Duration) float64 {
	if tokens <= 0 || elapsed <= 0 {
		return 0
	}
	return float64(tokens) / elapsed.Seconds()
}
