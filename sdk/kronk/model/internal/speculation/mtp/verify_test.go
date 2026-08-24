package mtp

import (
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestVerify(t *testing.T) {
	tests := []struct {
		name         string
		target       []llama.Token
		wantAccepted int
		wantBonus    llama.Token
	}{
		{"partial acceptance", []llama.Token{1, 9}, 1, 9},
		{"full acceptance", []llama.Token{1, 2, 7}, 2, 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := SlotState{AcceptanceEMA: 1}
			result := Verify(VerifyInput{
				State:      &state,
				Candidates: []llama.Token{1, 2},
				Sample:     func(index int) llama.Token { return tt.target[index] },
				Accept:     func(int, llama.Token) bool { return true },
			})
			if result.Accepted != tt.wantAccepted {
				t.Errorf("Accepted = %d, want %d", result.Accepted, tt.wantAccepted)
			}
			if result.Bonus != tt.wantBonus {
				t.Errorf("Bonus = %d, want %d", result.Bonus, tt.wantBonus)
			}
			if !result.Complete {
				t.Error("Complete = false, want true")
			}
			wantEMA := 0.9 + 0.1*float64(tt.wantAccepted)/2
			if state.AcceptanceEMA != wantEMA {
				t.Errorf("AcceptanceEMA = %f, want %f", state.AcceptanceEMA, wantEMA)
			}
		})
	}
}
