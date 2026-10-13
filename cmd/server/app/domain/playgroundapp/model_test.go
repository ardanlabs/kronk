package playgroundapp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

func TestSessionConfigPrefillBatchSizeJSON(t *testing.T) {
	var cfg SessionConfig
	if err := json.Unmarshal([]byte(`{"prefill_batch_size":4096,"physical_batch_size":512,"image_batch_token_capacity":768,"image_token_budget":1120}`), &cfg); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	got := cfg.ApplyTo(model.Config{})
	if got.PrefillBatchSize() != 4096 {
		t.Errorf("PrefillBatchSize() = %d, want 4096", got.PrefillBatchSize())
	}
	if got.PhysicalBatchSize() != 512 {
		t.Errorf("PhysicalBatchSize: got %d, want 512", got.PhysicalBatchSize())
	}
	for _, size := range []int{-1, 0, 512, 2147483648} {
		cfg := SessionConfig{PhysicalBatchSize: new(size)}
		if !cfg.HasOverrides() {
			t.Fatal("physical size alone must select a custom config")
		}
		if wantErr := size <= 0 || size > 2147483647; (cfg.Validate() != nil) != wantErr {
			t.Errorf("physical size %d: want error=%t", size, wantErr)
		}
	}
	if got.ImageBatchTokenCapacity() != 768 {
		t.Errorf("ImageBatchTokenCapacity() = %d, want 768", got.ImageBatchTokenCapacity())
	}
	if got.ImageTokenBudget() != 1120 {
		t.Errorf("ImageTokenBudget() = %d, want 1120", got.ImageTokenBudget())
	}
	for _, budget := range []int{-1, 0, 768, 2147483648} {
		cfg := SessionConfig{ImageTokenBudget: new(budget)}
		if !cfg.HasOverrides() {
			t.Fatal("image budget alone must select a custom config")
		}
		wantErr := budget <= 0 || budget > 2147483647
		if err := cfg.Validate(); (err != nil) != wantErr {
			t.Errorf("Validate budget %d: got %v, want error=%t", budget, err, wantErr)
		}
	}
	for _, capacity := range []int{0, 768, -1} {
		cfg := SessionConfig{ImageBatchTokenCapacity: new(capacity)}
		if !cfg.HasOverrides() {
			t.Fatal("image capacity alone must select a custom config")
		}
		if err := cfg.Validate(); (err != nil) != (capacity < 0) {
			t.Errorf("Validate capacity %d: got %v, want error=%t", capacity, err, capacity < 0)
		}
	}
}

func TestSessionConfigApplyTo_LoadMode(t *testing.T) {
	tests := []struct {
		name string
		base model.LoadMode
		sc   SessionConfig
		want model.LoadMode
	}{
		{"unset leaves base untouched", model.LoadModeDirectIO, SessionConfig{}, model.LoadModeDirectIO},
		{"mmap overrides a non-default base", model.LoadModeDirectIO, SessionConfig{LoadMode: new(model.LoadModeMMap)}, model.LoadModeMMap},
		{"mlock overrides base", model.LoadModeMMap, SessionConfig{LoadMode: new(model.LoadModeMLock)}, model.LoadModeMLock},
		{"mmap mlock overrides base", model.LoadModeMLock, SessionConfig{LoadMode: new(model.LoadModeMMapMLock)}, model.LoadModeMMapMLock},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.sc.ApplyTo(model.Config{LoadMode: tt.base})
			if got.LoadMode != tt.want {
				t.Errorf("LoadMode = %v, want %v", got.LoadMode, tt.want)
			}
			if gotHasOverrides := tt.sc.HasOverrides(); gotHasOverrides != (tt.sc.LoadMode != nil) {
				t.Errorf("HasOverrides() = %v, want %v", gotHasOverrides, tt.sc.LoadMode != nil)
			}
		})
	}
}

func TestSessionConfigKVCacheMode(t *testing.T) {
	for _, tt := range []struct {
		input    string
		want     model.KVCacheMode
		override bool
	}{
		{`{}`, model.KVCacheShared, false},
		{`{"kv_cache_mode":"partitioned"}`, model.KVCachePartitioned, true},
		{`{"kv_cache_mode":"shared"}`, model.KVCacheShared, true},
	} {
		var cfg SessionConfig
		if err := json.Unmarshal([]byte(tt.input), &cfg); err != nil {
			t.Fatal(err)
		}
		got := cfg.ApplyTo(model.Config{PtrKVCacheMode: new(model.KVCacheShared)})
		if got.KVCacheMode() != tt.want || cfg.HasOverrides() != tt.override {
			t.Errorf("%s: got mode %q, override %t, want %q, %t", tt.input, got.KVCacheMode(), cfg.HasOverrides(), tt.want, tt.override)
		}
	}
}

func TestSessionConfigApplyTo_DraftModel(t *testing.T) {
	tests := []struct {
		name      string
		base      model.Config
		sc        SessionConfig
		wantNil   bool
		wantFiles []string
		wantND    int
	}{
		{
			name:    "no draft fields leaves base untouched (nil)",
			sc:      SessionConfig{},
			wantNil: true,
		},
		{
			name:    "empty draft_model_id clears existing draft",
			base:    model.Config{PtrDraftModel: &model.DraftModelConfig{ModelFiles: []string{"d.gguf"}}},
			sc:      SessionConfig{DraftModelID: new("")},
			wantNil: true,
		},
		{
			name:    "draft_ndraft only creates an MTP override (no files)",
			sc:      SessionConfig{DraftNDraft: new(7)},
			wantND:  7,
			wantNil: false,
		},
		{
			name:   "draft_ndraft tunes an existing MTP override",
			base:   model.Config{PtrDraftModel: &model.DraftModelConfig{NDraft: 4}},
			sc:     SessionConfig{DraftNDraft: new(9)},
			wantND: 9,
		},
		{
			name:      "draft_model_id sets up a separate draft (files resolved later)",
			sc:        SessionConfig{DraftModelID: new("some-draft"), DraftNDraft: new(5)},
			wantND:    5,
			wantFiles: nil, // files are resolved in the handler, not ApplyTo
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.sc.ApplyTo(tt.base)

			if tt.wantNil {
				if got.PtrDraftModel != nil {
					t.Fatalf("PtrDraftModel = %+v, want nil", got.PtrDraftModel)
				}
				return
			}

			if got.PtrDraftModel == nil {
				t.Fatalf("PtrDraftModel = nil, want non-nil")
			}
			if got.PtrDraftModel.NDraft != tt.wantND {
				t.Errorf("NDraft = %d, want %d", got.PtrDraftModel.NDraft, tt.wantND)
			}
			if len(tt.wantFiles) != len(got.PtrDraftModel.ModelFiles) {
				t.Errorf("ModelFiles = %v, want %v", got.PtrDraftModel.ModelFiles, tt.wantFiles)
			}
		})
	}
}

func TestValidateTemplateName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"simple name is valid", "chatml", false},
		{"name with dashes and digits is valid", "llama-3-instruct", false},
		{"empty name is rejected", "", true},
		{"name over 255 chars is rejected", strings.Repeat("a", 256), true},
		{"parent traversal is rejected", "..", true},
		{"embedded parent traversal is rejected", "foo/../bar", true},
		{"forward slash is rejected", "dir/template", true},
		{"backslash is rejected", "dir\\template", true},
		{"leading dot is rejected", ".hidden", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTemplateName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTemplateName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestSessionConfigValidate_DraftNDraft(t *testing.T) {
	tests := []struct {
		name    string
		sc      SessionConfig
		wantErr bool
	}{
		{"MTP override without draft_model_id is valid", SessionConfig{DraftNDraft: new(6)}, false},
		{"separate draft with ndraft is valid", SessionConfig{DraftModelID: new("d"), DraftNDraft: new(6)}, false},
		{"ndraft below range is rejected", SessionConfig{DraftNDraft: new(0)}, true},
		{"ndraft above range is rejected", SessionConfig{DraftNDraft: new(21)}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.sc.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
