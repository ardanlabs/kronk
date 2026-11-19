package configfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditKMSDefaultFile(t *testing.T) {
	in := "version: 2\n\n# SERVER OPTIONS\n# kms:\n#   web: ...\nkms: {}\n\nmodels:\n  a/b:\n    nseq-max: 2\n"

	out, err := editKMS([]byte(in), []Change{
		{Key: "web.api-host", New: "0.0.0.0:11435"},
		{Key: "pool.budget-percent", New: 80},
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

	out, err := editKMS([]byte(in), []Change{
		{Key: "web.read-timeout", Del: true},
		{Key: "web.api-host", New: "127.0.0.1:9000"},
	})
	if err != nil {
		t.Fatal(err)
	}

	want := "version: 2\nkms:\n  web:\n    api-host: 127.0.0.1:9000\nmodels: {}\n"
	if string(out) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}

	out, err = editKMS(out, []Change{{Key: "web.api-host", Del: true}})
	if err != nil {
		t.Fatal(err)
	}
	if want := "version: 2\nkms: {}\nmodels: {}\n"; string(out) != want {
		t.Fatalf("removing everything:\n%s", out)
	}
}

func TestEditKMSMissingSection(t *testing.T) {
	in := "version: 2\n\nmodels:\n  a/b:\n    nseq-max: 2\n"

	out, err := editKMS([]byte(in), []Change{{Key: "download.enabled", New: true}})
	if err != nil {
		t.Fatal(err)
	}

	want := "version: 2\n\nkms:\n  download:\n    enabled: true\n\nmodels:\n  a/b:\n    nseq-max: 2\n"
	if string(out) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestSaveKMSValuesReplacesTheSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model_config.yaml")
	in := "version: 2\n\nkms:\n  download:\n    enabled: true\n  pool:\n    budget-percent: 80\n\nmodels: {}\n"
	if err := os.WriteFile(path, []byte(in), 0o644); err != nil {
		t.Fatal(err)
	}

	values := map[string]any{"web.api-host": "127.0.0.1:11435"}

	preview, err := PreviewKMSValues(path, values)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := SaveKMSValues(path, values); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), preview) {
		t.Fatalf("preview not in the saved file:\n%s\nfile:\n%s", preview, data)
	}

	kms, err := ReadKMS(path)
	if err != nil {
		t.Fatal(err)
	}
	if kms["web.api-host"] != "127.0.0.1:11435" {
		t.Fatalf("api-host = %v", kms["web.api-host"])
	}
	if _, ok := kms["pool.budget-percent"]; ok {
		t.Fatal("a value left out of the section survived: pool.budget-percent")
	}
}

func TestSaveKMSRejectsInvalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model_config.yaml")
	before := "version: 2\nkms: {}\nmodels: {}\n"
	if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}

	// Not longer than the default 60m inference timeout.
	_, err := SaveKMS(path, []Change{{Key: "web.write-timeout", New: "1m"}})
	if err == nil || !strings.Contains(err.Error(), "nothing was written") {
		t.Fatalf("err = %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != before {
		t.Fatalf("file changed:\n%s", data)
	}
}

func TestPreviewMatchesSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model_config.yaml")
	in := "version: 2\n\n# server\nkms: {}\n\nmodels:\n  a/b:\n    # keep me\n    nseq-max: 2\n"
	if err := os.WriteFile(path, []byte(in), 0o644); err != nil {
		t.Fatal(err)
	}

	entryChanges := []Change{{Key: "context-window", New: 8192}}
	kmsChanges := []Change{{Key: "web.api-host", New: "0.0.0.0:11435"}, {Key: "pool.budget-percent", New: 80}}

	entry, err := PreviewEntry(path, SectionModels, "a/b", entryChanges)
	if err != nil {
		t.Fatal(err)
	}
	kms, err := PreviewKMS(path, kmsChanges)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := SaveEntry(path, SectionModels, "a/b", entryChanges); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveKMS(path, kmsChanges); err != nil {
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
