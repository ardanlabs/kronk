package pool

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ardanlabs/kronk/sdk/pool/engine/loader"
	"github.com/ardanlabs/kronk/sdk/pool/engine/resman"
	malinamodels "github.com/ardanlabs/kronk/sdk/tools/malina/models"
)

func TestPlanMemoryTopology(t *testing.T) {
	models, _ := testModels(t, malinamodels.BundleSD15, 5_335_000_000)

	tests := []struct {
		name     string
		snapshot resman.Snapshot
		wantVRAM bool
	}{
		{
			name: "discrete gpu",
			snapshot: resman.Snapshot{
				Devices:  []resman.Device{{Name: "CUDA0", Type: "gpu_cuda", TotalBytes: 16 << 30}},
				RAMBytes: 32 << 30,
			},
			wantVRAM: true,
		},
		{
			name:     "unified memory",
			snapshot: resman.Snapshot{UnifiedMemory: true, RAMBytes: 32 << 30},
		},
		{
			name:     "cpu only",
			snapshot: resman.Snapshot{RAMBytes: 32 << 30},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rm, err := resman.New(resman.Config{Snapshot: tt.snapshot, BudgetPercent: 100})
			if err != nil {
				t.Fatalf("resman.New() error = %v", err)
			}
			sd := newStableDiffusion(discardLog, models, rm)
			req := loader.LoadRequest{ModelID: malinamodels.BundleSD15.String(), Key: "model"}
			req.Prepared, err = sd.Prepare(context.Background(), req)
			if err != nil {
				t.Fatalf("Prepare() error = %v", err)
			}

			plan, err := sd.Plan(context.Background(), req)
			if err != nil {
				t.Fatalf("Plan() error = %v", err)
			}
			want := req.Prepared.(preparedModel).memory.TotalVRAM
			if tt.wantVRAM {
				if plan.VRAMBytes != want || plan.RAMBytes != 0 {
					t.Errorf("memory: got vram=%d ram=%d, want vram=%d ram=0", plan.VRAMBytes, plan.RAMBytes, want)
				}
			} else if plan.VRAMBytes != 0 || plan.RAMBytes != want {
				t.Errorf("memory: got vram=%d ram=%d, want vram=0 ram=%d", plan.VRAMBytes, plan.RAMBytes, want)
			}
		})
	}
}

func TestResolveConfigMapsBundleComponents(t *testing.T) {
	models, files := testModels(t, malinamodels.BundleFlux2Klein4B, 1)
	sd := newStableDiffusion(discardLog, models, nil)

	cfg, err := sd.resolveConfig(malinamodels.BundleFlux2Klein4B.String())
	if err != nil {
		t.Fatalf("resolveConfig() error = %v", err)
	}
	if cfg.DiffusionModelPath != files[string(malinamodels.RoleDiffusion)] {
		t.Errorf("DiffusionModelPath: got %q, want %q", cfg.DiffusionModelPath, files[string(malinamodels.RoleDiffusion)])
	}
	if cfg.VAEPath != files[string(malinamodels.RoleVAE)] {
		t.Errorf("VAEPath: got %q, want %q", cfg.VAEPath, files[string(malinamodels.RoleVAE)])
	}
	if cfg.LLMPath != files[string(malinamodels.RoleLLM)] {
		t.Errorf("LLMPath: got %q, want %q", cfg.LLMPath, files[string(malinamodels.RoleLLM)])
	}
}

func TestResolveConfigRejectsUpscaler(t *testing.T) {
	models, _ := testModels(t, malinamodels.BundleRealESRGANX4Anime, 1)
	sd := newStableDiffusion(discardLog, models, nil)

	if _, err := sd.resolveConfig(malinamodels.BundleRealESRGANX4Anime.String()); err == nil {
		t.Fatal("resolveConfig() error = nil, want unsupported upscaler error")
	}
}

func TestFailedLoadReleasesReservation(t *testing.T) {
	models, files := testModels(t, malinamodels.BundleSD15, 1)
	if err := os.Remove(files[string(malinamodels.RoleModel)]); err != nil {
		t.Fatal(err)
	}
	rm, err := resman.New(resman.Config{
		Snapshot:      resman.Snapshot{UnifiedMemory: true, RAMBytes: 32 << 30},
		BudgetPercent: 100,
	})
	if err != nil {
		t.Fatalf("resman.New() error = %v", err)
	}
	p, err := New(Config{Log: discardLog, Models: models, Resman: rm, ModelsInPool: 2})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = p.AcquireModel(context.Background(), malinamodels.BundleSD15.String())
	if err == nil {
		t.Fatal("AcquireModel() error = nil, want model load error")
	}
	usage := rm.Usage()
	if len(usage.Reservations) != 0 || usage.RAMUsed != 0 {
		t.Errorf("usage after failed load: got reservations=%d ram=%d, want 0/0", len(usage.Reservations), usage.RAMUsed)
	}
}

func TestAcquireRejectsModelOutsideBudget(t *testing.T) {
	models, _ := testModels(t, malinamodels.BundleSD15, 1)
	rm, err := resman.New(resman.Config{
		Snapshot:      resman.Snapshot{UnifiedMemory: true, RAMBytes: 1 << 30},
		BudgetPercent: 100,
	})
	if err != nil {
		t.Fatalf("resman.New() error = %v", err)
	}
	p, err := New(Config{Log: discardLog, Models: models, Resman: rm})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = p.AcquireModel(context.Background(), malinamodels.BundleSD15.String())
	if !errors.Is(err, ErrNoCapacity) {
		t.Errorf("AcquireModel() error = %v, want ErrNoCapacity", err)
	}
	if len(rm.Usage().Reservations) != 0 {
		t.Errorf("Reservations: got %d, want 0", len(rm.Usage().Reservations))
	}
}

func testModels(t *testing.T, name malinamodels.BundleName, size int64) (*malinamodels.Models, map[string]string) {
	t.Helper()

	models, err := malinamodels.NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatalf("malinamodels.NewWithPaths() error = %v", err)
	}
	bundle, ok := malinamodels.BundleByName(name)
	if !ok {
		t.Fatalf("BundleByName(%q) not found", name)
	}
	dir := filepath.Join(models.Path(), name.String())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := malinamodels.Manifest{Bundle: name, Files: make(map[string]string, len(bundle.Files))}
	for _, file := range bundle.Files {
		path := filepath.Join(dir, file.Filename)
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Truncate(path, size); err != nil {
			t.Fatal(err)
		}
		manifest.Files[string(file.Role)] = path
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, malinamodels.ManifestFilename), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := models.BuildIndex(nil, false); err != nil {
		t.Fatalf("BuildIndex() error = %v", err)
	}

	return models, manifest.Files
}

func discardLog(context.Context, string, ...any) {}
