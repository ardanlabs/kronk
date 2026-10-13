package models

import (
	"encoding/json"
	"testing"

	"go.yaml.in/yaml/v2"
)

func TestFileRoleSerialization(t *testing.T) {
	for _, role := range []FileRole{RoleModel, RoleDiffusion, RoleVAE, RoleClipL, RoleClipG, RoleT5XXL, RoleLLM, RoleLLMVision, RoleTokenizer, RoleControlNet, RoleTAESD, RolePhotoMaker, RoleClipVision, RoleHighNoise, RoleEmbeddingsConn, RoleMotionModule, RoleUpscaler, RoleADetailer, RoleAudioEncoder, {}} {
		t.Run(role.String(), func(t *testing.T) {
			if parsed := MustParseFileRole(role.String()); !parsed.Equal(role) {
				t.Errorf("Parse: got %v, want %v", parsed, role)
			}
			data, err := json.Marshal(role)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != `"`+role.String()+`"` {
				t.Errorf("JSON: got %s, want %q", data, role.String())
			}
			var got FileRole
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatal(err)
			}
			if !got.Equal(role) {
				t.Errorf("JSON round trip: got %v, want %v", got, role)
			}
			data, err = yaml.Marshal(role)
			if err != nil {
				t.Fatal(err)
			}
			if err := yaml.Unmarshal(data, &got); err != nil {
				t.Fatal(err)
			}
			if !got.Equal(role) {
				t.Errorf("YAML round trip: got %v, want %v", got, role)
			}
			if err := got.UnmarshalText([]byte("unknown")); err == nil {
				t.Fatal("unknown role accepted")
			}
			if !got.Equal(role) {
				t.Errorf("failed parse changed value: got %v, want %v", got, role)
			}
		})
	}
	if !(FileRole{}).IsZero() || RoleModel.IsZero() {
		t.Fatal("incorrect zero semantics")
	}
}
