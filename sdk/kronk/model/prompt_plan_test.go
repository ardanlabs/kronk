package model

import (
	"crypto/sha256"
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestPromptPlanPrefix(t *testing.T) {
	imageA := sha256.Sum256([]byte("image-a"))
	imageB := sha256.Sum256([]byte("image-b"))
	text := func(token llama.Token) promptUnit { return promptUnit{token: token} }
	media := func(digest [sha256.Size]byte) promptUnit { return promptUnit{media: digest, isMedia: true} }

	base := promptPlan{units: []promptUnit{text(1), media(imageA), text(2)}}
	tests := []struct {
		name string
		plan promptPlan
		want bool
	}{
		{name: "exact", plan: base, want: true},
		{name: "text append", plan: promptPlan{units: []promptUnit{text(1), media(imageA), text(2), text(3)}}, want: true},
		{name: "changed media", plan: promptPlan{units: []promptUnit{text(1), media(imageB), text(2)}}, want: false},
		{name: "reordered media", plan: promptPlan{units: []promptUnit{text(1), media(imageB), media(imageA), text(2)}}, want: false},
		{name: "text divergence", plan: promptPlan{units: []promptUnit{text(1), media(imageA), text(9)}}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.plan.hasPrefix(base); got != tt.want {
				t.Errorf("hasPrefix() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPromptPlanTextTail(t *testing.T) {
	digest := sha256.Sum256([]byte("image"))
	base := promptPlan{units: []promptUnit{{token: 1}, {media: digest, isMedia: true}}}

	textPlan := promptPlan{units: append(append([]promptUnit{}, base.units...), promptUnit{token: 2}, promptUnit{token: 3})}
	if got, ok := textPlan.textTail(base); !ok || len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Errorf("textTail() = %v, %v, want [2 3], true", got, ok)
	}

	mediaPlan := promptPlan{units: append(append([]promptUnit{}, base.units...), promptUnit{media: digest, isMedia: true})}
	if _, ok := mediaPlan.textTail(base); ok {
		t.Fatal("textTail() accepted a media tail")
	}
}

func TestPromptPlanEqual(t *testing.T) {
	first := promptPlan{units: []promptUnit{{token: 1}, {token: 2}}}
	second := promptPlan{units: []promptUnit{{token: 1}, {token: 2}}}
	diverged := promptPlan{units: []promptUnit{{token: 1}, {token: 3}}}

	if !first.equal(second) {
		t.Error("equal() = false for identical plans")
	}
	if first.equal(diverged) {
		t.Error("equal() = true for divergent plans")
	}
}
