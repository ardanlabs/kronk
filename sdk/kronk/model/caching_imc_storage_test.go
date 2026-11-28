package model

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io/fs"
	"slices"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/imc"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// Only the persistence operations used by the engine are implemented here.
// These are synthetic native bytes: planner round trips do not exercise llama.
type imcTestStorage struct {
	imc.Store
	snapshots []imc.Snapshot
	saveErr   error
	findErr   error
	findLimit int
	readErr   error
	closeErr  error
	size      int64
	query     imc.FindQuery
	onSave    func()
	onFind    func()
	openCount int
}

func (s *imcTestStorage) Save(ctx context.Context, snapshot imc.Snapshot) error {
	if s.onSave != nil {
		s.onSave()
	}
	if err := errors.Join(ctx.Err(), s.saveErr); err != nil {
		return err
	}
	for i, saved := range s.snapshots {
		if saved.Record.Summary.Ref.Key == snapshot.Record.Summary.Ref.Key {
			if saved.Record.Summary.Ref.Revision >= snapshot.Record.Summary.Ref.Revision {
				return imc.ErrConflict
			}
			s.snapshots = slices.Delete(s.snapshots, i, i+1)
			break
		}
	}
	// Detach borrowed data just as a backend would consume it before return.
	data, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	var saved imc.Snapshot
	if err := json.Unmarshal(data, &saved); err != nil {
		return err
	}
	s.snapshots = append(s.snapshots, saved)
	return nil
}

func (s *imcTestStorage) Find(ctx context.Context, q imc.FindQuery) ([]imc.Summary, error) {
	s.query = q
	if s.onFind != nil {
		s.onFind()
	}
	if err := errors.Join(ctx.Err(), s.findErr); err != nil {
		return nil, err
	}
	var results []imc.Summary
	for _, saved := range s.snapshots {
		r := saved.Record.Summary
		if r.Ref.Key.Model == q.Model && r.Compatibility == q.Compatibility && r.Prefix.Length <= q.MaxPrefixLength {
			results = append(results, r)
		}
	}
	slices.SortStableFunc(results, func(a, b imc.Summary) int { return b.SavedAt.Compare(a.SavedAt) })
	if s.findLimit > 0 {
		results = results[:min(s.findLimit, len(results))]
	}
	return results[:min(q.Limit, len(results))], nil
}

func (s *imcTestStorage) Open(ctx context.Context, ref imc.Ref) (imc.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for _, saved := range s.snapshots {
		if saved.Record.Summary.Ref == ref {
			s.openCount++
			return &imcTestStoredSession{store: s, snapshot: saved}, nil
		}
	}
	return nil, fs.ErrNotExist
}

type imcTestStoredSession struct {
	store    *imcTestStorage
	snapshot imc.Snapshot
}

func (s *imcTestStoredSession) Record() imc.Record { return s.snapshot.Record }

func (s *imcTestStoredSession) Sizes() (int64, int64, []int64) {
	sizes := make([]int64, len(s.snapshot.Media))
	for i, data := range s.snapshot.Media {
		sizes[i] = int64(len(data))
	}
	target := int64(len(s.snapshot.Target))
	if s.store.size != 0 {
		target = s.store.size
	}
	return target, int64(len(s.snapshot.Draft)), sizes
}

func (s *imcTestStoredSession) Read(ctx context.Context, target, draft []byte, media [][]byte) error {
	if err := errors.Join(ctx.Err(), s.store.readErr); err != nil {
		return err
	}
	copy(target, s.snapshot.Target)
	copy(draft, s.snapshot.Draft)
	for i := range media {
		copy(media[i], s.snapshot.Media[i])
	}
	return nil
}

func (s *imcTestStoredSession) Close() error {
	s.store.openCount--
	return s.store.closeErr
}

func storageTestModel(store *imcTestStorage) *Model {
	m := Model{
		cfg: Config{Storage: StorageConfig{Store: store}, PtrIncrementalCache: new(true), PtrCacheMinTokens: new(1), PtrContextWindow: new(32)},
		log: applog.DiscardLogger, modelInfo: ModelInfo{ID: "test-model"}, imcCompatibility: [32]byte{3, 7},
		imcSessions: []*imcSession{{id: 0, cache: &imcPrefix{}, kvState: &imcCheckpoint{}}},
	}
	return &m
}

func publishStorageTestSession(t *testing.T, m *Model, media bool) (*chatJob, D) {
	t.Helper()
	d := D{"messages": []D{{"role": "user", "content": "hello"}}, "enable_thinking": false}
	s := m.imcSessions[0]
	s.cache = &imcPrefix{cachedTokens: []llama.Token{10, 20}, totalTokensCached: 2, cachedMsgCount: 1, cachedMsgsHash: documentMessagesHash(d)}
	s.cache.cachedRenderInputHash, _ = m.imcRenderFingerprint(d, dMessages(d))
	s.samplingSeed, s.hasSamplingSeed = 17, true
	s.reserved = true
	copy(prepareTestStore(t, s.kvState, 3), []byte{5, 9, 2})
	commitTestStore(t, s.kvState, 3)
	if media {
		payload := []byte("image one")
		s.cache.cachedTokens = nil
		s.cache.hasMedia, s.cache.useMRoPE = true, true
		s.cache.totalTokensCached, s.cache.nextLogicalPos = 5, 4
		s.cache.promptPlan = promptPlan{units: []promptUnit{{token: 10}, {isMedia: true, media: sha256.Sum256(payload)}, {token: 20}}, textTokens: 2, mediaCount: 1, mediaBytes: [][]byte{payload}}
		s.cache.mediaKVCounts = []int{3}
		s.cache.mediaNativeChunks = []imcMediaChunk{{kind: imcMediaChunkText, tokens: []llama.Token{10}}, {kind: imcMediaChunkImage, nTokens: 3, nPos: 2}, {kind: imcMediaChunkText, tokens: []llama.Token{20}}}
		s.cache.samplerPromptTokens = []llama.Token{10, 20}
		s.draftKVState = &imcCheckpoint{}
		copy(prepareTestStore(t, s.draftKVState, 2), []byte{7, 3})
		commitTestStore(t, s.draftKVState, 2)
		s.pendingH = []float32{0.25, -0.5}
	}
	m.imcPublishSession(s)
	job := chatJob{imcSession: s, imcSessionID: s.id, imcReservationHeld: true, imcUsageVersion: m.imcBeginRequestUsage(s),
		imcRequest: d.Clone(), d: d, params: Params{Temperature: 0.35}}
	return &job, d
}

func saveStorageTestSession(t *testing.T, m *Model, job *chatJob, text string) {
	t.Helper()
	request, err := imcStorageRequest(job)
	if err != nil {
		t.Fatal(err)
	}
	m.imcSaveCompleted(t.Context(), job, request, text)
	m.imcReleaseReservation(job.imcSessionID)
}

func TestIMCStorageRoundTrip(t *testing.T) {
	for _, media := range []bool{false, true} {
		name := "text"
		if media {
			name = "media-and-draft"
		}
		t.Run(name, func(t *testing.T) {
			store := &imcTestStorage{}
			m := storageTestModel(store)
			job, d := publishStorageTestSession(t, m, media)
			// Backend I/O must not hold cacheMu, and the RAM bytes must remain
			// reserved until the synchronous consumer returns.
			store.onSave = func() {
				if m.IMCSessions()[0].State != IMCSessionStateActive {
					t.Error("snapshot became available for mutation during Save")
				}
			}
			saveStorageTestSession(t, m, job, "final answer")
			if len(store.snapshots) != 1 {
				t.Fatalf("saved sessions = %d, want 1", len(store.snapshots))
			}
			saved := store.snapshots[0]
			if saved.Record.AssistantText != "final answer" || !bytes.Equal(saved.Target, []byte{5, 9, 2}) {
				t.Fatalf("saved response/checkpoint = %q/%v", saved.Record.AssistantText, saved.Target)
			}
			var request struct {
				Messages    []D     `json:"messages"`
				Temperature float64 `json:"temperature"`
			}
			if err := json.Unmarshal(saved.Record.Request, &request); err != nil {
				t.Fatal(err)
			}
			if len(request.Messages) != 1 || request.Messages[0]["content"] != "hello" || request.Temperature < 0.349 || request.Temperature > 0.351 {
				t.Fatalf("saved request = %+v, want only input and effective parameters", request)
			}
			resetTestSession(t, job.imcSession)
			store.onFind = func() { m.IMCSessions() }
			var result cacheResult
			if media {
				stable := promptPlan{units: []promptUnit{{token: 10}, {isMedia: true, media: sha256.Sum256([]byte("image one"))}, {token: 20}, {token: 30}}, textTokens: 3, mediaCount: 1, mediaBytes: [][]byte{[]byte("image one")}}
				actual := stable
				actual.units = append(slices.Clone(stable.units), promptUnit{token: 40})
				result = m.processIMCMediaPlans(t.Context(), d, d, actual, stable, []llama.Token{40}, nil, time.Now())
			} else {
				result = m.processIMCTokenPlan(t.Context(), d, []llama.Token{10, 20, 30, 40}, []llama.Token{10, 20, 30}, nil, time.Now(), false)
			}
			wantPosition := llama.Pos(2)
			if media {
				wantPosition = 4
			}
			if result.err != nil || result.cacheIdx != wantPosition || !slices.Equal(result.imcNewCacheTokens, []llama.Token{30}) {
				t.Fatalf("restored plan position/extension/error = %d/%v/%v", result.cacheIdx, result.imcNewCacheTokens, result.err)
			}
			s := result.imcSession
			if s.stored.Ref != saved.Record.Summary.Ref || s.samplingSeed != 17 || !s.hasSamplingSeed || !bytes.Equal(bytesTestStore(t, s.kvState), []byte{5, 9, 2}) {
				t.Error("import did not preserve conversation identity, sampling seed, and checkpoint")
			}
			if media && (!bytes.Equal(bytesTestStore(t, s.draftKVState), []byte{7, 3}) || !slices.Equal(s.pendingH, []float32{0.25, -0.5}) || len(s.cache.mediaNativeChunks) != 3) {
				t.Error("import did not preserve paired draft and native media metadata")
			}
			if store.openCount != 0 || !s.reserved || store.query.Limit != 100 || store.query.Model != "test-model" {
				t.Errorf("open handles/reservation/query = %d/%t/%+v", store.openCount, s.reserved, store.query)
			}
			// A later successful save replaces this same durable conversation,
			// including across an eviction/import, rather than creating history.
			job.imcSession = s
			job.imcUsageVersion = m.imcBeginRequestUsage(s)
			saveStorageTestSession(t, m, job, "next answer")
			if len(store.snapshots) != 1 || store.snapshots[0].Record.Summary.Ref.Key != saved.Record.Summary.Ref.Key || store.snapshots[0].Record.Summary.Ref.Revision != 2 {
				t.Error("next save did not replace the imported conversation at revision 2")
			}
		})
	}
}

func TestIMCStorageSaveFailurePreservesRAMAndRecovers(t *testing.T) {
	store := &imcTestStorage{saveErr: fs.ErrPermission}
	m := storageTestModel(store)
	job, d := publishStorageTestSession(t, m, false)
	saveStorageTestSession(t, m, job, "unsaved answer")
	if len(store.snapshots) != 0 || !bytes.Equal(bytesTestStore(t, job.imcSession.kvState), []byte{5, 9, 2}) {
		t.Fatal("failed save changed the usable RAM checkpoint")
	}
	store.findErr = errors.New("RAM hit must not need storage")
	result := m.processIMCTokenPlan(t.Context(), d, []llama.Token{10, 20, 40}, []llama.Token{10, 20}, nil, time.Now(), false)
	if result.err != nil || result.imcMatchKind != "exact" || result.cacheIdx != 2 {
		t.Fatalf("RAM reuse after save failure = %+v", result)
	}
	store.saveErr = nil
	job.imcUsageVersion = m.imcBeginRequestUsage(job.imcSession)
	saveStorageTestSession(t, m, job, "repaired answer")
	if len(store.snapshots) != 1 || store.snapshots[0].Record.Summary.Ref.Revision != 2 {
		t.Fatal("subsequent full save did not recover persistence")
	}
}

func TestIMCStorageRejectedImportsRebuild(t *testing.T) {
	for _, name := range []string{"find error", "read error", "close error", "oversized", "negative size", "incompatible", "schema", "render fingerprint", "different tokens", "active resident"} {
		t.Run(name, func(t *testing.T) {
			store := &imcTestStorage{}
			m := storageTestModel(store)
			job, d := publishStorageTestSession(t, m, false)
			saveStorageTestSession(t, m, job, "answer")
			resetTestSession(t, job.imcSession)
			switch name {
			case "find error":
				store.findErr = fs.ErrPermission
			case "read error":
				store.readErr = imc.ErrCorrupt
			case "close error":
				store.closeErr = fs.ErrPermission
			case "oversized":
				store.size = 5 << 30
			case "negative size":
				store.size = -1
			case "incompatible":
				m.imcCompatibility = [32]byte{99}
			case "schema":
				store.snapshots[0].Record.Version = 99
			case "render fingerprint":
				d["enable_thinking"] = true
			case "different tokens":
				// Discovery claims a matching digest, but the opened metadata
				// still describes the old tokens. Hashes cannot authorize reuse.
				store.snapshots[0].Record.Summary.Prefix.Hash = messagePrefixHash([]llama.Token{10, 99}, nil, []int{2})[2]
			case "active resident":
				m.imcSessions = append(m.imcSessions, &imcSession{id: 1, cache: &imcPrefix{}, kvState: &imcCheckpoint{}, reserved: true, stored: store.snapshots[0].Record.Summary})
			}
			stable := []llama.Token{10, 20}
			if name == "different tokens" {
				stable[1] = 99
			}
			result := m.processIMCTokenPlan(t.Context(), d, append(slices.Clone(stable), 40), stable, nil, time.Now(), false)
			if result.err != nil || result.cacheIdx != 0 || !result.imcClearSeq || result.imcMatchKind != "rebuild" {
				t.Fatalf("optional storage failure did not rebuild: %+v", result)
			}
			if store.openCount != 0 {
				t.Error("rejected import leaked a pinned session")
			}
		})
	}
}

func TestIMCStorageSearchWindowAndLongestPrefix(t *testing.T) {
	for _, limit := range []int{1, 2} {
		store := &imcTestStorage{findLimit: limit}
		m := storageTestModel(store)
		job, d := publishStorageTestSession(t, m, false)
		saveStorageTestSession(t, m, job, "older, longer")
		firstID := store.snapshots[0].Record.Summary.Ref.Key.ID
		resetTestSession(t, job.imcSession)
		job, _ = publishStorageTestSession(t, m, false)
		job.imcSession.cache.cachedTokens = []llama.Token{10}
		job.imcSession.cache.totalTokensCached = 1
		m.imcPublishSession(job.imcSession)
		saveStorageTestSession(t, m, job, "recent, shorter")
		store.snapshots[1].Record.Summary.SavedAt = store.snapshots[0].Record.Summary.SavedAt.Add(time.Minute)
		if store.snapshots[1].Record.Summary.Ref.Key.ID == firstID {
			t.Fatal("unrelated conversation reused a durable ID")
		}
		resetTestSession(t, job.imcSession)
		result := m.processIMCTokenPlan(t.Context(), d, []llama.Token{10, 20, 30, 40}, []llama.Token{10, 20, 30}, nil, time.Now(), false)
		if result.err != nil || int(result.cacheIdx) != limit || store.query.Limit != 100 || store.query.MaxPrefixLength != 3 {
			t.Fatalf("limit %d: prefix/query/error = %d/%+v/%v", limit, result.cacheIdx, store.query, result.err)
		}
	}
}

func TestIMCStorageDifferentMediaRebuilds(t *testing.T) {
	store := &imcTestStorage{}
	m := storageTestModel(store)
	job, d := publishStorageTestSession(t, m, true)
	saveStorageTestSession(t, m, job, "answer")
	resetTestSession(t, job.imcSession)
	// Force a media SHA collision in the incoming plan. Exact payload
	// comparison must still reject it, just as it does for resident sessions.
	stable := promptPlan{units: []promptUnit{{token: 10}, {isMedia: true, media: sha256.Sum256([]byte("image one"))}, {token: 20}}, textTokens: 2, mediaCount: 1, mediaBytes: [][]byte{[]byte("image two")}}
	actual := stable
	actual.units = append(slices.Clone(stable.units), promptUnit{token: 40})
	result := m.processIMCMediaPlans(t.Context(), d, d, actual, stable, []llama.Token{40}, nil, time.Now())
	if result.err != nil || result.cacheIdx != 0 || !result.imcMediaBuild {
		t.Fatalf("different payload reused a stored image checkpoint: %+v", result)
	}
	if store.openCount != 0 {
		t.Error("media mismatch leaked a pinned session")
	}
}

func TestStorageConfiguration(t *testing.T) {
	for _, tt := range []struct {
		name    string
		imc     *bool
		storage StorageConfig
		wantErr bool
	}{
		{name: "default IMC", storage: StorageConfig{Store: &imcTestStorage{}}},
		{name: "explicit IMC", imc: new(true), storage: StorageConfig{Store: &imcTestStorage{}}},
		{name: "disabled IMC", imc: new(false), storage: StorageConfig{Store: &imcTestStorage{}}, wantErr: true},
		{name: "negative byte limit", storage: StorageConfig{Store: &imcTestStorage{}, MaxBytes: -1}, wantErr: true},
		{name: "negative timeout", storage: StorageConfig{Store: &imcTestStorage{}, Timeout: -1}, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NewConfig(WithModelFiles([]string{"test.gguf"}), WithStorage(tt.storage))
			cfg.PtrIncrementalCache = tt.imc
			if err := validateConfig(t.Context(), cfg, applog.DiscardLogger); (err != nil) != tt.wantErr {
				t.Fatalf("storage configuration error = %v, wantErr %t", err, tt.wantErr)
			}
		})
	}
}
