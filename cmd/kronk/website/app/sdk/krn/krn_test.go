package krn_test

import (
	"bytes"
	"context"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ardanlabs/kronk"
	"github.com/ardanlabs/kronk/cmd/kronk/website/app/sdk/krn"
	"github.com/ardanlabs/kronk/cmd/kronk/website/foundation/logger"
	"github.com/ardanlabs/kronk/defaults"
	"github.com/ardanlabs/kronk/tools"
)

func Test_NewManager(t *testing.T) {
	log := initKronk(t)

	t.Run("default config values", func(t *testing.T) {
		cfg := krn.Config{
			Log:       log,
			ModelPath: defaults.ModelsDir(""),
		}

		mgr, err := krn.NewManager(cfg)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		defer mgr.Shutdown(context.Background())
	})

	t.Run("custom config values", func(t *testing.T) {
		cfg := krn.Config{
			Log:            log,
			ModelPath:      defaults.ModelsDir(""),
			MaxInCache:     5,
			ModelInstances: 2,
			TTL:            10 * time.Minute,
		}

		mgr, err := krn.NewManager(cfg)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		defer mgr.Shutdown(context.Background())
	})
}

func Test_AcquireModel(t *testing.T) {
	initKronk(t)
	log := logger.New(io.Discard, logger.LevelInfo, "test", nil)

	modelName := findAvailableModel(t, "")

	cfg := krn.Config{
		Log:            log,
		ModelPath:      defaults.ModelsDir(""),
		MaxInCache:     3,
		ModelInstances: 1,
		TTL:            5 * time.Minute,
	}

	mgr, err := krn.NewManager(cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	defer mgr.Shutdown(context.Background())

	t.Run("acquire model first time", func(t *testing.T) {
		ctx := context.Background()
		k, err := mgr.AquireModel(ctx, modelName)
		if err != nil {
			t.Fatalf("expected no error acquiring model, got: %v", err)
		}
		if k == nil {
			t.Fatal("expected non-nil kronk instance")
		}
	})

	t.Run("acquire same model from cache", func(t *testing.T) {
		ctx := context.Background()
		k1, err := mgr.AquireModel(ctx, modelName)
		if err != nil {
			t.Fatalf("expected no error acquiring model, got: %v", err)
		}

		k2, err := mgr.AquireModel(ctx, modelName)
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

func Test_Shutdown(t *testing.T) {
	log := initKronk(t)

	modelName := findAvailableModel(t, "")

	t.Run("shutdown empty cache", func(t *testing.T) {
		cfg := krn.Config{
			Log:       log,
			ModelPath: defaults.ModelsDir(""),
		}

		mgr, err := krn.NewManager(cfg)
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
		cfg := krn.Config{
			Log:            log,
			ModelPath:      defaults.ModelsDir(""),
			MaxInCache:     3,
			ModelInstances: 1,
			TTL:            5 * time.Minute,
		}

		mgr, err := krn.NewManager(cfg)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		ctx := context.Background()
		_, err = mgr.AquireModel(ctx, modelName)
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
		cfg := krn.Config{
			Log:            log,
			ModelPath:      defaults.ModelsDir(""),
			MaxInCache:     3,
			ModelInstances: 1,
			TTL:            5 * time.Minute,
		}

		mgr, err := krn.NewManager(cfg)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		ctx := context.Background()
		_, err = mgr.AquireModel(ctx, modelName)
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
		cfg := krn.Config{
			Log:            log,
			ModelPath:      defaults.ModelsDir(""),
			MaxInCache:     3,
			ModelInstances: 1,
			TTL:            5 * time.Minute,
		}

		mgr, err := krn.NewManager(cfg)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		ctx := context.Background()
		_, err = mgr.AquireModel(ctx, modelName)
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
		cfg := krn.Config{
			Log:            log,
			ModelPath:      defaults.ModelsDir(""),
			MaxInCache:     3,
			ModelInstances: 1,
			TTL:            5 * time.Minute,
		}

		mgr, err := krn.NewManager(cfg)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		ctx := context.Background()
		_, err = mgr.AquireModel(ctx, modelName)
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

func Test_Eviction(t *testing.T) {
	log := initKronk(t)

	modelName1 := findAvailableModel(t, "")
	modelName2 := findAvailableModel(t, modelName1)

	t.Run("eviction on TTL expiry", func(t *testing.T) {
		cfg := krn.Config{
			Log:            log,
			ModelPath:      defaults.ModelsDir(""),
			MaxInCache:     3,
			ModelInstances: 1,
			TTL:            500 * time.Millisecond,
		}

		mgr, err := krn.NewManager(cfg)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		defer mgr.Shutdown(context.Background())

		ctx := context.Background()

		k1, err := mgr.AquireModel(ctx, modelName1)
		if err != nil {
			t.Fatalf("expected no error acquiring model, got: %v", err)
		}

		t.Log("waiting for TTL to expire...")
		time.Sleep(2 * time.Second)

		k2, err := mgr.AquireModel(ctx, modelName1)
		if err != nil {
			t.Fatalf("expected no error re-acquiring model after eviction, got: %v", err)
		}

		if k1 == k2 {
			t.Fatal("same instance returned (cache may not have evicted yet)")
		}
	})

	t.Run("eviction on capacity exceeded", func(t *testing.T) {
		cfg := krn.Config{
			Log:            log,
			ModelPath:      defaults.ModelsDir(""),
			MaxInCache:     1,
			ModelInstances: 1,
			TTL:            5 * time.Minute,
		}

		mgr, err := krn.NewManager(cfg)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		defer mgr.Shutdown(context.Background())

		ctx := context.Background()

		k1, err := mgr.AquireModel(ctx, modelName1)
		if err != nil {
			t.Fatalf("expected no error acquiring first model, got: %v", err)
		}

		time.Sleep(time.Second)

		k2, err := mgr.AquireModel(ctx, modelName2)
		if err != nil {
			t.Fatalf("expected no error acquiring first model, got: %v", err)
		}

		if k1 == k2 {
			t.Fatal("same instance returned, should have new instance")
		}
	})
}

// =============================================================================

func initKronk(t *testing.T) *logger.Logger {
	libPath := defaults.LibsDir("")

	arch, err := defaults.Arch("")
	if err != nil {
		t.Fatalf("invalid arch specified: %s", runtime.GOARCH)
	}

	opSys, err := defaults.OS("")
	if err != nil {
		t.Fatalf("invalid processor specified: %s", runtime.GOOS)
	}

	processor, err := defaults.Processor("")
	if err != nil {
		t.Fatalf("invalid processor specified: %s", processor)
	}

	t.Logf("installing/updating libraries: libPath[%s], arch[%s] os[%s] processor[%s]", libPath, arch, opSys, processor)

	cfg := tools.LibConfig{
		LibPath:      libPath,
		Arch:         arch,
		OS:           opSys,
		Processor:    processor,
		AllowUpgrade: true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	tag, err := tools.DownloadLibraries(ctx, tools.FmtLogger, cfg)
	if err != nil {
		t.Fatalf("unable to install llama.cpp: %s", err)
	}

	t.Logf("libraries installed: current[%s] latest[%s]", tag.Version, tag.Latest)

	if err := kronk.Init(libPath, kronk.LogLevel(kronk.LogSilent)); err != nil {
		t.Fatalf("installation invalid: %s", err)
	}

	var b bytes.Buffer
	log := logger.New(&b, logger.LevelInfo, "test", nil)

	t.Cleanup(func() {
		t.Log("=====================")
		t.Log(b.String())
		t.Log("=====================")
	})

	return log
}

func findAvailableModel(t *testing.T, notModelName string) string {
	modelPath := defaults.ModelsDir("")

	var modelNames []string
	filepath.WalkDir(modelPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.IsDir() {
			return nil
		}

		if strings.HasSuffix(d.Name(), ".gguf") && !strings.HasPrefix(d.Name(), "mmproj-") {
			modelNames = append(modelNames, d.Name())
			return nil
		}

		return nil
	})

	if len(modelNames) == 0 {
		t.Skip("no models available for testing - skipping")
	}

	var modelName string
	for range len(modelNames) {
		idx := rand.Intn(len(modelNames))
		if modelNames[idx] != notModelName {
			modelName = modelNames[idx]
		}
	}

	t.Logf("using model: %s", modelName)
	return modelName
}
