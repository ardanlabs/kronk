package playgroundapp

import (
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

//go:fix inline
func ptr[T any](v T) *T { return new(v) }

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
			base:    model.Config{DraftModel: &model.DraftModelConfig{ModelFiles: []string{"d.gguf"}}},
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
			base:   model.Config{DraftModel: &model.DraftModelConfig{NDraft: 4}},
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
				if got.DraftModel != nil {
					t.Fatalf("DraftModel = %+v, want nil", got.DraftModel)
				}
				return
			}

			if got.DraftModel == nil {
				t.Fatalf("DraftModel = nil, want non-nil")
			}
			if got.DraftModel.NDraft != tt.wantND {
				t.Errorf("NDraft = %d, want %d", got.DraftModel.NDraft, tt.wantND)
			}
			if len(tt.wantFiles) != len(got.DraftModel.ModelFiles) {
				t.Errorf("ModelFiles = %v, want %v", got.DraftModel.ModelFiles, tt.wantFiles)
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
