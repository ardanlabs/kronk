package models

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.yaml.in/yaml/v2"
)

func TestIntegrityArtifact(t *testing.T) {
	const digest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	tests := []struct {
		name       string
		pointer    string
		sentinel   func(os.FileInfo) string
		wantStatus IntegrityStatus
		wantReason string
		wantDigest string
	}{
		{
			name:    "verified",
			pointer: fmt.Sprintf("oid sha256:%s\nsize 5\n", digest),
			sentinel: func(info os.FileInfo) string {
				return fmt.Sprintf(`{"sha256":%q,"size":5,"mtime_ns":%d,"verified_at":123}`, digest, info.ModTime().UnixNano())
			},
			wantStatus: IntegrityVerified,
			wantDigest: "sha256:" + digest,
		},
		{
			name:       "unverified",
			pointer:    fmt.Sprintf("oid sha256:%s\nsize 5\n", digest),
			wantStatus: IntegrityUnverified,
			wantDigest: "sha256:" + digest,
		},
		{
			name:    "stale digest",
			pointer: fmt.Sprintf("oid sha256:%s\nsize 5\n", digest),
			sentinel: func(info os.FileInfo) string {
				return fmt.Sprintf(`{"sha256":%q,"size":5,"mtime_ns":%d,"verified_at":123}`, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", info.ModTime().UnixNano())
			},
			wantStatus: IntegrityStale,
			wantReason: IntegrityReasonFileMetadataChanged,
			wantDigest: "sha256:" + digest,
		},
		{
			name:    "malformed sentinel",
			pointer: fmt.Sprintf("oid sha256:%s\nsize 5\n", digest),
			sentinel: func(info os.FileInfo) string {
				return "not-json"
			},
			wantStatus: IntegrityStale,
			wantReason: IntegrityReasonFileMetadataChanged,
			wantDigest: "sha256:" + digest,
		},
		{
			name:    "invalid verification timestamp",
			pointer: fmt.Sprintf("oid sha256:%s\nsize 5\n", digest),
			sentinel: func(info os.FileInfo) string {
				return fmt.Sprintf(`{"sha256":%q,"size":5,"mtime_ns":%d,"verified_at":9223372036854775807}`, digest, info.ModTime().UnixNano())
			},
			wantStatus: IntegrityStale,
			wantReason: IntegrityReasonFileMetadataChanged,
			wantDigest: "sha256:" + digest,
		},
		{
			name:       "missing pointer",
			wantStatus: IntegrityUnavailable,
			wantReason: IntegrityReasonDigestUnavailable,
		},
		{
			name:       "malformed pointer",
			pointer:    "oid sha256:not-a-digest\nsize 5\n",
			wantStatus: IntegrityUnavailable,
			wantReason: IntegrityReasonDigestUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			artifactPath := filepath.Join(dir, "model.gguf")
			if err := os.WriteFile(artifactPath, []byte("model"), 0o644); err != nil {
				t.Fatalf("WriteFile artifact: %v", err)
			}

			info, err := os.Stat(artifactPath)
			if err != nil {
				t.Fatalf("Stat artifact: %v", err)
			}

			shaDir := filepath.Join(dir, "sha")
			if err := os.MkdirAll(shaDir, 0o755); err != nil {
				t.Fatalf("MkdirAll sha: %v", err)
			}
			if tt.pointer != "" {
				if err := os.WriteFile(filepath.Join(shaDir, "model.gguf"), []byte(tt.pointer), 0o644); err != nil {
					t.Fatalf("WriteFile pointer: %v", err)
				}
			}
			if tt.sentinel != nil {
				if err := os.WriteFile(filepath.Join(shaDir, "model.gguf.verified"), []byte(tt.sentinel(info)), 0o644); err != nil {
					t.Fatalf("WriteFile sentinel: %v", err)
				}
			}

			got, err := integrityArtifact(artifactPath, IntegrityRoleWeights)
			if err != nil {
				t.Fatalf("integrityArtifact: %v", err)
			}
			if got.Status != tt.wantStatus {
				t.Errorf("Status: got %q, want %q", got.Status, tt.wantStatus)
			}
			if got.Reason != tt.wantReason {
				t.Errorf("Reason: got %q, want %q", got.Reason, tt.wantReason)
			}
			if got.Digest != tt.wantDigest {
				t.Errorf("Digest: got %q, want %q", got.Digest, tt.wantDigest)
			}
			if got.Verified != (tt.wantStatus == IntegrityVerified) {
				t.Errorf("Verified: got %t, want %t", got.Verified, tt.wantStatus == IntegrityVerified)
			}
			if tt.wantStatus == IntegrityVerified && !got.VerifiedAt.Equal(time.Unix(123, 0).UTC()) {
				t.Errorf("VerifiedAt: got %s, want %s", got.VerifiedAt, time.Unix(123, 0).UTC())
			}
		})
	}
}

func TestModelsIntegrity(t *testing.T) {
	const digest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	m, err := NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatalf("NewWithPaths: %v", err)
	}

	dir := filepath.Join(m.modelsPath, "unsloth", "Qwen-GGUF")
	if err := os.MkdirAll(filepath.Join(dir, "sha"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	paths := []string{
		filepath.Join(dir, "model-00002-of-00002.gguf"),
		filepath.Join(dir, "model-00001-of-00002.gguf"),
		filepath.Join(dir, "mmproj-model.gguf"),
		filepath.Join(dir, "mtp-model.gguf"),
	}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("model"), 0o644); err != nil {
			t.Fatalf("WriteFile artifact: %v", err)
		}
		pointer := fmt.Sprintf("oid sha256:%s\nsize 5\n", digest)
		if err := os.WriteFile(filepath.Join(dir, "sha", filepath.Base(path)), []byte(pointer), 0o644); err != nil {
			t.Fatalf("WriteFile pointer: %v", err)
		}
	}

	index := map[string]Path{
		"Qwen": {
			ModelFiles: paths[:2],
			ProjFile:   paths[2],
			MTPFile:    paths[3],
		},
	}
	data, err := yaml.Marshal(index)
	if err != nil {
		t.Fatalf("Marshal index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(m.modelsPath, indexFile), data, 0o644); err != nil {
		t.Fatalf("WriteFile index: %v", err)
	}

	got, err := m.Integrity()
	if err != nil {
		t.Fatalf("Integrity: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Integrity length: got %d, want 1", len(got))
	}

	modelInfo := got[0]
	if modelInfo.OwnedBy != "unsloth" {
		t.Errorf("OwnedBy: got %q, want %q", modelInfo.OwnedBy, "unsloth")
	}
	if modelInfo.ModelFamily != "Qwen-GGUF" {
		t.Errorf("ModelFamily: got %q, want %q", modelInfo.ModelFamily, "Qwen-GGUF")
	}
	if modelInfo.Status != IntegrityUnverified || modelInfo.Verified {
		t.Errorf("model integrity: got status %q verified %t, want unverified false", modelInfo.Status, modelInfo.Verified)
	}

	wantFiles := []string{
		"model-00001-of-00002.gguf",
		"model-00002-of-00002.gguf",
		"mmproj-model.gguf",
		"mtp-model.gguf",
	}
	wantRoles := []string{IntegrityRoleWeights, IntegrityRoleWeights, IntegrityRoleProjection, IntegrityRoleMTP}
	if len(modelInfo.Artifacts) != len(wantFiles) {
		t.Fatalf("Artifacts length: got %d, want %d", len(modelInfo.Artifacts), len(wantFiles))
	}
	for i, artifact := range modelInfo.Artifacts {
		if artifact.Filename != wantFiles[i] {
			t.Errorf("Artifacts[%d].Filename: got %q, want %q", i, artifact.Filename, wantFiles[i])
		}
		if artifact.Role != wantRoles[i] {
			t.Errorf("Artifacts[%d].Role: got %q, want %q", i, artifact.Role, wantRoles[i])
		}
	}

	single, err := m.IntegrityFor("Qwen")
	if err != nil {
		t.Fatalf("IntegrityFor: %v", err)
	}
	if single.ID != "Qwen" || len(single.Artifacts) != len(wantFiles) {
		t.Errorf("IntegrityFor: got ID %q with %d artifacts, want Qwen with %d", single.ID, len(single.Artifacts), len(wantFiles))
	}

	_, err = m.IntegrityFor("missing")
	if !errors.Is(err, ErrModelNotFound) {
		t.Errorf("IntegrityFor missing: got %v, want ErrModelNotFound", err)
	}
}

func TestModelsIntegrityCaseInsensitiveTieOrder(t *testing.T) {
	const digest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	m, err := NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatalf("NewWithPaths: %v", err)
	}

	dir := filepath.Join(m.modelsPath, "org", "family")
	if err := os.MkdirAll(filepath.Join(dir, "sha"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	index := make(map[string]Path)
	for _, modelID := range []string{"foo", "Foo"} {
		artifactPath := filepath.Join(dir, modelID+".gguf")
		if err := os.WriteFile(artifactPath, []byte("model"), 0o644); err != nil {
			t.Fatalf("WriteFile artifact: %v", err)
		}
		pointer := fmt.Sprintf("oid sha256:%s\nsize 5\n", digest)
		if err := os.WriteFile(filepath.Join(dir, "sha", filepath.Base(artifactPath)), []byte(pointer), 0o644); err != nil {
			t.Fatalf("WriteFile pointer: %v", err)
		}
		index[modelID] = Path{ModelFiles: []string{artifactPath}}
	}

	data, err := yaml.Marshal(index)
	if err != nil {
		t.Fatalf("Marshal index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(m.modelsPath, indexFile), data, 0o644); err != nil {
		t.Fatalf("WriteFile index: %v", err)
	}

	got, err := m.Integrity()
	if err != nil {
		t.Fatalf("Integrity: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Integrity length: got %d, want 2", len(got))
	}
	if got[0].ID != "Foo" || got[1].ID != "foo" {
		t.Errorf("model order: got [%q %q], want [Foo foo]", got[0].ID, got[1].ID)
	}
}
