package models

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/applog"
	"github.com/ardanlabs/kronk/sdk/tools/backend"
	malinadownload "github.com/ardanlabs/malina/pkg/download"
	getter "github.com/hashicorp/go-getter"
)

func TestCatalogValidity(t *testing.T) {
	for _, bundle := range Catalog() {
		if err := bundle.Validate(); err != nil {
			t.Errorf("bundle %q: %v", bundle.Name, err)
		}
	}
}

func TestCatalogIncludesMalinaCatalog(t *testing.T) {
	for _, malinaBundle := range malinadownload.Catalog() {
		name, err := ParseBundleName(malinaBundle.Name)
		if err != nil {
			t.Fatalf("ParseBundleName(%q): %v", malinaBundle.Name, err)
		}
		kronkBundle, ok := BundleByName(name)
		if !ok {
			t.Fatalf("BundleByName(%q): not found", name)
		}

		kronkJSON, err := json.Marshal(kronkBundle)
		if err != nil {
			t.Fatalf("Marshal Kronk bundle %q: %v", name, err)
		}
		malinaJSON, err := json.Marshal(malinaBundle)
		if err != nil {
			t.Fatalf("Marshal Malina bundle %q: %v", name, err)
		}
		if !bytes.Equal(kronkJSON, malinaJSON) {
			t.Errorf("Kronk bundle %q does not match the Malina catalog", name)
		}
	}
}

func TestBundleNameConstants(t *testing.T) {
	want := []BundleName{BundleFlux2Klein4B, BundleFlux2Klein9B, BundleSD15, BundleSDXLBase10}
	if !slices.Equal(SupportedBundles(), want) {
		t.Errorf("SupportedBundles(): got %v, want %v", SupportedBundles(), want)
	}
}

func TestParseBundleName(t *testing.T) {
	name, err := ParseBundleName("sd-1.5")
	if err != nil {
		t.Fatalf("ParseBundleName() error = %v", err)
	}
	if !name.Equal(BundleSD15) {
		t.Errorf("ParseBundleName(): got %q, want %q", name, BundleSD15)
	}
	if _, err := ParseBundleName("custom"); err == nil {
		t.Fatal("ParseBundleName() error = nil, want unknown bundle error")
	}
}

func TestBundleNameTextRoundTrip(t *testing.T) {
	data, err := BundleFlux2Klein4B.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText() error = %v", err)
	}
	var name BundleName
	if err := name.UnmarshalText(data); err != nil {
		t.Fatalf("UnmarshalText() error = %v", err)
	}
	if !name.Equal(BundleFlux2Klein4B) {
		t.Errorf("UnmarshalText(): got %q, want %q", name, BundleFlux2Klein4B)
	}
}

func TestBundleFlux2Klein4B(t *testing.T) {
	bundle, ok := BundleByName(BundleFlux2Klein4B)
	if !ok {
		t.Fatal("BundleByName(BundleFlux2Klein4B): not found")
	}
	if !bundle.Gated {
		t.Error("Gated: got false, want true")
	}

	want := []BundleFile{
		{Role: RoleDiffusion, Filename: "flux-2-klein-4b-Q4_0.gguf", URL: "https://huggingface.co/leejet/FLUX.2-klein-4B-GGUF/resolve/main/flux-2-klein-4b-Q4_0.gguf", Size: "2.5 GB"},
		{Role: RoleVAE, Filename: "ae.safetensors", URL: "https://huggingface.co/black-forest-labs/FLUX.2-dev/resolve/main/ae.safetensors", Size: "335 MB"},
		{Role: RoleLLM, Filename: "Qwen3-4B-Q4_K_M.gguf", URL: "https://huggingface.co/unsloth/Qwen3-4B-GGUF/resolve/main/Qwen3-4B-Q4_K_M.gguf", Size: "2.5 GB"},
	}
	if !slices.Equal(bundle.Files, want) {
		t.Errorf("Files: got %v, want %v", bundle.Files, want)
	}
}

func TestDownloadFileAuthorization(t *testing.T) {
	tests := []struct {
		name       string
		kronkToken string
		hfToken    string
		want       string
	}{
		{name: "kronk token takes precedence", kronkToken: "kronk", hfToken: "hf", want: "Bearer kronk"},
		{name: "HF token fallback", hfToken: "hf", want: "Bearer hf"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("KRONK_HF_TOKEN", tt.kronkToken)
			t.Setenv("HF_TOKEN", tt.hfToken)
			var authorization string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				authorization = r.Header.Get("Authorization")
				_, _ = w.Write([]byte("model"))
			}))
			defer server.Close()

			target := filepath.Join(t.TempDir(), "model.gguf")
			if err := downloadFile(t.Context(), server.URL+"/model.gguf", target, nil); err != nil {
				t.Fatalf("downloadFile() error = %v", err)
			}
			if authorization != tt.want {
				t.Errorf("Authorization: got %q, want %q", authorization, tt.want)
			}
		})
	}
}

func TestDownloadFileCancellation(t *testing.T) {
	requestStarted := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		result <- downloadFile(ctx, server.URL+"/model.gguf", filepath.Join(t.TempDir(), "model.gguf"), nil)
	}()
	<-requestStarted
	cancel()

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("downloadFile() error = nil, want cancellation")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("downloadFile() did not return after cancellation")
	}
}

func TestDownloadBundleWritesManifestAfterCompletion(t *testing.T) {
	m, err := NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatalf("NewWithPaths() error = %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "missing") {
			http.Error(w, "missing", http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte("model"))
	}))
	defer server.Close()

	bundle := Bundle{
		Name:    BundleName{value: "test-bundle"},
		License: "test",
		Files: []BundleFile{
			{Role: RoleModel, Filename: "model.gguf", URL: server.URL + "/model"},
			{Role: RoleVAE, Filename: "vae.safetensors", URL: server.URL + "/missing"},
		},
	}
	if _, err := m.downloadBundle(t.Context(), applog.DiscardLogger, bundle, nil); err == nil {
		t.Fatal("downloadBundle() error = nil, want download failure")
	}
	manifest := filepath.Join(m.Path(), bundle.Name.String(), ManifestFilename)
	if _, err := os.Stat(manifest); !os.IsNotExist(err) {
		t.Fatalf("manifest stat error = %v, want not exist", err)
	}
}

func TestDownloadBundleRejectsEmptyStagedFile(t *testing.T) {
	originalDownload := downloadModelFile
	t.Cleanup(func() { downloadModelFile = originalDownload })
	downloadModelFile = func(ctx context.Context, source string, target string, progress getter.ProgressTracker) error {
		return os.WriteFile(target, nil, 0o644)
	}

	m, err := NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatalf("NewWithPaths() error = %v", err)
	}
	bundle := Bundle{Name: BundleName{value: "test-bundle"}, Files: []BundleFile{{Role: RoleModel, Filename: "model.gguf", URL: "unused"}}}
	dir := filepath.Join(m.Path(), bundle.Name.String())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(dir, "existing")
	if err := os.WriteFile(existing, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := m.downloadBundle(t.Context(), applog.DiscardLogger, bundle, nil); err == nil {
		t.Fatal("downloadBundle() error = nil, want empty staged file error")
	}
	data, err := os.ReadFile(existing)
	if err != nil {
		t.Fatalf("existing bundle was removed: %v", err)
	}
	if string(data) != "keep" {
		t.Errorf("existing bundle: got %q, want keep", data)
	}
}

func TestGatedDownloadError(t *testing.T) {
	bundle := Bundle{Name: BundleName{value: "gated"}, Gated: true}
	file := BundleFile{Filename: "model.gguf"}
	err := bundleDownloadError(bundle, file, errors.New("request failed: 403 Forbidden"))
	if !strings.Contains(err.Error(), "KRONK_HF_TOKEN") || !strings.Contains(err.Error(), "accept the Hugging Face license") {
		t.Errorf("bundleDownloadError() = %q, want license and token guidance", err)
	}
}

func TestLoadManifestRoundTrip(t *testing.T) {
	m, err := NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatalf("NewWithPaths() error = %v", err)
	}
	bundle, _ := BundleByName(BundleSD15)
	dir := filepath.Join(m.Path(), bundle.Name.String())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	want := Manifest{Bundle: bundle.Name, License: bundle.License, Files: map[string]string{string(RoleModel): filepath.Join(dir, bundle.Files[0].Filename)}}
	if err := os.WriteFile(want.Files[string(RoleModel)], []byte("model"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestFilename), data, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := m.LoadManifest(BundleSD15)
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	if got.Bundle != want.Bundle || got.Files[string(RoleModel)] != want.Files[string(RoleModel)] {
		t.Errorf("LoadManifest() = %+v, want %+v", got, want)
	}
}

func TestIndexFullPathRemoveLifecycle(t *testing.T) {
	m, err := NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatalf("NewWithPaths() error = %v", err)
	}
	bundle, _ := BundleByName(BundleSD15)
	dir := filepath.Join(m.Path(), bundle.Name.String())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{Bundle: bundle.Name, Files: map[string]string{}}
	for _, file := range bundle.Files {
		path := filepath.Join(dir, file.Filename)
		if err := os.WriteFile(path, []byte("model"), 0o644); err != nil {
			t.Fatal(err)
		}
		manifest.Files[string(file.Role)] = path
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestFilename), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.BuildIndex(nil, false); err != nil {
		t.Fatalf("BuildIndex() error = %v", err)
	}
	mp, err := m.FullPath(bundle.Name.String())
	if err != nil {
		t.Fatalf("FullPath() error = %v", err)
	}
	originalDownload := downloadModelFile
	t.Cleanup(func() { downloadModelFile = originalDownload })
	called := false
	downloadModelFile = func(ctx context.Context, source string, target string, progress getter.ProgressTracker) error {
		called = true
		return errors.New("unexpected download")
	}
	if _, err := m.DownloadBundleWithProgress(t.Context(), bundle.Name, nil); err != nil {
		t.Fatalf("DownloadBundleWithProgress() reuse error = %v", err)
	}
	if called {
		t.Fatal("DownloadBundleWithProgress() downloaded an already valid bundle")
	}
	if err := m.Remove(mp, nil); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if _, err := m.FullPath(bundle.Name.String()); !errors.Is(err, ErrModelNotFound) {
		t.Errorf("FullPath() error = %v, want ErrModelNotFound", err)
	}
}

func TestRemoveRejectsOutsidePath(t *testing.T) {
	base := t.TempDir()
	m, err := NewWithPaths(base)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(base, "outside", "file")
	if err := os.MkdirAll(filepath.Dir(outside), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove(backend.ModelPath{ModelFiles: []string{outside}}, nil); err == nil {
		t.Fatal("Remove() error = nil, want containment error")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside file removed: %v", err)
	}
}

func TestBuildIndexRejectsInvalidManifest(t *testing.T) {
	tests := []struct {
		name       string
		manifestID BundleName
		modelPath  func(bundleDir string, expected string) string
		writeModel bool
	}{
		{
			name:       "mismatched bundle",
			manifestID: BundleSDXLBase10,
			modelPath:  func(_ string, expected string) string { return expected },
			writeModel: true,
		},
		{
			name:       "outside path",
			manifestID: BundleSD15,
			modelPath: func(bundleDir string, _ string) string {
				return filepath.Join(filepath.Dir(bundleDir), "outside.safetensors")
			},
			writeModel: true,
		},
		{
			name:       "missing model",
			manifestID: BundleSD15,
			modelPath:  func(_ string, expected string) string { return expected },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := NewWithPaths(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			bundle, _ := BundleByName(BundleSD15)
			dir := filepath.Join(m.Path(), bundle.Name.String())
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			expected := filepath.Join(dir, bundle.Files[0].Filename)
			path := tt.modelPath(dir, expected)
			if tt.writeModel {
				if err := os.WriteFile(path, []byte("model"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			manifest := Manifest{Bundle: tt.manifestID, Files: map[string]string{string(RoleModel): path}}
			data, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, ManifestFilename), data, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := m.BuildIndex(nil, false); err != nil {
				t.Fatalf("BuildIndex() error = %v", err)
			}
			if _, err := m.FullPath(bundle.Name.String()); !errors.Is(err, ErrModelNotFound) {
				t.Errorf("FullPath() error = %v, want ErrModelNotFound", err)
			}
		})
	}
}

func TestRemoveRejectsFabricatedKnownBundlePath(t *testing.T) {
	m, err := NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bundle, _ := BundleByName(BundleSD15)
	dir := filepath.Join(m.Path(), bundle.Name.String())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(keep, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	malicious := backend.ModelPath{ModelFiles: []string{filepath.Join(dir, "not-an-installed-model")}}
	if err := m.Remove(malicious, nil); err == nil {
		t.Fatal("Remove() error = nil, want installed-bundle validation error")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("known bundle directory was removed: %v", err)
	}
}
