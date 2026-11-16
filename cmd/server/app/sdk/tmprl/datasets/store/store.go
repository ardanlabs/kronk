// Package store owns reusable dataset files and immutable versions without job coordination.
package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/uploads"
)

var (
	ErrDatasetNotFound = errors.New("preparation dataset not found")
	ErrDatasetConflict = errors.New("preparation dataset changed; refresh before editing")
)

// These conflicts carry their own reason; errors.Is also matches ErrDatasetConflict.
var (
	ErrDocumentExists error = datasetConflict("this document and page range are already in this dataset; choose a different document, page range, or destination")
	ErrQuestionsExist error = datasetConflict("every prepared record repeats a question already in this dataset")
)

type datasetConflict string

func (e datasetConflict) Error() string { return string(e) }

func (datasetConflict) Is(target error) bool { return target == ErrDatasetConflict }

// Dataset describes one immutable version of a reusable preparation dataset.
type Dataset struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Version       int        `json:"version"`
	RecordCount   int        `json:"record_count"`
	TrainingReady bool       `json:"training_ready"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	Versions      []int      `json:"versions"`
	Documents     []Document `json:"documents"`
}

// Document identifies a prepared contribution, not the original upload.
type Document struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	RecordCount int    `json:"record_count"`
	PageStart   int    `json:"page_start"`
	PageEnd     int    `json:"page_end"`
}

// Addition binds verified records to an idempotent preparation run.
type Addition struct {
	PublicationID string
	Source        uploads.File
	PageStart     int
	PageEnd       int
	Records       []training.Record
}

type datasetContribution struct {
	Document Document          `json:"document"`
	SHA256   string            `json:"sha256"`
	Records  []training.Record `json:"records"`
}

type datasetSnapshot struct {
	Dataset       Dataset               `json:"dataset"`
	Contributions []datasetContribution `json:"contributions"`
	Operation     string                `json:"operation"`
	Digest        string                `json:"digest"`
}

// Store persists immutable snapshots outside individual job workspaces.
// Atomic exclusive links serialize publishers, including different processes.
type Store struct{ root string }

// Config supplies the directory that holds dataset snapshots.
type Config struct {
	Root string
}

// New creates the dataset directory when it does not exist.
func New(cfg Config) (*Store, error) {
	if strings.TrimSpace(cfg.Root) == "" {
		return nil, fmt.Errorf("%w: dataset storage root is required", tmprl.ErrInvalidArgument)
	}
	absolute, err := filepath.Abs(cfg.Root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, err
	}
	return &Store{root: absolute}, nil
}

func (s *Store) Create(ctx context.Context, name string) (Dataset, error) {
	return s.CreateOperation(ctx, "dataset-"+rand.Text(), name, "", "")
}

// CreateOperation recovers a creation using its allocated dataset and operation IDs.
func (s *Store) CreateOperation(ctx context.Context, id, name, operation, digest string) (Dataset, error) {
	return s.write(ctx, func(ctx context.Context) (Dataset, error) { return s.create(ctx, id, name, operation, digest) })
}

// write runs one publication under the store lock and refreshes the index. A caller
// that already holds the lock refreshes the index itself, after its catalog commit.
func (s *Store) write(ctx context.Context, publish func(context.Context) (Dataset, error)) (Dataset, error) {
	maintainIndex := ctx.Value(lockKey{}) != s.root
	var dataset Dataset
	err := s.WithLock(ctx, func(ctx context.Context) error {
		var err error
		dataset, err = publish(ctx)
		if err == nil && maintainIndex {
			err = s.BuildIndex(ctx)
		}
		return err
	})
	return dataset, err
}

func (s *Store) create(ctx context.Context, id, name, operation, digest string) (Dataset, error) {
	name = strings.TrimSpace(name)
	if !ValidID(id) || name == "" || len(name) > 200 {
		return Dataset{}, fmt.Errorf("%w: dataset name must contain 1–200 bytes", tmprl.ErrInvalidArgument)
	}
	if previous, err := s.read(ctx, id, 1); err == nil {
		if operation != "" && previous.Operation == operation && previous.Digest == digest {
			return previous.Dataset, nil
		}
		return Dataset{}, ErrDatasetConflict
	} else if !errors.Is(err, ErrDatasetNotFound) {
		return Dataset{}, err
	}
	now := time.Now().UTC()
	dataset := Dataset{ID: id, Name: name, Version: 1, CreatedAt: now, UpdatedAt: now, Versions: []int{1}, Documents: []Document{}}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return Dataset{}, err
	}
	defer root.Close()
	if err := root.Mkdir(dataset.ID, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return Dataset{}, err
	}
	return dataset, s.publish(ctx, datasetSnapshot{Dataset: dataset, Operation: operation, Digest: digest})
}

func (s *Store) List(ctx context.Context) ([]Dataset, error) {
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	directory, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	entries, err := directory.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	datasets := []Dataset{}
	for _, entry := range entries {
		if ValidID(entry.Name()) && entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("dataset directory must not be a symbolic link: %s", entry.Name())
		}
		if !entry.IsDir() || !ValidID(entry.Name()) {
			continue
		}
		dataset, err := s.Get(ctx, entry.Name(), 0)
		if errors.Is(err, ErrDatasetNotFound) {
			continue // A creator has not published its first snapshot yet.
		}
		if err != nil {
			return nil, err
		}
		datasets = append(datasets, dataset)
	}
	slices.SortFunc(datasets, func(a, b Dataset) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
	return datasets, nil
}

// Get opens latest when version is zero, or an exact historical snapshot.
func (s *Store) Get(ctx context.Context, id string, version int) (Dataset, error) {
	snapshot, err := s.read(ctx, id, version)
	return snapshot.Dataset, err
}

// Records returns a copy decoded from the requested immutable snapshot.
func (s *Store) Records(ctx context.Context, id string, version int) ([]training.Record, error) {
	snapshot, err := s.read(ctx, id, version)
	if err != nil {
		return nil, err
	}
	return snapshot.records(), nil
}

// Add publishes only this document's verified contribution. Concurrent additions
// merge against the latest version; removed documents are never resurrected.
func (s *Store) Add(ctx context.Context, id string, baseVersion int, addition Addition) (Dataset, error) {
	return s.write(ctx, func(ctx context.Context) (Dataset, error) { return s.add(ctx, id, baseVersion, addition) })
}

func (s *Store) add(ctx context.Context, id string, baseVersion int, addition Addition) (Dataset, error) {
	if addition.PublicationID == "" || addition.Source.ID == "" || addition.Source.Name == "" || baseVersion < 1 || len(addition.Records) == 0 {
		return Dataset{}, fmt.Errorf("%w: prepared document, records, and base version are required", tmprl.ErrInvalidArgument)
	}
	if !validSHA(addition.Source.SHA256) || !((addition.PageStart == 0 && addition.PageEnd == 0) || (addition.PageStart > 0 && addition.PageEnd >= addition.PageStart)) {
		return Dataset{}, fmt.Errorf("%w: valid source SHA-256 and paired page bounds are required", tmprl.ErrInvalidArgument)
	}
	for _, record := range addition.Records {
		if record.SourceDocument != addition.Source.ID {
			return Dataset{}, fmt.Errorf("%w: record belongs to a different source document", tmprl.ErrInvalidArgument)
		}
	}
	if err := training.ValidateRecords(addition.Records); err != nil {
		return Dataset{}, fmt.Errorf("%w: %s", tmprl.ErrInvalidArgument, err)
	}
	data, err := json.Marshal(addition)
	if err != nil {
		return Dataset{}, err
	}
	hash := sha256.Sum256(data)
	digest, operation := hex.EncodeToString(hash[:]), "add/"+addition.PublicationID
	for range 10 {
		snapshot, err := s.read(ctx, id, 0)
		if err != nil {
			return Dataset{}, err
		}
		for _, version := range snapshot.Dataset.Versions {
			previous, err := s.read(ctx, id, version)
			if err != nil {
				return Dataset{}, err
			}
			if previous.Operation == operation {
				if previous.Digest != digest {
					return Dataset{}, fmt.Errorf("%w: publication input changed", ErrDatasetConflict)
				}
				return previous.Dataset, nil
			}
		}
		if baseVersion > snapshot.Dataset.Version {
			return Dataset{}, ErrDatasetConflict
		}
		for _, contribution := range snapshot.Contributions {
			if sameSource(contribution.SHA256, contribution.Document, addition.Source.SHA256, addition.PageStart, addition.PageEnd) {
				return Dataset{}, ErrDocumentExists
			}
		}
		// A question already in the dataset is skipped, not saved twice.
		existing := make(map[string]bool)
		for _, record := range snapshot.records() {
			existing[training.Key(record.Prompt)] = true
		}
		records := slices.DeleteFunc(slices.Clone(addition.Records), func(r training.Record) bool { return existing[training.Key(r.Prompt)] })
		if len(records) == 0 {
			return Dataset{}, ErrQuestionsExist
		}
		document := Document{ID: addition.PublicationID, Name: addition.Source.Name, RecordCount: len(records), PageStart: addition.PageStart, PageEnd: addition.PageEnd}
		snapshot.Contributions = append(snapshot.Contributions, datasetContribution{Document: document, SHA256: addition.Source.SHA256, Records: records})
		if err := training.ValidateRecords(snapshot.records()); err != nil {
			return Dataset{}, fmt.Errorf("%w: combined dataset: %s", ErrDatasetConflict, err)
		}
		snapshot.advance(operation, digest)
		if err := s.publish(ctx, snapshot); errors.Is(err, ErrDatasetConflict) {
			continue
		} else if err != nil {
			return Dataset{}, err
		}
		return snapshot.Dataset, nil
	}
	return Dataset{}, ErrDatasetConflict
}

// Remove excludes a contribution without touching uploads, historical versions,
// or trained adapters. The base version prevents stale destructive edits.
func (s *Store) Remove(ctx context.Context, id string, version int, documentID string) (Dataset, error) {
	return s.RemoveOperation(ctx, id, version, documentID, "remove/"+documentID, "")
}

// RemoveOperation records the exact operation identity for publication recovery.
func (s *Store) RemoveOperation(ctx context.Context, id string, version int, documentID, operation, digest string) (Dataset, error) {
	return s.write(ctx, func(ctx context.Context) (Dataset, error) {
		return s.remove(ctx, id, version, documentID, operation, digest)
	})
}

func (s *Store) remove(ctx context.Context, id string, version int, documentID, operation, digest string) (Dataset, error) {
	snapshot, err := s.read(ctx, id, 0)
	if err != nil {
		return Dataset{}, err
	}
	if version < 1 || version != snapshot.Dataset.Version {
		return Dataset{}, ErrDatasetConflict
	}
	index := slices.IndexFunc(snapshot.Contributions, func(c datasetContribution) bool { return c.Document.ID == documentID })
	if index < 0 {
		return Dataset{}, ErrDatasetNotFound
	}
	snapshot.Contributions = slices.Delete(snapshot.Contributions, index, index+1)
	snapshot.advance(operation, digest)
	return snapshot.Dataset, s.publish(ctx, snapshot)
}

func sameSource(sha string, document Document, source string, pageStart, pageEnd int) bool {
	return sha == source && document.PageStart == pageStart && document.PageEnd == pageEnd
}

func (snapshot datasetSnapshot) records() []training.Record {
	records := []training.Record{}
	for _, contribution := range snapshot.Contributions {
		records = append(records, contribution.Records...)
	}
	return records
}

func (snapshot *datasetSnapshot) advance(operation, digest string) {
	snapshot.Dataset.Version++
	snapshot.Dataset.UpdatedAt = time.Now().UTC()
	snapshot.Dataset.Versions = append(snapshot.Dataset.Versions, snapshot.Dataset.Version)
	snapshot.Dataset.Documents = []Document{}
	for _, contribution := range snapshot.Contributions {
		snapshot.Dataset.Documents = append(snapshot.Dataset.Documents, contribution.Document)
	}
	records := snapshot.records()
	snapshot.Dataset.RecordCount = len(records)
	snapshot.Dataset.TrainingReady = training.TrainingReadiness(records) == nil
	snapshot.Operation, snapshot.Digest = operation, digest
}

func (s *Store) read(ctx context.Context, id string, version int) (datasetSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return datasetSnapshot{}, err
	}
	if !ValidID(id) || version < 0 {
		return datasetSnapshot{}, fmt.Errorf("%w: invalid dataset identity or version", tmprl.ErrInvalidArgument)
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return datasetSnapshot{}, err
	}
	defer root.Close()
	dir, err := root.Open(id)
	if errors.Is(err, os.ErrNotExist) {
		return datasetSnapshot{}, ErrDatasetNotFound
	}
	if err != nil {
		return datasetSnapshot{}, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return datasetSnapshot{}, err
	}
	versions := []int{}
	for _, entry := range entries {
		if number, err := strconv.Atoi(strings.TrimSuffix(entry.Name(), ".json")); err == nil && number > 0 && strings.HasSuffix(entry.Name(), ".json") {
			versions = append(versions, number)
		}
	}
	if len(versions) == 0 {
		return datasetSnapshot{}, ErrDatasetNotFound
	}
	slices.Sort(versions)
	if version == 0 {
		version = versions[len(versions)-1]
	}
	file, err := root.Open(filepath.Join(id, strconv.Itoa(version)+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return datasetSnapshot{}, ErrDatasetNotFound
	}
	if err != nil {
		return datasetSnapshot{}, err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return datasetSnapshot{}, err
	}
	if !stat.Mode().IsRegular() || stat.Size() > 100<<20 {
		return datasetSnapshot{}, fmt.Errorf("dataset snapshot must be a regular file no larger than 100 MiB")
	}
	var snapshot datasetSnapshot
	decoder := json.NewDecoder(io.LimitReader(file, (100<<20)+1))
	if err := decoder.Decode(&snapshot); err != nil {
		return datasetSnapshot{}, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return datasetSnapshot{}, fmt.Errorf("dataset snapshot has trailing or oversized content")
	}
	if snapshot.Dataset.ID != id || snapshot.Dataset.Version != version {
		return datasetSnapshot{}, fmt.Errorf("dataset snapshot identity mismatch")
	}
	snapshot.Dataset.Versions = versions
	return snapshot, nil
}

func (s *Store) publish(ctx context.Context, snapshot datasetSnapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	if len(data) > 100<<20 {
		return fmt.Errorf("%w: dataset version exceeds 100 MiB", tmprl.ErrInvalidArgument)
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return err
	}
	defer root.Close()
	staging := filepath.Join(snapshot.Dataset.ID, ".pending-"+rand.Text())
	file, err := root.OpenFile(staging, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer root.Remove(staging)
	_, writeErr := file.Write(data)
	err = errors.Join(writeErr, file.Sync(), file.Close())
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	err = root.Link(staging, filepath.Join(snapshot.Dataset.ID, strconv.Itoa(snapshot.Dataset.Version)+".json"))
	if errors.Is(err, os.ErrExist) {
		return ErrDatasetConflict
	}
	if err != nil {
		return err
	}
	dir, err := root.Open(snapshot.Dataset.ID)
	if err != nil {
		return err
	}
	err = errors.Join(dir.Sync(), dir.Close())
	if err != nil {
		return err
	}
	// Persist a newly created dataset directory before a caller commits SQL.
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	return errors.Join(directory.Sync(), directory.Close())
}

// ValidID checks the dataset identity format accepted by storage and preparation.
func ValidID(id string) bool {
	if !strings.HasPrefix(id, "dataset-") || len(id) < 9 || len(id) > 128 {
		return false
	}
	for _, char := range id {
		if char != '-' && (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') {
			return false
		}
	}
	return true
}
