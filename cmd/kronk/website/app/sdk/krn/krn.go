// Package krn manages the creation and unloading of kronk APIs for
// specific models.
package krn

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/ardanlabs/kronk"
	"github.com/ardanlabs/kronk/cmd/kronk/website/foundation/logger"
	"github.com/ardanlabs/kronk/defaults"
	"github.com/ardanlabs/kronk/model"
	"github.com/ardanlabs/kronk/tools"
	"github.com/hybridgroup/yzma/pkg/download"
	"github.com/maypok86/otter/v2"
)

// Config represents setting for the kronk manager.
//
// ModelPath: Location of models. Leave empty for default location.
//
// Device: Specify a specific device. To see the list of devices run this command:
// $HOME/kronk/libraries/llama-bench --list-devices
// Leave empty for the system to pick the device.
//
// MaxInCache: Defines the maximum number of unique models will be available at a
// time. Defaults to 3 if the value is 0.
//
// ModelInstances: Defines how many instances of the same model should be
// loaded. Defaults to 1 if the value is 0.
//
// ContextWindow: Sets the global context window for all models. Defaults to
// what is in the model metadata if set to 0. If no metadata is found, 4096
// is the default.
//
// TTL: Defines the time an existing model can live in the cache without
// being used.
type Config struct {
	Log            *logger.Logger
	LibPath        string
	Arch           download.Arch
	OS             download.OS
	Processor      download.Processor
	ModelPath      string
	Device         string
	MaxInCache     int
	ModelInstances int
	ContextWindow  int
	CacheTTL       time.Duration
}

func validateConfig(cfg Config) Config {
	cfg.LibPath = defaults.ModelsDir(cfg.LibPath)
	cfg.ModelPath = defaults.ModelsDir(cfg.ModelPath)

	if cfg.MaxInCache <= 0 {
		cfg.MaxInCache = 3
	}

	if cfg.ModelInstances <= 0 {
		cfg.ModelInstances = 1
	}

	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = 5 * time.Minute
	}

	return cfg
}

// Manager manages a set of Kronk APIs for use. It maintains a cache of these
// APIs and will unload over time if not in use.
type Manager struct {
	log           *logger.Logger
	libPath       string
	arch          download.Arch
	os            download.OS
	processor     download.Processor
	modelPath     string
	device        string
	instances     int
	contextWindow int
	cache         *otter.Cache[string, *kronk.Kronk]
	itemsInCache  atomic.Int32
}

// NewManager constructs the manager for use.
func NewManager(cfg Config) (*Manager, error) {
	cfg = validateConfig(cfg)

	mgr := Manager{
		log:           cfg.Log,
		arch:          cfg.Arch,
		os:            cfg.OS,
		processor:     cfg.Processor,
		libPath:       cfg.LibPath,
		modelPath:     cfg.ModelPath,
		device:        cfg.Device,
		instances:     cfg.ModelInstances,
		contextWindow: cfg.ContextWindow,
	}

	opt := otter.Options[string, *kronk.Kronk]{
		MaximumSize:      cfg.MaxInCache,
		ExpiryCalculator: otter.ExpiryAccessing[string, *kronk.Kronk](cfg.CacheTTL),
		OnDeletion:       mgr.eviction,
	}

	cache, err := otter.New(&opt)
	if err != nil {
		return nil, fmt.Errorf("constructing cache: %w", err)
	}

	mgr.cache = cache

	return &mgr, nil
}

// Shutdown releases all apis from the cache and performs a proper unloading.
func (mgr *Manager) Shutdown(ctx context.Context) error {
	if _, exists := ctx.Deadline(); !exists {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
	}

	mgr.cache.InvalidateAll()

	for mgr.itemsInCache.Load() > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-time.NewTimer(100 * time.Millisecond).C:
		}
	}

	return nil
}

// LibPath returns the location of the llama.cpp libraries.
func (mgr *Manager) LibPath() string {
	return mgr.libPath
}

// ModelPath returns the location of the models.
func (mgr *Manager) ModelPath() string {
	return mgr.modelPath
}

// Arch returns the hardware being used.
func (mgr *Manager) Arch() download.Arch {
	return mgr.arch
}

// OS returns the operating system being used.
func (mgr *Manager) OS() download.OS {
	return mgr.os
}

// Processor returns the processor being used.
func (mgr *Manager) Processor() download.Processor {
	return mgr.processor
}

// AquireModel will provide a kronk API for the specified model. If the model
// is not in the cache, an API for the model will be created.
func (mgr *Manager) AquireModel(ctx context.Context, modelName string) (*kronk.Kronk, error) {
	krn, exists := mgr.cache.GetIfPresent(modelName)
	if exists {
		return krn, nil
	}

	fi, err := tools.FindModel(mgr.modelPath, modelName)
	if err != nil {
		return nil, fmt.Errorf("aquire-model: %w", err)
	}

	krn, err = kronk.New(mgr.instances, model.Config{
		ModelFile:      fi.ModelFile,
		ProjectionFile: fi.ProjFile,
		Device:         mgr.device,
		ContextWindow:  mgr.contextWindow,
	})

	if err != nil {
		return nil, fmt.Errorf("unable to create inference model: %w", err)
	}

	mgr.cache.Set(modelName, krn)
	mgr.itemsInCache.Add(1)

	totalEntries := len(krn.SystemInfo())*2 + (5 * 2)
	info := make([]any, 0, totalEntries)
	for k, v := range krn.SystemInfo() {
		info = append(info, k)
		info = append(info, v)
	}

	info = append(info, "status")
	info = append(info, "kronk cache add")
	info = append(info, "model-name")
	info = append(info, modelName)
	info = append(info, "contextWindow")
	info = append(info, krn.ModelConfig().ContextWindow)
	info = append(info, "isGPTModel")
	info = append(info, krn.ModelInfo().IsGPTModel)
	info = append(info, "isEmbedModel")
	info = append(info, krn.ModelInfo().IsEmbedModel)

	mgr.log.Info(ctx, "acquire-model", info...)

	return krn, nil
}

func (mgr *Manager) eviction(event otter.DeletionEvent[string, *kronk.Kronk]) {
	const unloadTimeout = 5 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), unloadTimeout)
	defer cancel()

	mgr.log.Info(ctx, "kronk cache eviction", "key", event.Key, "cause", event.Cause, "was-evicted", event.WasEvicted())
	if err := event.Value.Unload(ctx); err != nil {
		mgr.log.Info(ctx, "kronk cache eviction", "key", event.Key, "ERROR", err)
	}

	mgr.itemsInCache.Add(-1)
}
