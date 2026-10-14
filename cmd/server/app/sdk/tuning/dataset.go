package tuning

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
	"strings"
	"time"
)

var ErrNotFound = errors.New("tuning dataset not found")

// ImportDataset synchronously stages, validates and atomically publishes an
// immutable source. The caller owns reader. Extraction is a separate Temporal
// job, so accepting an upload never requires loading a model.
func (b *Business) ImportDataset(ctx context.Context, input ImportInput, reader io.Reader) (Dataset, error) {
	if reader == nil {
		return Dataset{}, fmt.Errorf("%w: source reader is required", ErrInvalidArgument)
	}
	name := strings.TrimSpace(input.Name)
	if name == "" || len(name) > 255 || strings.ContainsAny(name, "/\\\x00") || name == "." || name == ".." {
		return Dataset{}, fmt.Errorf("%w: name must be a filename, not a path", ErrInvalidArgument)
	}
	format := strings.ToLower(filepath.Ext(name))
	switch format {
	case ".pdf", ".docx", ".md", ".txt", ".jsonl":
	default:
		return Dataset{}, fmt.Errorf("%w: unsupported source format %q", ErrInvalidArgument, format)
	}
	if err := ctx.Err(); err != nil {
		return Dataset{}, err
	}
	if err := os.MkdirAll(b.cfg.DatasetRoot, 0o700); err != nil {
		return Dataset{}, err
	}
	root, err := os.OpenRoot(b.cfg.DatasetRoot)
	if err != nil {
		return Dataset{}, err
	}
	defer root.Close()
	if err := root.MkdirAll("imports", 0o700); err != nil {
		return Dataset{}, err
	}
	staging := filepath.Join("imports", ".import-"+rand.Text())
	if err := root.Mkdir(staging, 0o700); err != nil {
		return Dataset{}, err
	}
	defer root.RemoveAll(staging)
	file, err := root.OpenFile(filepath.Join(staging, "source"+format), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return Dataset{}, err
	}
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(contextReader{Context: ctx, Reader: reader}, b.cfg.MaximumImportBytes+1))
	closeErr := file.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return Dataset{}, err
	}
	if size == 0 || size > b.cfg.MaximumImportBytes {
		return Dataset{}, fmt.Errorf("%w: source must contain 1–%d bytes", ErrInvalidArgument, b.cfg.MaximumImportBytes)
	}
	dataset := Dataset{ID: "dataset-" + rand.Text(), Name: name, Format: format, SizeBytes: size, SHA256: hex.EncodeToString(hash.Sum(nil)), CreatedAt: time.Now().UTC()}
	if format == ".jsonl" {
		source, err := root.Open(filepath.Join(staging, "source.jsonl"))
		if err != nil {
			return Dataset{}, err
		}
		records, err := readDatasetRecords(ctx, source)
		closeErr := source.Close()
		if err != nil {
			return Dataset{}, fmt.Errorf("%w: prepared dataset: %s", ErrInvalidArgument, err)
		}
		if closeErr != nil {
			return Dataset{}, closeErr
		}
		if err := TrainingReadiness(records); err != nil {
			return Dataset{}, fmt.Errorf("%w: prepared dataset: %s", ErrInvalidArgument, err)
		}
		dataset.Records = len(records)
	}
	if err := ctx.Err(); err != nil {
		return Dataset{}, err
	}
	if err := writeJSON(root, filepath.Join(staging, "metadata.json"), dataset); err != nil {
		return Dataset{}, err
	}
	if err := root.Rename(staging, filepath.Join("imports", dataset.ID)); err != nil {
		return Dataset{}, err
	}
	return dataset, nil
}

// GetDataset verifies the persisted source identity before returning metadata.
// IDs, not caller-supplied filesystem paths, address imports.
func (b *Business) GetDataset(ctx context.Context, id string) (Dataset, error) {
	if !validDatasetID(id) {
		return Dataset{}, fmt.Errorf("%w: invalid dataset ID", ErrInvalidArgument)
	}
	root, err := os.OpenRoot(b.cfg.DatasetRoot)
	if errors.Is(err, os.ErrNotExist) {
		return Dataset{}, ErrNotFound
	}
	if err != nil {
		return Dataset{}, err
	}
	defer root.Close()
	file, err := root.Open(filepath.Join("imports", id, "metadata.json"))
	if errors.Is(err, os.ErrNotExist) {
		return Dataset{}, ErrNotFound
	}
	if err != nil {
		return Dataset{}, err
	}
	var dataset Dataset
	decodeErr := json.NewDecoder(io.LimitReader(file, 4096)).Decode(&dataset)
	closeErr := file.Close()
	if decodeErr != nil {
		if _, ok := errors.AsType[*os.PathError](decodeErr); ok {
			return Dataset{}, decodeErr
		}
		return Dataset{}, fmt.Errorf("%w: invalid dataset metadata: %s", ErrInvalidArgument, decodeErr)
	}
	if closeErr != nil {
		return Dataset{}, closeErr
	}
	switch dataset.Format {
	case ".pdf", ".docx", ".md", ".txt", ".jsonl":
	default:
		return Dataset{}, fmt.Errorf("%w: invalid persisted dataset format", ErrInvalidArgument)
	}
	if dataset.ID != id {
		return Dataset{}, fmt.Errorf("%w: dataset metadata identity mismatch", ErrInvalidArgument)
	}
	source, err := root.Open(filepath.Join("imports", id, "source"+dataset.Format))
	if errors.Is(err, os.ErrNotExist) {
		return Dataset{}, ErrNotFound
	}
	if err != nil {
		return Dataset{}, err
	}
	defer source.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, io.LimitReader(contextReader{Context: ctx, Reader: source}, b.cfg.MaximumImportBytes+1))
	if err != nil {
		return Dataset{}, err
	}
	if size != dataset.SizeBytes || hex.EncodeToString(hash.Sum(nil)) != dataset.SHA256 {
		return Dataset{}, fmt.Errorf("%w: dataset source integrity check failed", ErrInvalidArgument)
	}
	return dataset, nil
}

// DatasetSource resolves a verified import for preparation. It never accepts
// a caller-supplied filesystem path.
func (b *Business) DatasetSource(ctx context.Context, id string) (PreparationSource, error) {
	dataset, err := b.GetDataset(ctx, id)
	if err != nil {
		return PreparationSource{}, err
	}
	return PreparationSource{ID: dataset.ID, Format: dataset.Format, SHA256: dataset.SHA256}, nil
}

func validDatasetID(id string) bool {
	if !strings.HasPrefix(id, "dataset-") || len(id) != 34 {
		return false
	}
	for _, r := range id[8:] {
		if !(r >= 'A' && r <= 'Z' || r >= '2' && r <= '7') {
			return false
		}
	}
	return true
}
