package malina_test

import (
	"path/filepath"
	"testing"

	"github.com/ardanlabs/kronk/sdk/malina"
	"github.com/ardanlabs/kronk/sdk/tools/backend"
	malinalibs "github.com/ardanlabs/kronk/sdk/tools/malina/libs"
	malinamodels "github.com/ardanlabs/kronk/sdk/tools/malina/models"
)

func TestInitRegistersToolingBeforeNativeFailure(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if err := malina.Init(
		malina.WithLibPath(missing),
		malina.WithLogLevel(malina.LogNormal),
		malina.WithProgress(malina.DiscardProgress),
	); err == nil {
		t.Fatal("Init() error = nil, want native load failure")
	}

	registered, ok := backend.Get(backend.KindStableDiffusion)
	if !ok {
		t.Fatal("stable-diffusion backend was not registered")
	}
	libs, err := registered.NewLibs()
	if err != nil {
		t.Fatalf("NewLibs() error = %v", err)
	}
	if _, ok := libs.(*malinalibs.Libs); !ok {
		t.Errorf("NewLibs() type = %T, want *libs.Libs", libs)
	}
	catalog, err := registered.NewCatalog(t.TempDir())
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}
	if _, ok := catalog.(*malinamodels.Models); !ok {
		t.Errorf("NewCatalog() type = %T, want *models.Models", catalog)
	}
}
