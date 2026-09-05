package libs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	version "github.com/hashicorp/go-version"
	"github.com/hybridgroup/yzma/pkg/download"
)

func TestWithValidation(t *testing.T) {
	var options Options
	WithValidation(true)(&options)

	if !options.Validation {
		t.Error("Validation: got false, want true")
	}
}

func TestAllowUnavailableFileValidation(t *testing.T) {
	var logged bool
	log := func(_ context.Context, _ string, _ ...any) {
		logged = true
	}

	err := fmt.Errorf("verify: %w", ErrNoFileDigests)
	if err := allowUnavailableFileValidation(t.Context(), log, "b10809", err); err != nil {
		t.Fatalf("allowUnavailableFileValidation: %v", err)
	}
	if !logged {
		t.Error("log: got no warning, want warning")
	}

	want := errors.New("changed file")
	if got := allowUnavailableFileValidation(t.Context(), log, "b10809", want); !errors.Is(got, want) {
		t.Errorf("other error: got %v, want %v", got, want)
	}
}

func TestVerificationVersion(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		override string
		want     string
	}{
		{"default tag uses pin", versionTag(defaultVersion), "", defaultVersion},
		{"other tag stays unchanged", "v0.4.1", "", "v0.4.1"},
		{"override wins", versionTag(defaultVersion), "v0.4.0", "v0.4.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lib := Libs{version: tt.override}
			if got := lib.verificationVersion(tt.version); got != tt.want {
				t.Errorf("verificationVersion(%q) = %q, want %q", tt.version, got, tt.want)
			}
		})
	}
}

// =============================================================================
// Pure-function tests for the Download policy matrix.
//
// chooseVersion encodes the entire decision tree documented on Download.
// These tests cover every row of the matrix plus boundary cases without
// touching disk or the network.

func TestChooseVersion(t *testing.T) {
	const def = "b8937"

	tests := []struct {
		name         string
		override     string
		allowUpgrade bool
		installed    string
		latest       string
		want         string
	}{
		// Row 1: override wins regardless of any other input.
		{"row1: override, no install", "b9999", false, "", "", "b9999"},
		{"row1: override, older install", "b9999", false, "b1000", "", "b9999"},
		{"row1: override beats allowUpgrade+latest", "b9999", true, "b1000", "b10000", "b9999"},
		{"row1: override equal to install", def, false, def, "", def},

		// Row 2: AllowUpgrade returns the published latest.
		{"row2: upgrade, nothing installed", "", true, "", "b10000", "b10000"},
		{"row2: upgrade, older install", "", true, "b8000", "b10000", "b10000"},
		{"row2: upgrade, newer install (still tracks latest)", "", true, "b11000", "b10000", "b10000"},

		// Row 3: nothing installed, no upgrade, no override -> default.
		{"row3: empty install", "", false, "", "", def},

		// Row 4: install <= default -> default.
		{"row4: older install", "", false, "b1000", "", def},
		{"row4: install equal to default", "", false, def, "", def},

		// Row 5: install > default -> keep installed.
		{"row5: newer install", "", false, "b9999", "", "b9999"},
		{"row5: much newer install", "", false, "b20000", "", "b20000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chooseVersion(tt.override, tt.allowUpgrade, tt.installed, tt.latest, def)
			if got != tt.want {
				t.Errorf("chooseVersion(%q, %v, %q, %q, %q) = %q, want %q",
					tt.override, tt.allowUpgrade, tt.installed, tt.latest, def, got, tt.want)
			}
		})
	}
}

func TestVersionGreater(t *testing.T) {
	tests := []struct {
		v1, v2 string
		want   bool
	}{
		{"b8937", "b8000", true},
		{"b8000", "b8937", false},
		{"b8937", "b8937", false},
		{"", "b1000", false},
		{"b1000", "", false},
		{"b20000", "b8937", true},
		{"v0.4.1", "v0.4.0", true},
		{"v0.4.0", "v0.4.1", false},
		{"v0.4.0-rc.2", "v0.4.0-rc.1", true},
		{"v0.4.0", "v0.4.0-rc.2", true},
		{"v0.4.1", "v0.4.0@sha256:" + strings.Repeat("a", 64), true},
		{"v0.4.0@sha256:" + strings.Repeat("a", 64), "v0.4.1", false},
		{"v0.4.0", "b10675", false},
		{"b10675", "v0.4.0", false},
		{"invalid", "v0.4.0", false},
	}

	for _, tt := range tests {
		got := versionGreater(tt.v1, tt.v2)
		if got != tt.want {
			t.Errorf("versionGreater(%q, %q) = %v, want %v", tt.v1, tt.v2, got, tt.want)
		}
	}
}

// =============================================================================
// Wiring tests for Download.
//
// These exercise the policy decisions through the full Download entry point
// without performing any real downloads. They are kept fast and deterministic
// by:
//
//   - Using t.TempDir() as the libraries root so nothing touches ~/.kronk/.
//   - Scrubbing every KRONK_* env var that influences resolution.
//   - Pinning the (arch, os, processor) triple explicitly.
//   - Setting testMode so hasNetwork() and VersionInformation() are bypassed.
//   - Pre-populating version.json so that the chosen version matches the
//     installed version, which makes Download return before calling out to
//     download libraries.

const (
	testArch = "arm64"
	testOS   = "darwin"
	testProc = "metal"
)

func scrubKronkEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"KRONK_LIB_PATH",
		"KRONK_LIB_VERSION",
		"KRONK_ARCH",
		"KRONK_OS",
		"KRONK_PROCESSOR",
	} {
		t.Setenv(k, "")
	}
}

func mustParseTriple(t *testing.T) (download.Arch, download.OS, download.Processor) {
	t.Helper()
	a, err := download.ParseArch(testArch)
	if err != nil {
		t.Fatalf("parse arch: %v", err)
	}
	o, err := download.ParseOS(testOS)
	if err != nil {
		t.Fatalf("parse os: %v", err)
	}
	p, err := download.ParseProcessor(testProc)
	if err != nil {
		t.Fatalf("parse processor: %v", err)
	}
	return a, o, p
}

// writeInstalled creates the per-triple install directory under root and
// writes a version.json describing the supplied version. This simulates an
// existing install.
func writeInstalled(t *testing.T, root string, version string) string {
	t.Helper()
	path := filepath.Join(root, localFolder, testOS, testArch, testProc)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir install path: %v", err)
	}

	tag := VersionTag{
		Version:   version,
		Arch:      testArch,
		OS:        testOS,
		Processor: testProc,
	}
	data, err := json.Marshal(tag)
	if err != nil {
		t.Fatalf("marshal version tag: %v", err)
	}
	if err := os.WriteFile(filepath.Join(path, versionFile), data, 0o644); err != nil {
		t.Fatalf("write version.json: %v", err)
	}
	return path
}

func newTestLibs(t *testing.T, baseDir string, opts ...Option) *Libs {
	t.Helper()
	a, o, p := mustParseTriple(t)
	all := []Option{
		WithBasePath(baseDir),
		WithArch(a),
		WithOS(o),
		WithProcessor(p),
	}
	all = append(all, opts...)
	lib, err := New(all...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	lib.testMode = true
	return lib
}

func noopLog(_ context.Context, _ string, _ ...any) {}

func versionAfterDefault(t *testing.T) string {
	t.Helper()

	tag := versionTag(defaultVersion)
	if build, ok := strings.CutPrefix(tag, "b"); ok {
		n, err := strconv.Atoi(build)
		if err != nil {
			t.Fatalf("parse default build %q: %v", tag, err)
		}
		return fmt.Sprintf("b%d", n+1)
	}

	semantic, err := version.NewVersion(tag)
	if err != nil || !strings.HasPrefix(tag, "v") {
		t.Fatalf("default version %q uses an unsupported version scheme", tag)
	}

	return fmt.Sprintf("v%d.0.0", semantic.Segments()[0]+1)
}

// TestDownload_AlreadyInstalled_Default covers matrix rows 3 and 4 in the
// "already installed" form: default version is on disk, AllowUpgrade is
// false, no override -> Download returns without attempting a download.
func TestDownload_AlreadyInstalled_Default(t *testing.T) {
	scrubKronkEnv(t)
	tmp := t.TempDir()
	want := versionTag(defaultVersion)
	writeInstalled(t, tmp, want)

	lib := newTestLibs(t, tmp)

	tag, err := lib.Download(context.Background(), noopLog)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if tag.Version != want {
		t.Errorf("Version = %q, want %q", tag.Version, want)
	}
}

// TestDownload_AlreadyInstalled_NewerKept covers matrix row 5: an installed
// version greater than the default is kept (no downgrade) when AllowUpgrade
// is false and no override is set.
func TestDownload_AlreadyInstalled_NewerKept(t *testing.T) {
	scrubKronkEnv(t)
	tmp := t.TempDir()
	installed := versionAfterDefault(t)
	writeInstalled(t, tmp, installed)

	lib := newTestLibs(t, tmp)

	tag, err := lib.Download(context.Background(), noopLog)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if tag.Version != installed {
		t.Errorf("Version = %q, want %q (never downgrade)", tag.Version, installed)
	}
}

// TestDownload_OverrideMatchesInstalled covers matrix row 1 in the
// short-circuit form: override is set, the same version is on disk, and
// Download returns without attempting a download regardless of
// AllowUpgrade.
func TestDownload_OverrideMatchesInstalled(t *testing.T) {
	scrubKronkEnv(t)
	tmp := t.TempDir()
	const pinned = "b12345"
	writeInstalled(t, tmp, pinned)

	lib := newTestLibs(t, tmp, WithVersion(pinned), WithAllowUpgrade(true))
	// AllowUpgrade is set true to prove the override truly wins; the latest
	// is irrelevant when an override is present.
	lib.testLatest = "b99999"

	tag, err := lib.Download(context.Background(), noopLog)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if tag.Version != pinned {
		t.Errorf("Version = %q, want %q", tag.Version, pinned)
	}
}

// TestDownload_AllowUpgrade_AlreadyAtLatest covers matrix row 2 in the
// short-circuit form: testLatest matches what is on disk, so Download
// returns without attempting a download.
func TestDownload_AllowUpgrade_AlreadyAtLatest(t *testing.T) {
	scrubKronkEnv(t)
	tmp := t.TempDir()
	const installed = "b15000"
	writeInstalled(t, tmp, installed)

	lib := newTestLibs(t, tmp, WithAllowUpgrade(true))
	lib.testLatest = installed

	tag, err := lib.Download(context.Background(), noopLog)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if tag.Version != installed {
		t.Errorf("Version = %q, want %q", tag.Version, installed)
	}
}

func TestDownload_InstallFailure(t *testing.T) {
	scrubKronkEnv(t)
	tmp := t.TempDir()
	want := versionTag(defaultVersion)
	writeInstalled(t, tmp, want)

	lib := newTestLibs(t, tmp, WithVersion("invalid"))

	if _, err := lib.Download(context.Background(), noopLog); !errors.Is(err, download.ErrInvalidVersion) {
		t.Errorf("Download error = %v, want wrapping ErrInvalidVersion", err)
	}

	tag, err := lib.InstalledVersion()
	if err != nil {
		t.Fatalf("InstalledVersion: %v", err)
	}
	if tag.Version != want {
		t.Errorf("Version = %q, want preserved %q", tag.Version, want)
	}
}

// TestDownload_ReadOnly verifies that a user-supplied directory containing
// other files but no version.json is treated as read-only and Download
// reports ErrReadOnly without mutating anything.
func TestDownload_ReadOnly(t *testing.T) {
	scrubKronkEnv(t)
	userDir := t.TempDir()

	// Drop a fake library file so resolvePaths classifies the directory as
	// a non-empty user-managed install (and not as an empty libraries root).
	if err := os.WriteFile(filepath.Join(userDir, "libllama.so"), []byte("fake"), 0o644); err != nil {
		t.Fatalf("seed user dir: %v", err)
	}

	a, o, p := mustParseTriple(t)
	lib, err := New(
		WithLibPath(userDir),
		WithArch(a), WithOS(o), WithProcessor(p),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !lib.ReadOnly() {
		t.Fatalf("expected ReadOnly() to be true")
	}
	lib.testMode = true

	if _, err := lib.Download(context.Background(), noopLog); !errors.Is(err, ErrReadOnly) {
		t.Errorf("Download error = %v, want wrapping ErrReadOnly", err)
	}
}
