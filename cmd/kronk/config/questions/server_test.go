package questions

import (
	"testing"

	"github.com/ardanlabs/kronk/cmd/kronk/config/source"
	kronkserver "github.com/ardanlabs/kronk/cmd/server/api/services/kronk"
)

func TestServerQuestions(t *testing.T) {
	settings := kronkserver.Settings()
	existing := map[string]any{
		"web.cors-allowed-origins": []any{"https://a", "https://b"},
		"hf-token":                 "secret",
	}

	var picked []string
	qs := Server(settings, &picked, existing)
	if len(qs) != len(settings)-len(skipServer) {
		t.Fatalf("%d questions for %d settings, %d skipped", len(qs), len(settings), len(skipServer))
	}
	for _, q := range qs {
		if skipServer[q.Key] {
			t.Fatalf("%s must not be asked", q.Key)
		}
	}

	a := NewAnswers(qs)
	a.Prefill(source.Facts{}, existing)

	if got := a.Value("web.cors-allowed-origins"); got != "https://a,https://b" {
		t.Fatalf("list prefill = %q", got)
	}
	if got := a.Value("hf-token"); got != "" {
		t.Fatalf("a secret must not be prefilled, got %q", got)
	}

	if a.Applies(source.Facts{}, a.ByKey["web.api-host"]) {
		t.Fatal("a section that is not picked must not be asked")
	}
	picked = []string{"web", "runtime"}
	if !a.Applies(source.Facts{}, a.ByKey["web.api-host"]) {
		t.Fatal("a picked section must be asked")
	}

	// Unchanged list and empty secret: nothing to write.
	changes, err := Diff(qs, source.Facts{}, a, existing)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("unchanged answers produced changes: %+v", changes)
	}

	*a.Sel[keyAdminPassword] = "hunter2"
	changes, err = Diff(qs, source.Facts{}, a, existing)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].New != hashPassword("hunter2") || len(hashPassword("x")) != 64 {
		t.Fatalf("password change = %+v", changes)
	}
}
