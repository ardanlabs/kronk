package toolapp

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/sdk/kronk/vram"
	buckymodels "github.com/ardanlabs/kronk/sdk/tools/bucky/models"
	llamamodels "github.com/ardanlabs/kronk/sdk/tools/models"
)

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

func TestModelDetailsNotFound(t *testing.T) {
	models, err := llamamodels.NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatalf("NewWithPaths: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/models/missing", nil)
	req.SetPathValue("model", "missing")
	resp := (&app{models: models}).showModel(t.Context(), req)

	assertNotFound(t, resp)
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

func TestListModelsIntegrity(t *testing.T) {
	models, err := llamamodels.NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatalf("NewWithPaths: %v", err)
	}
	if err := os.WriteFile(filepath.Join(models.Path(), ".index.yaml"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile index: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/kronk/models/integrity", nil)
	encoder := (&app{models: models}).listModelsIntegrity(t.Context(), req)
	response, ok := encoder.(ModelIntegrityResponse)
	if !ok {
		t.Fatalf("response type: got %T, want ModelIntegrityResponse", encoder)
	}
	if response.Object != "model_integrity.list" {
		t.Errorf("Object: got %q, want %q", response.Object, "model_integrity.list")
	}
	if len(response.Data) != 0 {
		t.Errorf("Data length: got %d, want 0", len(response.Data))
	}

	retrieveReq := httptest.NewRequest(http.MethodGet, "/v1/kronk/models/integrity/missing", nil)
	retrieveReq.SetPathValue("model", "missing")
	retrieveResponse := (&app{models: models}).retrieveModelIntegrity(t.Context(), retrieveReq)
	assertNotFound(t, retrieveResponse)
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

func TestBuckyModelDetailsNotFound(t *testing.T) {
	models, err := buckymodels.NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatalf("NewWithPaths: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/bucky/models/missing", nil)
	req.SetPathValue("model", "missing")
	resp := (&app{buckyModels: models}).detailsBuckyModel(t.Context(), req)

	assertNotFound(t, resp)
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

func assertNotFound(t *testing.T, resp any) {
	t.Helper()

	appErr, ok := resp.(*errs.Error)
	if !ok {
		t.Fatalf("response: got %T, want *errs.Error", resp)
	}
	if !appErr.Code.Equal(errs.NotFound) {
		t.Errorf("Code: got %s, want %s", appErr.Code, errs.NotFound)
	}
}
