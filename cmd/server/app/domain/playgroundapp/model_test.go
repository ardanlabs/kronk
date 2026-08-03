package playgroundapp

import (
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

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
