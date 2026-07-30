package toolapp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	buckymodels "github.com/ardanlabs/kronk/sdk/tools/bucky/models"
	llamamodels "github.com/ardanlabs/kronk/sdk/tools/models"
)

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
