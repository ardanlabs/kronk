package libs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/ardanlabs/bucky/pkg/download"
	"github.com/hashicorp/go-getter"
)

func TestWithValidation(t *testing.T) {
	var options Options
	WithValidation(true)(&options)

	if !options.Validation {
		t.Error("Validation: got false, want true")
	}
}

func TestDownloadAcceptsNilLogger(t *testing.T) {
	root := t.TempDir()
	if err := writeVersionFile(root, defaultVersion, "arm64", "darwin", "metal"); err != nil {
		t.Fatalf("writeVersionFile: %v", err)
	}

	lib := Libs{path: root, readOnly: true}
	if _, err := lib.Download(context.Background(), nil); err != nil {
		t.Fatalf("Download: %v", err)
	}
}

func TestSwapInstallRestoresExistingInstallWhenActivationFails(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "cpu")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	existing := filepath.Join(path, "libwhisper.dylib")
	if err := os.WriteFile(existing, []byte("working"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := swapInstall(path, filepath.Join(root, "missing-stage")); err == nil {
		t.Fatal("swapInstall: got nil error, want activation failure")
	}

	data, err := os.ReadFile(existing)
	if err != nil {
		t.Fatalf("ReadFile existing install: %v", err)
	}
	if string(data) != "working" {
		t.Errorf("existing install: got %q, want working", data)
	}
}

func TestChooseVersion(t *testing.T) {
	tests := []struct {
		name, override, latest, want string
		upgrade                      bool
	}{
		{"default ignores newer discovery", "", "v9.0.0", defaultVersion, false},
		{"upgrade retains manifest pin", "", bareVersion(defaultVersion), defaultVersion, true},
		{"explicit override", "v1.9.4", "v9.0.0", "v1.9.4", true},
		{"explicit upgrade", "", "v9.0.0", "v9.0.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chooseVersion(tt.override, tt.upgrade, tt.latest, defaultVersion)
			if got != tt.want {
				t.Errorf("chooseVersion: got %q, want %q", got, tt.want)
			}
		})
	}
}

type bundleTransport func(*http.Request) (*http.Response, error)

func (f bundleTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDownloadCUDA13(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			scrubRuntimeEnv(t)
			body := []byte("CUDA 13 fixture for " + arch)
			suffix := "arm64"
			if arch == "amd64" {
				suffix = "x64"
			}
			filename := "whisper-v1.9.5-bin-ubuntu-cuda-13-" + suffix + ".tar.gz"
			assetURL := "https://github.com/ardanlabs/bucky-builder/releases/download/v1.9.5/" + filename
			asset := download.InstallAsset{
				SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("fixture archive"))),
				Files:  map[string]string{"libwhisper.so": fmt.Sprintf("%x", sha256.Sum256(body))},
			}
			manifest, err := json.Marshal(map[string]any{
				"version": 1, "tag": "v1.9.5",
				"sources": map[string]any{"ardanlabs/bucky-builder": map[string]any{
					"tag": "v1.9.5", "assets": map[string]any{filename: asset},
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			manifestSHA := fmt.Sprintf("%x", sha256.Sum256(manifest))
			pin := "v1.9.5@sha256:" + manifestSHA
			oldTransport, oldDownload := http.DefaultTransport, downloadLibraries
			t.Cleanup(func() { http.DefaultTransport, downloadLibraries = oldTransport, oldDownload })
			http.DefaultTransport = bundleTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != "https://ardanlabs.github.io/bucky-builder/digests/v1.9.5.json" {
					return nil, fmt.Errorf("unexpected download URL: %s", r.URL)
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(manifest)), ContentLength: int64(len(manifest)), Request: r}, nil
			})
			corrupt := false
			downloadLibraries = func(ctx context.Context, architecture, osName, processor, version, dest string, progress getter.ProgressTracker) error {
				if architecture != arch || osName != "linux" || processor != "cuda13" || version != pin {
					return fmt.Errorf("unexpected download: arch=%s os=%s processor=%s version=%s", architecture, osName, processor, version)
				}
				data := body
				if corrupt {
					data = []byte("partial download")
				}
				if err := os.WriteFile(filepath.Join(dest, "libwhisper.so"), data, 0o755); err != nil {
					return err
				}
				if corrupt {
					return download.ErrDigestMismatch
				}
				return download.WriteInstallRecord(dest, download.InstallRecord{
					Tag: "v1.9.5", Arch: architecture, OS: osName, Processor: processor,
					ManifestSHA256: manifestSHA,
					Asset:          download.InstallAsset{URL: assetURL, SHA256: asset.SHA256, Files: asset.Files},
				})
			}
			lib, err := New(WithBasePath(t.TempDir()), WithOS("linux"), WithArch(arch), WithProcessor("cuda13"))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(lib.LibsPath(), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := writeVersionFile(lib.LibsPath(), "v1.9.4", arch, "linux", "cuda13"); err != nil {
				t.Fatal(err)
			}
			tag, err := lib.DownloadFor(context.Background(), nil, arch, "linux", "cuda13", pin)
			if err != nil || tag.Version != pin || tag.Arch != arch || tag.OS != "linux" || tag.Processor != "cuda13" {
				t.Fatalf("download: tag=%+v error=%v", tag, err)
			}
			record, err := download.ReadInstallRecord(lib.LibsPath())
			if err != nil || record.Asset.URL != assetURL {
				t.Fatalf("record: %+v error=%v", record, err)
			}
			report, err := lib.Verify(context.Background(), pin)
			if err != nil || !report.OK() || !report.ManifestAuthenticated {
				t.Fatalf("verification: %+v error=%v", report, err)
			}
			corrupt = true
			if _, err := lib.DownloadFor(context.Background(), nil, arch, "linux", "cuda13", pin); !errors.Is(err, download.ErrDigestMismatch) {
				t.Fatalf("download: got %v, want digest mismatch", err)
			}
			got, err := os.ReadFile(filepath.Join(lib.LibsPath(), "libwhisper.so"))
			if err != nil || !bytes.Equal(got, body) {
				t.Fatalf("failed download changed active library: %q error=%v", got, err)
			}
			tag, err = lib.InstalledVersion()
			if err != nil || tag.Version != pin {
				t.Fatalf("failed download changed active version: tag=%+v error=%v", tag, err)
			}
		})
	}
}

func TestOfflineDownloadRequiresCompatiblePin(t *testing.T) {
	t.Setenv("KRONK_SKIP_NETWORK_CHECK", "")
	oldTransport := http.DefaultTransport
	http.DefaultTransport = bundleTransport(func(*http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("offline fixture")
	})
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
	for _, version := range []string{"v1.9.4", "v9.0.0", defaultVersion} {
		lib := Libs{path: t.TempDir(), arch: "amd64", os: "linux", processor: "cuda13"}
		if err := writeVersionFile(lib.path, version, lib.arch, lib.os, lib.processor); err != nil {
			t.Fatal(err)
		}
		_, err := lib.Download(context.Background(), nil)
		if (err == nil) != (version == defaultVersion) {
			t.Fatalf("version=%s error=%v", version, err)
		}
	}
}
