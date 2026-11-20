package moxierun

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/datasets"
	datasetstore "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/datasets/store"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/moxiedb"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/providers"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/uploads"
	"github.com/ardanlabs/kronk/sdk/kronk/pool"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"go.temporal.io/sdk/client"
)

// Config supplies Moxie settings and borrowed model resources. Root defaults to
// ~/.moxie. An empty DatabaseURL retains file-only storage.
type Config struct {
	Address      string
	Namespace    string
	Models       []string
	Root         string
	DatabaseURL  string
	ModelCatalog *models.Models
	ModelPool    *pool.Pool
}

// Services contains the services owned by a Runtime and borrowed by its callers.
// They remain valid until the runtime is closed.
type Services struct {
	Job      *training.Job
	Uploads  *uploads.Store
	Datasets *datasets.Service
}

// start constructs services and workers, undoing partial initialization on error.
// serviceCtx owns background work beyond the initialization request.
func start(ctx, serviceCtx context.Context, log *logger.Logger, cfg Config) (_ Services, stop func(), err error) {
	if err := validateConfig(cfg); err != nil {
		return Services{}, func() {}, err
	}

	var cleanup []func()
	stop = func() {
		for _, c := range slices.Backward(cleanup) {
			c()
		}
	}

	defer func() {
		if err != nil {
			stop()
			stop = func() {}
		}
	}()

	root, err := storageRoot(cfg.Root)
	if err != nil {
		return Services{}, stop, err
	}

	var database *moxiedb.DB
	if cfg.DatabaseURL != "" {
		database, err = moxiedb.Open(ctx, cfg.DatabaseURL)
		if err != nil {
			return Services{}, stop, fmt.Errorf("opening Moxie database: %w", err)
		}
		cleanup = append(cleanup, func() { database.Close() })
		if err := database.CheckSchema(ctx); err != nil {
			return Services{}, stop, fmt.Errorf("checking Moxie schema; run make moxie-up: %w", err)
		}
	}

	store, err := uploads.New(uploads.Config{Root: filepath.Join(root, "uploads")})
	if err != nil {
		return Services{}, stop, fmt.Errorf("initializing training uploads: %w", err)
	}

	datasetFiles, err := datasetstore.New(datasetstore.Config{Root: filepath.Join(root, "preparation-datasets")})
	if err != nil {
		return Services{}, stop, fmt.Errorf("initializing preparation datasets: %w", err)
	}

	services := Services{Uploads: store}
	approvedModels := make(map[string]string, len(cfg.Models))
	for _, id := range cfg.Models {
		approvedModels[id] = ""
	}

	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	temporalClient, err := client.DialContext(dialCtx, client.Options{HostPort: cfg.Address, Namespace: cfg.Namespace})
	cancel()
	if err != nil {
		return Services{}, stop, fmt.Errorf("connecting training Temporal client: %w", err)
	}
	cleanup = append(cleanup, temporalClient.Close)

	extractor, err := providers.NewDoclingExtractor("", nil)
	if err != nil {
		return Services{}, stop, fmt.Errorf("initializing document extractor: %w", err)
	}

	job, err := training.New(temporalClient, training.Config{
		Uploads: store, Datasets: datasetFiles, MoxieDB: database, StorageRoot: root,
		Namespace: cfg.Namespace,
		Models:    approvedModels, ModelCatalog: cfg.ModelCatalog, ModelPool: cfg.ModelPool, Extractor: extractor, Log: log.Info,
	})
	if err != nil {
		return Services{}, stop, fmt.Errorf("initializing training job: %w", err)
	}
	trainingWorkers := job.Workers()
	services.Job = job
	services.Datasets = datasets.New(datasets.Config{
		Files: datasetFiles, Job: services.Job, DB: database, Uploads: store, Namespace: cfg.Namespace,
	})

	// A damaged dataset version affects only that dataset.
	if err := services.Datasets.Initialize(ctx); err != nil {
		if moxiedb.IsUnavailable(err) || ctx.Err() != nil {
			return Services{}, stop, fmt.Errorf("initializing dataset catalog: %w", err)
		}
		log.Error(ctx, "startup", "status", "initializing datasets", "ERROR", err)
	}

	for _, w := range trainingWorkers {
		if err := ctx.Err(); err != nil {
			return Services{}, stop, err
		}
		if err := w.Start(); err != nil {
			w.Stop()
			return Services{}, stop, fmt.Errorf("starting training worker: %w", err)
		}
		cleanup = append(cleanup, w.Stop)
	}

	if len(trainingWorkers) > 0 {
		cleanup = append(cleanup, func() {
			// Running preparation units finish and save before the worker stops,
			// within the 120 seconds SageMaker and Unsloth allow (the
			// systemd/launchd stop limit is 150). Unsaved units repeat after restart.
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()
			log.Info(ctx, "shutdown", "status", "saving preparation work")
			if err := services.Job.Jobs().Drain(ctx); err != nil {
				log.Error(ctx, "shutdown", "status", "preparation save incomplete; unsaved units repeat after restart", "ERROR", err)
			}
		})
		log.Info(ctx, "startup", "status", "training worker started", "namespace", cfg.Namespace)
	}

	if database != nil && services.Job != nil {
		catalogCtx, cancelCatalog := context.WithCancel(serviceCtx)
		catalogDone := make(chan struct{})
		go func() {
			defer close(catalogDone)
			services.Job.RunCatalog(catalogCtx)
		}()
		cleanup = append(cleanup, func() {
			cancelCatalog()
			<-catalogDone
		})
	}

	return services, stop, nil
}

func validateConfig(cfg Config) error {
	if strings.TrimSpace(cfg.Address) == "" || strings.TrimSpace(cfg.Namespace) == "" || len(cfg.Models) == 0 {
		return errors.New("configuration: enabled Moxie requires a Temporal address, namespace, and approved model IDs")
	}
	return nil
}

// storageRoot preserves the existing default and resolves one root for all owners.
func storageRoot(root string) (string, error) {
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("locating Moxie root: %w", err)
		}
		root = filepath.Join(home, ".moxie")
	}
	if strings.TrimSpace(root) == "" {
		return "", errors.New("configuration: Moxie root must not be blank")
	}
	return filepath.Abs(root)
}
