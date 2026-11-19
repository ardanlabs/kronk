package chatapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/toolapp"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/apitest"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/mux"
	buckymodels "github.com/ardanlabs/kronk/sdk/tools/bucky/models"
	malinamodels "github.com/ardanlabs/kronk/sdk/tools/malina/models"
	llamamodels "github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/google/go-cmp/cmp"
)

func TestMoECacheCPUShareTransport(t *testing.T) {
	api := newTransportAPI(t, mux.Config{})
	var table []apitest.Table
	for _, share := range []float32{-0.01, 1.01} {
		table = append(table, apitest.Table{
			Name: fmt.Sprint(share), Method: http.MethodPost, URL: "/v1/kronk/models/vram",
			RawBody:    fmt.Appendf(nil, `{"model_id":"example/model","moe_cache_cpu_share":%v}`, share),
			Headers:    map[string]string{"Content-Type": "application/json"},
			StatusCode: http.StatusBadRequest, GotResp: &errs.Error{}, ExpResp: errs.InvalidArgument,
			ResponseCmpFunc: jsonContentType,
			CmpFunc: func(got, exp any) string {
				return cmp.Diff(exp.(errs.ErrCode).String(), got.(*errs.Error).Code.String())
			},
		})
	}
	api.Run(t, table, "moe-cache-cpu-share")
}

func TestModelDetailsNotFound(t *testing.T) {
	models, err := llamamodels.NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatalf("NewWithPaths: %v", err)
	}
	api := newTransportAPI(t, mux.Config{Models: models})
	api.Run(t, []apitest.Table{{
		Name: "missing", Method: http.MethodGet, URL: "/v1/kronk/models/provider%2Fmissing",
		StatusCode: http.StatusNotFound, GotResp: &errs.Error{}, ExpResp: errs.NotFound,
		ResponseCmpFunc: jsonContentType,
		CmpFunc: func(got, exp any) string {
			return cmp.Diff(exp.(errs.ErrCode).String(), got.(*errs.Error).Code.String())
		},
	}}, "model-details")
}

func TestListModelsIntegrity(t *testing.T) {
	models, err := llamamodels.NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatalf("NewWithPaths: %v", err)
	}
	if err := os.WriteFile(filepath.Join(models.Path(), ".index.yaml"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile index: %v", err)
	}
	api := newTransportAPI(t, mux.Config{Models: models})
	api.Run(t, []apitest.Table{
		{
			Name: "empty", Method: http.MethodGet, URL: "/v1/kronk/models/integrity",
			StatusCode: http.StatusOK, GotResp: &toolapp.ModelIntegrityResponse{},
			ResponseCmpFunc: jsonContentType,
			CmpFunc: func(got, _ any) string {
				response := got.(*toolapp.ModelIntegrityResponse)
				return cmp.Diff("model_integrity.list", response.Object) + cmp.Diff(0, len(response.Data))
			},
		},
		{
			Name: "missing", Method: http.MethodGet, URL: "/v1/kronk/models/integrity/missing",
			StatusCode: http.StatusNotFound, GotResp: &errs.Error{}, ExpResp: errs.NotFound,
			ResponseCmpFunc: jsonContentType,
			CmpFunc: func(got, exp any) string {
				return cmp.Diff(exp.(errs.ErrCode).String(), got.(*errs.Error).Code.String())
			},
		},
	}, "model-integrity")
}

func TestBuckyModelDetailsNotFound(t *testing.T) {
	models, err := buckymodels.NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatalf("NewWithPaths: %v", err)
	}
	api := newTransportAPI(t, mux.Config{BuckyModels: models})
	api.Run(t, []apitest.Table{{
		Name: "missing", Method: http.MethodGet, URL: "/v1/bucky/models/missing/details",
		StatusCode: http.StatusNotFound, GotResp: &errs.Error{}, ExpResp: errs.NotFound,
		ResponseCmpFunc: jsonContentType,
		CmpFunc: func(got, exp any) string {
			return cmp.Diff(exp.(errs.ErrCode).String(), got.(*errs.Error).Code.String())
		},
	}}, "bucky-model-details")
}

func TestListMalinaModelsEmpty(t *testing.T) {
	models, err := malinamodels.NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatalf("NewWithPaths: %v", err)
	}
	api := newTransportAPI(t, mux.Config{MalinaModels: models})
	api.Run(t, []apitest.Table{{
		Name: "empty", Method: http.MethodGet, URL: "/v1/malina/models",
		StatusCode: http.StatusOK, GotResp: &toolapp.MalinaModelsResponse{},
		ResponseCmpFunc: jsonContentType,
		CmpFunc: func(got, _ any) string {
			return cmp.Diff(0, len(got.(*toolapp.MalinaModelsResponse).Models))
		},
	}}, "malina-models")
}

func TestListMalinaCatalog(t *testing.T) {
	api := newTransportAPI(t, mux.Config{})
	api.Run(t, []apitest.Table{{
		Name: "bundles", Method: http.MethodGet, URL: "/v1/malina/models/catalog",
		StatusCode: http.StatusOK, GotResp: &toolapp.MalinaCatalogResponse{},
		ResponseCmpFunc: jsonContentType,
		CmpFunc: func(got, _ any) string {
			catalog := got.(*toolapp.MalinaCatalogResponse)
			if diff := cmp.Diff(len(malinamodels.Catalog()), len(catalog.Models)); diff != "" {
				return "models: " + diff
			}
			entries := make(map[string]toolapp.MalinaCatalogEntry, len(catalog.Models))
			for _, entry := range catalog.Models {
				entries[entry.ID] = entry
			}
			flux := entries["flux2-klein-9b"]
			if !flux.Gated || !flux.BasicTextToImage || len(flux.Files) != 3 {
				return fmt.Sprintf("flux2-klein-9b: got %+v, want gated text-to-image bundle with 3 files", flux)
			}
			llada := entries["llada-image-turbo"]
			if llada.Gated || !llada.BasicTextToImage || len(llada.Files) != 5 {
				return fmt.Sprintf("llada-image-turbo: got %+v, want ungated text-to-image bundle with 5 files", llada)
			}
			upscaler := entries["realesrgan-x4-anime"]
			if upscaler.BasicTextToImage || len(upscaler.Files) != 1 || upscaler.Files[0].Role != "upscaler" {
				return fmt.Sprintf("realesrgan-x4-anime: got %+v, want one-file support bundle", upscaler)
			}
			return ""
		},
	}}, "malina-catalog")
}

func TestListMalinaModelsIncludesSupportBundles(t *testing.T) {
	models, err := malinamodels.NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatalf("NewWithPaths: %v", err)
	}
	bundle, ok := malinamodels.BundleByName(malinamodels.BundleRealESRGANX4Anime)
	if !ok {
		t.Fatal("BundleByName did not find realesrgan-x4-anime")
	}
	bundlePath := filepath.Join(models.Path(), bundle.Name.String())
	if err := os.MkdirAll(bundlePath, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	modelPath := filepath.Join(bundlePath, bundle.Files[0].Filename)
	if err := os.WriteFile(modelPath, []byte("model"), 0o644); err != nil {
		t.Fatalf("WriteFile model: %v", err)
	}
	manifest := malinamodels.Manifest{
		Bundle: bundle.Name, License: bundle.License, Gated: bundle.Gated,
		Files: map[string]string{bundle.Files[0].Role.String(): modelPath},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bundlePath, malinamodels.ManifestFilename), data, 0o644); err != nil {
		t.Fatalf("WriteFile manifest: %v", err)
	}
	api := newTransportAPI(t, mux.Config{MalinaModels: models})
	api.Run(t, []apitest.Table{{
		Name: "support-bundle", Method: http.MethodGet, URL: "/v1/malina/models",
		StatusCode: http.StatusOK, GotResp: &toolapp.MalinaModelsResponse{},
		ResponseCmpFunc: jsonContentType,
		CmpFunc: func(got, _ any) string {
			list := got.(*toolapp.MalinaModelsResponse)
			if diff := cmp.Diff(1, len(list.Models)); diff != "" {
				return "models: " + diff
			}
			return cmp.Diff(bundle.Name.String(), list.Models[0].ID) + cmp.Diff(false, list.Models[0].BasicTextToImage)
		},
	}}, "malina-models")
}

func TestMalinaLibsUnavailable(t *testing.T) {
	api := newTransportAPI(t, mux.Config{})
	table := []apitest.Table{
		{Name: "list", Method: http.MethodGet, URL: "/v1/malina/libs"},
		{Name: "verify", Method: http.MethodGet, URL: "/v1/malina/libs/integrity"},
		{Name: "pull", Method: http.MethodPost, URL: "/v1/malina/libs/pull"},
		{Name: "installs", Method: http.MethodGet, URL: "/v1/malina/libs/installs"},
		{Name: "remove", Method: http.MethodDelete, URL: "/v1/malina/libs/installs?arch=amd64&os=linux&processor=cpu"},
	}
	for i := range table {
		table[i].StatusCode = http.StatusServiceUnavailable
		table[i].GotResp = &errs.Error{}
		table[i].ExpResp = errs.Unavailable
		table[i].ResponseCmpFunc = jsonContentType
		table[i].CmpFunc = func(got, exp any) string {
			return cmp.Diff(exp.(errs.ErrCode).String(), got.(*errs.Error).Code.String())
		}
	}
	table = append(table, apitest.Table{
		Name: "combinations", Method: http.MethodGet, URL: "/v1/malina/libs/combinations",
		StatusCode: http.StatusOK, GotResp: &toolapp.CombinationsResponse{},
		ResponseCmpFunc: jsonContentType,
		CmpFunc: func(got, _ any) string {
			if len(got.(*toolapp.CombinationsResponse).Combinations) == 0 {
				return "combinations: got empty list, want published combinations"
			}
			return ""
		},
	})
	api.Run(t, table, "malina-libs")
}
