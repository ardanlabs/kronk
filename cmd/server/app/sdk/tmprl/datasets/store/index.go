package store

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/gofrs/flock"
	"go.yaml.in/yaml/v3"
)

// SnapshotInfo describes verified immutable files, never SQL rows or record bodies.
type SnapshotInfo struct {
	Dataset                         Dataset
	Path, SHA256, Operation, Digest string
	Size                            int64
	Documents                       []DocumentInfo
}

type DocumentInfo struct {
	Document Document
	SHA256   string
}

// HasDocument reports whether this version already contains the exact source
// document and page range.
func (info SnapshotInfo) HasDocument(sha string, pageStart, pageEnd int) bool {
	return slices.ContainsFunc(info.Documents, func(d DocumentInfo) bool { return sameSource(d.SHA256, d.Document, sha, pageStart, pageEnd) })
}

type indexVersion struct {
	Snapshot string `yaml:"snapshot"`
	SHA256   string `yaml:"sha256"`
	Size     int64  `yaml:"size_bytes"`
}

type indexDataset struct {
	Latest   int                  `yaml:"latest_file_version"`
	Versions map[int]indexVersion `yaml:"versions"`
}

type fileIndex struct {
	Version  int                     `yaml:"schema_version"`
	Datasets map[string]indexDataset `yaml:"datasets"`
}

type lockKey struct{}

// Path is the already-resolved dataset directory, not a second Moxie root.
func (s *Store) Path() string { return s.root }

// WithLock serializes file publication, index rebuild and caller coordination
// across processes. The context permits nested operations on the same store.
// OS locks are released on process exit, including unexpected shutdown.
// Nested writers defer index maintenance to this caller, after catalog commit.
func (s *Store) WithLock(ctx context.Context, fn func(context.Context) error) error {
	if ctx.Value(lockKey{}) == s.root {
		return fn(ctx)
	}
	lock := flock.New(filepath.Join(s.root, ".publication.lock"))
	defer lock.Close()
	ok, err := lock.TryLockContext(ctx, 20*time.Millisecond)
	if err != nil {
		return err
	}
	if !ok {
		return ctx.Err()
	}
	return fn(context.WithValue(ctx, lockKey{}, s.root))
}

// Inspect validates an exact immutable snapshot and calculates its file digest.
func (s *Store) Inspect(ctx context.Context, id string, version int) (SnapshotInfo, error) {
	snapshot, err := s.read(ctx, id, version)
	if err != nil {
		return SnapshotInfo{}, err
	}
	dataset := snapshot.Dataset
	if dataset.Name == "" || len(dataset.Name) > 200 || dataset.CreatedAt.IsZero() || dataset.UpdatedAt.IsZero() || len(dataset.Documents) != len(snapshot.Contributions) {
		return SnapshotInfo{}, fmt.Errorf("invalid dataset metadata: %s/%d", id, version)
	}
	info := SnapshotInfo{Dataset: dataset, Path: filepath.ToSlash(filepath.Join(id, strconv.Itoa(dataset.Version)+".json")), Operation: snapshot.Operation, Digest: snapshot.Digest}
	for i, contribution := range snapshot.Contributions {
		doc := contribution.Document
		if doc != dataset.Documents[i] || doc.RecordCount != len(contribution.Records) || doc.ID == "" || doc.Name == "" || !validSHA(contribution.SHA256) || !((doc.PageStart == 0 && doc.PageEnd == 0) || (doc.PageStart > 0 && doc.PageEnd >= doc.PageStart)) {
			return SnapshotInfo{}, fmt.Errorf("invalid dataset contribution: %s/%d", id, version)
		}
		info.Documents = append(info.Documents, DocumentInfo{Document: doc, SHA256: contribution.SHA256})
	}
	records := snapshot.records()
	if training.ValidateRecords(records) != nil {
		return SnapshotInfo{}, fmt.Errorf("invalid dataset records: %s/%d", id, version)
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return SnapshotInfo{}, err
	}
	defer root.Close()
	file, err := root.Open(info.Path)
	if err != nil {
		return SnapshotInfo{}, err
	}
	defer file.Close()
	hash := sha256.New()
	info.Size, err = io.Copy(hash, file)
	info.SHA256 = hex.EncodeToString(hash.Sum(nil))
	return info, err
}

// Lookup reads the generated inventory but verifies the selected immutable file.
// The index never authorizes a version; SQL callers pin their published version.
// Missing, malformed and stale inventories are rebuilt, not treated as no data.
func (s *Store) Lookup(ctx context.Context, id string, version int) (SnapshotInfo, error) {
	var info SnapshotInfo
	err := s.WithLock(ctx, func(ctx context.Context) error {
		var err error
		info, err = s.Inspect(ctx, id, version)
		if err != nil {
			return err
		}
		root, err := os.OpenRoot(s.root)
		if err != nil {
			return err
		}
		defer root.Close()
		data, readErr := root.ReadFile(".index.yaml")
		var index fileIndex
		decodeErr := yaml.Unmarshal(data, &index)
		entry := index.Datasets[id]
		indexed := entry.Versions[info.Dataset.Version]
		versions := info.Dataset.Versions
		if readErr == nil && decodeErr == nil && index.Version == 1 && entry.Latest == versions[len(versions)-1] && len(entry.Versions) == len(versions) && indexed.Snapshot == info.Path && indexed.SHA256 == info.SHA256 && indexed.Size == info.Size {
			return nil
		}
		return s.BuildIndex(ctx)
	})
	return info, err
}

// Inventory reads snapshots themselves; an old or missing index is never authority.
func (s *Store) Inventory(ctx context.Context) ([]SnapshotInfo, error) {
	datasets, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(datasets, func(a, b Dataset) int { return cmp.Compare(a.ID, b.ID) })
	var result []SnapshotInfo
	for _, dataset := range datasets {
		for _, version := range dataset.Versions {
			info, err := s.Inspect(ctx, dataset.ID, version)
			if err != nil {
				return nil, err
			}
			result = append(result, info)
		}
	}
	return result, nil
}

// BuildIndex follows Kronk's generated inventory/atomic rename pattern, with
// the dataset publisher's cross-process lock and durable writes.
func (s *Store) BuildIndex(ctx context.Context) error {
	return s.WithLock(ctx, func(ctx context.Context) error {
		inventory, err := s.Inventory(ctx)
		if err != nil {
			return err
		}
		index := fileIndex{Version: 1, Datasets: make(map[string]indexDataset)}
		for _, info := range inventory {
			entry := index.Datasets[info.Dataset.ID]
			if entry.Versions == nil {
				entry.Versions = make(map[int]indexVersion)
			}
			entry.Latest = max(entry.Latest, info.Dataset.Version)
			entry.Versions[info.Dataset.Version] = indexVersion{Snapshot: info.Path, SHA256: info.SHA256, Size: info.Size}
			index.Datasets[info.Dataset.ID] = entry
		}
		data, err := yaml.Marshal(index)
		if err != nil {
			return err
		}
		file, err := os.CreateTemp(s.root, ".index-*.tmp")
		if err != nil {
			return err
		}
		defer os.Remove(file.Name())
		_, writeErr := file.Write(data)
		if err := errors.Join(writeErr, file.Sync(), file.Close()); err != nil {
			return err
		}
		if err := os.Rename(file.Name(), filepath.Join(s.root, ".index.yaml")); err != nil {
			return err
		}
		dir, err := os.Open(s.root)
		if err != nil {
			return err
		}
		return errors.Join(dir.Sync(), dir.Close())
	})
}

func validSHA(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}
