//go:build malina_integration

package imageapp

import (
	"bytes"
	"context"
	"encoding/base64"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/sdk/applog"
	"github.com/ardanlabs/kronk/sdk/malina"
	malinapool "github.com/ardanlabs/kronk/sdk/malina/pool"
	"github.com/ardanlabs/kronk/sdk/pool"
	"github.com/ardanlabs/kronk/sdk/pool/engine/resman"
	malinalibs "github.com/ardanlabs/kronk/sdk/tools/malina/libs"
	malinamodels "github.com/ardanlabs/kronk/sdk/tools/malina/models"
)

func TestImageGenerationModel(t *testing.T) {
	models, err := malinamodels.New()
	if err != nil {
		t.Fatalf("malinamodels.New() error = %v", err)
	}
	if _, err := models.FullPath(malinamodels.BundleSD15.String()); err != nil {
		t.Fatalf("%s bundle is not installed: %v", malinamodels.BundleSD15, err)
	}
	if err := malina.Init(
		malina.WithLibPath(malinalibs.Path("")),
		malina.WithProgress(malina.DiscardProgress),
	); err != nil {
		t.Fatalf("malina.Init() error = %v", err)
	}

	rm, err := resman.New(resman.Config{
		Snapshot:      resman.Snapshot{RAMBytes: 64 << 30},
		BudgetPercent: 100,
	})
	if err != nil {
		t.Fatalf("resman.New() error = %v", err)
	}
	malinaPool, err := malinapool.New(malinapool.Config{
		Log:    applog.DiscardLogger,
		Models: models,
		Resman: rm,
	})
	if err != nil {
		t.Fatalf("malinapool.New() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := malinaPool.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown() error = %v", err)
		}
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewBufferString(`{
		"model":"sd-1.5",
		"prompt":"a red square",
		"size":"128x128",
		"steps":1,
		"seed":42
	}`))
	api := app{
		log:  logger.New(io.Discard, logger.LevelInfo, "image-test", func(context.Context) string { return "" }),
		pool: &pool.Pool{Malina: malinaPool},
	}
	response := api.generations(t.Context(), request)
	generated, ok := response.(generationResponse)
	if !ok {
		t.Fatalf("generations() response = %T, want generationResponse", response)
	}
	if len(generated.Data) != 1 {
		t.Fatalf("generated images: got %d, want 1", len(generated.Data))
	}
	data, err := base64.StdEncoding.DecodeString(generated.Data[0].B64JSON)
	if err != nil {
		t.Fatalf("decode b64_json: %v", err)
	}
	image, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode PNG: %v", err)
	}
	if image.Bounds().Dx() != 128 || image.Bounds().Dy() != 128 {
		t.Errorf("image dimensions: got %dx%d, want 128x128", image.Bounds().Dx(), image.Bounds().Dy())
	}
	if generated.Data[0].Seed != 42 {
		t.Errorf("seed: got %d, want 42", generated.Data[0].Seed)
	}
}
