package model

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeArtifact(t *testing.T, payload []byte) (string, ArtifactDigest) {
	t.Helper()

	modelPath := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(modelPath, payload, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	sum := sha256.Sum256(payload)
	digest := ArtifactDigest{
		SHA256: fmt.Sprintf("%x", sum),
		Size:   int64(len(payload)),
	}

	return modelPath, digest
}

func TestVerifyArtifact(t *testing.T) {
	modelPath, digest := writeArtifact(t, []byte("hello kronk"))

	verification, changed, err := VerifyArtifact(modelPath, digest, nil, true)
	if err != nil {
		t.Fatalf("VerifyArtifact: %v", err)
	}
	if !changed {
		t.Error("changed: got false, want true")
	}
	if verification.SHA256 != digest.SHA256 {
		t.Errorf("SHA256: got %q, want %q", verification.SHA256, digest.SHA256)
	}
	if verification.Size != digest.Size {
		t.Errorf("Size: got %d, want %d", verification.Size, digest.Size)
	}
	if verification.VerifiedAt == 0 {
		t.Error("VerifiedAt: got 0, want verification time")
	}

	got, changed, err := VerifyArtifact(modelPath, digest, &verification, true)
	if err != nil {
		t.Fatalf("VerifyArtifact cached: %v", err)
	}
	if changed {
		t.Error("cached changed: got true, want false")
	}
	if got != verification {
		t.Errorf("cached verification: got %+v, want %+v", got, verification)
	}
}

func TestVerifyArtifactMetadataChangeRehashes(t *testing.T) {
	modelPath, digest := writeArtifact(t, []byte("hello kronk"))

	verification, _, err := VerifyArtifact(modelPath, digest, nil, true)
	if err != nil {
		t.Fatalf("VerifyArtifact: %v", err)
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

	if _, _, err := VerifyArtifact(modelPath, digest, &verification, true); err == nil {
		t.Fatal("VerifyArtifact: got nil, want sha256 mismatch")
	}
}

func TestVerifyArtifactSizeOnly(t *testing.T) {
	modelPath, digest := writeArtifact(t, []byte("hello kronk"))

	verification, changed, err := VerifyArtifact(modelPath, digest, nil, false)
	if err != nil {
		t.Fatalf("VerifyArtifact: %v", err)
	}
	if changed {
		t.Error("changed: got true, want false")
	}
	if verification != (ArtifactVerification{}) {
		t.Errorf("verification: got %+v, want zero value", verification)
	}

	if err := os.WriteFile(modelPath, []byte("different size"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, _, err := VerifyArtifact(modelPath, digest, nil, false); err == nil {
		t.Fatal("VerifyArtifact size mismatch: got nil, want error")
	}
}

func TestVerifyArtifactInvalidDigest(t *testing.T) {
	modelPath, _ := writeArtifact(t, []byte("hello kronk"))

	if _, _, err := VerifyArtifact(modelPath, ArtifactDigest{SHA256: "invalid", Size: 11}, nil, true); err == nil {
		t.Fatal("VerifyArtifact: got nil, want invalid digest error")
	}
}
