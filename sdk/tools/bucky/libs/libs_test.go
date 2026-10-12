package libs

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
			var archive bytes.Buffer
			gz := gzip.NewWriter(&archive)
			tw := tar.NewWriter(gz)
			body := []byte("CUDA 13 fixture for " + arch)
			if err := tw.WriteHeader(&tar.Header{Name: "whisper-v1.9.5/libwhisper.so", Mode: 0o755, Size: int64(len(body))}); err != nil {
				t.Fatal(err)
			}
			if _, err := tw.Write(body); err != nil {
				t.Fatal(err)
			}
			if err := tw.Close(); err != nil {
				t.Fatal(err)
			}
			if err := gz.Close(); err != nil {
				t.Fatal(err)
			}
			suffix := "arm64"
			if arch == "amd64" {
				suffix = "x64"
			}
			filename := "whisper-v1.9.5-bin-ubuntu-cuda-13-" + suffix + ".tar.gz"
			assetURL := "https://github.com/ardanlabs/bucky-builder/releases/download/v1.9.5/" + filename
			asset := download.InstallAsset{
				SHA256: fmt.Sprintf("%x", sha256.Sum256(archive.Bytes())),
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
			pin := fmt.Sprintf("v1.9.5@sha256:%x", sha256.Sum256(manifest))
			corrupt := false
			transport := bundleTransport(func(r *http.Request) (*http.Response, error) {
				var data []byte
				switch strings.Split(r.URL.String(), "?")[0] {
				case "https://ardanlabs.github.io/bucky-builder/digests/v1.9.5.json":
					data = manifest
				case assetURL:
					data = archive.Bytes()
					if corrupt {
						data = []byte("corrupt archive")
					}
				default:
					return nil, fmt.Errorf("unexpected download URL: %s", r.URL)
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(data)), ContentLength: int64(len(data)), Request: r}, nil
			})
			oldTransport, oldGetters := http.DefaultTransport, getter.Getters
			http.DefaultTransport = transport
			getter.Getters = maps.Clone(oldGetters)
			getter.Getters["https"] = &getter.HttpGetter{Client: &http.Client{Transport: transport}}
			t.Cleanup(func() { http.DefaultTransport, getter.Getters = oldTransport, oldGetters })
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
			if err != nil || tag.Version != pin || tag.Processor != "cuda13" {
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
			if _, err := lib.DownloadFor(context.Background(), nil, arch, "linux", "cuda13", pin); err == nil {
				t.Fatal("corrupt archive accepted")
			}
			got, err := os.ReadFile(filepath.Join(lib.LibsPath(), "libwhisper.so"))
			if err != nil || !bytes.Equal(got, body) {
				t.Fatalf("failed download changed active library: %q error=%v", got, err)
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
