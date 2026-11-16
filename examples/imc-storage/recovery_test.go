package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/imc"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/storage/disk"
)

// TestFailureRecovery is opt-in because it loads a real model. Only the failing
// storage operation is injected; inference and checkpoint serialization are real.
func TestFailureRecovery(t *testing.T) {
	if os.Getenv("KRONK_IMC_TEST_MODEL") == "" {
		t.Skip("set KRONK_IMC_TEST_MODEL to an installed text GGUF to run native recovery verification")
	}
	if err := kronk.Init(kronk.WithLibPath(os.Getenv("KRONK_IMC_TEST_LIB_PATH"))); err != nil {
		t.Fatal(err)
	}
	for _, fault := range []string{"initial-save", "save", "save-timeout", "save-after"} {
		t.Run(fault, func(t *testing.T) { testSaveRecovery(t, fault) })
	}
	for _, fault := range []string{"find", "find-timeout", "open", "read", "read-timeout", "close", "corrupt", "truncate", "missing"} {
		t.Run(fault, func(t *testing.T) { testRestoreRecovery(t, fault) })
	}
}

func testSaveRecovery(t *testing.T, fault string) {
	directory := t.TempDir()
	store := recoveryDisk(t, directory)
	failing := faultStore{Store: store}
	var observed evidence
	krn, unload := recoveryModel(t, &failing, &observed)
	request, err := initialRequest("text")
	if err != nil {
		t.Fatal(err)
	}
	var before imc.Summary
	if fault != "initial-save" {
		request = recoveryChat(t, krn, &observed, request, false)
		before = onlySummary(t, store)
	}
	mode := fault
	if fault == "initial-save" {
		mode = "save"
	}
	failing.arm(mode)
	request = recoveryChat(t, krn, &observed, request, fault != "initial-save")
	failing.requireTriggered(t)
	failed := failing.attemptedRef()
	page, err := store.List(t.Context(), imc.ListQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	switch fault {
	case "initial-save":
		if len(page.Sessions) != 0 {
			t.Fatal("failed initial save became discoverable")
		}
	case "save-after":
		if len(page.Sessions) != 1 || page.Sessions[0].Ref != failed {
			t.Fatal("post-publication error did not leave the committed revision visible")
		}
	default:
		if len(page.Sessions) != 1 || page.Sessions[0] != before {
			t.Fatal("failed replacement changed the previous saved snapshot")
		}
		// Read the entire old checkpoint, including its integrity checks.
		handle, err := store.Open(t.Context(), before.Ref)
		if err != nil {
			t.Fatal(err)
		}
		size, _, _ := handle.Sizes()
		if err := errors.Join(handle.Read(t.Context(), make([]byte, size), nil, nil), handle.Close()); err != nil {
			t.Fatal(err)
		}
	}
	failing.arm("")
	recoveryChat(t, krn, &observed, request, true) // RAM survived the failure.
	repaired := onlySummary(t, store)
	if repaired.Ref.Key != failed.Key || repaired.Ref.Revision <= failed.Revision {
		t.Fatalf("repair: got %+v, want same ID and revision newer than %+v", repaired.Ref, failed)
	}
	unload()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	verifyReopened(t, directory, repaired)
}

func testRestoreRecovery(t *testing.T, fault string) {
	directory := t.TempDir()
	store := recoveryDisk(t, directory)
	var observed evidence
	krn, unload := recoveryModel(t, store, &observed)
	request, err := initialRequest("text")
	if err != nil {
		t.Fatal(err)
	}
	recoveryChat(t, krn, &observed, request, false)
	before := onlySummary(t, store)
	request = savedContinuation(t, store, before.Ref)
	unload()

	failing := faultStore{Store: store}
	switch fault {
	case "corrupt", "truncate", "missing":
		files, err := filepath.Glob(filepath.Join(directory, "snapshot-*.bin"))
		if err != nil || len(files) != 1 {
			t.Fatalf("snapshot files: got %v/%v, want one", files, err)
		}
		data, err := os.ReadFile(files[0])
		if err != nil {
			t.Fatal(err)
		}
		switch fault {
		case "corrupt":
			data[len(data)-5] ^= 1 // Change the gzip checksum, not native bytes.
			err = os.WriteFile(files[0], data, 0600)
		case "truncate":
			err = os.Truncate(files[0], int64(len(data)-5))
		case "missing":
			err = os.Remove(files[0])
		}
		if err != nil {
			t.Fatal(err)
		}
	default:
		failing.arm(fault)
	}
	krn, unload = recoveryModel(t, &failing, &observed)
	recoveryChat(t, krn, &observed, request, false) // Must rebuild, not restore.
	switch fault {
	case "corrupt", "truncate", "missing":
		want := imc.ErrCorrupt
		if fault == "missing" {
			want = fs.ErrNotExist
		}
		failing.mu.Lock()
		importErr := failing.importErr
		failing.mu.Unlock()
		if !errors.Is(importErr, want) {
			t.Fatalf("damaged snapshot import: got %v, want %v", importErr, want)
		}
		t.Logf("rejected damaged snapshot: %v", importErr)
	default:
		failing.requireTriggered(t)
	}
	page, err := store.List(t.Context(), imc.ListQuery{Limit: 10})
	if err != nil || len(page.Sessions) != 2 || page.Sessions[1] != before {
		t.Fatalf("fallback history: got %+v/%v, want new save and unchanged old summary", page, err)
	}
	repaired := page.Sessions[0]
	if repaired.Ref.Key == before.Ref.Key {
		t.Fatal("failed import overwrote the old conversation")
	}
	unload()
	if err := store.Close(); err != nil {
		t.Fatal(err) // Also checks that rejected imports released pinned readers.
	}
	verifyReopened(t, directory, repaired)
}

func recoveryDisk(t *testing.T, directory string) *disk.Store {
	t.Helper()
	store, err := disk.New(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	return store
}

func recoveryModel(t *testing.T, store imc.Store, observed *evidence) (*kronk.Kronk, func()) {
	t.Helper()
	krn, err := kronk.NewWithContext(t.Context(),
		model.WithModelFiles([]string{os.Getenv("KRONK_IMC_TEST_MODEL")}),
		model.WithContextWindow(2048), model.WithNSeqMax(1),
		model.WithIncrementalCache(true), model.WithCacheMinTokens(1),
		model.WithSpeculationMode(model.SpeculationDisabled),
		model.WithStorage(model.StorageConfig{Store: store, Timeout: 500 * time.Millisecond}),
		model.WithLog(observed.log))
	if err != nil {
		t.Fatal(err)
	}
	unload := sync.OnceFunc(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := krn.Unload(ctx); err != nil {
			t.Error(err)
		}
	})
	t.Cleanup(unload)
	return krn, unload
}

func recoveryChat(t *testing.T, krn *kronk.Kronk, observed *evidence, request model.D, reused bool) model.D {
	t.Helper()
	observed.mu.Lock()
	observed.target, observed.draft, observed.resume = false, false, ""
	observed.mu.Unlock()
	response, err := krn.Chat(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Choices) == 0 || response.Choices[0].Message == nil || response.Usage == nil {
		t.Fatal("missing response or usage")
	}
	phase := "save"
	if reused {
		phase = "restore"
	}
	if err := observed.check("text", phase, response); err != nil {
		t.Fatal(err)
	}
	if err := t.Context().Err(); err != nil {
		t.Fatalf("storage timeout canceled the parent request: %v", err)
	}
	t.Logf("answer=%q cached=%d", response.Choices[0].Message.Content, response.Usage.PromptTokensDetails.CachedTokens)
	next := request.Clone()
	next["messages"] = append(next["messages"].([]model.D),
		model.D{"role": "assistant", "content": response.Choices[0].Message.Content},
		model.D{"role": "user", "content": followUp("text")})
	return next
}

func onlySummary(t *testing.T, store imc.Store) imc.Summary {
	t.Helper()
	page, err := store.List(t.Context(), imc.ListQuery{Limit: 10})
	if err != nil || len(page.Sessions) != 1 {
		t.Fatalf("history: got %+v/%v, want one session", page, err)
	}
	return page.Sessions[0]
}

func savedContinuation(t *testing.T, store imc.Store, ref imc.Ref) model.D {
	t.Helper()
	record, err := inspectSnapshot(t.Context(), store, ref, "text", false)
	if err != nil {
		t.Fatal(err)
	}
	var request model.D
	var conversation struct {
		Messages []model.D `json:"messages"`
	}
	if err := errors.Join(json.Unmarshal(record.Request, &request), json.Unmarshal(record.Request, &conversation)); err != nil {
		t.Fatal(err)
	}
	request["messages"] = append(conversation.Messages,
		model.D{"role": "assistant", "content": record.AssistantText},
		model.D{"role": "user", "content": followUp("text")})
	return request
}

func verifyReopened(t *testing.T, directory string, saved imc.Summary) {
	t.Helper()
	store := recoveryDisk(t, directory)
	var observed evidence
	krn, _ := recoveryModel(t, store, &observed)
	recoveryChat(t, krn, &observed, savedContinuation(t, store, saved.Ref), true)
	page, err := store.List(t.Context(), imc.ListQuery{Limit: 10})
	if err != nil || len(page.Sessions) == 0 || page.Sessions[0].Ref.Key != saved.Ref.Key || page.Sessions[0].Ref.Revision <= saved.Ref.Revision {
		t.Fatalf("reopened restore did not advance the repaired session: %+v/%v", page, err)
	}
	t.Logf("recovered ID=%s revision=%d→%d", saved.Ref.Key.ID, saved.Ref.Revision, page.Sessions[0].Ref.Revision)
}

// faultStore delegates everything except the explicitly armed failure. Its
// borrowed snapshot data is never retained. Timeout faults honor the real
// storage context; save-after models an uncertain, already-published save.
type faultStore struct {
	imc.Store
	mu        sync.Mutex
	mode      string
	fired     int
	attempt   imc.Ref
	importErr error
}

func (s *faultStore) arm(mode string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mode, s.fired = mode, 0
}

func (s *faultStore) take(operation string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mode != operation && !strings.HasPrefix(s.mode, operation+"-") {
		return ""
	}
	s.fired++
	return s.mode
}

func (s *faultStore) requireTriggered(t *testing.T) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fired == 0 {
		t.Fatalf("storage fault %q was not exercised", s.mode)
	}
	t.Logf("exercised fault=%s occurrences=%d", s.mode, s.fired)
}

func (s *faultStore) attemptedRef() imc.Ref {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.attempt
}

func storageFault(ctx context.Context, mode string) error {
	if strings.HasSuffix(mode, "-timeout") {
		<-ctx.Done()
		return ctx.Err()
	}
	return fmt.Errorf("injected %s failure: %w", mode, fs.ErrPermission)
}

func (s *faultStore) Save(ctx context.Context, snapshot imc.Snapshot) error {
	s.mu.Lock()
	s.attempt = snapshot.Record.Summary.Ref
	s.mu.Unlock()
	mode := s.take("save")
	if mode == "" || mode == "save-after" {
		if err := s.Store.Save(ctx, snapshot); err != nil {
			return err
		}
		if mode == "" {
			return nil
		}
	}
	return storageFault(ctx, mode)
}

func (s *faultStore) Find(ctx context.Context, query imc.FindQuery) ([]imc.Summary, error) {
	if mode := s.take("find"); mode != "" {
		return nil, storageFault(ctx, mode)
	}
	return s.Store.Find(ctx, query)
}

func (s *faultStore) Open(ctx context.Context, ref imc.Ref) (imc.Session, error) {
	if mode := s.take("open"); mode != "" {
		return nil, storageFault(ctx, mode)
	}
	handle, err := s.Store.Open(ctx, ref)
	if err != nil {
		s.mu.Lock()
		s.importErr = err
		s.mu.Unlock()
		return nil, err
	}
	session := faultSession{Session: handle, store: s}
	return &session, nil
}

type faultSession struct {
	imc.Session
	store *faultStore
}

func (s *faultSession) Read(ctx context.Context, target, draft []byte, media [][]byte) error {
	if mode := s.store.take("read"); mode != "" {
		return storageFault(ctx, mode)
	}
	err := s.Session.Read(ctx, target, draft, media)
	s.store.mu.Lock()
	s.store.importErr = err
	s.store.mu.Unlock()
	return err
}

func (s *faultSession) Close() error {
	err := s.Session.Close()
	if mode := s.store.take("close"); mode != "" {
		return errors.Join(err, storageFault(context.Background(), mode))
	}
	return err
}
