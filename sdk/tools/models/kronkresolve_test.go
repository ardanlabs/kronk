package models

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"go.yaml.in/yaml/v2"
)

func TestResolveAdapters(t *testing.T) {
	basePath := t.TempDir()
	m := Models{basePath: basePath}

	idPath := filepath.Join(basePath, "lora", "acme", "support.gguf")
	if err := os.MkdirAll(filepath.Dir(idPath), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(idPath, []byte("adapter"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	plainIDPath := filepath.Join(basePath, "lora", "support.gguf")
	if err := os.WriteFile(plainIDPath, []byte("adapter"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	externalPath := filepath.Join(t.TempDir(), "external.gguf")
	if err := os.WriteFile(externalPath, []byte("adapter"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	directoryPath := filepath.Join(basePath, "directory.gguf")
	if err := os.Mkdir(directoryPath, 0755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	zero := float32(0)
	tests := []struct {
		name      string
		adapters  []AdapterConfig
		wantPath  string
		wantScale float32
		wantErr   string
	}{
		{
			name:      "id resolves under lora folder with default scale",
			adapters:  []AdapterConfig{{ID: "acme/support"}},
			wantPath:  idPath,
			wantScale: 1,
		},
		{
			name:      "id without organization resolves under lora folder",
			adapters:  []AdapterConfig{{ID: "support"}},
			wantPath:  plainIDPath,
			wantScale: 1,
		},
		{
			name:      "absolute path preserves explicit zero scale",
			adapters:  []AdapterConfig{{Path: externalPath, PtrScale: &zero}},
			wantPath:  externalPath,
			wantScale: 0,
		},
		{name: "id and path are mutually exclusive", adapters: []AdapterConfig{{ID: "acme/support", Path: externalPath}}, wantErr: "exactly one"},
		{name: "id is required when path is absent", adapters: []AdapterConfig{{}}, wantErr: "exactly one"},
		{name: "id rejects traversal", adapters: []AdapterConfig{{ID: "../support"}}, wantErr: "invalid path component"},
		{name: "id rejects extension", adapters: []AdapterConfig{{ID: "acme/support.gguf"}}, wantErr: "omit the .gguf extension"},
		{name: "path must be absolute", adapters: []AdapterConfig{{Path: "support.gguf"}}, wantErr: "must be absolute"},
		{name: "adapter must exist", adapters: []AdapterConfig{{Path: filepath.Join(basePath, "missing.gguf")}}, wantErr: "no such file"},
		{name: "adapter must be a regular file", adapters: []AdapterConfig{{Path: directoryPath}}, wantErr: "not a regular file"},
		{name: "scale must be finite", adapters: []AdapterConfig{{Path: externalPath, PtrScale: new(float32(math.NaN()))}}, wantErr: "must be finite"},
		{name: "duplicate paths are rejected", adapters: []AdapterConfig{{Path: externalPath}, {Path: externalPath}}, wantErr: "duplicate adapter path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := m.resolveAdapters(tt.adapters)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("resolveAdapters() error = %v, want error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveAdapters() error = %v", err)
			}
			if len(got) != 1 {
				t.Fatalf("resolveAdapters() returned %d adapters, want 1", len(got))
			}
			if got[0].Path != tt.wantPath {
				t.Errorf("Path = %q, want %q", got[0].Path, tt.wantPath)
			}
			if got[0].Scale != tt.wantScale {
				t.Errorf("Scale = %g, want %g", got[0].Scale, tt.wantScale)
			}
		})
	}
}

func TestAdapterConfigYAML(t *testing.T) {
	data := []byte(`test-model:
  adapters:
    - id: acme/support
    - path: /opt/adapters/legal.gguf
      scale: 0
`)

	var configs map[string]ModelConfig
	if err := yaml.Unmarshal(data, &configs); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	adapters := configs["test-model"].Adapters
	if len(adapters) != 2 {
		t.Fatalf("Adapters length = %d, want 2", len(adapters))
	}
	if adapters[0].ID != "acme/support" {
		t.Errorf("Adapters[0].ID = %q, want %q", adapters[0].ID, "acme/support")
	}
	if adapters[0].PtrScale != nil {
		t.Errorf("Adapters[0].PtrScale = %v, want nil", adapters[0].PtrScale)
	}
	if adapters[1].Path != "/opt/adapters/legal.gguf" {
		t.Errorf("Adapters[1].Path = %q, want %q", adapters[1].Path, "/opt/adapters/legal.gguf")
	}
	if adapters[1].PtrScale == nil || *adapters[1].PtrScale != 0 {
		t.Errorf("Adapters[1].PtrScale = %v, want pointer to 0", adapters[1].PtrScale)
	}
}

func TestMergeModelConfigAdapters(t *testing.T) {
	dst := ModelConfig{Adapters: []AdapterConfig{{ID: "old"}}}
	src := ModelConfig{Adapters: []AdapterConfig{{ID: "new"}}}

	MergeModelConfig(&dst, src)

	if len(dst.Adapters) != 1 || dst.Adapters[0].ID != "new" {
		t.Errorf("Adapters = %+v, want replacement adapter", dst.Adapters)
	}
}

func TestModelConfigLoadMode(t *testing.T) {
	data := []byte(`test-model:
  load-mode: mlock
`)

	var configs map[string]ModelConfig
	if err := yaml.Unmarshal(data, &configs); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	cfg := configs["test-model"]
	if cfg.PtrLoadMode == nil || *cfg.PtrLoadMode != model.LoadModeMLock {
		t.Fatalf("PtrLoadMode = %v, want pointer to mlock", cfg.PtrLoadMode)
	}
	if got := cfg.ToKronkConfig().LoadMode; got != model.LoadModeMLock {
		t.Errorf("ToKronkConfig().LoadMode = %v, want %v", got, model.LoadModeMLock)
	}

	mmap := model.LoadModeMMap
	dst := ModelConfig{PtrLoadMode: new(model.LoadModeDirectIO)}
	MergeModelConfig(&dst, ModelConfig{PtrLoadMode: &mmap})
	if dst.PtrLoadMode == nil || *dst.PtrLoadMode != model.LoadModeMMap {
		t.Errorf("merged PtrLoadMode = %v, want pointer to mmap", dst.PtrLoadMode)
	}
}

func TestModelConfigAdmissionAndQueue(t *testing.T) {
	data := []byte(`test-model:
  admission-timeout: 3m
  queue-depth: 2
`)

	var configs map[string]ModelConfig
	if err := yaml.Unmarshal(data, &configs); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	cfg := configs["test-model"]
	kronkCfg := cfg.ToKronkConfig()
	if got := kronkCfg.AdmissionTimeout(); got != 3*time.Minute {
		t.Errorf("AdmissionTimeout() = %s, want %s", got, 3*time.Minute)
	}
	if got := kronkCfg.QueueDepth(); got != 2 {
		t.Errorf("QueueDepth() = %d, want 2", got)
	}

	overrideTimeout := Duration(45 * time.Second)
	overrideDepth := 4
	MergeModelConfig(&cfg, ModelConfig{
		PtrAdmissionTimeout: &overrideTimeout,
		PtrQueueDepth:       &overrideDepth,
	})
	kronkCfg = cfg.ToKronkConfig()
	if got := kronkCfg.AdmissionTimeout(); got != 45*time.Second {
		t.Errorf("merged AdmissionTimeout() = %s, want %s", got, 45*time.Second)
	}
	if got := kronkCfg.QueueDepth(); got != 4 {
		t.Errorf("merged QueueDepth() = %d, want 4", got)
	}
}

func TestModelConfigFlashAttentionPresence(t *testing.T) {
	unset := ModelConfig{}.ToKronkConfig()
	if unset.PtrFlashAttention != nil {
		t.Errorf("unset PtrFlashAttention: got %v, want nil", unset.PtrFlashAttention)
	}

	enabled := model.FlashAttentionEnabled
	explicit := ModelConfig{FlashAttention: &enabled}.ToKronkConfig()
	if explicit.PtrFlashAttention == nil {
		t.Fatal("explicit PtrFlashAttention: got nil, want enabled")
	}
	if explicit.FlashAttention() != model.FlashAttentionEnabled {
		t.Errorf("FlashAttention: got %s, want enabled", explicit.FlashAttention())
	}
}

func TestRestoreAutoTunedSizing(t *testing.T) {
	contextWindow := 131072
	nSeqMax := 2
	sizing := ModelConfig{
		PtrContextWindow: &contextWindow,
		PtrNSeqMax:       &nSeqMax,
		CacheTypeK:       model.GGMLTypeQ8_0,
		CacheTypeV:       model.GGMLTypeQ8_0,
	}
	cfg := ModelConfig{
		PtrContextWindow: new(0),
		PtrNSeqMax:       new(0),
		CacheTypeK:       model.GGMLTypeAuto,
		CacheTypeV:       model.GGMLTypeAuto,
	}

	restoreAutoTunedSizing(&cfg, sizing)

	if cfg.PtrContextWindow == nil || *cfg.PtrContextWindow != contextWindow {
		t.Errorf("PtrContextWindow: got %v, want %d", cfg.PtrContextWindow, contextWindow)
	}
	if cfg.PtrNSeqMax == nil || *cfg.PtrNSeqMax != nSeqMax {
		t.Errorf("PtrNSeqMax: got %v, want %d", cfg.PtrNSeqMax, nSeqMax)
	}
	if cfg.CacheTypeK != model.GGMLTypeQ8_0 {
		t.Errorf("CacheTypeK: got %s, want q8_0", cfg.CacheTypeK)
	}
	if cfg.CacheTypeV != model.GGMLTypeQ8_0 {
		t.Errorf("CacheTypeV: got %s, want q8_0", cfg.CacheTypeV)
	}
}

func TestRestoreAutoTunedSizingEmpty(t *testing.T) {
	contextWindow := 131072
	nSeqMax := 2
	cfg := ModelConfig{
		PtrContextWindow: &contextWindow,
		PtrNSeqMax:       &nSeqMax,
		CacheTypeK:       model.GGMLTypeQ8_0,
		CacheTypeV:       model.GGMLTypeQ8_0,
	}

	restoreAutoTunedSizing(&cfg, ModelConfig{})

	if cfg.PtrContextWindow == nil || *cfg.PtrContextWindow != contextWindow {
		t.Errorf("PtrContextWindow: got %v, want %d", cfg.PtrContextWindow, contextWindow)
	}
	if cfg.PtrNSeqMax == nil || *cfg.PtrNSeqMax != nSeqMax {
		t.Errorf("PtrNSeqMax: got %v, want %d", cfg.PtrNSeqMax, nSeqMax)
	}
	if cfg.CacheTypeK != model.GGMLTypeQ8_0 {
		t.Errorf("CacheTypeK: got %s, want q8_0", cfg.CacheTypeK)
	}
	if cfg.CacheTypeV != model.GGMLTypeQ8_0 {
		t.Errorf("CacheTypeV: got %s, want q8_0", cfg.CacheTypeV)
	}
}

func TestAutoTuneApplied(t *testing.T) {
	contextWindow := 131072
	nSeqMax := 2

	tests := []struct {
		name string
		cfg  ModelConfig
		want bool
	}{
		{"empty analysis", ModelConfig{}, false},
		{"context only", ModelConfig{PtrContextWindow: &contextWindow}, false},
		{"complete recommendation", ModelConfig{PtrContextWindow: &contextWindow, PtrNSeqMax: &nSeqMax}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := autoTuneApplied(tt.cfg); got != tt.want {
				t.Errorf("autoTuneApplied: got %t, want %t", got, tt.want)
			}
		})
	}
}
