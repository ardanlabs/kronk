package backend

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildBundleManifest(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "lib"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "lib", "runtime.bin"), []byte("runtime"), 0o644); err != nil {
		t.Fatalf("write runtime: %v", err)
	}
	if err := os.Symlink("runtime.bin", filepath.Join(root, "lib", "current.bin")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	want, err := BuildBundleManifest(context.Background(), root, []string{"lib/runtime.bin", "lib/current.bin"})
	if err != nil {
		t.Fatalf("build manifest: %v", err)
	}
	got, err := BuildBundleManifest(context.Background(), root, []string{"lib/current.bin", "lib/runtime.bin"})
	if err != nil {
		t.Fatalf("build reordered manifest: %v", err)
	}

	if got.Digest != want.Digest {
		t.Errorf("Digest: got %q, want %q", got.Digest, want.Digest)
	}
	const wantDigest = "sha256:57ebed0c5537414c0f57cbf070878d9e93e17ba78b783b20a7e405adc3334361"
	if got.Digest != wantDigest {
		t.Errorf("Canonical digest: got %q, want %q", got.Digest, wantDigest)
	}
	if got.Version != BundleManifestVersion {
		t.Errorf("Version: got %q, want %q", got.Version, BundleManifestVersion)
	}
	if len(got.Files) != 2 {
		t.Fatalf("Files: got %d, want 2", len(got.Files))
	}
	if got.Files[0].Name != "lib/current.bin" || got.Files[0].SymlinkTarget != "runtime.bin" {
		t.Errorf("Symlink: got %+v, want current.bin -> runtime.bin", got.Files[0])
	}
	if got.Files[1].Name != "lib/runtime.bin" || got.Files[1].Size != 7 || got.Files[1].SHA256 == "" {
		t.Errorf("File: got %+v, want hashed 7-byte runtime", got.Files[1])
	}
}

func TestBuildBundleManifestRejectsPathEscape(t *testing.T) {
	_, err := BuildBundleManifest(context.Background(), t.TempDir(), []string{"../runtime.bin"})
	if err == nil {
		t.Fatal("BuildBundleManifest: got nil, want path escape error")
	}
}
