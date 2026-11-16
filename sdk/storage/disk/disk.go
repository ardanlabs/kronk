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

	"github.com/ardanlabs/kronk/sdk/kronk/imc"
)

const maxResults = 1000

// Store persists the latest complete snapshot of each model/conversation pair.
// Construct it with New. Calls are safe concurrently; writes are serialized.
// Open sessions pin old files until closed, including after Delete or Save.
type Store struct {
	directory string
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

// New opens or creates a dedicated storage directory. It rejects an already
// open directory and invalid metadata rather than silently losing history.
// Missing snapshots do not prevent opening: their summaries remain listable,
// and Open returns an error matching fs.ErrNotExist for those snapshots.
// Interrupted writes are reclaimed on reopening. No files outside directory
// are used; do not share this directory with other application data.
func New(directory string) (*Store, error) {
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

	s := Store{
		directory: directory, lock: lock, gate: make(chan struct{}, 1),
		entries: make(map[imc.Key]entry), pins: make(map[string]int), retired: make(map[string]bool),
	}

	if err := s.load(); err != nil {
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
func (s *Store) Close() error {
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

func (s *Store) load() error {
	files, err := os.ReadDir(s.directory)
	if err != nil {
		return err
	}

	live := make(map[string]bool)

	for _, file := range files {
		name := file.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}

		var e entry
		if err := readJSONFile(filepath.Join(s.directory, name), 64<<10, &e); err != nil {
			return fmt.Errorf("disk: read summary %s: %w", name, err)
		}

		if e.Version != 1 {
			return imc.ErrVersion
		}

		if !validSummary(e.Summary) || keyName(e.Summary.Ref.Key) != name ||
			filepath.Base(e.Blob) != e.Blob || !strings.HasPrefix(e.Blob, "snapshot-") || !strings.HasSuffix(e.Blob, ".bin") {
			return fmt.Errorf("disk: invalid summary %s: %w", name, imc.ErrCorrupt)
		}

		if _, err := os.Stat(filepath.Join(s.directory, e.Blob)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("disk: missing snapshot: %w", errors.Join(imc.ErrCorrupt, err))
		}

		s.entries[e.Summary.Ref.Key] = e
		live[e.Blob] = true
	}

	for _, file := range files {
		name := file.Name()
		if strings.HasPrefix(name, ".tmp-") || (strings.HasPrefix(name, "snapshot-") && strings.HasSuffix(name, ".bin") && !live[name]) {
			if err := os.Remove(filepath.Join(s.directory, name)); err != nil {
				return fmt.Errorf("disk: reclaim interrupted write: %w", err)
			}
		}
	}

	return nil
}

// Save compresses borrowed payloads directly to a new file and atomically
// replaces the conversation summary only after that file has been synced.
// Failures before publication leave the previous snapshot intact. A failure
// syncing the directory after publication has an uncertain commit outcome.
func (s *Store) Save(ctx context.Context, snapshot imc.Snapshot) (retErr error) {
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

	published := false
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
func (s *Store) Find(ctx context.Context, query imc.FindQuery) ([]imc.Summary, error) {
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
func (s *Store) List(ctx context.Context, query imc.ListQuery) (imc.Page, error) {
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
	page := imc.Page{Sessions: summaries[:min(query.Limit, len(summaries))]}

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
func (s *Store) Delete(ctx context.Context, key imc.Key) error {
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
	if err := syncDirectory(s.directory); err != nil {
		return err
	}

	return s.retire(e.Blob)
}
