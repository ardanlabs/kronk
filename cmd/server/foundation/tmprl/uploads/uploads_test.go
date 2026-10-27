package uploads_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/uploads"
)

func TestConfig(t *testing.T) {
	for _, cfg := range []uploads.Config{
		{}, {Root: " "}, {Root: t.TempDir(), MaximumBytes: -1}, {Root: t.TempDir(), MaximumBytes: 100<<20 + 1},
	} {
		if _, err := uploads.New(cfg); !errors.Is(err, uploads.ErrInvalidArgument) {
			t.Errorf("New(%+v): got %v, want invalid argument", cfg, err)
		}
	}
	s, err := uploads.New(uploads.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if got := s.MaximumBytes(); got != 100<<20 {
		t.Errorf("MaximumBytes: got %d, want %d", got, 100<<20)
	}
}

func TestRoundTrip(t *testing.T) {
	for _, name := range []string{" source.CSV ", "binary.EXE", "archive.tar.GZ", "README", "raw.jsonl", "image.weird", "trailing."} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			s := newStore(t, root, 12)
			text := "Hello source"
			file := put(t, s, name, text)
			hash := sha256.Sum256([]byte(text))
			if file.Name != strings.TrimSpace(name) || file.Format != strings.ToLower(filepath.Ext(file.Name)) || file.SizeBytes != 12 || file.SHA256 != hex.EncodeToString(hash[:]) || file.CreatedAt.IsZero() || !strings.HasPrefix(file.ID, "upload-") {
				t.Fatalf("metadata: got %+v, want normalized identity, size and digest", file)
			}
			// Reconstructing the store exercises the persisted metadata, not a cache.
			s = newStore(t, root, 12)
			stored, err := s.Get(t.Context(), file.ID)
			if err != nil || !reflect.DeepEqual(stored, file) {
				t.Fatalf("Get: got %+v, %v, want %+v", stored, err, file)
			}
			r, err := s.Open(t.Context(), file.ID)
			if err != nil {
				t.Fatal(err)
			}
			data, readErr := io.ReadAll(r)
			if err := errors.Join(readErr, r.Close()); err != nil {
				t.Fatal(err)
			}
			if string(data) != text {
				t.Errorf("Open: got %q, want %q", data, text)
			}
			if _, err := os.Stat(filepath.Join(root, "uploads", file.ID, "source"+file.Format)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRejectionAndRollback(t *testing.T) {
	root := t.TempDir()
	s := newStore(t, root, 12)
	file := put(t, s, "good", "ok")
	for _, input := range []struct {
		name   string
		reader io.Reader
	}{
		{"nil", nil}, {"empty", strings.NewReader("")}, {"large", strings.NewReader(strings.Repeat("x", 13))},
		{"", strings.NewReader("ok")}, {" ", strings.NewReader("ok")}, {".", strings.NewReader("ok")}, {"..", strings.NewReader("ok")},
		{"../path", strings.NewReader("ok")}, {"/absolute", strings.NewReader("ok")}, {`dir\file`, strings.NewReader("ok")},
		{"nul\x00.txt", strings.NewReader("ok")}, {strings.Repeat("a", 256), strings.NewReader("ok")},
	} {
		if _, err := s.Put(t.Context(), uploads.Input{Name: input.name}, input.reader); !errors.Is(err, uploads.ErrInvalidArgument) {
			t.Errorf("Put(%q): got %v, want invalid argument", input.name, err)
		}
	}
	failure := errors.New("reader failed")
	if _, err := s.Put(t.Context(), uploads.Input{Name: "failure"}, io.MultiReader(strings.NewReader("ok"), failingReader{failure})); !errors.Is(err, failure) {
		t.Errorf("reader failure: got %v, want %v", err, failure)
	}
	bounded := &countingReader{}
	if _, err := s.Put(t.Context(), uploads.Input{Name: "bounded"}, bounded); !errors.Is(err, uploads.ErrInvalidArgument) {
		t.Errorf("oversize reader: got %v, want invalid argument", err)
	}
	if bounded.bytes != 13 {
		t.Errorf("bytes consumed: got %d, want 13", bounded.bytes)
	}
	assertOnlyUpload(t, root, file.ID)
	for _, id := range []string{"", "../path", "upload-../../escape", "dataset-AAAAAAAAAAAAAAAAAAAAAAAAAA"} {
		if _, err := s.Get(t.Context(), id); !errors.Is(err, uploads.ErrInvalidArgument) {
			t.Errorf("Get(%q): got %v, want invalid argument", id, err)
		}
		if _, err := s.Open(t.Context(), id); !errors.Is(err, uploads.ErrInvalidArgument) {
			t.Errorf("Open(%q): got %v, want invalid argument", id, err)
		}
	}
}

func TestCancellation(t *testing.T) {
	root := t.TempDir()
	s := newStore(t, root, 12)
	file := put(t, s, "good", "ok")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.Put(ctx, uploads.Input{Name: "cancel"}, strings.NewReader("ok")); !errors.Is(err, context.Canceled) {
		t.Errorf("Put: got %v, want cancellation", err)
	}
	if _, err := s.Get(ctx, file.ID); !errors.Is(err, context.Canceled) {
		t.Errorf("Get: got %v, want cancellation", err)
	}
	if _, err := s.Open(ctx, file.ID); !errors.Is(err, context.Canceled) {
		t.Errorf("Open: got %v, want cancellation", err)
	}
	ctx, cancel = context.WithCancel(t.Context())
	defer cancel()
	reader := cancelReader{cancel: cancel}
	if _, err := s.Put(ctx, uploads.Input{Name: "mid-read"}, reader); !errors.Is(err, context.Canceled) {
		t.Errorf("mid-read Put: got %v, want cancellation", err)
	}
	assertOnlyUpload(t, root, file.ID)
}

func TestNotFound(t *testing.T) {
	root := t.TempDir()
	s := newStore(t, root, 12)
	file := put(t, s, "missing.txt", "ok")
	for _, path := range []string{
		filepath.Join(root, "uploads", file.ID, "source.txt"),
		filepath.Join(root, "uploads", file.ID, "metadata.json"),
		root,
	} {
		if err := os.RemoveAll(path); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Get(t.Context(), file.ID); !errors.Is(err, uploads.ErrNotFound) {
			t.Errorf("Get after removing %s: got %v, want not found", path, err)
		}
		if _, err := s.Open(t.Context(), file.ID); !errors.Is(err, uploads.ErrNotFound) {
			t.Errorf("Open after removing %s: got %v, want not found", path, err)
		}
	}
}

func TestIntegrity(t *testing.T) {
	for _, test := range []string{"same-size source", "oversize source", "digest", "size", "identity", "format traversal", "format mismatch", "malformed metadata"} {
		t.Run(test, func(t *testing.T) {
			root := t.TempDir()
			s := newStore(t, root, 12)
			file := put(t, s, "source.txt", "ok")
			dir := filepath.Join(root, "uploads", file.ID)
			switch test {
			case "same-size source", "oversize source":
				data := "no"
				if test == "oversize source" {
					data = strings.Repeat("x", 13)
				}
				if err := os.WriteFile(filepath.Join(dir, "source.txt"), []byte(data), 0o600); err != nil {
					t.Fatal(err)
				}
			default:
				switch test {
				case "digest":
					file.SHA256 = strings.Repeat("0", 64)
				case "size":
					file.SizeBytes++
				case "identity":
					file.ID = "upload-AAAAAAAAAAAAAAAAAAAAAAAAAA"
				case "format traversal":
					file.Format = "/../../outside"
				case "format mismatch":
					file.Format = ".other"
				}
				data, err := json.Marshal(file)
				if err != nil {
					t.Fatal(err)
				}
				if test == "malformed metadata" {
					data = []byte("{")
				}
				if err := os.WriteFile(filepath.Join(dir, "metadata.json"), data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			id := filepath.Base(dir)
			if _, err := s.Get(t.Context(), id); !errors.Is(err, uploads.ErrInvalidArgument) {
				t.Errorf("Get: got %v, want invalid argument", err)
			}
			if _, err := s.Open(t.Context(), id); !errors.Is(err, uploads.ErrInvalidArgument) {
				t.Errorf("Open: got %v, want invalid argument", err)
			}
		})
	}
}

func TestEscapingSymlinks(t *testing.T) {
	t.Run("uploads directory", func(t *testing.T) {
		root, outside := t.TempDir(), t.TempDir()
		if err := os.Symlink(outside, filepath.Join(root, "uploads")); err != nil {
			t.Fatal(err)
		}
		s := newStore(t, root, 12)
		if _, err := s.Put(t.Context(), uploads.Input{Name: "source.txt"}, strings.NewReader("ok")); err == nil {
			t.Fatal("Put: got success, want escaping symlink error")
		}
		entries, err := os.ReadDir(outside)
		if err != nil || len(entries) != 0 {
			t.Fatalf("outside entries: got %v, %v, want empty", entries, err)
		}
	})
	for _, target := range []string{"source.txt", "metadata.json", "directory"} {
		t.Run(target, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			s := newStore(t, root, 12)
			file := put(t, s, "source.txt", "ok")
			dir := filepath.Join(root, "uploads", file.ID)
			path := filepath.Join(dir, target)
			if target == "directory" {
				path = dir
			}
			data, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(outside, "metadata.json"), data, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(outside, "source.txt"), []byte("ok"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.RemoveAll(path); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(outside, target)
			if target == "directory" {
				link = outside
			}
			if err := os.Symlink(link, path); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Get(t.Context(), file.ID); err == nil {
				t.Error("Get: got success, want escaping symlink error")
			}
			if _, err := s.Open(t.Context(), file.ID); err == nil {
				t.Error("Open: got success, want escaping symlink error")
			}
		})
	}
}

// =============================================================================

func newStore(t *testing.T, root string, limit int64) *uploads.Store {
	t.Helper()
	s, err := uploads.New(uploads.Config{Root: root, MaximumBytes: limit})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func put(t *testing.T, s *uploads.Store, name, text string) uploads.File {
	t.Helper()
	file, err := s.Put(t.Context(), uploads.Input{Name: name}, strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	return file
}

func assertOnlyUpload(t *testing.T, root, id string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, "uploads"))
	if err != nil || len(entries) != 1 || entries[0].Name() != id {
		t.Fatalf("stored entries: got %v, %v, want only %s", entries, err, id)
	}
}

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

type countingReader struct{ bytes int }

func (r *countingReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	r.bytes += len(p)
	return len(p), nil
}

type cancelReader struct{ cancel context.CancelFunc }

func (r cancelReader) Read(p []byte) (int, error) {
	r.cancel()
	return copy(p, "ok"), io.EOF
}
