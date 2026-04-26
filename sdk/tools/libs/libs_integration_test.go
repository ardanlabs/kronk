//go:build integration

// Integration tests for the Download workflow.
//
// These tests perform real network calls and real downloads of llama.cpp
// release bundles. They are gated behind the "integration" build tag so the
// default `go test ./...` run stays fast and offline-friendly.
//
// Run them explicitly with:
//
//	go test -tags=integration ./sdk/tools/libs/...
//
// Every test uses t.TempDir() as the libraries root so nothing is ever
// written to ~/.kronk/.

package libs

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/hybridgroup/yzma/pkg/download"
)

// pickIntegrationTriple returns a (arch, os, processor) triple that is part
// of the upstream build matrix and matches the host architecture/OS where
// possible. The CPU processor is preferred because its bundle is the
// smallest and works on the widest range of hardware.
func pickIntegrationTriple(t *testing.T) (download.Arch, download.OS, download.Processor) {
	t.Helper()

	candidates := []Combination{
		{Arch: runtime.GOARCH, OS: runtime.GOOS, Processor: "cpu"},
	}

	// Apple Silicon's CPU build is published; Metal is available too. CPU
	// is preferred for the smallest possible download.
	for _, c := range candidates {
		if !IsSupported(c.Arch, c.OS, c.Processor) {
			continue
		}
		a, err := download.ParseArch(c.Arch)
		if err != nil {
			continue
		}
		o, err := download.ParseOS(c.OS)
		if err != nil {
			continue
		}
		p, err := download.ParseProcessor(c.Processor)
		if err != nil {
			continue
		}
		return a, o, p
	}

	t.Skipf("no supported integration triple for %s/%s", runtime.GOOS, runtime.GOARCH)
	return download.Arch{}, download.OS{}, download.Processor{}
}

func integrationLibs(t *testing.T, opts ...Option) (*Libs, string) {
	t.Helper()

	for _, k := range []string{
		"KRONK_LIB_PATH", "KRONK_LIB_VERSION",
		"KRONK_ARCH", "KRONK_OS", "KRONK_PROCESSOR",
	} {
		t.Setenv(k, "")
	}

	tmp := t.TempDir()
	a, o, p := pickIntegrationTriple(t)

	all := []Option{
		WithBasePath(tmp),
		WithArch(a), WithOS(o), WithProcessor(p),
	}
	all = append(all, opts...)

	lib, err := New(all...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return lib, tmp
}

func intLog(_ context.Context, _ string, _ ...any) {}

// TestIntegration_Download_Default exercises matrix row 3: empty libraries
// root, no override, no upgrade -> defaultVersion is downloaded and
// installed. Asserts the install lands under t.TempDir() and the
// version.json on disk reports the default version.
func TestIntegration_Download_Default(t *testing.T) {
	lib, tmp := integrationLibs(t)

	tag, err := lib.Download(context.Background(), intLog)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}

	if tag.Version != defaultVersion {
		t.Errorf("installed Version = %q, want default %q", tag.Version, defaultVersion)
	}

	// Confirm files actually landed under the temp dir, not somewhere else.
	if !filepath.HasPrefix(lib.LibsPath(), tmp) {
		t.Errorf("LibsPath %q is outside temp root %q", lib.LibsPath(), tmp)
	}
	if _, err := os.Stat(filepath.Join(lib.LibsPath(), versionFile)); err != nil {
		t.Errorf("version.json not written: %v", err)
	}
}

// TestIntegration_Download_AllowUpgrade exercises matrix row 2: empty
// libraries root, AllowUpgrade=true -> the latest published version is
// queried from llama.cpp and downloaded. We can only assert that the
// installed version is at least the well-known default (since the actual
// "latest" floats over time).
func TestIntegration_Download_AllowUpgrade(t *testing.T) {
	lib, _ := integrationLibs(t, WithAllowUpgrade(true))

	tag, err := lib.Download(context.Background(), intLog)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}

	if tag.Version == "" {
		t.Fatalf("installed Version is empty after Download")
	}
	if tag.Version != defaultVersion && !versionGreater(tag.Version, defaultVersion) {
		t.Errorf("installed Version = %q, want >= default %q", tag.Version, defaultVersion)
	}
}
