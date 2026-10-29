package source

import (
	"testing"

	"github.com/ardanlabs/kronk/sdk/tools/modelconfig"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func TestEntriesFor(t *testing.T) {
	doc := modelconfig.Document{Models: map[string]models.ModelConfig{
		"a/b":       {},
		"a/b/LONG":  {},
		"a/b/AGENT": {},
		"a/bc":      {},
		"a/b/x/y":   {},
	}}
	got := EntriesFor(doc, "a/b")
	if len(got) != 3 || got[0] != "a/b" || got[1] != "a/b/AGENT" || got[2] != "a/b/LONG" {
		t.Fatalf("entries = %v", got)
	}
}
