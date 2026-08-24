package mtp

import (
	"fmt"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestPolicyLocksDraftTwoWhenItIsFastest(t *testing.T) {
	policy, state := startPolicyTrials(t)

	decision := runPolicyTrial(policy, state, 2, 2, 5*time.Millisecond)
	if decision.Made || policy.phase != policyTrialOne {
		t.Fatalf("draft 2 trial decided before draft 1 trial: decision=%+v policy=%+v", decision, policy)
	}
	if got := policy.draftCountAt(state, 3, 3, time.Now()); got != 1 {
		t.Fatalf("draft count after draft 2 trial = %d, want draft 1 trial", got)
	}
	decision = runPolicyTrial(policy, state, 1, 2, 10*time.Millisecond)
	if !decision.Made || decision.Draft != 2 {
		t.Fatalf("decision = %+v, want locked draft 2", decision)
	}
	if decision.Reason != "throughput-trial" {
		t.Fatalf("decision reason = %q, want throughput-trial", decision.Reason)
	}
	if math.Abs(decision.BaselineTPS-200) > 1e-9 || math.Abs(decision.Draft2TPS-400) > 1e-9 || math.Abs(decision.Draft1TPS-200) > 1e-9 {
		t.Fatalf("throughput decision = %+v, want baseline 200, draft 2 at 400, and draft 1 at 200", decision)
	}

	state.Reset()
	if got := policy.draftCountAt(state, 3, 3, time.Now()); got != 2 {
		t.Fatalf("draft count for next request = %d, want learned draft 2", got)
	}
}

func TestPolicyLocksConfiguredDraftWhenTrialsAreSlower(t *testing.T) {
	policy, state := startPolicyTrials(t)

	decision := runPolicyTrial(policy, state, 2, 2, 20*time.Millisecond)
	if decision.Made {
		t.Fatalf("draft 2 trial decided before draft 1 trial: %+v", decision)
	}
	decision = runPolicyTrial(policy, state, 1, 2, 20*time.Millisecond)
	if !decision.Made || decision.Draft != 3 {
		t.Fatalf("decision = %+v, want configured draft 3", decision)
	}

	state.Reset()
	if got := policy.draftCountAt(state, 3, 3, time.Now()); got != 3 {
		t.Fatalf("draft count for next request = %d, want configured draft 3", got)
	}
}

func TestPolicyLocksDraftOneWhenItIsFastest(t *testing.T) {
	policy, state := startPolicyTrials(t)

	decision := runPolicyTrial(policy, state, 2, 2, 5*time.Millisecond)
	if decision.Made {
		t.Fatalf("draft 2 trial decided before draft 1 trial: %+v", decision)
	}
	decision = runPolicyTrial(policy, state, 1, 2, 2500*time.Microsecond)
	if !decision.Made || decision.Draft != 1 {
		t.Fatalf("decision = %+v, want locked draft 1", decision)
	}
	if math.Abs(decision.BaselineTPS-200) > 1e-9 || math.Abs(decision.Draft2TPS-400) > 1e-9 || math.Abs(decision.Draft1TPS-800) > 1e-9 {
		t.Fatalf("throughput decision = %+v, want baseline 200, draft 2 at 400, and draft 1 at 800", decision)
	}

	state.Reset()
	if got := policy.draftCountAt(state, 3, 3, time.Now()); got != 1 {
		t.Fatalf("draft count for next request = %d, want learned draft 1", got)
	}
}

func TestPolicyDoesNotTrialAfterTransientDip(t *testing.T) {
	var policy Policy
	state := SlotState{
		AcceptanceEMA: 0.50,
		Rounds:        8,
		WindowTokens:  16,
		WindowElapsed: 80 * time.Millisecond,
	}
	if got := policy.draftCountAt(&state, 3, 3, time.Now()); got != 3 {
		t.Fatalf("draft count after low window = %d, want 3", got)
	}

	state.Rounds = 16
	state.AcceptanceEMA = 0.80
	state.WindowTokens = 16
	state.WindowElapsed = 80 * time.Millisecond
	if got := policy.draftCountAt(&state, 3, 3, time.Now()); got != 3 {
		t.Fatalf("draft count after recovery = %d, want 3", got)
	}
	if state.LowWindows != 0 || policy.phase != policyObserving {
		t.Fatalf("policy trialed after transient dip: state=%+v policy=%+v", state, policy)
	}
}

func TestPolicyLocksHealthyConfiguredDraftAcrossRequests(t *testing.T) {
	var policy Policy
	state := SlotState{
		AcceptanceEMA: 0.90,
		Rounds:        observationRounds,
		WindowTokens:  24,
		WindowElapsed: 80 * time.Millisecond,
	}
	if got := policy.draftCountAt(&state, 3, 3, time.Now()); got != 3 {
		t.Fatalf("draft count after observation = %d, want 3", got)
	}
	if policy.phase != policyLocked {
		t.Fatalf("policy phase = %d, want locked", policy.phase)
	}
	decision := completePolicyRound(&policy, &state, 3, time.Millisecond)
	if !decision.Made || decision.Draft != 3 || decision.Reason != "healthy-acceptance" {
		t.Fatalf("decision = %+v, want healthy lock at draft 3", decision)
	}

	state.Reset()
	state.AcceptanceEMA = 0.10
	state.Rounds = observationRounds
	if got := policy.draftCountAt(&state, 3, 3, time.Now()); got != 3 {
		t.Fatalf("draft count for next request = %d, want locked draft 3", got)
	}
}

func TestPolicyGivesLowBoundaryWindowOneMoreCheck(t *testing.T) {
	var policy Policy
	state := SlotState{
		AcceptanceEMA: 0.50,
		Rounds:        observationRounds,
		WindowTokens:  16,
		WindowElapsed: 80 * time.Millisecond,
	}
	if got := policy.draftCountAt(&state, 3, 3, time.Now()); got != 3 {
		t.Fatalf("draft count at low boundary = %d, want 3", got)
	}
	if policy.phase != policyObserving || state.LowWindows != 1 {
		t.Fatalf("policy locked before confirming low acceptance: state=%+v policy=%+v", state, policy)
	}

	state.AcceptanceEMA = 0.80
	state.Rounds += adaptationWindow
	state.WindowTokens = 16
	state.WindowElapsed = 80 * time.Millisecond
	if got := policy.draftCountAt(&state, 3, 3, time.Now()); got != 3 {
		t.Fatalf("draft count after recovery = %d, want 3", got)
	}
	if policy.phase != policyLocked {
		t.Fatalf("policy phase = %d, want locked", policy.phase)
	}
}

func TestPolicyHonorsExplicitOneAndTwo(t *testing.T) {
	for _, configured := range []int{1, 2} {
		t.Run(fmt.Sprintf("nDraft=%d", configured), func(t *testing.T) {
			var policy Policy
			var state SlotState
			if got := policy.draftCountAt(&state, configured, configured, time.Now()); got != configured {
				t.Fatalf("draft count = %d, want configured count %d", got, configured)
			}
			if policy.phase != policyLocked {
				t.Fatalf("policy phase = %d, want locked", policy.phase)
			}
		})
	}
}

func startPolicyTrials(t *testing.T) (*Policy, *SlotState) {
	t.Helper()

	var policy Policy
	state := &SlotState{
		AcceptanceEMA: 0.40,
		Rounds:        8,
		WindowTokens:  16,
		WindowElapsed: 80 * time.Millisecond,
	}
	if got := policy.draftCountAt(state, 3, 3, time.Now()); got != 3 {
		t.Fatalf("draft count after first low window = %d, want 3", got)
	}

	state.Rounds = 16
	state.AcceptanceEMA = 0.34
	state.WindowTokens = 16
	state.WindowElapsed = 80 * time.Millisecond
	if got := policy.draftCountAt(state, 3, 3, time.Now()); got != 2 {
		t.Fatalf("draft count after second low window = %d, want draft 2 trial", got)
	}
	if policy.phase != policyTrialTwo {
		t.Fatalf("policy phase = %d, want draft 2 trial", policy.phase)
	}

	return &policy, state
}

func completePolicyRound(policy *Policy, state *SlotState, tokens int, elapsed time.Duration) RoundResult {
	state.RoundStarted = time.Unix(0, 0)
	return policy.CompleteRound(state, tokens, state.RoundStarted.Add(elapsed))
}

func runPolicyTrial(policy *Policy, state *SlotState, draft, tokens int, elapsed time.Duration) RoundResult {
	var result RoundResult
	for range trialRounds {
		state.RoundDraft = draft
		result = completePolicyRound(policy, state, tokens, elapsed)
	}
	return result
}

func TestGenerate(t *testing.T) {
	tests := []struct {
		name          string
		fixedPosition bool
		wantPositions []llama.Pos
		wantPosition  llama.Pos
	}{
		{"own KV advances", false, []llama.Pos{7, 8, 9}, 10},
		{"shared KV stays fixed", true, []llama.Pos{7, 7, 7}, 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var positions []llama.Pos
			result, err := Generate(DraftInput{
				Token:         10,
				Position:      7,
				Hidden:        []float32{1},
				Count:         3,
				FixedPosition: tt.fixedPosition,
				IsEOG:         func(llama.Token) bool { return false },
				DecodeStep: func(token llama.Token, position llama.Pos, hidden []float32) (llama.Token, []float32, bool, error) {
					positions = append(positions, position)
					return token + 1, []float32{hidden[0] + 1}, true, nil
				},
			})
			if err != nil {
				t.Fatalf("Generate() error = %v, want nil", err)
			}
			if !slices.Equal(positions, tt.wantPositions) {
				t.Errorf("positions = %v, want %v", positions, tt.wantPositions)
			}
			if result.Position != tt.wantPosition {
				t.Errorf("Position = %d, want %d", result.Position, tt.wantPosition)
			}
			if want := []llama.Token{11, 12, 13}; !slices.Equal(result.Candidates, want) {
				t.Errorf("Candidates = %v, want %v", result.Candidates, want)
			}
		})
	}
}

func TestGenerateStopsAtEOG(t *testing.T) {
	hidden := []float32{1}
	result, err := Generate(DraftInput{
		Token:    10,
		Position: 2,
		Hidden:   hidden,
		Count:    3,
		IsEOG:    func(token llama.Token) bool { return token == 11 },
		DecodeStep: func(token llama.Token, _ llama.Pos, _ []float32) (llama.Token, []float32, bool, error) {
			return token + 1, []float32{2}, true, nil
		},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v, want nil", err)
	}
	if want := []llama.Token{11}; !slices.Equal(result.Candidates, want) {
		t.Errorf("Candidates = %v, want %v", result.Candidates, want)
	}
	result.Hidden[0] = 9
	if hidden[0] != 1 {
		t.Errorf("input hidden state = %v, want [1]", hidden)
	}
}
