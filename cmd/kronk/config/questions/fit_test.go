package questions

import (
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/kronk/config/configfile"
)

func TestApplyFix(t *testing.T) {
	f := sampleFacts()
	qs := Chat()

	for _, tt := range []struct {
		name string
		fix  Fix
		want map[string]any
	}{
		{"context option", Fix{kind: fixSmallerContext, context: 32768}, map[string]any{"context-window": 32768}},
		{"context other value", Fix{kind: fixSmallerContext, context: 16384}, map[string]any{"context-window": 16384}},
		{"q8_0 cache", Fix{kind: fixQ8Cache}, map[string]any{"cache-type-k": "q8_0", "cache-type-v": "q8_0"}},
		{"fewer requests", Fix{kind: fixFewerRequests, requests: 2}, map[string]any{"nseq-max": 2}},
		{"both", Fix{kind: fixQ8AndContext, context: 8192}, map[string]any{"context-window": 8192, "cache-type-k": "q8_0", "cache-type-v": "q8_0"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a := NewAnswers(qs)
			*a.Sel["nseq-max"] = "4"
			*a.Sel["cache-type-v"] = "f16" // a different value precision is replaced
			a.ApplyFix(f, tt.fix)

			got := changesFor(t, qs, f, a, nil)
			for key, v := range tt.want {
				if got[key].New != v {
					t.Errorf("%s = %v, want %v", key, got[key].New, v)
				}
			}

			var changes []configfile.Change
			for _, c := range got {
				changes = append(changes, c)
			}
			if _, err := ToModelConfig(Apply(nil, changes)); err != nil {
				t.Fatalf("the fixed answers must be a valid model config: %v", err)
			}
		})
	}
}

func TestFixLabels(t *testing.T) {
	for _, x := range []Fix{
		{kind: fixSmallerContext, context: 32768, mem: 4 << 30},
		{kind: fixQ8Cache, mem: 4 << 30},
		{kind: fixFewerRequests, requests: 2, mem: 4 << 30},
		{kind: fixQ8AndContext, context: 8192, mem: 4 << 30},
	} {
		if l := x.Label(); !strings.Contains(l, "fits, 4.0 GB") {
			t.Errorf("%s label = %q", x.kind, l)
		}
	}
}
