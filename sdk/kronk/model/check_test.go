package model

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func TestVerifyArtifactWithLogger(t *testing.T) {
	for _, tt := range []struct {
		name     string
		cached   bool
		checkSHA bool
		corrupt  bool
		wantLogs int
	}{
		{name: "hash", checkSHA: true, wantLogs: 2},
		{name: "cached", cached: true, checkSHA: true},
		{name: "size only"},
		{name: "mismatch", checkSHA: true, corrupt: true, wantLogs: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			modelPath, digest := writeArtifact(t, []byte("hello kronk"))
			var previous *ArtifactVerification
			if tt.cached {
				verification, _, err := VerifyArtifact(modelPath, digest, nil, true)
				if err != nil {
					t.Fatalf("VerifyArtifact: %v", err)
				}
				previous = &verification
			}
			if tt.corrupt {
				digest.SHA256 = strings.Repeat("0", 64)
			}

			var messages []string
			log := func(ctx context.Context, msg string, args ...any) {
				messages = append(messages, msg)
			}
			_, _, err := VerifyArtifactWithLogger(t.Context(), log, modelPath, digest, previous, tt.checkSHA)
			if (err != nil) != tt.corrupt {
				t.Fatalf("error: got %v, want mismatch %t", err, tt.corrupt)
			}
			if len(messages) != tt.wantLogs {
				t.Fatalf("logs: got %v, want %d messages", messages, tt.wantLogs)
			}
			if tt.wantLogs > 0 && !strings.Contains(messages[0], "verifying SHA-256 for model.gguf") {
				t.Errorf("start log: got %q, want verification start", messages[0])
			}
			if tt.wantLogs == 2 && !strings.Contains(messages[1], "SHA-256 verified for model.gguf in ") {
				t.Errorf("completion log: got %q, want success with duration", messages[1])
			}
		})
	}
}
