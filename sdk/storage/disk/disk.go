// Package disk provides compressed, latest-only IMC persistence in a private
// local directory. It implements imc.Store without loading native model code.
//
// Snapshots use gzip compression and checksummed metadata. Discovery reads only
// small conversation summaries. Files are private (0600 in a 0700 directory),
// but are not encrypted; the application owns access isolation and disk quotas.
// One Store may be open per directory, enforced by an OS lock released on exit.
// Use a local filesystem supporting atomic rename, not a network filesystem.
// Saves sync snapshot and metadata files before publication. On Unix the
// directory is also synced for crash durability. Windows flushes files but does
// not sync directory entries, so it has a weaker power-loss durability guarantee.
// Each record is limited to 64 MiB of JSON metadata and each summary to 64 KiB.
// Checkpoint and media sizes are governed by the caller, not a disk quota here.
package disk

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/sdk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/imc"
)

const maxResults = 1000

// Store persists the latest complete snapshot of each model/conversation pair.
// Construct it with New. Calls are safe concurrently; writes are serialized.
// Open sessions pin old files until closed, including after Delete or Save.
type Store struct {
	directory string
	logFunc   applog.Logger
	lock      *os.File
	gate      chan struct{}
	closed    bool
	entries   map[imc.Key]entry
	pins      map[string]int
	retired   map[string]bool
}

type entry struct {
	Version uint32
	Summary imc.Summary
	Blob    string
}

var _ imc.Store = (*Store)(nil)

// Option configures a Store during construction.
type Option func(*Store)

// WithLog supplies an optional logging function. A nil function disables logging.
// The function must be concurrency-safe. Calls are synchronous, outside storage
// locks, and receive operation context and key/value fields, never conversation
// content. New and Close, which have no context parameter, use a background
// context. Errors are passed as returned and may contain filesystem paths.
func WithLog(log applog.Logger) Option {
	return func(s *Store) { s.logFunc = log }
}

func (s *Store) log(ctx context.Context, operation string, start time.Time, err error, args ...any) {
	if s.logFunc != nil {
		args = append(args, "elapsed", time.Since(start), "err", err)
		s.logFunc(ctx, operation, args...)
	}
}

// New opens or creates a dedicated storage directory. It rejects an already
// open directory and invalid metadata rather than silently losing history.
// Missing snapshots do not prevent opening: their summaries remain listable,
// and Open returns an error matching fs.ErrNotExist for those snapshots.
// Interrupted writes are reclaimed on reopening. No files outside directory
// are used; do not share this directory with other application data.
func New(directory string, opts ...Option) (_ *Store, retErr error) {
	var s Store
	for _, opt := range opts {
		opt(&s)
	}

	start := time.Now()
	var missing, reclaimed int

	defer func() {
		s.log(context.Background(), "disk-new", start, retErr,
			"sessions", len(s.entries), "missing_snapshots", missing, "reclaimed_files", reclaimed)
	}()

	if directory == "" {
		return nil, errors.New("disk: directory is required")
	}

	directory, err := filepath.Abs(directory)
	if err != nil {
		return nil, fmt.Errorf("disk: resolve directory: %w", err)
	}

	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, fmt.Errorf("disk: create directory: %w", err)
	}

	lock, err := lockDirectory(directory)
	if err != nil {
		return nil, fmt.Errorf("disk: lock directory: %w", err)
	}

	s.directory, s.lock, s.gate = directory, lock, make(chan struct{}, 1)
	s.entries, s.pins, s.retired = make(map[imc.Key]entry), make(map[string]int), make(map[string]bool)

	missing, reclaimed, err = s.load()
	if err != nil {
		return nil, errors.Join(err, lock.Close())
	}

	return &s, nil
}

func (s *Store) acquire(ctx context.Context) error {
	select {
	case s.gate <- struct{}{}:

	case <-ctx.Done():
		return ctx.Err()
	}

	if err := ctx.Err(); err != nil {
		s.release()
		return err
	}

	if s.closed {
		s.release()
		return fs.ErrClosed
	}

	return nil
}

func (s *Store) release() {
	<-s.gate
}

// Close releases the directory lock. Unload models and close all Session handles
// first. If handles remain, Close returns an error and the Store stays open.
// Repeated successful closes are harmless. No saved conversations are deleted.
func (s *Store) Close() (retErr error) {
	start := time.Now()
	defer func() { s.log(context.Background(), "disk-close", start, retErr) }()

	s.gate <- struct{}{}
	defer s.release()

	if s.closed {
		return nil
	}

	if len(s.pins) != 0 {
		return errors.New("disk: close sessions before closing storage")
	}

	s.closed = true

	return s.lock.Close()
}

func keyName(key imc.Key) string {
	data, _ := json.Marshal(key) // A struct of strings cannot fail to marshal.
	return fmt.Sprintf("%x.json", sha256.Sum256(data))
}

func validSummary(summary imc.Summary) bool {
	return summary.Ref.Key.Model != "" && summary.Ref.Key.ID != "" && summary.Ref.Revision > 0 &&
		summary.Prefix.Length > 0 && !summary.CreatedAt.IsZero() && !summary.SavedAt.IsZero()
}

func (s *Store) load() (missing, reclaimed int, retErr error) {
	files, err := os.ReadDir(s.directory)
	if err != nil {
		return missing, reclaimed, err
	}

	live := make(map[string]bool)

	for _, file := range files {
		name := file.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}

		var e entry
		if err := readJSONFile(filepath.Join(s.directory, name), 64<<10, &e); err != nil {
			return missing, reclaimed, fmt.Errorf("disk: read summary %s: %w", name, err)
		}

		if e.Version != 1 {
			return missing, reclaimed, imc.ErrVersion
		}

		if !validSummary(e.Summary) || keyName(e.Summary.Ref.Key) != name ||
			filepath.Base(e.Blob) != e.Blob || !strings.HasPrefix(e.Blob, "snapshot-") || !strings.HasSuffix(e.Blob, ".bin") {
			return missing, reclaimed, fmt.Errorf("disk: invalid summary %s: %w", name, imc.ErrCorrupt)
		}

		_, err := os.Stat(filepath.Join(s.directory, e.Blob))
		switch {
		case errors.Is(err, fs.ErrNotExist):
			missing++
		case err != nil:
			return missing, reclaimed, fmt.Errorf("disk: missing snapshot: %w", errors.Join(imc.ErrCorrupt, err))
		}

		s.entries[e.Summary.Ref.Key] = e
		live[e.Blob] = true
	}

	for _, file := range files {
		name := file.Name()
		if strings.HasPrefix(name, ".tmp-") || (strings.HasPrefix(name, "snapshot-") && strings.HasSuffix(name, ".bin") && !live[name]) {
			if err := os.Remove(filepath.Join(s.directory, name)); err != nil {
				return missing, reclaimed, fmt.Errorf("disk: reclaim interrupted write: %w", err)
			}
			reclaimed++
		}
	}

	return missing, reclaimed, nil
}

// Save compresses borrowed payloads directly to a new file and atomically
// replaces the conversation summary only after that file has been synced.
// Failures before publication leave the previous snapshot intact. A failure
// syncing the directory after publication has an uncertain commit outcome.
func (s *Store) Save(ctx context.Context, snapshot imc.Snapshot) (retErr error) {
	start := time.Now()
	var exists, published bool
	defer func() {
		if s.logFunc == nil {
			return
		}
		mediaBytes := int64(0)
		for _, media := range snapshot.Media {
			mediaBytes += int64(len(media))
		}
		ref := snapshot.Record.Summary.Ref
		s.log(ctx, "disk-save", start, retErr, "model_id", ref.Key.Model, "session_id", ref.Key.ID,
			"revision", ref.Revision, "replaced", exists, "published", published,
			"target_bytes", len(snapshot.Target), "draft_bytes", len(snapshot.Draft), "media_bytes", mediaBytes)
	}()

	if err := s.acquire(ctx); err != nil {
		return err
	}

	defer s.release()

	if snapshot.Record.Version != 1 {
		return imc.ErrVersion
	}

	if !validSummary(snapshot.Record.Summary) || len(snapshot.Target) == 0 {
		return errors.New("disk: invalid snapshot")
	}

	key := snapshot.Record.Summary.Ref.Key

	old, exists := s.entries[key]
	if exists && old.Summary.Ref.Revision >= snapshot.Record.Summary.Ref.Revision {
		return imc.ErrConflict
	}

	if exists && !old.Summary.CreatedAt.Equal(snapshot.Record.Summary.CreatedAt) {
		return errors.New("disk: conversation creation time changed")
	}

	blob, err := writeSnapshot(ctx, s.directory, snapshot)
	if err != nil {
		return err
	}

	defer func() {
		if !published {
			retErr = errors.Join(retErr, os.Remove(filepath.Join(s.directory, blob)))
		}
	}()

	e := entry{Version: 1, Summary: snapshot.Record.Summary, Blob: blob}

	data, err := json.Marshal(e)
	if err != nil {
		return err
	}

	if len(data) > 64<<10 {
		return errors.New("disk: summary exceeds 64 KiB")
	}

	if err := json.Unmarshal(data, &e); err != nil {
		return err
	}

	if err := s.publish(ctx, keyName(key), data); err != nil {
		return err
	}

	published = true
	s.entries[key] = e

	if err := syncDirectory(s.directory); err != nil {
		return err // Keep both files when publication durability is uncertain.
	}

	if exists {
		return s.retire(old.Blob)
	}

	return nil
}

func (s *Store) publish(ctx context.Context, name string, data []byte) (retErr error) {
	f, err := os.CreateTemp(s.directory, ".tmp-*")
	if err != nil {
		return err
	}

	defer func() {
		retErr = errors.Join(retErr, removeFile(f.Name()))
	}()

	_, writeErr := f.Write(data)
	if err := errors.Join(writeErr, f.Sync(), f.Close(), ctx.Err()); err != nil {
		return err
	}

	return os.Rename(f.Name(), filepath.Join(s.directory, name))
}

func removeFile(name string) error {
	err := os.Remove(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	return err
}

func (s *Store) retire(blob string) error {
	if s.pins[blob] > 0 {
		s.retired[blob] = true
		return nil
	}

	return removeFile(filepath.Join(s.directory, blob))
}

// Find returns recent eligible summaries without reading checkpoint or request
// files. Limit must be between 1 and 1000; MaxPrefixLength must be positive.
func (s *Store) Find(ctx context.Context, query imc.FindQuery) (result []imc.Summary, retErr error) {
	start := time.Now()
	var eligible int

	defer func() {
		s.log(ctx, "disk-find", start, retErr, "model_id", query.Model, "limit", query.Limit,
			"max_prefix_length", query.MaxPrefixLength, "saved_after", query.SavedAfter,
			"eligible", eligible, "returned", len(result))
	}()

	if query.Model == "" || query.Limit < 1 || query.Limit > maxResults || query.MaxPrefixLength < 1 {
		return nil, errors.New("disk: invalid find query")
	}

	if err := s.acquire(ctx); err != nil {
		return nil, err
	}

	defer s.release()

	var summaries []imc.Summary
	for _, e := range s.entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		r := e.Summary

		if r.Ref.Key.Model == query.Model && r.Compatibility == query.Compatibility &&
			r.Prefix.Length <= query.MaxPrefixLength && !r.SavedAt.Before(query.SavedAfter) {
			summaries = append(summaries, r)
		}
	}

	slices.SortFunc(summaries, compareSummary)

	eligible = len(summaries)

	return summaries[:min(query.Limit, len(summaries))], nil
}

func compareSummary(a, b imc.Summary) int {
	return cmp.Or(b.SavedAt.Compare(a.SavedAt), strings.Compare(a.Ref.Key.Model, b.Ref.Key.Model), strings.Compare(a.Ref.Key.ID, b.Ref.Key.ID))
}

type cursor struct {
	Model string
	Limit int
	After imc.Summary
}

// List returns a live, keyset-paginated history of latest conversation snapshots.
// Limit must be between 1 and 1000. Cursors are opaque, tied to the query's model
// and limit, and not interchangeable with other storage implementations.
func (s *Store) List(ctx context.Context, query imc.ListQuery) (page imc.Page, retErr error) {
	start := time.Now()
	defer func() {
		s.log(ctx, "disk-list", start, retErr, "model_id", query.Model, "limit", query.Limit,
			"returned", len(page.Sessions), "has_more", page.NextCursor != "")
	}()

	if query.Limit < 1 || query.Limit > maxResults {
		return imc.Page{}, errors.New("disk: invalid list limit")
	}

	var c cursor

	if query.Cursor != "" {
		if len(query.Cursor) > 256<<10 {
			return imc.Page{}, errors.New("disk: invalid list cursor")
		}

		data, err := base64.RawURLEncoding.DecodeString(query.Cursor)
		if err != nil || len(data) > 128<<10 {
			return imc.Page{}, errors.New("disk: invalid list cursor")
		}

		if err := json.Unmarshal(data, &c); err != nil || c.Model != query.Model || c.Limit != query.Limit || !validSummary(c.After) {
			return imc.Page{}, errors.New("disk: mismatched list cursor")
		}
	}

	if err := s.acquire(ctx); err != nil {
		return imc.Page{}, err
	}

	defer s.release()

	var summaries []imc.Summary
	for _, e := range s.entries {
		if err := ctx.Err(); err != nil {
			return imc.Page{}, err
		}

		if (query.Model == "" || e.Summary.Ref.Key.Model == query.Model) &&
			(query.Cursor == "" || compareSummary(e.Summary, c.After) > 0) {
			summaries = append(summaries, e.Summary)
		}
	}

	slices.SortFunc(summaries, compareSummary)
	page = imc.Page{Sessions: summaries[:min(query.Limit, len(summaries))]}

	if len(summaries) > query.Limit {
		data, err := json.Marshal(cursor{query.Model, query.Limit, page.Sessions[len(page.Sessions)-1]})
		if err != nil {
			return imc.Page{}, err
		}

		page.NextCursor = base64.RawURLEncoding.EncodeToString(data)
	}

	return page, nil
}

// Delete removes a conversation from discovery. Open sessions remain readable.
// An absent conversation is already deleted and succeeds.
func (s *Store) Delete(ctx context.Context, key imc.Key) (retErr error) {
	start := time.Now()
	var exists, published bool

	defer func() {
		s.log(ctx, "disk-delete", start, retErr, "model_id", key.Model, "session_id", key.ID,
			"found", exists, "published", published)
	}()

	if key.Model == "" || key.ID == "" {
		return errors.New("disk: invalid key")
	}

	if err := s.acquire(ctx); err != nil {
		return err
	}

	defer s.release()

	e, exists := s.entries[key]
	if !exists {
		return nil
	}

	if err := os.Remove(filepath.Join(s.directory, keyName(key))); err != nil {
		return err
	}

	delete(s.entries, key)

	published = true

	if err := syncDirectory(s.directory); err != nil {
		return err
	}

	return s.retire(e.Blob)
}
