package libs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/go-getter"
)

func TestVersionGreater(t *testing.T) {
	tests := []struct {
		name  string
		left  string
		right string
		want  bool
	}{
		{name: "older", left: "master-812-aaaaaaa", right: "master-813-bfbef5b"},
		{name: "equal", left: "master-813-aaaaaaa", right: "master-813-bfbef5b"},
		{name: "newer", left: "master-814-aaaaaaa", right: "master-813-bfbef5b", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := versionGreater(tt.left, tt.right); got != tt.want {
				t.Errorf("versionGreater(%q, %q): got %v, want %v", tt.left, tt.right, got, tt.want)
			}
		})
	}
}

func TestExplicitRuntimeSelection(t *testing.T) {
	if _, err := New(WithArch("arm64"), WithOS("linux"), WithProcessor("cuda")); err == nil {
		t.Fatal("New() error = nil, want unsupported tuple error")
	}
	lib, err := New(WithArch("amd64"), WithOS("linux"), WithProcessor("rocm"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if got := lib.Processor(); got != "rocm" {
		t.Errorf("Processor(): got %q, want rocm", got)
	}
}

func TestSupportedCombinationsCopy(t *testing.T) {
	first := SupportedCombinations()
	first[0].Arch = "changed"
	if got := SupportedCombinations()[0].Arch; got == "changed" {
		t.Fatal("SupportedCombinations returned shared storage")
	}
}

func TestExplicitPathRejectsInvalidVersionMarker(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, versionFile), []byte(`{"version":"813","arch":"arm64","os":"darwin","processor":"metal"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := New(WithLibPath(dir)); err == nil {
		t.Fatal("New() error = nil, want invalid version marker error")
	}
}

func TestExplicitPathIsCleaned(t *testing.T) {
	dir := t.TempDir()
	lib, err := New(WithLibPath(dir + string(filepath.Separator)))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if lib.Root() != filepath.Clean(dir) {
		t.Errorf("Root(): got %q, want %q", lib.Root(), filepath.Clean(dir))
	}
}

func TestDownloadInstallsStagedBundle(t *testing.T) {
	originalNetwork := networkAvailable
	originalDownload := downloadLibraries
	t.Cleanup(func() {
		networkAvailable = originalNetwork
		downloadLibraries = originalDownload
	})
	networkAvailable = func(context.Context) bool { return true }
	downloadLibraries = func(ctx context.Context, architecture, osName, processor, version, dest string, progress getter.ProgressTracker) error {
		return os.WriteFile(filepath.Join(dest, "libstable-diffusion.dylib"), []byte("native"), 0o644)
	}

	lib, err := New(
		WithBasePath(t.TempDir()),
		WithArch("arm64"),
		WithOS("darwin"),
		WithProcessor("metal"),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	tag, err := lib.Download(t.Context(), nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if tag.Version != defaultVersion {
		t.Errorf("Version: got %q, want %q", tag.Version, defaultVersion)
	}
	data, err := os.ReadFile(filepath.Join(lib.LibsPath(), "libstable-diffusion.dylib"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "native" {
		t.Errorf("library contents: got %q, want native", data)
	}
	if matches, _ := filepath.Glob(filepath.Join(filepath.Dir(lib.LibsPath()), ".metal.stage-*")); len(matches) != 0 {
		t.Errorf("staging directories: got %v, want none", matches)
	}
}

func TestDownloadFailurePreservesExistingInstall(t *testing.T) {
	originalNetwork := networkAvailable
	originalDownload := downloadLibraries
	t.Cleanup(func() {
		networkAvailable = originalNetwork
		downloadLibraries = originalDownload
	})
	networkAvailable = func(context.Context) bool { return true }
	downloadLibraries = func(ctx context.Context, architecture, osName, processor, version, dest string, progress getter.ProgressTracker) error {
		if err := os.WriteFile(filepath.Join(dest, "partial"), []byte("partial"), 0o644); err != nil {
			return err
		}
		return errors.New("download failed")
	}

	lib, err := New(
		WithBasePath(t.TempDir()),
		WithArch("arm64"),
		WithOS("darwin"),
		WithProcessor("metal"),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := os.MkdirAll(lib.LibsPath(), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(lib.LibsPath(), "existing.dylib")
	if err := os.WriteFile(existing, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := lib.Download(t.Context(), nil); err == nil {
		t.Fatal("Download() error = nil, want download failure")
	}
	data, err := os.ReadFile(existing)
	if err != nil {
		t.Fatalf("existing installation was removed: %v", err)
	}
	if string(data) != "keep" {
		t.Errorf("existing installation: got %q, want keep", data)
	}
	if matches, _ := filepath.Glob(filepath.Join(filepath.Dir(lib.LibsPath()), ".metal.stage-*")); len(matches) != 0 {
		t.Errorf("staging directories: got %v, want none", matches)
	}
}

func TestDownloadHonorsCancellation(t *testing.T) {
	originalNetwork := networkAvailable
	originalDownload := downloadLibraries
	t.Cleanup(func() {
		networkAvailable = originalNetwork
		downloadLibraries = originalDownload
	})
	networkAvailable = func(context.Context) bool { return true }
	ctx, cancel := context.WithCancel(t.Context())
	downloadLibraries = func(ctx context.Context, architecture, osName, processor, version, dest string, progress getter.ProgressTracker) error {
		cancel()
		return errors.New("metadata request failed")
	}

	lib, err := New(
		WithBasePath(t.TempDir()),
		WithArch("arm64"),
		WithOS("darwin"),
		WithProcessor("metal"),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := lib.Download(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Errorf("Download() error = %v, want context.Canceled", err)
	}
}

func TestDownloadLatestVersionCancellationOverridesFallback(t *testing.T) {
	originalNetwork := networkAvailable
	originalLatest := latestVersion
	t.Cleanup(func() {
		networkAvailable = originalNetwork
		latestVersion = originalLatest
	})
	networkAvailable = func(context.Context) bool { return true }
	ctx, cancel := context.WithCancel(t.Context())
	latestVersion = func() (string, error) {
		cancel()
		return "", errors.New("latest version failed")
	}

	lib, err := New(
		WithBasePath(t.TempDir()),
		WithArch("arm64"),
		WithOS("darwin"),
		WithProcessor("metal"),
		WithAllowUpgrade(true),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := os.MkdirAll(lib.LibsPath(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeVersionFile(lib.LibsPath(), defaultVersion, lib.Arch(), lib.OS(), lib.Processor()); err != nil {
		t.Fatal(err)
	}

	if _, err := lib.Download(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Errorf("Download() error = %v, want context.Canceled", err)
	}
}
