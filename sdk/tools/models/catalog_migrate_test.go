package models

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.yaml.in/yaml/v2"
)

func TestCatalogV2MigrationOffline(t *testing.T) {
	blockDraftTestNetwork(t)
	const input = `version: 2
models:
  custom/renamed-Q4:
    provider: custom
    family: private-repository
    revision: pinned-commit
    files: [nested/renamed-Q4.gguf]
    file_sizes: [1234]
    mmproj: mmproj-renamed-Q4.gguf
    mmproj_orig: nested/projection.gguf
    mmproj_size: 456
    mtp: mtp-custom-name.gguf
    mtp_orig: drafts/original.gguf
    mtp_url: https://huggingface.co/other/drafter/resolve/pinned/drafts/original.gguf?download=true
    mtp_size: 789
    mtp_checked: true
    mtp_source: companion
    model_type: Customized
    architecture: custom-arch
    sequence_processing: Custom
    capabilities: {endpoint: chat_completion, streaming: true, reasoning: true, tooling: true, mtp: true}
    resolved_at: 2026-01-02T03:04:05Z
  custom/embedded:
    provider: custom
    family: embedded-repo
    files: [embedded.gguf]
    mtp_checked: true
    mtp_source: embedded
    capabilities: {mtp: true}
  custom/unchecked:
    provider: custom
    family: unknown-repo
    files: [unknown.gguf]
`
	file := filepath.Join(t.TempDir(), "catalog.yaml")
	mustWriteFile(t, file, input)
	var want Catalog
	if err := yaml.Unmarshal([]byte(input), &want); err != nil {
		t.Fatal(err)
	}
	want.Version = "3.2"
	custom := want.Models["custom/renamed-Q4"]
	custom.Draft = CatalogDraft{
		Method: DraftMethodMTP, File: "mtp-custom-name.gguf", Original: "drafts/original.gguf",
		URL: "https://huggingface.co/other/drafter/resolve/pinned/drafts/original.gguf?download=true", Size: 789,
	}
	custom.DraftChecked = true
	custom.Capabilities.Draft = DraftMethodMTP
	custom.Capabilities.MTP = true
	want.Models["custom/renamed-Q4"] = custom
	embedded := want.Models["custom/embedded"]
	embedded.DraftChecked = true
	embedded.Capabilities.Draft = DraftMethodMTP
	embedded.Capabilities.MTP = true
	want.Models["custom/embedded"] = embedded
	r := newResolver(nil, file, &fakeHF{}, func() bool { t.Fatal("migration probed network"); return false })
	got, err := r.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("migration: got %+v, want %+v", got, want)
	}
	first, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(first, []byte("mtp_orig:")) || bytes.Contains(first, []byte("mtp_checked:")) || !bytes.HasPrefix(first, []byte("version: \"3.2\"\n")) {
		t.Fatalf("migration retained obsolete schema: %s", first)
	}
	got, err = NewResolver(nil, file).Load()
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("second Load: got %+v, %v, want %+v", got, err, want)
	}
	second, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatalf("idempotence: contents changed: %v", err)
	}
	leftovers, err := filepath.Glob(file + ".*.tmp")
	if err != nil || len(leftovers) != 0 {
		t.Fatalf("temporary files: got %v, %v", leftovers, err)
	}
}

func TestCatalogVersionRejection(t *testing.T) {
	for _, version := range []string{"", "-1", "0", "1", "3.0", "3.01", "3.10", "3.20", "3.3", "4", "99"} {
		t.Run(version, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "catalog.yaml")
			input := fmt.Sprintf("version: %s\nmodels: {}\n", version)
			mustWriteFile(t, file, input)
			r := NewResolver(nil, file)
			if _, err := r.Load(); err == nil || !strings.Contains(err.Error(), "unsupported catalog version") {
				t.Fatalf("Load: got %v, want unsupported version", err)
			}
			if err := r.Save(Catalog{Version: version}); err == nil {
				t.Fatal("Save accepted unsupported version")
			}
			if data, err := os.ReadFile(file); err != nil || string(data) != input {
				t.Fatalf("rejection changed catalog: %q, %v", data, err)
			}
		})
	}
}

func TestCatalogMigrationValidationIsAtomic(t *testing.T) {
	file := filepath.Join(t.TempDir(), "catalog.yaml")
	const input = "version: 2\nmodels:\n  owner/model:\n    mtp: ../bad.gguf\n    mtp_checked: true\n"
	mustWriteFile(t, file, input)
	if _, err := NewResolver(nil, file).Load(); err == nil {
		t.Fatal("Load accepted invalid artifact")
	}
	if data, err := os.ReadFile(file); err != nil || string(data) != input {
		t.Fatalf("failed validation changed catalog: %q, %v", data, err)
	}
}

func TestNewEmptyCatalogV32(t *testing.T) {
	cat, err := NewResolver(nil, filepath.Join(t.TempDir(), "absent.yaml")).Load()
	if err != nil || cat.Version != "3.2" || cat.Models == nil || len(cat.Models) != 0 {
		t.Fatalf("new catalog: got %+v, %v", cat, err)
	}
}

func TestCatalogDraftCapabilities(t *testing.T) {
	for _, version := range []string{"3", "3.1"} {
		t.Run(version, func(t *testing.T) {
			blockDraftTestNetwork(t)
			const input = `version: 3
models:
  none:
    capabilities: {endpoint: decision, decision: true, mtp: false}
  embedded:
    mtp_source: embedded
    capabilities: {mtp: true}
  legacy:
    capabilities: {mtp: true}
  mtp:
    draft: {method: mtp, file: head.gguf}
    capabilities: {mtp: true}
  dspark:
    draft: {method: dspark, file: head.gguf, original: nested/head.gguf_file, url: "https://huggingface.co/other/drafts/resolve/main/nested/head.gguf_file", size: 73}
    draft_checked: true
    capabilities: {mtp: false}
  conventional:
    draft: {method: conventional, file: head.gguf}
    capabilities: {mtp: false}
  both:
    mtp_source: embedded
    draft: {method: dspark, file: head.gguf}
    capabilities: {mtp: true}
  declared:
    capabilities: {draft: dspark}
`
			file := filepath.Join(t.TempDir(), "catalog.yaml")
			mustWriteFile(t, file, strings.Replace(input, "version: 3\n", "version: \""+version+"\"\n", 1))
			r := NewResolver(nil, file)
			cat, err := r.Load()
			if err != nil {
				t.Fatal(err)
			}
			if cat.Version != "3.2" {
				t.Fatalf("version: got %q, want 3.2", cat.Version)
			}
			for _, tt := range []struct {
				id     string
				method DraftMethod
				mtp    bool
			}{
				{"none", DraftMethod{}, false},
				{"embedded", DraftMethodMTP, true},
				{"legacy", DraftMethodMTP, true},
				{"mtp", DraftMethodMTP, true},
				{"dspark", DraftMethodDSpark, false},
				{"conventional", DraftMethodConventional, false},
				{"both", DraftMethodDSpark, true},
				{"declared", DraftMethodDSpark, false},
			} {
				if got := cat.Models[tt.id].Capabilities; got.Draft != tt.method || got.MTP != tt.mtp {
					t.Errorf("%s: got %+v, want draft %s and MTP %t", tt.id, got, tt.method, tt.mtp)
				}
			}
			if caps := cat.Models["none"].Capabilities; caps.Endpoint != "decision" || !caps.Decision {
				t.Errorf("unrelated capabilities changed: %+v", caps)
			}
			wantDraft := CatalogDraft{Method: DraftMethodDSpark, File: "head.gguf", Original: "nested/head.gguf_file",
				URL: "https://huggingface.co/other/drafts/resolve/main/nested/head.gguf_file", Size: 73}
			if entry := cat.Models["dspark"]; entry.Draft != wantDraft || !entry.DraftChecked {
				t.Errorf("v3 companion metadata changed: got %+v, want %+v and checked", entry, wantDraft)
			}
			first, err := os.ReadFile(file)
			if err != nil || bytes.Contains(first, []byte("mtp: true")) || bytes.Contains(first, []byte("mtp: false")) {
				t.Fatalf("obsolete capability retained: %s, %v", first, err)
			}
			second, err := r.Load()
			if err != nil {
				t.Fatal(err)
			}
			for id, entry := range cat.Models {
				got := second.Models[id]
				if got.Capabilities != entry.Capabilities || got.Draft != entry.Draft || got.MTPSource != entry.MTPSource {
					t.Errorf("%s: reload changed drafting metadata: %+v", id, got)
				}
			}
			data, err := os.ReadFile(file)
			if err != nil || !bytes.Equal(first, data) {
				t.Fatalf("capability migration is not idempotent: %v", err)
			}
		})
	}
}
