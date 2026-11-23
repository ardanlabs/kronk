// Package datasets provides dataset management and preparation-run operations to callers.
// Jobs own execution; store owns immutable files. This package owns their application-facing composition.
package datasets

import (
	"cmp"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/datasets/store"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	optimization "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/moxiedb"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/uploads"
)

// Config supplies the stores and services the datasets service borrows.
// Job and DB are optional: without Job, runs are unavailable; without DB,
// datasets are read and written as files only.
type Config struct {
	Files *store.Store
	Job   *training.Job
	DB    *moxiedb.DB
	// Uploads names each run's source document.
	Uploads *uploads.Store
	// Namespace selects catalog rows. It defaults to the job's namespace, or
	// "default" when execution is disabled.
	Namespace string
}

// Service connects the host's dataset store and optional application job.
// Dataset management remains available when execution is disabled.
type Service struct {
	files     *store.Store
	job       *training.Job
	db        *moxiedb.DB
	uploads   *uploads.Store
	namespace string
	names     sync.Map // Immutable dataset and upload IDs to display names.
}

// New borrows initialized stores and an optional job; it creates no storage or workers.
func New(cfg Config) *Service {
	namespace := cfg.Namespace
	if namespace == "" && cfg.Job != nil {
		namespace = cfg.Job.Namespace()
	}
	return &Service{files: cfg.Files, job: cfg.Job, db: cfg.DB, uploads: cfg.Uploads, namespace: cmp.Or(namespace, "default")}
}

// PreparationInstructions returns saved server-wide guidance or the built-in default.
func (s *Service) PreparationInstructions(ctx context.Context) (string, error) {
	if s.files == nil {
		return "", training.ErrUnavailable
	}
	instructions, err := s.files.PreparationInstructions(ctx)
	if err != nil {
		return "", err
	}
	return cmp.Or(instructions, preparation.DefaultInstructions), nil
}

// SavePreparationInstructions remembers guidance for future forms. Clearing it
// restores the built-in default; submitted runs keep their own instruction copy.
func (s *Service) SavePreparationInstructions(ctx context.Context, instructions string) (string, error) {
	if s.files == nil {
		return "", training.ErrUnavailable
	}
	instructions = cmp.Or(strings.TrimSpace(instructions), preparation.DefaultInstructions)
	if err := s.files.SavePreparationInstructions(ctx, instructions); err != nil {
		return "", err
	}
	return instructions, nil
}

// Create preserves file-only callers and coordinates a durable create intent in SQL mode.
func (s *Service) Create(ctx context.Context, name string) (store.Dataset, error) {
	if s.db == nil {
		return s.files.Create(ctx, name)
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 200 {
		return store.Dataset{}, fmt.Errorf("%w: dataset name must contain 1–200 bytes", tmprl.ErrInvalidArgument)
	}
	id, operation := "dataset-"+rand.Text(), "create/"+rand.Text()
	return s.publish(ctx, moxiedb.Publication{ID: operation, DatasetID: id, Kind: "create", InputDigest: inputDigest(name)}, func(ctx context.Context) (store.Dataset, error) {
		return s.files.CreateOperation(ctx, id, name, operation, inputDigest(name))
	})
}

func (s *Service) Remove(ctx context.Context, id string, version int, documentID string) (store.Dataset, error) {
	if s.db == nil {
		return s.files.Remove(ctx, id, version, documentID)
	}
	operation := "remove/" + rand.Text()
	digest := inputDigest([]any{id, version, documentID})
	return s.publish(ctx, moxiedb.Publication{ID: operation, DatasetID: id, Kind: "remove", BaseVersion: version, InputDigest: digest}, func(ctx context.Context) (store.Dataset, error) {
		next, err := s.db.NextVersion(ctx, id)
		if err != nil {
			return store.Dataset{}, err
		}
		return s.files.RemoveOperation(ctx, id, version, documentID, operation, digest, next)
	})
}

// Add uses the same stable preparation identity as the job's publication adapter.
func (s *Service) Add(ctx context.Context, id string, version int, addition store.Addition) (store.Dataset, error) {
	if s.db == nil {
		return s.files.Add(ctx, id, version, addition)
	}
	publication := moxiedb.Publication{ID: "add/" + addition.PublicationID, DatasetID: id, Kind: "add", BaseVersion: version, InputDigest: inputDigest(addition)}
	return s.publish(ctx, publication, func(ctx context.Context) (store.Dataset, error) {
		next, err := s.db.NextVersion(ctx, id)
		if err != nil {
			return store.Dataset{}, err
		}
		return s.files.AddAt(ctx, id, version, addition, next)
	})
}

func (s *Service) publish(ctx context.Context, publication moxiedb.Publication, write func(context.Context) (store.Dataset, error)) (store.Dataset, error) {
	var saved store.Dataset
	err := s.files.WithLock(ctx, func(ctx context.Context) error {
		if err := s.reconcile(ctx); err != nil {
			return err
		}
		if err := s.db.BeginPublication(ctx, publication); err != nil {
			return err
		}
		var err error
		saved, err = write(ctx)
		if err != nil {
			if errors.Is(err, tmprl.ErrInvalidArgument) || errors.Is(err, store.ErrDatasetConflict) || errors.Is(err, store.ErrDatasetNotFound) {
				return errors.Join(err, s.db.MarkPublication(ctx, publication.ID, "conflict", "invalid_or_stale_input"))
			}
			return err
		}
		info, err := s.files.Inspect(ctx, saved.ID, saved.Version)
		if err != nil {
			return err
		}
		if err := s.commitInfo(ctx, info, publication.ID); err != nil {
			return err
		}
		if err := s.files.BuildIndex(ctx); err != nil {
			return fmt.Errorf("dataset %s version %d is saved; index refresh failed: %w", saved.ID, saved.Version, err)
		}
		return nil
	})
	return saved, catalogError(err)
}

func (s *Service) Get(ctx context.Context, id string, version int) (store.Dataset, error) {
	if s.db == nil {
		return s.files.Get(ctx, id, version)
	}
	v, err := s.db.GetVersion(ctx, id, version)
	if err != nil {
		return store.Dataset{}, catalogError(err)
	}
	info, err := s.files.Lookup(ctx, id, v.Version)
	if err != nil {
		return store.Dataset{}, err
	}
	if err := s.verifyVersion(v, info); err != nil {
		return store.Dataset{}, err
	}
	versions, err := s.db.ListVersions(ctx, id)
	if err != nil {
		return store.Dataset{}, err
	}
	info.Dataset.Versions = make([]int, 0, len(versions))
	for _, version := range versions {
		info.Dataset.Versions = append(info.Dataset.Versions, version.Version)
	}
	slices.Sort(info.Dataset.Versions)
	return info.Dataset, nil
}

func (s *Service) List(ctx context.Context) ([]store.Dataset, error) {
	if s.db == nil {
		return s.files.List(ctx)
	}
	rows, err := s.db.ListDatasets(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]store.Dataset, 0, len(rows))
	for _, row := range rows {
		dataset, err := s.Get(ctx, row.ID, row.CurrentVersion)
		if err != nil {
			return nil, err
		}
		result = append(result, dataset)
	}
	return result, nil
}

func (s *Service) Records(ctx context.Context, id string, version int) ([]optimization.Record, error) {
	dataset, err := s.Get(ctx, id, version)
	if err != nil {
		return nil, err
	}
	return s.files.Records(ctx, id, dataset.Version)
}

func inputDigest(value any) string {
	data, _ := json.Marshal(value)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func catalogError(err error) error {
	switch {
	case errors.Is(err, moxiedb.ErrNotFound):
		return fmt.Errorf("%w: %w", store.ErrDatasetNotFound, err)
	case errors.Is(err, moxiedb.ErrConflict):
		return fmt.Errorf("%w: %w", store.ErrDatasetConflict, err)
	default:
		return err
	}
}
