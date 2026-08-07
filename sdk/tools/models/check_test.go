package models

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

func writeManagedArtifact(t *testing.T, payload []byte) string {
	t.Helper()

	dir := t.TempDir()
	modelPath := filepath.Join(dir, "model.gguf")
	if err := os.WriteFile(modelPath, payload, 0o644); err != nil {
		t.Fatalf("WriteFile model: %v", err)
	}

	sum := sha256.Sum256(payload)
	pointer := fmt.Sprintf("oid sha256:%x\nsize %d\n", sum, len(payload))
	if err := os.MkdirAll(filepath.Dir(artifactDigestPath(modelPath)), 0o755); err != nil {
		t.Fatalf("MkdirAll sha: %v", err)
	}
	if err := os.WriteFile(artifactDigestPath(modelPath), []byte(pointer), 0o644); err != nil {
		t.Fatalf("WriteFile digest: %v", err)
	}

	return modelPath
}

func TestCheckModelWritesAndUsesVerification(t *testing.T) {
	modelPath := writeManagedArtifact(t, []byte("hello kronk"))

	if err := checkModel(modelPath, true); err != nil {
		t.Fatalf("checkModel: %v", err)
	}
	verification, err := readArtifactVerification(modelPath)
	if err != nil {
		t.Fatalf("readArtifactVerification: %v", err)
	}

	info, err := os.Stat(modelPath)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	garbage := make([]byte, info.Size())
	for i := range garbage {
		garbage[i] = 0xff
	}
	if err := os.WriteFile(modelPath, garbage, 0o644); err != nil {
		t.Fatalf("WriteFile garbage: %v", err)
	}
	if err := os.Chtimes(modelPath, time.Now(), info.ModTime()); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	if err := checkModel(modelPath, true); err != nil {
		t.Fatalf("checkModel cached: %v", err)
	}
	got, err := readArtifactVerification(modelPath)
	if err != nil {
		t.Fatalf("readArtifactVerification cached: %v", err)
	}
	if got != verification {
		t.Errorf("verification: got %+v, want %+v", got, verification)
	}
}

func TestCheckModelWithoutDigestIsUnmanaged(t *testing.T) {
	modelPath := filepath.Join(t.TempDir(), "model.gguf")

	if err := checkModel(modelPath, true); err != nil {
		t.Fatalf("checkModel: %v", err)
	}
}

func TestLoadArtifactIntegrityMalformedVerification(t *testing.T) {
	modelPath := writeManagedArtifact(t, []byte("hello kronk"))
	if err := os.WriteFile(artifactVerificationPath(modelPath), []byte("not-json"), 0o644); err != nil {
		t.Fatalf("WriteFile verification: %v", err)
	}

	integrity, exists, err := loadArtifactIntegrity(modelPath)
	if err != nil {
		t.Fatalf("loadArtifactIntegrity: %v", err)
	}
	if !exists {
		t.Fatal("exists: got false, want true")
	}
	if integrity.Verification != nil {
		t.Errorf("Verification: got %+v, want nil", integrity.Verification)
	}

	if err := checkModel(modelPath, true); err != nil {
		t.Fatalf("checkModel: %v", err)
	}
	if _, err := readArtifactVerification(modelPath); err != nil {
		t.Fatalf("readArtifactVerification replaced malformed record: %v", err)
	}
}

func TestCheckModelMetadataChangeRehashes(t *testing.T) {
	modelPath := writeManagedArtifact(t, []byte("hello kronk"))
	if err := checkModel(modelPath, true); err != nil {
		t.Fatalf("checkModel: %v", err)
	}

	info, err := os.Stat(modelPath)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	garbage := make([]byte, info.Size())
	for i := range garbage {
		garbage[i] = 0xff
	}
	if err := os.WriteFile(modelPath, garbage, 0o644); err != nil {
		t.Fatalf("WriteFile garbage: %v", err)
	}
	if err := os.Chtimes(modelPath, time.Now(), info.ModTime().Add(time.Second)); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	if err := checkModel(modelPath, true); err == nil {
		t.Fatal("checkModel: got nil, want sha256 mismatch")
	}
}

func TestRemoveArtifactVerification(t *testing.T) {
	modelPath := writeManagedArtifact(t, []byte("hello kronk"))
	if err := checkModel(modelPath, true); err != nil {
		t.Fatalf("checkModel: %v", err)
	}

	if err := removeArtifactVerification(modelPath); err != nil {
		t.Fatalf("removeArtifactVerification: %v", err)
	}
	if _, err := os.Stat(artifactVerificationPath(modelPath)); !os.IsNotExist(err) {
		t.Errorf("Stat verification: got %v, want not exist", err)
	}
	if err := removeArtifactVerification(modelPath); err != nil {
		t.Fatalf("removeArtifactVerification missing: %v", err)
	}
}

func TestConfigureArtifactIntegrity(t *testing.T) {
	payload := []byte("hello kronk")
	paths := []string{
		writeManagedArtifact(t, payload),
		writeManagedArtifact(t, payload),
		writeManagedArtifact(t, payload),
		writeManagedArtifact(t, payload),
	}

	cfg := model.Config{
		ModelFiles:     paths[:1],
		ProjFile:       paths[1],
		MTPDrafterFile: paths[2],
		PtrDraftModel:  &model.DraftModelConfig{ModelFiles: paths[3:]},
	}
	if err := configureArtifactIntegrity(&cfg); err != nil {
		t.Fatalf("configureArtifactIntegrity: %v", err)
	}
	if len(cfg.ArtifactIntegrity) != len(paths) {
		t.Fatalf("ArtifactIntegrity length: got %d, want %d", len(cfg.ArtifactIntegrity), len(paths))
	}
	for _, path := range paths {
		if _, exists := cfg.ArtifactIntegrity[path]; !exists {
			t.Errorf("ArtifactIntegrity missing path %q", path)
		}
	}
	if cfg.RecordArtifactVerification == nil {
		t.Fatal("RecordArtifactVerification: got nil, want recorder")
	}
}
