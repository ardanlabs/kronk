// Package uploads stores immutable job inputs in confined local storage.
package uploads

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

// ErrNotFound identifies a missing upload.
var ErrNotFound = errors.New("upload not found")

// ErrInvalidArgument identifies invalid arguments or persisted data.
var ErrInvalidArgument = errors.New("invalid upload argument")

// Config specifies the storage root and maximum size of each upload.
type Config struct {
	Root         string
	MaximumBytes int64
}

// Input identifies the filename of an uploaded source.
type Input struct {
	Name string `json:"name"`
}

// File identifies an immutable upload whose contents remain in managed storage.
type File struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Format    string    `json:"format"`
	SizeBytes int64     `json:"size_bytes"`
	SHA256    string    `json:"sha256"`
	CreatedAt time.Time `json:"created_at"`
}

// Store owns uploaded files and their confined local storage.
type Store struct {
	cfg Config
}

// New constructs a store with a required root and a limit of at most 100 MiB.
// A zero MaximumBytes selects the default limit of 100 MiB.
func New(cfg Config) (*Store, error) {
	if strings.TrimSpace(cfg.Root) == "" {
		return nil, fmt.Errorf("%w: root is required", ErrInvalidArgument)
	}
	root, err := filepath.Abs(cfg.Root)
	if err != nil {
		return nil, fmt.Errorf("resolving upload root: %w", err)
	}
	cfg.Root = root
	if cfg.MaximumBytes == 0 {
		cfg.MaximumBytes = 100 << 20
	}
	if cfg.MaximumBytes < 1 || cfg.MaximumBytes > 100<<20 {
		return nil, fmt.Errorf("%w: limit must be 1–104857600 bytes", ErrInvalidArgument)
	}
	s := Store{cfg: cfg}
	return &s, nil
}

// MaximumBytes returns the maximum accepted source size.
func (s *Store) MaximumBytes() int64 { return s.cfg.MaximumBytes }

// Put stages and atomically publishes an upload without interpreting its contents.
// The caller owns reader. Cancellation is checked between reads; it cannot
// interrupt a reader blocked inside Read.
func (s *Store) Put(ctx context.Context, input Input, reader io.Reader) (result File, err error) {
	if reader == nil {
		return File{}, fmt.Errorf("%w: source reader is required", ErrInvalidArgument)
	}
	name := strings.TrimSpace(input.Name)
	if !validName(name) {
		return File{}, fmt.Errorf("%w: name must be a filename, not a path", ErrInvalidArgument)
	}
	if err := ctx.Err(); err != nil {
		return File{}, err
	}
	if err := os.MkdirAll(s.cfg.Root, 0o700); err != nil {
		return File{}, fmt.Errorf("creating upload root: %w", err)
	}
	root, err := os.OpenRoot(s.cfg.Root)
	if err != nil {
		return File{}, fmt.Errorf("opening upload root: %w", err)
	}
	defer root.Close()
	if err := root.MkdirAll("uploads", 0o700); err != nil {
		return File{}, fmt.Errorf("creating uploads directory: %w", err)
	}
	staging := filepath.Join("uploads", ".upload-"+rand.Text())
	if err := root.Mkdir(staging, 0o700); err != nil {
		return File{}, fmt.Errorf("creating upload staging: %w", err)
	}
	defer func() {
		err = errors.Join(err, root.RemoveAll(staging))
	}()
	format := strings.ToLower(filepath.Ext(name))
	source, err := root.OpenFile(filepath.Join(staging, "source"+format), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return File{}, fmt.Errorf("creating upload source: %w", err)
	}
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(source, hash), io.LimitReader(contextReader{ctx: ctx, reader: reader}, s.cfg.MaximumBytes+1))
	if err := errors.Join(copyErr, source.Close()); err != nil {
		return File{}, fmt.Errorf("writing upload source: %w", err)
	}
	if size == 0 || size > s.cfg.MaximumBytes {
		return File{}, fmt.Errorf("%w: source must contain 1–%d bytes", ErrInvalidArgument, s.cfg.MaximumBytes)
	}
	file := File{ID: "upload-" + rand.Text(), Name: name, Format: format, SizeBytes: size, SHA256: hex.EncodeToString(hash.Sum(nil)), CreatedAt: time.Now().UTC()}
	metadata, err := root.OpenFile(filepath.Join(staging, "metadata.json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return File{}, fmt.Errorf("creating upload metadata: %w", err)
	}
	if err := errors.Join(json.NewEncoder(metadata).Encode(file), metadata.Close()); err != nil {
		return File{}, fmt.Errorf("writing upload metadata: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return File{}, err
	}
	if err := root.Rename(staging, filepath.Join("uploads", file.ID)); err != nil {
		return File{}, fmt.Errorf("publishing upload: %w", err)
	}
	return file, nil
}

// Get verifies the stored source integrity before returning its metadata.
func (s *Store) Get(ctx context.Context, id string) (File, error) {
	file, source, err := s.open(ctx, id)
	if err != nil {
		return File{}, err
	}
	if err := source.Close(); err != nil {
		return File{}, fmt.Errorf("closing upload source: %w", err)
	}
	return file, nil
}

// Open verifies stored integrity and returns the same source descriptor rewound
// to its beginning. The caller must close it. External writes after verification
// are not prevented; the storage root must be managed exclusively by the store.
func (s *Store) Open(ctx context.Context, id string) (io.ReadCloser, error) {
	_, source, err := s.open(ctx, id)
	if err != nil {
		return nil, err
	}
	return source, nil
}

// =============================================================================

func (s *Store) open(ctx context.Context, id string) (file File, source *os.File, err error) {
	if !validID(id) {
		return File{}, nil, fmt.Errorf("%w: invalid upload ID", ErrInvalidArgument)
	}
	if err := ctx.Err(); err != nil {
		return File{}, nil, err
	}
	root, err := os.OpenRoot(s.cfg.Root)
	if err != nil {
		return File{}, nil, storageError(err)
	}
	defer root.Close()
	metadata, err := root.Open(filepath.Join("uploads", id, "metadata.json"))
	if err != nil {
		return File{}, nil, storageError(err)
	}
	decodeErr := json.NewDecoder(io.LimitReader(metadata, 4096)).Decode(&file)
	closeErr := metadata.Close()
	if decodeErr != nil {
		return File{}, nil, errors.Join(fmt.Errorf("%w: invalid upload metadata: %w", ErrInvalidArgument, decodeErr), closeErr)
	}
	if closeErr != nil {
		return File{}, nil, fmt.Errorf("closing upload metadata: %w", closeErr)
	}
	if file.ID != id || !validName(file.Name) || file.Format != strings.ToLower(filepath.Ext(file.Name)) || strings.ContainsAny(file.Format, "/\\\x00") || file.SizeBytes < 1 || file.SizeBytes > s.cfg.MaximumBytes || file.CreatedAt.IsZero() {
		return File{}, nil, fmt.Errorf("%w: invalid persisted upload metadata", ErrInvalidArgument)
	}
	source, err = root.Open(filepath.Join("uploads", id, "source"+file.Format))
	if err != nil {
		return File{}, nil, storageError(err)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, source.Close())
			source = nil
		}
	}()
	info, err := source.Stat()
	if err != nil {
		return File{}, source, fmt.Errorf("checking upload source: %w", err)
	}
	if !info.Mode().IsRegular() {
		return File{}, source, fmt.Errorf("%w: source is not a regular file", ErrInvalidArgument)
	}
	hash := sha256.New()
	size, err := io.Copy(hash, io.LimitReader(contextReader{ctx: ctx, reader: source}, s.cfg.MaximumBytes+1))
	if err != nil {
		return File{}, source, fmt.Errorf("verifying upload source: %w", err)
	}
	if size != file.SizeBytes || hex.EncodeToString(hash.Sum(nil)) != file.SHA256 {
		return File{}, source, fmt.Errorf("%w: upload source integrity check failed", ErrInvalidArgument)
	}
	if err := ctx.Err(); err != nil {
		return File{}, source, err
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return File{}, source, fmt.Errorf("rewinding upload source: %w", err)
	}
	return file, source, nil
}

func storageError(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	}
	return fmt.Errorf("opening upload storage: %w", err)
}

func validName(name string) bool {
	return name != "" && len(name) <= 255 && !strings.ContainsAny(name, "/\\\x00") && name != "." && name != ".."
}

func validID(id string) bool {
	if !strings.HasPrefix(id, "upload-") || len(id) != 33 {
		return false
	}
	for _, r := range id[7:] {
		if !(r >= 'A' && r <= 'Z' || r >= '2' && r <= '7') {
			return false
		}
	}
	return true
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
