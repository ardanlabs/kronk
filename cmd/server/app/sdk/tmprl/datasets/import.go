package datasets

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/datasets/store"
	optimization "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/moxiedb"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// Initialize follows Kronk's package-owned index initialization. Schema creation
// belongs to tuning-up. SQL metadata is registered only after file validation.
func (s *Service) Initialize(ctx context.Context) error {
	return s.files.WithLock(ctx, func(ctx context.Context) error {
		if err := s.reconcile(ctx); err != nil {
			return err
		}
		if err := s.files.BuildIndex(ctx); err != nil {
			return err
		}
		if s.db != nil {
			return s.db.MarkCatalogReady(ctx)
		}
		return nil
	})
}

// reconcile never uses the index to authorize a file or hides catalog corruption.
// The caller holds the file store's publication lock through SQL confirmation.
func (s *Service) reconcile(ctx context.Context) error {
	if s.db == nil {
		return nil
	}
	inventory, err := s.files.Inventory(ctx)
	if err != nil {
		return err
	}
	byVersion := make(map[string]store.SnapshotInfo, len(inventory))
	for _, info := range inventory {
		byVersion[versionKey(info.Dataset.ID, info.Dataset.Version)] = info
	}
	catalog, err := s.db.ListDatasets(ctx)
	if err != nil {
		return err
	}
	for _, dataset := range catalog {
		versions, err := s.db.ListVersions(ctx, dataset.ID)
		if err != nil {
			return err
		}
		for _, version := range versions {
			info, ok := byVersion[versionKey(dataset.ID, version.Version)]
			if !ok {
				return fmt.Errorf("catalog snapshot missing: %s/%d", dataset.ID, version.Version)
			}
			if err := s.verifyVersion(version, info); err != nil {
				return err
			}
		}
	}
	for _, info := range inventory {
		_, err := s.db.GetVersion(ctx, info.Dataset.ID, info.Dataset.Version)
		switch {
		case err == nil:
			continue
		case !errors.Is(err, moxiedb.ErrNotFound):
			return err
		}
		publication := moxiedb.Publication{ID: "import/" + versionKey(info.Dataset.ID, info.Dataset.Version), DatasetID: info.Dataset.ID, Kind: "import", InputDigest: info.SHA256}
		if info.Operation != "" {
			pending, err := s.db.GetPublication(ctx, info.Operation)
			switch {
			case err == nil:
				if pending.DatasetID != info.Dataset.ID || pending.InputDigest != info.Digest {
					return fmt.Errorf("%w: snapshot operation identity changed", store.ErrDatasetConflict)
				}
				publication = pending
			case !errors.Is(err, moxiedb.ErrNotFound):
				return err
			}
		}
		if err := s.db.BeginPublication(ctx, publication); err != nil {
			return err
		}
		if err := s.commitInfo(ctx, info, publication.ID); err != nil {
			return err
		}
	}
	pending, err := s.db.PendingPublications(ctx)
	if err != nil {
		return err
	}
	if len(pending) > 0 {
		operation := pending[0]
		if err := s.db.MarkPublication(ctx, operation.ID, "needs_attention", "snapshot_not_published"); err != nil {
			return err
		}
		return fmt.Errorf("dataset publication %s needs attention: no verified snapshot; no files were reset", operation.ID)
	}
	return nil
}

func versionKey(id string, version int) string { return id + "/" + strconv.Itoa(version) }

func (s *Service) verifyVersion(version moxiedb.Version, info store.SnapshotInfo) error {
	want := filepath.ToSlash(filepath.Join(filepath.Base(s.files.Path()), info.Path))
	if version.SnapshotSHA256 != info.SHA256 || version.SnapshotBytes != info.Size || version.SnapshotPath != want {
		return fmt.Errorf("%w: catalog snapshot integrity mismatch: %s/%d", store.ErrDatasetConflict, info.Dataset.ID, info.Dataset.Version)
	}
	return nil
}

func (s *Service) commitInfo(ctx context.Context, info store.SnapshotInfo, publication string) error {
	dataset, version, documents := toDBSnapshot(info, filepath.Base(s.files.Path()), publication)
	return s.db.CommitVersion(ctx, dataset, version, documents)
}

func toDBSnapshot(info store.SnapshotInfo, directory, publication string) (moxiedb.Dataset, moxiedb.Version, []moxiedb.Document) {
	d := info.Dataset
	dataset := moxiedb.Dataset{ID: d.ID, Name: d.Name, CurrentVersion: d.Version, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt}
	version := moxiedb.Version{DatasetID: d.ID, Version: d.Version, SnapshotPath: filepath.ToSlash(filepath.Join(directory, info.Path)), SnapshotSHA256: info.SHA256, SnapshotBytes: info.Size, RecordCount: int64(d.RecordCount), TrainingReady: d.TrainingReady, CreatedAt: d.UpdatedAt, PublicationID: publication}
	documents := make([]moxiedb.Document, 0, len(info.Documents))
	for i, c := range info.Documents {
		documents = append(documents, moxiedb.Document{ID: c.Document.ID, Name: c.Document.Name, SourceSHA256: c.SHA256, Position: i, PageStart: c.Document.PageStart, PageEnd: c.Document.PageEnd, RecordCount: int64(c.Document.RecordCount)})
	}
	return dataset, version, documents
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
		return s.files.RemoveOperation(ctx, id, version, documentID, operation, digest)
	})
}

// Add uses the same stable preparation identity as the job's publication adapter.
func (s *Service) Add(ctx context.Context, id string, version int, addition store.DatasetAddition) (store.Dataset, error) {
	if s.db == nil {
		return s.files.Add(ctx, id, version, addition)
	}
	publication := moxiedb.Publication{ID: "add/" + addition.PublicationID, DatasetID: id, Kind: "add", BaseVersion: version, InputDigest: inputDigest(addition)}
	return s.publish(ctx, publication, func(ctx context.Context) (store.Dataset, error) {
		return s.files.Add(ctx, id, version, addition)
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
