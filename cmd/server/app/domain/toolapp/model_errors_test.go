package toolapp

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

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
