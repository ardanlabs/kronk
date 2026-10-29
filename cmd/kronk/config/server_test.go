package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	kronkserver "github.com/ardanlabs/kronk/cmd/server/api/services/kronk"
)

func TestEditKMSDefaultFile(t *testing.T) {
	in := "version: 2\n\n# SERVER OPTIONS\n# kms:\n#   web: ...\nkms: {}\n\nmodels:\n  a/b:\n    nseq-max: 2\n"

	out, err := editKMS([]byte(in), []change{
		{key: "web.api-host", new: "0.0.0.0:11435"},
		{key: "pool.budget-percent", new: 80},
	})
	if err != nil {
		t.Fatal(err)
	}

	want := "version: 2\n\n# SERVER OPTIONS\n# kms:\n#   web: ...\nkms:\n  web:\n    api-host: 0.0.0.0:11435\n  pool:\n    budget-percent: 80\n\nmodels:\n  a/b:\n    nseq-max: 2\n"
	if string(out) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestEditKMSChangeAndRemove(t *testing.T) {
	in := "version: 2\nkms:\n  web:\n    api-host: 0.0.0.0:11435\n    read-timeout: 30s\nmodels: {}\n"

	out, err := editKMS([]byte(in), []change{
		{key: "web.read-timeout", del: true},
		{key: "web.api-host", new: "127.0.0.1:9000"},
	})
	if err != nil {
		t.Fatal(err)
	}

	want := "version: 2\nkms:\n  web:\n    api-host: 127.0.0.1:9000\nmodels: {}\n"
	if string(out) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}

	out, err = editKMS(out, []change{{key: "web.api-host", del: true}})
	if err != nil {
		t.Fatal(err)
	}
	if want := "version: 2\nkms: {}\nmodels: {}\n"; string(out) != want {
		t.Fatalf("removing everything:\n%s", out)
	}
}

func TestEditKMSMissingSection(t *testing.T) {
	in := "version: 2\n\nmodels:\n  a/b:\n    nseq-max: 2\n"

	out, err := editKMS([]byte(in), []change{{key: "download.enabled", new: true}})
	if err != nil {
		t.Fatal(err)
	}

	want := "version: 2\n\nkms:\n  download:\n    enabled: true\n\nmodels:\n  a/b:\n    nseq-max: 2\n"
	if string(out) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestSaveKMSRejectsInvalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model_config.yaml")
	before := "version: 2\nkms: {}\nmodels: {}\n"
	if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}

	// Not longer than the default 60m inference timeout.
	_, err := saveKMS(path, []change{{key: "web.write-timeout", new: "1m"}})
	if err == nil || !strings.Contains(err.Error(), "nothing was written") {
		t.Fatalf("err = %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != before {
		t.Fatalf("file changed:\n%s", data)
	}
}

func TestServerQuestions(t *testing.T) {
	settings := kronkserver.Settings()
	existing := map[string]any{
		"web.cors-allowed-origins": []any{"https://a", "https://b"},
		"hf-token":                 "secret",
	}

	var picked []string
	qs := serverQuestions(settings, &picked, existing)
	if len(qs) != len(settings)-len(skipServer) {
		t.Fatalf("%d questions for %d settings, %d skipped", len(qs), len(settings), len(skipServer))
	}
	for _, q := range qs {
		if skipServer[q.key] {
			t.Fatalf("%s must not be asked", q.key)
		}
	}

	a := newAnswers(qs)
	a.prefill(facts{}, existing)

	if got := a.value("web.cors-allowed-origins"); got != "https://a,https://b" {
		t.Fatalf("list prefill = %q", got)
	}
	if got := a.value("hf-token"); got != "" {
		t.Fatalf("a secret must not be prefilled, got %q", got)
	}

	if a.applies(facts{}, a.byKey["web.api-host"]) {
		t.Fatal("a section that is not picked must not be asked")
	}
	picked = []string{"web", "runtime"}
	if !a.applies(facts{}, a.byKey["web.api-host"]) {
		t.Fatal("a picked section must be asked")
	}

	// Unchanged list and empty secret: nothing to write.
	changes, err := diff(qs, facts{}, a, existing)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("unchanged answers produced changes: %+v", changes)
	}

	*a.sel[keyAdminPassword] = "hunter2"
	changes, err = diff(qs, facts{}, a, existing)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].new != hashPassword("hunter2") || len(hashPassword("x")) != 64 {
		t.Fatalf("password change = %+v", changes)
	}
}

func TestPreviewMatchesSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model_config.yaml")
	in := "version: 2\n\n# server\nkms: {}\n\nmodels:\n  a/b:\n    # keep me\n    nseq-max: 2\n"
	if err := os.WriteFile(path, []byte(in), 0o644); err != nil {
		t.Fatal(err)
	}

	entryChanges := []change{{key: "context-window", new: 8192}}
	kmsChanges := []change{{key: "web.api-host", new: "0.0.0.0:11435"}, {key: "pool.budget-percent", new: 80}}

	entry, err := previewEntry(path, "a/b", entryChanges)
	if err != nil {
		t.Fatal(err)
	}
	kms, err := previewKMS(path, kmsChanges)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := saveEntry(path, "a/b", entryChanges); err != nil {
		t.Fatal(err)
	}
	if _, err := saveKMS(path, kmsChanges); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)

	for name, want := range map[string]string{"entry": entry, "kms": kms} {
		if want == "" || !strings.Contains(string(data), want) {
			t.Errorf("%s preview not in the saved file:\n%s\nfile:\n%s", name, want, data)
		}
	}
	if !strings.Contains(entry, "# keep me") {
		t.Errorf("entry preview lost the comment:\n%s", entry)
	}
}
