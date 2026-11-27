package speculation

import (
	"slices"
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestVerifyMatches(t *testing.T) {
	for _, tt := range []struct {
		name     string
		target   []llama.Token
		stopAt   int
		accepted int
		bonus    llama.Token
	}{
		{"first rejection", []llama.Token{71}, 0, 0, 71},
		{"middle rejection", []llama.Token{11, 72}, 0, 1, 72},
		{"last rejection", []llama.Token{11, 23, 73}, 0, 2, 73},
		{"full acceptance and bonus", []llama.Token{11, 23, 47, 79}, 0, 3, 79},
		{"output finishes inside accepted prefix", []llama.Token{11, 23}, 2, 2, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var emitted []llama.Token
			result := VerifyMatches(MatchInput{
				Candidates: []llama.Token{11, 23, 47},
				Sample:     func(index int) llama.Token { return tt.target[index] },
				Accept: func(_ int, token llama.Token) bool {
					emitted = append(emitted, token)
					return len(emitted) != tt.stopAt
				},
			})
			if result.Accepted != tt.accepted || result.Drafted != 3 || result.Bonus != tt.bonus || result.Complete != (tt.stopAt == 0) {
				t.Errorf("result: got %+v, want accepted=%d bonus=%d complete=%t", result, tt.accepted, tt.bonus, tt.stopAt == 0)
			}
			if !slices.Equal(emitted, []llama.Token{11, 23, 47}[:tt.accepted]) {
				t.Errorf("emitted: got %v, want accepted prefix only", emitted)
			}
		})
	}
}
