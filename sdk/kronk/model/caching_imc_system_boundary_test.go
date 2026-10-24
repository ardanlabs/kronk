package model

import (
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// Token stand-ins: 100 = turn start, 101 = turn end (both control), 1-9 system
// text, 20 "user", 21 "\n", 22 "\nX" (a newline merged with the next word),
// 30+ content.
func TestIMCSystemBoundary(t *testing.T) {
	isControl := func(tok llama.Token) bool { return tok >= 100 }
	system := []llama.Token{100, 1, 2, 3, 101, 21, 100, 20}
	// The cut lands just after the user turn's start marker (the 100 at index 6),
	// the same place whatever the content starts with.
	const cut = 7

	cases := []struct {
		name   string
		stable []llama.Token
		probe  []llama.Token
		want   int
	}{
		{"newline kept separate in both", append(append([]llama.Token{}, system...), 21, 30, 31, 101), append(append([]llama.Token{}, system...), 21, 40, 101), cut},
		{"newline merged in the real prompt only", append(append([]llama.Token{}, system...), 22, 31, 101), append(append([]llama.Token{}, system...), 21, 40, 101), cut},
		{"content starting like the placeholder", append(append([]llama.Token{}, system...), 21, 40, 41, 101), append(append([]llama.Token{}, system...), 21, 40, 101), cut},
		{"no shared control token", []llama.Token{1, 2, 3}, []llama.Token{1, 2, 4}, 0},
		{"probe covers the whole prompt", []llama.Token{100, 1, 100}, []llama.Token{100, 1, 100, 5}, 0},
	}
	for _, c := range cases {
		if got := imcSystemBoundary(c.stable, c.probe, isControl); got != c.want {
			t.Errorf("%s: boundary = %d, want %d", c.name, got, c.want)
		}
	}
}
