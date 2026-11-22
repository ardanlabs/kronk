package toolapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/vram"
	llamamodels "github.com/ardanlabs/kronk/sdk/tools/models"
)

func TestModelSummaryEncoding(t *testing.T) {
	metadata := map[string]string{
		"general.architecture": "qwen35moe", "general.size_label": "35B",
		"qwen35moe.block_count": "8", "qwen35moe.expert_count": "256",
		"qwen35moe.context_length": "131072", "qwen35moe.nextn_predict_layers": "1",
		"general.sampling.temp": "0.8", "general.sampling.top_k": "31",
		"general.sampling.top_p": "invalid", "general.sampling.penalty_repeat": "1.15",
	}
	for _, tt := range []struct {
		name     string
		id       string
		config   *llamamodels.ModelConfig
		wantCtx  *int
		wantTemp float64
		wantMode string
	}{
		{"base", "test/model", &llamamodels.ModelConfig{PtrContextWindow: new(8192)}, new(8192), 0.8, "auto"},
		{"profile", "test/model/LONG", &llamamodels.ModelConfig{
			PtrContextWindow: new(262144), Speculation: model.SpeculationDisabled,
			Sampling: llamamodels.SamplingConfig{Temperature: 0.3},
		}, new(262144), 0.3, "disabled"},
		{"unloaded", "test/unloaded", &llamamodels.ModelConfig{}, nil, 0.8, "auto"},
		{"remote catalog", "test/remote", nil, nil, 0, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			info := llamamodels.ModelInfoFromMetadata(tt.id, metadata, 0, true, false)
			summary := toModelSummary(info, tt.config)
			for _, response := range []interface {
				Encode() ([]byte, string, error)
			}{
				ModelInfoResponse{Summary: &summary, Metadata: metadata},
				CatalogDetailResponse{Summary: &summary},
			} {
				data, _, err := response.Encode()
				if err != nil {
					t.Fatal(err)
				}
				var got struct {
					Summary struct {
						ModelType string             `json:"model_type"`
						Sequence  string             `json:"sequence_processing"`
						Context   *int               `json:"context_window"`
						Source    string             `json:"mtp_source"`
						Mode      string             `json:"speculation"`
						Sampling  map[string]float64 `json:"sampling"`
					} `json:"summary"`
				}
				if err := json.Unmarshal(data, &got); err != nil {
					t.Fatal(err)
				}
				s := got.Summary
				if s.ModelType != "MoE" || s.Sequence != "Hybrid" || s.Source != "embedded" || s.Mode != tt.wantMode {
					t.Errorf("summary facts: got %+v, want MoE/Hybrid/embedded/%s", s, tt.wantMode)
				}
				if (s.Context == nil) != (tt.wantCtx == nil) || (s.Context != nil && *s.Context != *tt.wantCtx) {
					t.Errorf("context: got %v, want %v", s.Context, tt.wantCtx)
				}
				if tt.config == nil {
					if s.Sampling != nil {
						t.Errorf("remote defaults: got %v, want unknown", s.Sampling)
					}
					continue
				}
				for key, want := range map[string]float64{"temperature": tt.wantTemp, "top_k": 31, "top_p": 0.95, "min_p": 0, "repeat_penalty": 1.15, "repeat_last_n": 64} {
					if value, ok := s.Sampling[key]; !ok || math.Abs(value-want) > 0.000001 {
						t.Errorf("%s: got %v (present %t), want %v", key, value, ok, want)
					}
				}
			}
		})
	}
}

func TestMoECacheCPUShareEncoding(t *testing.T) {
	for _, share := range []float32{0, 0.25, 1} {
		resp := toModelInfo(llamamodels.FileInfo{}, llamamodels.ModelInfo{},
			llamamodels.ModelConfig{MoE: &model.MoEConfig{PtrCacheCPUShare: new(share)}}, nil)
		data, _, err := resp.Encode()
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}
		if want := fmt.Sprintf(`"cache-cpu-share":%v`, share); !bytes.Contains(data, []byte(want)) {
			t.Errorf("Encode: got %s, want %s", data, want)
		}
	}
}

func TestVRAMResponsePreservesZeroExpertLayers(t *testing.T) {
	resp := toVRAMResponse(vram.Result{
		Input: vram.Input{ExpertLayersOnGPU: 0},
	}, nil)

	data, _, err := resp.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !bytes.Contains(data, []byte(`"expert_layers_on_gpu":0`)) {
		t.Errorf("Encode: got %s, want expert_layers_on_gpu zero value", data)
	}
}

func TestVRAMResponseIncludesDeviceCacheMirrors(t *testing.T) {
	result := vram.Result{
		Input:      vram.Input{ModelSizeBytes: 1000, Slots: 2},
		SlotMemory: 120, KVVRAMBytes: 80, KVCPUBytes: 40,
		TotalVRAM: 1080,
		PerDevice: []vram.PerDeviceVRAM{
			{WeightsBytes: 250, KVBytes: 30, TotalBytes: 280},
			{WeightsBytes: 750, KVBytes: 70, TotalBytes: 820},
		},
	}
	resp := toVRAMResponse(result, nil)
	if resp.TotalVRAM != 1100 || resp.KVVRAMBytes != 100 || resp.SlotMemory != 140 || resp.KVPerSlot != 70 || resp.UnifiedFootprint != 1140 {
		t.Fatalf("device-aware totals: got VRAM/cache/slots/per-slot/unified %d/%d/%d/%d/%d, want 1100/100/140/70/1140",
			resp.TotalVRAM, resp.KVVRAMBytes, resp.SlotMemory, resp.KVPerSlot, resp.UnifiedFootprint)
	}
}

func TestOpenAIModel(t *testing.T) {
	modified := time.Unix(1_234_567_890, 987_654_321)

	model := toOpenAIModel(llamamodels.File{
		ID:       "test-model",
		Modified: modified,
	})

	if model.ID != "test-model" {
		t.Errorf("ID: got %q, want %q", model.ID, "test-model")
	}
	if model.Object != "model" {
		t.Errorf("Object: got %q, want %q", model.Object, "model")
	}
	if model.Created != modified.Unix() {
		t.Errorf("Created: got %d, want %d", model.Created, modified.Unix())
	}
	if model.OwnedBy != "kronk" {
		t.Errorf("OwnedBy: got %q, want %q", model.OwnedBy, "kronk")
	}
}

func TestOpenAIModelsCanonicalID(t *testing.T) {
	response := toOpenAIModels([]llamamodels.File{
		{
			ID:      "test-model",
			OwnedBy: "test-provider",
		},
	})

	if response.Data[0].ID != "test-provider/test-model" {
		t.Errorf("ID: got %q, want %q", response.Data[0].ID, "test-provider/test-model")
	}
}

func TestModelIntegrityResponse(t *testing.T) {
	verifiedAt := time.Unix(123, 0).UTC()
	response := toModelIntegrity([]llamamodels.IntegrityModel{
		{
			ID:          "test-model",
			OwnedBy:     "test-org",
			ModelFamily: "test-family",
			Status:      llamamodels.IntegrityVerified,
			Verified:    true,
			Artifacts: []llamamodels.IntegrityArtifact{
				{
					Role:       llamamodels.IntegrityRoleWeights,
					Filename:   "test-model.gguf",
					Digest:     "sha256:abc",
					Size:       42,
					Status:     llamamodels.IntegrityVerified,
					Verified:   true,
					VerifiedAt: verifiedAt,
				},
			},
		},
	})

	data, mediaType, err := response.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if mediaType != "application/json" {
		t.Errorf("media type: got %q, want %q", mediaType, "application/json")
	}

	want := `{"object":"model_integrity.list","data":[{"id":"test-model","owned_by":"test-org","model_family":"test-family","status":"verified","verified":true,"artifacts":[{"role":"weights","filename":"test-model.gguf","digest":"sha256:abc","size":42,"status":"verified","verified":true,"verified_at":"1970-01-01T00:02:03Z"}]}]}`
	if string(data) != want {
		t.Errorf("Encode: got %s, want %s", data, want)
	}

	detailData, _, err := response.Data[0].Encode()
	if err != nil {
		t.Fatalf("detail Encode: %v", err)
	}
	wantDetail := `{"id":"test-model","owned_by":"test-org","model_family":"test-family","status":"verified","verified":true,"artifacts":[{"role":"weights","filename":"test-model.gguf","digest":"sha256:abc","size":42,"status":"verified","verified":true,"verified_at":"1970-01-01T00:02:03Z"}]}`
	if string(detailData) != wantDetail {
		t.Errorf("detail Encode: got %s, want %s", detailData, wantDetail)
	}
}

func TestModelInfoAdmissionCapacity(t *testing.T) {
	tests := []struct {
		name          string
		nSeqMax       *int
		queueDepth    *int
		isEmbedModel  bool
		isRerankModel bool
		wantDepth     int
		wantCapacity  int
	}{
		{name: "generation defaults", wantDepth: 2, wantCapacity: 2},
		{name: "generation configured", nSeqMax: new(4), queueDepth: new(3), wantDepth: 3, wantCapacity: 12},
		{name: "embedding uses effective queue depth one", nSeqMax: new(4), queueDepth: new(3), isEmbedModel: true, wantDepth: 1, wantCapacity: 4},
		{name: "reranking uses effective queue depth one", nSeqMax: new(4), queueDepth: new(3), isRerankModel: true, wantDepth: 1, wantCapacity: 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := toModelInfo(
				llamamodels.FileInfo{},
				llamamodels.ModelInfo{IsEmbedModel: tt.isEmbedModel, IsRerankModel: tt.isRerankModel},
				llamamodels.ModelConfig{PtrNSeqMax: tt.nSeqMax, PtrQueueDepth: tt.queueDepth},
				nil,
			)

			if resp.ModelConfig.QueueDepth != tt.wantDepth {
				t.Errorf("QueueDepth: got %d, want %d", resp.ModelConfig.QueueDepth, tt.wantDepth)
			}
			if resp.ModelConfig.AdmissionCapacity != tt.wantCapacity {
				t.Errorf("AdmissionCapacity: got %d, want %d", resp.ModelConfig.AdmissionCapacity, tt.wantCapacity)
			}
		})
	}
}

func TestModelInfoQuantization(t *testing.T) {
	resp := toModelInfo(
		llamamodels.FileInfo{},
		llamamodels.ModelInfo{FileType: 15, Quantization: "Q4_K_M"},
		llamamodels.ModelConfig{},
		nil,
	)

	if resp.FileType != 15 {
		t.Errorf("FileType: got %d, want 15", resp.FileType)
	}
	if resp.Quantization != "Q4_K_M" {
		t.Errorf("Quantization: got %q, want %q", resp.Quantization, "Q4_K_M")
	}
}

func TestVRAMConfigFromRMCSWAFull(t *testing.T) {
	enabled := true
	disabled := false

	tests := []struct {
		name string
		ptr  *bool
		want bool
	}{
		{name: "unset defaults enabled", want: true},
		{name: "explicitly enabled", ptr: &enabled, want: true},
		{name: "explicitly disabled", ptr: &disabled, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rmc := llamamodels.ModelConfig{PtrSWAFull: tt.ptr}
			got := vramConfigFromRMC(rmc)
			if got.SWAFull != tt.want {
				t.Errorf("SWAFull: got %t, want %t", got.SWAFull, tt.want)
			}
		})
	}
}

func TestVRAMConfigFromRMCGPULayers(t *testing.T) {
	tests := []struct {
		name string
		ptr  *int
		want int64
	}{
		{name: "unset uses all GPU default", want: 0},
		{name: "CPU only", ptr: new(-1), want: -1},
		{name: "partial GPU", ptr: new(12), want: 12},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rmc := llamamodels.ModelConfig{PtrNGpuLayers: tt.ptr}
			got := vramConfigFromRMC(rmc)
			if got.GPULayers != tt.want {
				t.Errorf("GPULayers: got %d, want %d", got.GPULayers, tt.want)
			}
		})
	}
}

func TestVRAMConfigFromRMCPrefillBatchSize(t *testing.T) {
	prefillBatchSize := 4096
	rmc := llamamodels.ModelConfig{PtrPrefillBatchSize: &prefillBatchSize}

	got := vramConfigFromRMC(rmc)
	if got.NUBatch != int64(prefillBatchSize) {
		t.Errorf("NUBatch: got %d, want %d", got.NUBatch, prefillBatchSize)
	}
	rmc.PtrPhysicalBatchSize = new(512)
	got = vramConfigFromRMC(rmc)
	if got.NUBatch != 512 {
		t.Errorf("explicit physical NUBatch: got %d, want 512", got.NUBatch)
	}
}

func TestResolveSWAFull(t *testing.T) {
	enabled := true
	disabled := false

	tests := []struct {
		name       string
		requested  *bool
		configured *bool
		want       bool
	}{
		{name: "unset uses llama default", want: true},
		{name: "configured enabled", configured: &enabled, want: true},
		{name: "configured disabled", configured: &disabled, want: false},
		{name: "request enabled overrides configured disabled", requested: &enabled, configured: &disabled, want: true},
		{name: "request disabled overrides configured enabled", requested: &disabled, configured: &enabled, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveSWAFull(tt.requested, tt.configured); got != tt.want {
				t.Errorf("resolveSWAFull: got %t, want %t", got, tt.want)
			}
		})
	}
}
