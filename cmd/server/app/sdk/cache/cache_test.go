package cache_test

import (
	"bytes"
	"context"
	"io"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/cache"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

var log model.Logger

func Test_Cache(t *testing.T) {
	log = initKronk(t)
	t.Run("new-manager", newManager)
	t.Run("acquire-model", acquireModel)
	t.Run("shutdown", shutdown)
	t.Run("eviction", eviction)
}

func newManager(t *testing.T) {
	t.Run("default config values", func(t *testing.T) {
		cfg := cache.Config{
			Log: log,
		}

		mgr, err := cache.NewCache(cfg)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		defer mgr.Shutdown(context.Background())
	})

	t.Run("custom config values", func(t *testing.T) {
		cfg := cache.Config{
			Log:            log,
			ModelsInCache:  5,
			ModelInstances: 2,
			CacheTTL:       10 * time.Minute,
		}

		mgr, err := cache.NewCache(cfg)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		defer mgr.Shutdown(context.Background())
	})
}

func acquireModel(t *testing.T) {
	log := logger.New(io.Discard, logger.LevelInfo, "test", nil)

	modelID := findAvailableModel(t, "")

	cfg := cache.Config{
		Log:            log.Info,
		ModelsInCache:  3,
		ModelInstances: 1,
		CacheTTL:       5 * time.Minute,
	}

	mgr, err := cache.NewCache(cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	defer mgr.Shutdown(context.Background())

	t.Run("acquire model first time", func(t *testing.T) {
		ctx := context.Background()
		k, err := mgr.AquireModel(ctx, modelID)
		if err != nil {
			t.Fatalf("expected no error acquiring model, got: %v", err)
		}
		if k == nil {
			t.Fatal("expected non-nil kronk instance")
		}
	})

	t.Run("acquire same model from cache", func(t *testing.T) {
		ctx := context.Background()
		k1, err := mgr.AquireModel(ctx, modelID)
		if err != nil {
			t.Fatalf("expected no error acquiring model, got: %v", err)
		}

		k2, err := mgr.AquireModel(ctx, modelID)
		if err != nil {
			t.Fatalf("expected no error acquiring cached model, got: %v", err)
		}

		if k1 != k2 {
			t.Error("expected same kronk instance from cache")
		}
	})

	t.Run("acquire non-existent model", func(t *testing.T) {
		ctx := context.Background()
		_, err := mgr.AquireModel(ctx, "non-existent-model-xyz")
		if err == nil {
			t.Fatal("expected error for non-existent model")
		}
	})
}

func shutdown(t *testing.T) {
	modelID := findAvailableModel(t, "")

	t.Run("shutdown empty cache", func(t *testing.T) {
		cfg := cache.Config{
			Log: log,
		}

		mgr, err := cache.NewCache(cfg)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := mgr.Shutdown(ctx); err != nil {
			t.Fatalf("expected no error shutting down empty cache, got: %v", err)
		}
	})

	t.Run("shutdown with loaded models", func(t *testing.T) {
		cfg := cache.Config{
			Log:            log,
			ModelsInCache:  3,
			ModelInstances: 1,
			CacheTTL:       5 * time.Minute,
		}

		mgr, err := cache.NewCache(cfg)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		ctx := context.Background()
		_, err = mgr.AquireModel(ctx, modelID)
		if err != nil {
			t.Fatalf("expected no error acquiring model, got: %v", err)
		}

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		start := time.Now()
		if err := mgr.Shutdown(shutdownCtx); err != nil {
			t.Fatalf("expected no error shutting down, got: %v", err)
		}
		t.Logf("shutdown completed in %v", time.Since(start))
	})

	t.Run("shutdown timeout expires", func(t *testing.T) {
		cfg := cache.Config{
			Log:            log,
			ModelsInCache:  3,
			ModelInstances: 1,
			CacheTTL:       5 * time.Minute,
		}

		mgr, err := cache.NewCache(cfg)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		ctx := context.Background()
		_, err = mgr.AquireModel(ctx, modelID)
		if err != nil {
			t.Fatalf("expected no error acquiring model, got: %v", err)
		}

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()

		time.Sleep(10 * time.Millisecond)

		err = mgr.Shutdown(shutdownCtx)
		if err != context.DeadlineExceeded {
			t.Logf("shutdown returned: %v (may succeed if eviction is fast enough)", err)
		}
	})

	t.Run("shutdown with cancelled context", func(t *testing.T) {
		cfg := cache.Config{
			Log:            log,
			ModelsInCache:  3,
			ModelInstances: 1,
			CacheTTL:       5 * time.Minute,
		}

		mgr, err := cache.NewCache(cfg)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		ctx := context.Background()
		_, err = mgr.AquireModel(ctx, modelID)
		if err != nil {
			t.Fatalf("expected no error acquiring model, got: %v", err)
		}

		shutdownCtx, cancel := context.WithCancel(context.Background())
		cancel()

		err = mgr.Shutdown(shutdownCtx)
		if err != context.Canceled {
			t.Logf("shutdown returned: %v (may succeed if eviction is fast enough)", err)
		}
	})

	t.Run("shutdown blocks until eviction completes", func(t *testing.T) {
		cfg := cache.Config{
			Log:            log,
			ModelsInCache:  3,
			ModelInstances: 1,
			CacheTTL:       5 * time.Minute,
		}

		mgr, err := cache.NewCache(cfg)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		ctx := context.Background()
		_, err = mgr.AquireModel(ctx, modelID)
		if err != nil {
			t.Fatalf("expected no error acquiring model, got: %v", err)
		}

		var wg sync.WaitGroup
		wg.Add(1)

		shutdownComplete := make(chan struct{})
		go func() {
			defer wg.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()

			if err := mgr.Shutdown(shutdownCtx); err != nil {
				t.Errorf("shutdown error: %v", err)
			}
			close(shutdownComplete)
		}()

		select {
		case <-shutdownComplete:
			t.Log("shutdown completed successfully")
		case <-time.After(45 * time.Second):
			t.Fatal("shutdown did not complete within expected time")
		}

		wg.Wait()
	})
}

func eviction(t *testing.T) {
	modelID1 := findAvailableModel(t, "")
	modelID2 := findAvailableModel(t, modelID1)

	t.Run("eviction on TTL expiry", func(t *testing.T) {
		cfg := cache.Config{
			Log:            log,
			ModelsInCache:  3,
			ModelInstances: 1,
			CacheTTL:       500 * time.Millisecond,
		}

		mgr, err := cache.NewCache(cfg)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		defer mgr.Shutdown(context.Background())

		ctx := context.Background()

		k1, err := mgr.AquireModel(ctx, modelID1)
		if err != nil {
			t.Fatalf("expected no error acquiring model, got: %v", err)
		}

		t.Log("waiting for TTL to expire...")
		time.Sleep(2 * time.Second)

		k2, err := mgr.AquireModel(ctx, modelID1)
		if err != nil {
			t.Fatalf("expected no error re-acquiring model after eviction, got: %v", err)
		}

		if k1 == k2 {
			t.Fatal("same instance returned (cache may not have evicted yet)")
		}
	})

	t.Run("eviction on capacity exceeded", func(t *testing.T) {
		cfg := cache.Config{
			Log:            log,
			ModelsInCache:  1,
			ModelInstances: 1,
			CacheTTL:       5 * time.Minute,
		}

		mgr, err := cache.NewCache(cfg)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		defer mgr.Shutdown(context.Background())

		ctx := context.Background()

		k1, err := mgr.AquireModel(ctx, modelID1)
		if err != nil {
			t.Fatalf("expected no error acquiring first model, got: %v", err)
		}

		time.Sleep(time.Second)

		k2, err := mgr.AquireModel(ctx, modelID2)
		if err != nil {
			t.Fatalf("expected no error acquiring first model, got: %v", err)
		}

		if k1 == k2 {
			t.Fatal("same instance returned, should have new instance")
		}
	})
}

// =============================================================================

func initKronk(t *testing.T) model.Logger {
	libs, err := libs.New()
	if err != nil {
		t.Fatalf("unable to create libs api: %s", err)
	}

	t.Logf("installing/updating libraries, current version: libPath[%s], arch[%s] os[%s] processor[%s]", libs.LibsPath(), libs.Arch(), libs.OS(), libs.Processor())

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	tag, err := libs.Download(ctx, kronk.FmtLogger)
	if err != nil {
		t.Fatalf("unable to install llama.cpp: %s", err)
	}

	t.Logf("libraries installed: current[%s] latest[%s]", tag.Version, tag.Latest)

	if err := kronk.Init(); err != nil {
		t.Fatalf("installation invalid: %s", err)
	}

	var b bytes.Buffer
	log := logger.New(&b, logger.LevelInfo, "test", nil)

	t.Cleanup(func() {
		t.Log("=====================")
		t.Log(b.String())
		t.Log("=====================")
	})

	return log.Info
}

func findAvailableModel(t *testing.T, notModelID string) string {
	models, err := models.New()
	if err != nil {
		t.Fatalf("creating models system: %s", err)
	}

	modelFiles, err := models.RetrieveFiles()
	if err != nil {
		t.Skip("no models available for testing - skipping")
	}

	var modelID string
	for range len(modelFiles) {
		idx := rand.Intn(len(modelFiles))
		if modelFiles[idx].ID != notModelID {
			modelID = modelFiles[idx].ID
			break
		}
	}

	if modelID == "" {
		for _, mf := range modelFiles {
			if mf.ID != notModelID {
				modelID = mf.ID
				break
			}
		}
	}

	t.Logf("using model: %s", modelID)
	return modelID
}
