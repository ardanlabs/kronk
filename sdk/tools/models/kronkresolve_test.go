package models

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/kvstorage"
	"github.com/ardanlabs/kronk/sdk/kronk/kvstorage/ram"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"go.yaml.in/yaml/v2"
)

func TestResolveSessionStoreFactory(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ModelConfig
		wantErr bool
	}{
		{"default RAM", ModelConfig{}, false},
		{"explicit RAM", ModelConfig{SessionStoreKind: kvstorage.RAM}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory, err := resolveSessionStoreFactory(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolveSessionStoreFactory() error = %v, wantErr %t", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			store, err := factory()
			if err != nil {
				t.Fatalf("factory() error = %v, want nil", err)
			}
			t.Cleanup(func() {
				if err := store.Close(); err != nil {
					t.Errorf("store.Close() error = %v, want nil", err)
				}
			})

			if _, ok := store.(*ram.Store); !ok {
				t.Errorf("factory() store = %T, want *ram.Store", store)
			}
		})
	}
}

func TestSessionStoreKindYAML(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    kvstorage.Kind
		wantErr bool
	}{
		{"RAM", "ram", kvstorage.RAM, false},
		{"unknown", "unknown", kvstorage.Kind{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := []byte("session-store-kind: " + tt.value + "\n")

			var cfg ModelConfig
			err := yaml.Unmarshal(data, &cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("yaml.Unmarshal() error = %v, wantErr %t", err, tt.wantErr)
			}
			if !cfg.SessionStoreKind.Equal(tt.want) {
				t.Errorf("SessionStoreKind = %q, want %q", cfg.SessionStoreKind, tt.want)
			}
		})
	}
}

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

func TestModelConfigChatTemplateKwargs(t *testing.T) {
	data := []byte(`test-model:
  chat-template-kwargs:
    preserve_thinking: true
    custom_mode: configured
    nested:
      modes:
        - name: precise
`)

	var configs map[string]ModelConfig
	if err := yaml.Unmarshal(data, &configs); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	cfg := configs["test-model"]
	if got := cfg.ChatTemplateKwargs["preserve_thinking"]; got != true {
		t.Errorf("preserve_thinking = %v, want true", got)
	}

	resolved := cfg.ToKronkConfig()
	if got := resolved.ChatTemplateKwargs["custom_mode"]; got != "configured" {
		t.Errorf("ChatTemplateKwargs custom_mode = %v, want configured", got)
	}
	nested, ok := resolved.ChatTemplateKwargs["nested"].(model.D)
	if !ok {
		t.Fatalf("resolved nested = %T, want model.D", resolved.ChatTemplateKwargs["nested"])
	}
	modes, ok := nested["modes"].([]any)
	if !ok || len(modes) != 1 {
		t.Fatalf("resolved nested modes = %T(%v), want one-element []any", nested["modes"], nested["modes"])
	}
	mode, ok := modes[0].(model.D)
	if !ok || mode["name"] != "precise" {
		t.Errorf("resolved nested mode = %T(%v), want name precise", modes[0], modes[0])
	}

	resolved.ChatTemplateKwargs["custom_mode"] = "mutated"
	mode["name"] = "mutated"
	if got := cfg.ChatTemplateKwargs["custom_mode"]; got != "configured" {
		t.Errorf("source custom_mode mutated: got %v, want configured", got)
	}
	sourceNested := cfg.ChatTemplateKwargs["nested"].(map[string]any)
	sourceMode := sourceNested["modes"].([]any)[0].(map[string]any)
	if got := sourceMode["name"]; got != "precise" {
		t.Errorf("source nested mode mutated: got %v, want precise", got)
	}
}

func TestMergeModelConfigChatTemplateKwargs(t *testing.T) {
	dst := ModelConfig{ChatTemplateKwargs: ChatTemplateKwargs{
		"preserve_thinking": false,
		"base_only":         true,
	}}
	src := ModelConfig{ChatTemplateKwargs: ChatTemplateKwargs{
		"preserve_thinking": true,
		"nested": map[string]any{
			"mode": "source",
		},
	}}

	MergeModelConfig(&dst, src)

	if got := dst.ChatTemplateKwargs["preserve_thinking"]; got != true {
		t.Errorf("preserve_thinking = %v, want true", got)
	}
	if got := dst.ChatTemplateKwargs["base_only"]; got != true {
		t.Errorf("base_only = %v, want true", got)
	}
	dst.ChatTemplateKwargs["preserve_thinking"] = false
	dst.ChatTemplateKwargs["nested"].(model.D)["mode"] = "mutated"
	if got := src.ChatTemplateKwargs["preserve_thinking"]; got != true {
		t.Errorf("source preserve_thinking mutated: got %v, want true", got)
	}
	if got := src.ChatTemplateKwargs["nested"].(map[string]any)["mode"]; got != "source" {
		t.Errorf("source nested mode mutated: got %v, want source", got)
	}
}

func TestChatTemplateKwargsRejectsNonStringNestedKey(t *testing.T) {
	data := []byte(`test-model:
  chat-template-kwargs:
    nested:
      1: invalid
`)

	var configs map[string]ModelConfig
	if err := yaml.Unmarshal(data, &configs); err == nil {
		t.Fatal("Unmarshal() error = nil, want non-string key error")
	}
}

func TestModelConfigLoadMode(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want model.LoadMode
	}{
		{"auto", "auto", model.LoadModeAuto},
		{"mlock", "mlock", model.LoadModeMLock},
		{"mmap mlock", "mmap+mlock", model.LoadModeMMapMLock},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := []byte("test-model:\n  load-mode: " + tt.yaml + "\n")

			var configs map[string]ModelConfig
			if err := yaml.Unmarshal(data, &configs); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}

			cfg := configs["test-model"]
			if cfg.PtrLoadMode == nil || *cfg.PtrLoadMode != tt.want {
				t.Fatalf("PtrLoadMode = %v, want pointer to %v", cfg.PtrLoadMode, tt.want)
			}
			if got := cfg.ToKronkConfig().LoadMode; got != tt.want {
				t.Errorf("ToKronkConfig().LoadMode = %v, want %v", got, tt.want)
			}
		})
	}

	if got := (ModelConfig{}).ToKronkConfig().LoadMode; got != model.LoadModeAuto {
		t.Errorf("default ToKronkConfig().LoadMode = %v, want %v", got, model.LoadModeAuto)
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
  imc-session-capacity: 8
  prefill-batch-size: 4096
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
	if got := kronkCfg.IMCSessionCapacity(); got != 8 {
		t.Errorf("IMCSessionCapacity() = %d, want 8", got)
	}
	if got := kronkCfg.PrefillBatchSize(); got != 4096 {
		t.Errorf("PrefillBatchSize() = %d, want 4096", got)
	}

	overrideTimeout := Duration(45 * time.Second)
	overrideDepth := 4
	overridePrefillBatchSize := 1024
	overrideSessions := 10
	MergeModelConfig(&cfg, ModelConfig{
		PtrAdmissionTimeout:   &overrideTimeout,
		PtrIMCSessionCapacity: &overrideSessions,
		PtrPrefillBatchSize:   &overridePrefillBatchSize,
		PtrQueueDepth:         &overrideDepth,
	})
	kronkCfg = cfg.ToKronkConfig()
	if got := kronkCfg.AdmissionTimeout(); got != 45*time.Second {
		t.Errorf("merged AdmissionTimeout() = %s, want %s", got, 45*time.Second)
	}
	if got := kronkCfg.QueueDepth(); got != 4 {
		t.Errorf("merged QueueDepth() = %d, want 4", got)
	}
	if got := kronkCfg.IMCSessionCapacity(); got != 10 {
		t.Errorf("merged IMCSessionCapacity() = %d, want 10", got)
	}
	if got := kronkCfg.PrefillBatchSize(); got != 1024 {
		t.Errorf("merged PrefillBatchSize() = %d, want 1024", got)
	}
}

func TestModelConfigSpeculation(t *testing.T) {
	data := []byte(`test-model:
  speculation: disabled
`)

	var configs map[string]ModelConfig
	if err := yaml.Unmarshal(data, &configs); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	cfg := configs["test-model"]
	if got := cfg.ToKronkConfig().SpeculationMode(); got != model.SpeculationDisabled {
		t.Errorf("SpeculationMode() = %q, want %q", got, model.SpeculationDisabled)
	}

	MergeModelConfig(&cfg, ModelConfig{Speculation: model.SpeculationMTP})
	if got := cfg.ToKronkConfig().SpeculationMode(); got != model.SpeculationMTP {
		t.Errorf("merged SpeculationMode() = %q, want %q", got, model.SpeculationMTP)
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

func TestModelConfigLeavesSamplingDefaultsUnset(t *testing.T) {
	unset := ModelConfig{}.ToKronkConfig()
	if got := unset.DefaultParams.TopK; got != 0 {
		t.Errorf("unset TopK: got %d, want 0 for GGUF resolution", got)
	}
	if got := unset.DefaultParams.Temperature; got != 0 {
		t.Errorf("unset Temperature: got %v, want 0 for GGUF resolution", got)
	}

	explicit := ModelConfig{
		Sampling: SamplingConfig{
			TopK:        20,
			Temperature: 0.6,
		},
	}.ToKronkConfig()
	if got, want := explicit.DefaultParams.TopK, int32(20); got != want {
		t.Errorf("explicit TopK: got %d, want %d", got, want)
	}
	if got, want := explicit.DefaultParams.Temperature, float32(0.6); got != want {
		t.Errorf("explicit Temperature: got %v, want %v", got, want)
	}
}

func TestModelConfigSamplingSeed(t *testing.T) {
	data := []byte(`test-model:
  sampling-parameters:
    seed: 0
`)

	var configs map[string]ModelConfig
	if err := yaml.Unmarshal(data, &configs); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	seed := configs["test-model"].ToKronkConfig().DefaultParams.Seed
	if seed == nil || *seed != 0 {
		t.Errorf("Seed = %v, want pointer to 0", seed)
	}

	baseSeed := uint32(99)
	merged := mergeSampling(
		SamplingConfig{Seed: &baseSeed},
		configs["test-model"].Sampling,
	)
	if merged.Seed == nil || *merged.Seed != 0 {
		t.Errorf("merged Seed = %v, want pointer to 0", merged.Seed)
	}
}

func TestSamplingConfigWithMetadataDefaults(t *testing.T) {
	metadata := map[string]string{
		"general.sampling.temp":  "1.0",
		"general.sampling.top_k": "20",
		"general.sampling.top_p": "0.95",
	}

	got := (SamplingConfig{Temperature: 0.6}).WithMetadataDefaults(metadata)

	if want := float32(0.6); got.Temperature != want {
		t.Errorf("Temperature: got %v, want configured value %v", got.Temperature, want)
	}
	if want := int32(20); got.TopK != want {
		t.Errorf("TopK: got %d, want GGUF value %d", got.TopK, want)
	}
	if want := float32(0.95); got.TopP != want {
		t.Errorf("TopP: got %v, want GGUF value %v", got.TopP, want)
	}
	if want := int32(-1); got.DryPenaltyLast != want {
		t.Errorf("DryPenaltyLast: got %d, want Kronk fallback %d", got.DryPenaltyLast, want)
	}
	if got.ReasoningEffort != "" {
		t.Errorf("ReasoningEffort: got %q, want template default", got.ReasoningEffort)
	}
	explicit := (SamplingConfig{ReasoningEffort: model.ReasoningEffortHigh}).WithMetadataDefaults(metadata)
	if explicit.ReasoningEffort != model.ReasoningEffortHigh {
		t.Errorf("explicit ReasoningEffort: got %q, want %q", explicit.ReasoningEffort, model.ReasoningEffortHigh)
	}

	malformed := (SamplingConfig{}).WithMetadataDefaults(map[string]string{
		"general.sampling.temp":  "1e100",
		"general.sampling.top_p": "NaN",
	})
	if want := float32(model.DefTemp); malformed.Temperature != want {
		t.Errorf("malformed Temperature: got %v, want Kronk fallback %v", malformed.Temperature, want)
	}
	if want := float32(model.DefTopP); malformed.TopP != want {
		t.Errorf("malformed TopP: got %v, want Kronk fallback %v", malformed.TopP, want)
	}
}

func TestSamplingConfigWithDefaultsLeavesReasoningEffortUnset(t *testing.T) {
	got := (SamplingConfig{}).WithDefaults()
	if got.ReasoningEffort != "" {
		t.Errorf("ReasoningEffort: got %q, want template default", got.ReasoningEffort)
	}

	got = (SamplingConfig{ReasoningEffort: model.ReasoningEffortLow}).WithDefaults()
	if got.ReasoningEffort != model.ReasoningEffortLow {
		t.Errorf("explicit ReasoningEffort: got %q, want %q", got.ReasoningEffort, model.ReasoningEffortLow)
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

func TestFallbackAutoTuneSizing(t *testing.T) {
	contextWindow := 16_384
	nSeqMax := 2

	tests := []struct {
		name        string
		metadata    map[string]string
		constraints ModelConfig
		wantContext int
		wantNSeqMax int
	}{
		{
			name: "trained context below default cap",
			metadata: map[string]string{
				"general.architecture": "test",
				"test.context_length":  "4096",
			},
			wantContext: 4096,
			wantNSeqMax: 1,
		},
		{
			name: "missing training context uses default cap",
			metadata: map[string]string{
				"general.architecture": "test",
			},
			wantContext: 8192,
			wantNSeqMax: 1,
		},
		{
			name: "explicit sizing is preserved",
			constraints: ModelConfig{
				PtrContextWindow: &contextWindow,
				PtrNSeqMax:       &nSeqMax,
			},
			wantContext: contextWindow,
			wantNSeqMax: nSeqMax,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := fallbackAutoTuneSizing(ModelInfo{Metadata: tt.metadata}, tt.constraints)

			if cfg.PtrContextWindow == nil || *cfg.PtrContextWindow != tt.wantContext {
				t.Errorf("PtrContextWindow: got %v, want %d", cfg.PtrContextWindow, tt.wantContext)
			}
			if cfg.PtrNSeqMax == nil || *cfg.PtrNSeqMax != tt.wantNSeqMax {
				t.Errorf("PtrNSeqMax: got %v, want %d", cfg.PtrNSeqMax, tt.wantNSeqMax)
			}
		})
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
