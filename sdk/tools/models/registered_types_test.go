package models

import (
	"encoding"
	"encoding/json"
	"testing"

	"go.yaml.in/yaml/v2"
)

func TestRegisteredArtifactTypes(t *testing.T) {
	tests := []struct {
		name   string
		value  encoding.TextMarshaler
		target encoding.TextUnmarshaler
		want   string
	}{
		{"embedded", MTPSourceEmbedded, new(MTPSource), "embedded"},
		{"companion", MTPSourceCompanion, new(MTPSource), "companion"},
		{"verified", IntegrityVerified, new(IntegrityStatus), "verified"},
		{"unverified", IntegrityUnverified, new(IntegrityStatus), "unverified"},
		{"stale", IntegrityStale, new(IntegrityStatus), "stale"},
		{"unavailable", IntegrityUnavailable, new(IntegrityStatus), "unavailable"},
		{"weights", IntegrityRoleWeights, new(IntegrityRole), "weights"},
		{"projection", IntegrityRoleProjection, new(IntegrityRole), "projection"},
		{"mtp", IntegrityRoleMTP, new(IntegrityRole), "mtp"},
		{"digest", IntegrityReasonDigestUnavailable, new(IntegrityReason), "digest_unavailable"},
		{"metadata", IntegrityReasonFileMetadataChanged, new(IntegrityReason), "file_metadata_changed"},
		{"zero", MTPSource{}, new(MTPSource), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.value)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != `"`+tt.want+`"` {
				t.Fatalf("JSON: got %s, want %q", data, tt.want)
			}
			if err := json.Unmarshal(data, tt.target); err != nil {
				t.Fatal(err)
			}
			text, err := tt.target.(encoding.TextMarshaler).MarshalText()
			if err != nil || string(text) != tt.want {
				t.Fatalf("round trip: got %q, %v, want %q", text, err, tt.want)
			}
			data, err = yaml.Marshal(tt.value)
			if err != nil {
				t.Fatal(err)
			}
			if err := yaml.Unmarshal(data, tt.target); err != nil {
				t.Fatal(err)
			}
			if err := tt.target.UnmarshalText([]byte("unknown")); err == nil {
				t.Fatal("unknown value accepted")
			}
			text, err = tt.target.(encoding.TextMarshaler).MarshalText()
			if err != nil || string(text) != tt.want {
				t.Fatalf("failed parse changed value: got %q, %v, want %q", text, err, tt.want)
			}
		})
	}
}

func TestMTPSourceOmission(t *testing.T) {
	value := struct {
		Source MTPSource `json:"mtp_source,omitzero" yaml:"mtp_source,omitempty"`
	}{}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{}" {
		t.Errorf("JSON: got %s, want {}", data)
	}
	data, err = yaml.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{}\n" {
		t.Errorf("YAML: got %q, want {}", data)
	}
}
