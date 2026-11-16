package datasets

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/datasets/store"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/moxiedb"
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
