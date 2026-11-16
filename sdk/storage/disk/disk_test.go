package disk_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/imc"
	"github.com/ardanlabs/kronk/sdk/storage/disk"
)

func snapshot(id string) imc.Snapshot {
	stamp := time.Date(2026, 11, 16, 10, 0, 0, 0, time.FixedZone("test", 3*60*60))
	return imc.Snapshot{
		Record: imc.Record{
			Version: 1,
			Summary: imc.Summary{
				Ref:       imc.Ref{Key: imc.Key{Model: "model/a", ID: id}, Revision: 1},
				CreatedAt: stamp, SavedAt: stamp, Preview: "final answer",
				Prefix: imc.Prefix{Length: 17, Hash: sha256.Sum256([]byte("prefix"))}, Compatibility: [32]byte{3},
			},
			Request: json.RawMessage(`{"messages":[{"role":"user","content":"hello"}]}`),
			Restore: json.RawMessage(`{"tokens":[5,9,2]}`), AssistantText: "final answer",
		},
		Target: bytes.Repeat([]byte{5, 9, 2}, 50000), Draft: []byte{8, 1, 7},
		Media: [][]byte{{1, 2}, {}, {5, 6, 7, 8}},
	}
}

func newStore(t *testing.T, directory string, opts ...disk.Option) *disk.Store {
	t.Helper()
	s, err := disk.New(directory, opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}

func open(t *testing.T, s *disk.Store, ref imc.Ref) imc.Session {
	t.Helper()
	h, err := s.Open(t.Context(), ref)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := h.Close(); err != nil {
			t.Error(err)
		}
	})
	return h
}

func checkSession(t *testing.T, h imc.Session, want imc.Snapshot) {
	t.Helper()
	r := h.Record()
	gotJSON, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, err := json.Marshal(want.Record)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotJSON, wantJSON) {
		t.Errorf("record: got %s, want %s", gotJSON, wantJSON)
	}
	r.Request[0] = '!'
	r.Restore[0] = '!'
	if h.Record().Request[0] != '{' || h.Record().Restore[0] != '{' {
		t.Fatal("Record returned shared mutable data")
	}
	targetSize, draftSize, sizes := h.Sizes()
	if targetSize != int64(len(want.Target)) || draftSize != int64(len(want.Draft)) || len(sizes) != len(want.Media) {
		t.Fatal("incorrect payload sizes")
	}
	target, draft := make([]byte, int(targetSize)), make([]byte, int(draftSize))
	media := make([][]byte, len(sizes))
	for i, size := range sizes {
		media[i] = make([]byte, int(size))
	}
	if len(sizes) > 0 {
		sizes[0]++
	}
	if err := h.Read(t.Context(), target, draft, media); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(target, want.Target) || !bytes.Equal(draft, want.Draft) || !slices.EqualFunc(media, want.Media, bytes.Equal) {
		t.Fatal("read payload does not match the saved target/draft/media")
	}
	if err := h.Read(t.Context(), target[:len(target)-1], draft, media); err == nil {
		t.Error("accepted an incorrectly sized buffer")
	}
}

func TestPersistenceAndPinnedRevisions(t *testing.T) {
	directory := t.TempDir()
	s := newStore(t, directory)
	first := snapshot("../conversation")
	if err := s.Save(t.Context(), first); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = newStore(t, directory)
	old := open(t, s, first.Record.Summary.Ref)
	checkSession(t, old, first)
	if err := s.Close(); err == nil {
		t.Error("closed storage with pinned sessions")
	}
	second := first
	second.Record.Summary.Ref.Revision = 3
	second.Record.AssistantText = "new answer"
	second.Target = []byte{41, 19, 7}
	second.Draft, second.Media = nil, nil
	if err := s.Save(t.Context(), second); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(t.Context(), first); !errors.Is(err, imc.ErrConflict) {
		t.Errorf("stale save: got %v, want ErrConflict", err)
	}
	if _, err := s.Open(t.Context(), first.Record.Summary.Ref); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("open replaced revision: got %v, want not-exist", err)
	}
	checkSession(t, old, first)
	latest := open(t, s, second.Record.Summary.Ref)
	checkSession(t, latest, second)
	if err := s.Delete(t.Context(), second.Record.Summary.Ref.Key); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(t.Context(), second.Record.Summary.Ref.Key); err != nil {
		t.Fatal(err)
	}
	checkSession(t, latest, second)
	if _, err := s.Open(t.Context(), second.Record.Summary.Ref); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("open deleted conversation: got %v", err)
	}
	page, err := s.List(t.Context(), imc.ListQuery{Limit: 10})
	if err != nil || len(page.Sessions) != 0 {
		t.Fatalf("deleted history: got %+v/%v", page, err)
	}
	if err := errors.Join(old.Close(), latest.Close(), s.Close()); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(directory, "snapshot-*.bin"))
	if err != nil || len(files) != 0 {
		t.Fatalf("retired files were not reclaimed: %v/%v", files, err)
	}
}

func TestFindAndPagedHistory(t *testing.T) {
	s := newStore(t, t.TempDir())
	for i, id := range []string{"c", "b", "a", "other-model", "other-runtime", "too-long"} {
		v := snapshot(id)
		v.Record.Summary.SavedAt = v.Record.Summary.SavedAt.Add(time.Duration(i/3) * time.Hour)
		switch id {
		case "other-model":
			v.Record.Summary.Ref.Key.Model = "model/b"
		case "other-runtime":
			v.Record.Summary.Compatibility = [32]byte{99}
		case "too-long":
			v.Record.Summary.Prefix.Length = 99
		}
		if err := s.Save(t.Context(), v); err != nil {
			t.Fatal(err)
		}
	}
	q := imc.FindQuery{Model: "model/a", Compatibility: [32]byte{3}, MaxPrefixLength: 17, Limit: 2}
	found, err := s.Find(t.Context(), q)
	if err != nil || len(found) != 2 || found[0].Ref.Key.ID != "a" || found[1].Ref.Key.ID != "b" {
		t.Fatalf("eligible bounded candidates: got %+v/%v", found, err)
	}
	q.SavedAfter = found[0].SavedAt
	if found, err := s.Find(t.Context(), q); err != nil || len(found) != 2 {
		t.Fatalf("inclusive age boundary: got %+v/%v", found, err)
	}
	q.SavedAfter = found[0].SavedAt.Add(time.Nanosecond)
	if found, err := s.Find(t.Context(), q); err != nil || len(found) != 0 {
		t.Fatalf("age-filtered candidates: got %+v/%v", found, err)
	}
	query := imc.ListQuery{Limit: 2}
	var ids []string
	for {
		page, err := s.List(t.Context(), query)
		if err != nil {
			t.Fatal(err)
		}
		for _, summary := range page.Sessions {
			ids = append(ids, summary.Ref.Key.ID)
		}
		if page.NextCursor == "" {
			break
		}
		if _, err := s.List(t.Context(), imc.ListQuery{Model: "model/b", Limit: 2, Cursor: page.NextCursor}); err == nil {
			t.Error("accepted a cursor with different filters")
		}
		query.Cursor = page.NextCursor
	}
	want := []string{"other-runtime", "too-long", "other-model", "a", "b", "c"}
	if !slices.Equal(ids, want) {
		t.Errorf("paged history: got %v, want %v", ids, want)
	}
}

func TestFailedSavePreservesSnapshot(t *testing.T) {
	directory := t.TempDir()
	s := newStore(t, directory)
	v := snapshot("failure")
	if err := s.Save(t.Context(), v); err != nil {
		t.Fatal(err)
	}
	bad := v
	bad.Record.Summary.Ref.Revision++
	bad.Record.Restore = json.RawMessage(`{broken`)
	if err := s.Save(t.Context(), bad); err == nil {
		t.Fatal("saved invalid metadata")
	}
	bad.Record.Restore = v.Record.Restore
	bad.Record.Summary.Preview = strings.Repeat("x", 65<<10)
	if err := s.Save(t.Context(), bad); err == nil {
		t.Fatal("published an oversized summary")
	}
	files, err := filepath.Glob(filepath.Join(directory, "snapshot-*.bin"))
	if err != nil || len(files) != 1 {
		t.Fatalf("failed publication left an extra snapshot: %v/%v", files, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := s.Save(ctx, v); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled save: got %v", err)
	}
	checkSession(t, open(t, s, v.Record.Summary.Ref), v)
	if err := s.Save(t.Context(), imc.Snapshot{Record: imc.Record{Version: 99}}); !errors.Is(err, imc.ErrVersion) {
		t.Errorf("unsupported schema: got %v", err)
	}
	if other, err := disk.New(directory); err == nil {
		t.Error("opened the same directory twice")
		if err := other.Close(); err != nil {
			t.Error(err)
		}
	}
}

func TestCorruptionAndMetadataOnlyDiscovery(t *testing.T) {
	for _, damage := range []string{"record", "payload", "truncate"} {
		t.Run(damage, func(t *testing.T) {
			directory := t.TempDir()
			s := newStore(t, directory)
			v := snapshot("corrupt")
			if err := s.Save(t.Context(), v); err != nil {
				t.Fatal(err)
			}
			files, err := filepath.Glob(filepath.Join(directory, "snapshot-*.bin"))
			if err != nil || len(files) != 1 {
				t.Fatalf("snapshot files: %v/%v", files, err)
			}
			data, err := os.ReadFile(files[0])
			if err != nil {
				t.Fatal(err)
			}
			if len(data) >= len(v.Target) {
				t.Error("repetitive checkpoint was not compressed")
			}
			switch damage {
			case "record":
				data[50] ^= 1
			case "payload":
				data[len(data)-5] ^= 1 // Corrupt the gzip checksum.
			case "truncate":
				data = data[:len(data)-5]
			}
			if err := os.WriteFile(files[0], data, 0600); err != nil {
				t.Fatal(err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			s = newStore(t, directory)
			page, err := s.List(t.Context(), imc.ListQuery{Limit: 1})
			if err != nil || len(page.Sessions) != 1 {
				t.Fatalf("discovery read damaged payload: %+v/%v", page, err)
			}
			found, err := s.Find(t.Context(), imc.FindQuery{Model: "model/a", Compatibility: [32]byte{3}, Limit: 1, MaxPrefixLength: 17})
			if err != nil || len(found) != 1 {
				t.Fatalf("Find read damaged payload: %+v/%v", found, err)
			}
			h, err := s.Open(t.Context(), v.Record.Summary.Ref)
			if damage == "record" {
				if !errors.Is(err, imc.ErrCorrupt) {
					t.Fatalf("record corruption: got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := h.Close(); err != nil {
					t.Error(err)
				}
			}()
			err = h.Read(t.Context(), make([]byte, len(v.Target)), make([]byte, len(v.Draft)), [][]byte{make([]byte, 2), {}, make([]byte, 4)})
			if !errors.Is(err, imc.ErrCorrupt) {
				t.Errorf("payload corruption: got %v", err)
			}
		})
	}
}

func TestMissingSnapshotPreservesHistory(t *testing.T) {
	directory := t.TempDir()
	s := newStore(t, directory)
	missing := snapshot("missing")
	if err := s.Save(t.Context(), missing); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(directory, "snapshot-*.bin"))
	if err != nil || len(files) != 1 {
		t.Fatalf("snapshot files: got %v/%v, want one", files, err)
	}
	if err := os.Remove(files[0]); err != nil {
		t.Fatal(err)
	}
	healthy := snapshot("healthy")
	if err := s.Save(t.Context(), healthy); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, ".tmp-interrupted"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	var startup map[string]any
	s = newStore(t, directory, disk.WithLog(func(_ context.Context, operation string, args ...any) {
		if operation == "disk-new" {
			startup = make(map[string]any)
			for i := 0; i+1 < len(args); i += 2 {
				startup[args[i].(string)] = args[i+1]
			}
		}
	}))
	if startup["sessions"] != 2 || startup["missing_snapshots"] != 1 || startup["reclaimed_files"] != 1 || startup["err"] != nil {
		t.Fatalf("startup logging: %v", startup)
	}
	page, err := s.List(t.Context(), imc.ListQuery{Limit: 10})
	if err != nil || len(page.Sessions) != 2 || !slices.ContainsFunc(page.Sessions, func(summary imc.Summary) bool {
		return summary.Ref == missing.Record.Summary.Ref && summary.Prefix == missing.Record.Summary.Prefix
	}) {
		t.Fatalf("history: got %+v/%v, want both sessions including the missing snapshot's summary", page, err)
	}
	if _, err := s.Open(t.Context(), missing.Record.Summary.Ref); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing snapshot: got %v, want not-exist", err)
	}
	checkSession(t, open(t, s, healthy.Record.Summary.Ref), healthy)
}

func TestInvalidSummaryRejectedOnReopen(t *testing.T) {
	for _, damage := range []string{"malformed JSON", "invalid filesystem path"} {
		t.Run(damage, func(t *testing.T) {
			directory := t.TempDir()
			s := newStore(t, directory)
			v := snapshot("invalid")
			if err := s.Save(t.Context(), v); err != nil {
				t.Fatal(err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			files, err := filepath.Glob(filepath.Join(directory, "*.json"))
			if err != nil || len(files) != 1 {
				t.Fatalf("summary files: got %v/%v, want one", files, err)
			}
			data := []byte("{")
			if damage == "invalid filesystem path" {
				data, err = json.Marshal(map[string]any{
					"Version": 1, "Summary": v.Record.Summary, "Blob": "snapshot-\x00.bin",
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(files[0], data, 0600); err != nil {
				t.Fatal(err)
			}
			if reopened, err := disk.New(directory); err == nil {
				if err := reopened.Close(); err != nil {
					t.Error(err)
				}
				t.Fatal("accepted invalid summary or non-not-found filesystem error")
			}
		})
	}
}

func TestConcurrentConversations(t *testing.T) {
	s := newStore(t, t.TempDir())
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			v := snapshot(fmt.Sprintf("conversation-%d", i))
			if err := s.Save(t.Context(), v); err != nil {
				t.Error(err)
				return
			}
			h, err := s.Open(t.Context(), v.Record.Summary.Ref)
			if err != nil {
				t.Error(err)
				return
			}
			if h.Record().Summary.Ref != v.Record.Summary.Ref {
				t.Error("opened another conversation")
			}
			if err := h.Close(); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	page, err := s.List(t.Context(), imc.ListQuery{Limit: 20})
	if err != nil || len(page.Sessions) != 8 {
		t.Fatalf("concurrent saved sessions: got %+v/%v", page, err)
	}
}

func TestReopenReclaimsInterruptedWrites(t *testing.T) {
	directory := t.TempDir()
	s := newStore(t, directory)
	v := snapshot("recovery")
	if err := s.Save(t.Context(), v); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".tmp-interrupted", "snapshot-unpublished.bin"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("incomplete"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	s = newStore(t, directory)
	checkSession(t, open(t, s, v.Record.Summary.Ref), v)
	for _, name := range []string{".tmp-interrupted", "snapshot-unpublished.bin"} {
		if _, err := os.Stat(filepath.Join(directory, name)); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("interrupted file was not reclaimed: %s/%v", name, err)
		}
	}
}

func TestLogging(t *testing.T) {
	ctx := t.Context()
	events := make(map[string][]map[string]any)
	var s *disk.Store
	log := func(gotCtx context.Context, operation string, args ...any) {
		fields := make(map[string]any)
		for i := 0; i+1 < len(args); i += 2 {
			fields[args[i].(string)] = args[i+1]
		}
		events[operation] = append(events[operation], fields)
		if strings.Contains(fmt.Sprint(args...), "final answer") || strings.Contains(fmt.Sprint(args...), "hello") {
			t.Error("conversation content was logged")
		}
		if operation == "disk-save" {
			if gotCtx != ctx {
				t.Error("Save did not propagate its context")
			}
			// A synchronous callback can query storage without waiting for the
			// operation's lock. A bounded context turns a lock leak into failure.
			queryCtx, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			if _, err := s.List(queryCtx, imc.ListQuery{Limit: 1}); err != nil {
				t.Errorf("callback query: %v", err)
			}
		}
	}
	var err error
	s, err = disk.New(t.TempDir(), disk.WithLog(log))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	v := snapshot("logged")
	if err := s.Save(ctx, v); err != nil {
		t.Fatal(err)
	}
	first := events["disk-save"][0]
	if first["published"] != true || first["replaced"] != false || first["revision"] != uint64(1) || first["media_bytes"] != int64(6) {
		t.Fatalf("initial save fields: %v", first)
	}
	if err := s.Save(ctx, v); !errors.Is(err, imc.ErrConflict) {
		t.Fatalf("duplicate save: got %v, want conflict", err)
	}
	failed := events["disk-save"][1]
	if loggedErr, ok := failed["err"].(error); !ok || !errors.Is(loggedErr, imc.ErrConflict) || failed["published"] != false {
		t.Fatalf("failed save fields: %v", failed)
	}
	v.Record.Summary.Ref.Revision++
	if err := s.Save(ctx, v); err != nil {
		t.Fatal(err)
	}
	if fields := events["disk-save"][2]; fields["replaced"] != true || fields["published"] != true {
		t.Fatalf("replacement fields: %v", fields)
	}
	if err := s.Save(ctx, snapshot("other")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Find(ctx, imc.FindQuery{Model: "model/a", Compatibility: [32]byte{3}, Limit: 1, MaxPrefixLength: 17}); err != nil {
		t.Fatal(err)
	}
	if fields := events["disk-find"][0]; fields["eligible"] != 2 || fields["returned"] != 1 {
		t.Fatalf("find fields: %v", fields)
	}
	if _, err := s.List(ctx, imc.ListQuery{Limit: 1}); err != nil {
		t.Fatal(err)
	}
	if fields := events["disk-list"][len(events["disk-list"])-1]; fields["returned"] != 1 || fields["has_more"] != true {
		t.Fatalf("list fields: %v", fields)
	}
	h := open(t, s, v.Record.Summary.Ref)
	if err := s.Close(); err == nil {
		t.Fatal("Close succeeded with pinned session")
	}
	checkSession(t, h, v)
	if err := s.Delete(ctx, v.Record.Summary.Ref.Key); err != nil {
		t.Fatal(err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"disk-new", "disk-save", "disk-find", "disk-open", "disk-read", "disk-list", "disk-delete", "disk-session-close", "disk-close"} {
		if len(events[operation]) == 0 {
			t.Errorf("no callback for %s", operation)
		}
		for _, fields := range events[operation] {
			if elapsed, ok := fields["elapsed"].(time.Duration); !ok || elapsed < 0 {
				t.Errorf("invalid elapsed for %s: %v", operation, fields)
			}
		}
	}
}

func TestLoggingSaveAfterPublication(t *testing.T) {
	directory := t.TempDir()
	var published bool
	var loggedErr error
	s := newStore(t, directory, disk.WithLog(func(_ context.Context, operation string, args ...any) {
		if operation == "disk-save" {
			for i := 0; i+1 < len(args); i += 2 {
				switch args[i] {
				case "published":
					published = args[i+1].(bool)
				case "err":
					loggedErr, _ = args[i+1].(error)
				}
			}
		}
	}))
	v := snapshot("retirement-error")
	if err := s.Save(t.Context(), v); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(directory, "snapshot-*.bin"))
	if err != nil || len(files) != 1 {
		t.Fatalf("snapshot files: %v/%v", files, err)
	}
	// Prevent retirement of the old file, after the replacement is published.
	if err := os.Remove(files[0]); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(files[0], 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(files[0], "blocked"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	v.Record.Summary.Ref.Revision++
	err = s.Save(t.Context(), v)
	if err == nil || loggedErr != err || !published {
		t.Fatalf("post-publication error: returned=%v logged=%v published=%t", err, loggedErr, published)
	}
	checkSession(t, open(t, s, v.Record.Summary.Ref), v)
}
