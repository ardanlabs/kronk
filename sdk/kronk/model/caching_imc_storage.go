package model

import (
	"cmp"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"
	"slices"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/imc"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// imcRestoreV1 describes the input checkpoint, never the live generation
// sequence. Keep this schema independent of native struct layouts and resident
// bookkeeping. Media payloads correspond to media units in encounter order.
type imcRestoreV1 struct {
	Tokens        []llama.Token
	Cells         int
	Position      int
	Messages      int
	MessagesHash  string
	RenderHash    string
	HasMedia      bool
	MRoPE         bool
	NonCausal     bool
	MediaKVCounts []int
	Units         []imcStoredUnit
	Chunks        []imcStoredChunk
	SamplerTokens []llama.Token
	Seed          uint32
	HasSeed       bool
	PendingH      []float32
}

type imcStoredUnit struct {
	Token   llama.Token
	Media   [sha256.Size]byte
	IsMedia bool
}

type imcStoredChunk struct {
	Kind    uint8
	Tokens  []llama.Token
	NTokens int
	NPos    int
}

func (cfg StorageConfig) limits() (timeout time.Duration, maxBytes int64) {
	timeout, maxBytes = cfg.Timeout, cfg.MaxBytes
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	if maxBytes == 0 {
		maxBytes = 4 << 30
	}
	return timeout, maxBytes
}

func (m *Model) imcStorageEnabled() bool {
	return m.cfg.Storage.Store != nil && m.imcCompatibility != [sha256.Size]byte{}
}

// imcStorageCompatibility deliberately rejects more than the native format
// version alone: changing weights, adapters, projector, template, runtime, or
// execution configuration must not reuse opaque state from another setup.
// This runs once at model construction, not on the batch engine.
func (m *Model) imcStorageCompatibility(ctx context.Context) ([sha256.Size]byte, error) {
	files := slices.Clone(m.cfg.ModelFiles)
	for _, path := range []string{m.cfg.ProjFile, m.cfg.MTPDrafterFile} {
		if path != "" {
			files = append(files, path)
		}
	}
	if m.cfg.PtrDraftModel != nil {
		files = append(files, m.cfg.PtrDraftModel.ModelFiles...)
	}
	for _, adapter := range m.cfg.Adapters {
		files = append(files, adapter.Path)
	}
	digests := make([]string, 0, len(files))
	for _, path := range files {
		if err := ctx.Err(); err != nil {
			return [sha256.Size]byte{}, err
		}
		// Configured digests have already been verified by validateConfig.
		digest := m.cfg.ArtifactIntegrity[path].Digest.SHA256
		if digest == "" {
			f, err := os.Open(path)
			if err != nil {
				return [sha256.Size]byte{}, fmt.Errorf("open compatibility artifact: %w", err)
			}
			h := sha256.New()
			_, readErr := io.Copy(h, f)
			if err := errors.Join(readErr, f.Close()); err != nil {
				return [sha256.Size]byte{}, fmt.Errorf("hash compatibility artifact: %w", err)
			}
			digest = fmt.Sprintf("%x", h.Sum(nil))
		}
		digests = append(digests, digest)
	}
	bindings := ""
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range info.Deps {
			if dep.Path == "github.com/hybridgroup/yzma" {
				bindings = dep.Version
				if dep.Replace != nil {
					return [sha256.Size]byte{}, errors.New("storage compatibility requires versioned yzma bindings")
				}
			}
		}
	}
	data, err := json.Marshal(struct {
		Schema, NativeState                          int
		Native, Bindings, OS, Arch, Config, Template string
		Artifacts                                    []string
		Context                                      llama.ContextParams
	}{1, llama.StateSeqVersion, llama.Version(), bindings, runtime.GOOS, runtime.GOARCH, m.cfg.String(), m.template.Script, digests, m.ctxParams})
	if err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("encode compatibility: %w", err)
	}
	return sha256.Sum256(data), nil
}

// imcStorageRequest preserves recoverable input and effective settings without
// appending the completion or retaining any caller-owned JSON containers.
func imcStorageRequest(job *chatJob) (json.RawMessage, error) {
	params, err := json.Marshal(job.params)
	if err != nil {
		return nil, fmt.Errorf("encode request parameters: %w", err)
	}
	var request map[string]any
	if err := json.Unmarshal(params, &request); err != nil {
		return nil, fmt.Errorf("decode request parameters: %w", err)
	}
	for key, value := range job.imcRequest {
		if _, exists := request[key]; !exists {
			request[key] = value
		}
	}
	for _, key := range []string{"enable_thinking", "reasoning_effort", "chat_template_kwargs"} {
		if value, ok := job.d[key]; ok {
			request[key] = value
		}
	}
	return json.Marshal(request)
}

// imcNewStoredIdentity runs under cacheMu when a RAM entry starts unrelated
// work. Revisions are attempt numbers, so a later full save also repairs an
// earlier uncertain publication without depending on its payload.
func (m *Model) imcNewStoredIdentity(s *imcSession) {
	if !m.imcStorageEnabled() {
		return
	}
	s.stored = imc.Summary{
		Ref:       imc.Ref{Key: imc.Key{Model: m.modelInfo.ID, ID: rand.Text()}},
		CreatedAt: time.Now().UTC(), Compatibility: m.imcCompatibility,
	}
}

// imcSaveCompleted runs while the job owns the session reservation. No cache
// lock is held during encoding or storage I/O, and all buffers remain borrowed.
func (m *Model) imcSaveCompleted(ctx context.Context, job *chatJob, request json.RawMessage, assistant string) {
	s := job.imcSession
	if !m.imcStorageEnabled() || !job.hasIMCReservation() || s.storageConflict || len(request) == 0 ||
		s.usageVersion != job.imcUsageVersion || s.cache.totalTokensCached == 0 || s.kvState.Len() == 0 {
		return
	}
	timeout, maxBytes := m.cfg.Storage.limits()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := m.imcSaveSnapshot(ctx, s, request, assistant, maxBytes); err != nil {
		if errors.Is(err, imc.ErrConflict) {
			s.storageConflict = true
		}
		m.log(ctx, "imc-storage", "status", "save-failed", "imc_cache_entry", s.id, "err", err)
	}
}

func (m *Model) imcSaveSnapshot(ctx context.Context, s *imcSession, request json.RawMessage, assistant string, maxBytes int64) error {
	c := s.cache
	r := imcRestoreV1{
		Tokens: c.cachedTokens, Cells: c.totalTokensCached, Position: c.logicalPosition(),
		Messages: c.cachedMsgCount, MessagesHash: c.cachedMsgsHash, RenderHash: c.cachedRenderInputHash,
		HasMedia: c.hasMedia, MRoPE: c.useMRoPE, NonCausal: c.useNonCausal,
		MediaKVCounts: c.mediaKVCounts, SamplerTokens: c.samplerPromptTokens,
		Seed: s.samplingSeed, HasSeed: s.hasSamplingSeed,
	}
	for _, unit := range c.promptPlan.units {
		r.Units = append(r.Units, imcStoredUnit{unit.token, unit.media, unit.isMedia})
	}
	for _, chunk := range c.mediaNativeChunks {
		r.Chunks = append(r.Chunks, imcStoredChunk{chunk.kind, chunk.tokens, chunk.nTokens, chunk.nPos})
	}
	target, err := s.kvState.Bytes(ctx)
	if err != nil {
		return err
	}
	var draft []byte
	if s.draftKVState != nil && s.draftKVState.Len() > 0 && len(s.pendingH) > 0 {
		draft, err = s.draftKVState.Bytes(ctx)
		if err != nil {
			return err
		}
		r.PendingH = s.pendingH
	}
	media := c.promptPlan.mediaBytes
	sizes := make([]int64, len(media))
	for i, data := range media {
		sizes[i] = int64(len(data))
	}
	if err := imcCheckPayloadSizes(int64(len(target)), int64(len(draft)), sizes, maxBytes); err != nil {
		return err
	}
	restore, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("encode restore metadata: %w", err)
	}
	m.cacheMu.Lock()
	if s.stored.Ref.Key.ID == "" {
		m.imcNewStoredIdentity(s)
	}
	if s.stored.Ref.Revision == ^uint64(0) {
		m.cacheMu.Unlock()
		return imc.ErrConflict
	}
	s.stored.Ref.Revision++
	s.stored.SavedAt = time.Now().UTC()
	s.stored.Prefix = imc.Prefix{Length: s.prefixLen, Hash: s.prefixHash}
	preview := []rune(assistant)
	s.stored.Preview = string(preview[:min(len(preview), 160)])
	summary := s.stored
	m.cacheMu.Unlock()
	return m.cfg.Storage.Store.Save(ctx, imc.Snapshot{
		Record: imc.Record{Version: 1, Summary: summary, Request: request, AssistantText: assistant, Restore: restore},
		Target: target, Draft: draft, Media: media,
	})
}

func imcCheckPayloadSizes(target, draft int64, media []int64, limit int64) error {
	if target <= 0 || draft < 0 {
		return imc.ErrCorrupt
	}
	remaining := limit
	for _, n := range append([]int64{target, draft}, media...) {
		if n < 0 || n > remaining || uint64(n) > uint64(^uint(0)>>1) {
			return fmt.Errorf("storage payload exceeds configured or platform limit")
		}
		remaining -= n
	}
	return nil
}

// imcFindStoredSession is entered and returns with cacheMu held. It reserves
// one destination before dropping the lock for I/O. Exact verification uses the
// same predicate as the resident planner; a digest never authorizes restoration.
func (m *Model) imcFindStoredSession(ctx context.Context, tokens []llama.Token, units []promptUnit, matches func(*imcSession) bool) *imcSession {
	if !m.imcStorageEnabled() {
		return nil
	}
	destination := m.imcRebuildSession()
	if destination == nil {
		return nil
	}
	destination.reserved = true
	m.cacheMu.Unlock()
	loaded, err := m.imcLoadStoredSession(ctx, tokens, units, matches)
	m.cacheMu.Lock()
	defer func() { destination.reserved = false }()
	if err != nil {
		m.log(ctx, "imc-storage", "status", "find-failed", "err", err)
		return nil
	}
	if loaded == nil {
		return nil
	}
	// Do not fork a conversation that another slot/import already owns, nor
	// replace a newer RAM version with an older successfully persisted one.
	for _, resident := range m.imcSessions {
		if resident.stored.Ref.Key == loaded.stored.Ref.Key {
			return nil
		}
	}
	// No native objects are changed here. startSlot uses the ordinary RAM
	// restore path, with the same native error handling as every other hit.
	id := destination.id
	*destination = *loaded
	destination.id = id
	destination.seqID = imcSeqIDUnbound
	destination.lastUsed = time.Now()
	return destination
}

func (m *Model) imcLoadStoredSession(ctx context.Context, tokens []llama.Token, units []promptUnit, matches func(*imcSession) bool) (*imcSession, error) {
	timeout, maxBytes := m.cfg.Storage.limits()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	length := len(tokens)
	if units != nil {
		length = len(units)
	}
	query := imc.FindQuery{Model: m.modelInfo.ID, Compatibility: m.imcCompatibility, Limit: 100, MaxPrefixLength: length}
	summaries, err := m.cfg.Storage.Store.Find(ctx, query)
	if err != nil {
		return nil, err
	}
	if len(summaries) > query.Limit {
		return nil, errors.New("storage returned more candidates than requested")
	}
	lengths := make([]int, 0, len(summaries))
	for _, summary := range summaries {
		if summary.Ref.Key.Model == query.Model && summary.Ref.Key.ID != "" &&
			summary.Ref.Revision > 0 && summary.Compatibility == query.Compatibility && summary.Prefix.Length > 0 && summary.Prefix.Length <= length {
			lengths = append(lengths, summary.Prefix.Length)
		}
	}
	slices.Sort(lengths)
	hashes := messagePrefixHash(tokens, units, slices.Compact(lengths))
	// Prefer the longest complete prefix inside the backend's recent window.
	slices.SortStableFunc(summaries, func(a, b imc.Summary) int { return cmp.Compare(b.Prefix.Length, a.Prefix.Length) })
	for _, summary := range summaries {
		if summary.Ref.Key.Model != query.Model || summary.Ref.Key.ID == "" ||
			summary.Ref.Revision == 0 || summary.Compatibility != query.Compatibility || summary.Prefix.Length <= 0 {
			continue
		}
		if hash, ok := hashes[summary.Prefix.Length]; !ok || hash != summary.Prefix.Hash {
			continue
		}
		s, err := m.imcOpenStoredSession(ctx, summary, maxBytes)
		if err != nil {
			m.log(ctx, "imc-storage", "status", "open-failed", "err", err)
			continue
		}
		if matches(s) {
			return s, nil
		}
	}
	return nil, nil
}

func (m *Model) imcOpenStoredSession(ctx context.Context, summary imc.Summary, maxBytes int64) (_ *imcSession, err error) {
	handle, err := m.cfg.Storage.Store.Open(ctx, summary.Ref)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, handle.Close()) }()
	record := handle.Record()
	if record.Version != 1 {
		return nil, imc.ErrVersion
	}
	if record.Summary != summary {
		return nil, imc.ErrCorrupt
	}
	var r imcRestoreV1
	if err := json.Unmarshal(record.Restore, &r); err != nil {
		return nil, fmt.Errorf("decode restore metadata: %w", err)
	}
	if r.Cells <= 0 || r.Cells > m.cfg.ContextWindow() || r.Position <= 0 || r.Position > m.cfg.ContextWindow() ||
		r.Messages <= 0 || r.RenderHash == "" || len(r.Tokens) > m.cfg.ContextWindow() || len(r.Units) > m.cfg.ContextWindow() {
		return nil, imc.ErrCorrupt
	}
	targetSize, draftSize, mediaSizes := handle.Sizes()
	if err := imcCheckPayloadSizes(targetSize, draftSize, mediaSizes, maxBytes); err != nil {
		return nil, err
	}
	c := imcPrefix{cachedTokens: r.Tokens, totalTokensCached: r.Cells, nextLogicalPos: r.Position,
		cachedMsgCount: r.Messages, cachedMsgsHash: r.MessagesHash, cachedRenderInputHash: r.RenderHash,
		hasMedia: r.HasMedia, useMRoPE: r.MRoPE, useNonCausal: r.NonCausal, mediaKVCounts: r.MediaKVCounts, samplerPromptTokens: r.SamplerTokens}
	for _, unit := range r.Units {
		c.promptPlan.units = append(c.promptPlan.units, promptUnit{unit.Token, unit.Media, unit.IsMedia})
		if unit.IsMedia {
			c.promptPlan.mediaCount++
		} else {
			c.promptPlan.textTokens++
		}
	}
	if len(mediaSizes) != c.promptPlan.mediaCount || (!r.HasMedia && (len(r.Tokens) != r.Cells || r.Position != r.Cells || len(r.Units) != 0)) {
		return nil, imc.ErrCorrupt
	}
	for _, chunk := range r.Chunks {
		if chunk.Kind < imcMediaChunkText || chunk.Kind > imcMediaChunkAudio ||
			(chunk.Kind != imcMediaChunkText && (chunk.NTokens <= 0 || chunk.NTokens > r.Cells || chunk.NPos <= 0 || chunk.NPos > r.Position)) {
			return nil, imc.ErrCorrupt
		}
		c.mediaNativeChunks = append(c.mediaNativeChunks, imcMediaChunk{chunk.Kind, chunk.Tokens, chunk.NTokens, chunk.NPos})
	}
	s := imcSession{cache: &c, kvState: &imcCheckpoint{}, stored: summary, samplingSeed: r.Seed, hasSamplingSeed: r.HasSeed}
	s.updatePrefixHash()
	if s.prefixLen != summary.Prefix.Length || s.prefixHash != summary.Prefix.Hash {
		return nil, imc.ErrCorrupt
	}
	target, err := s.kvState.Prepare(ctx, int(targetSize))
	if err != nil {
		return nil, err
	}
	var draft []byte
	if draftSize > 0 {
		s.draftKVState = &imcCheckpoint{}
		draft, err = s.draftKVState.Prepare(ctx, int(draftSize))
		if err != nil {
			return nil, err
		}
	}
	c.promptPlan.mediaBytes = make([][]byte, len(mediaSizes))
	for i, size := range mediaSizes {
		c.promptPlan.mediaBytes[i] = make([]byte, int(size))
	}
	if err := handle.Read(ctx, target, draft, c.promptPlan.mediaBytes); err != nil {
		return nil, err
	}
	mediaIndex := 0
	for _, unit := range c.promptPlan.units {
		if unit.isMedia {
			if sha256.Sum256(c.promptPlan.mediaBytes[mediaIndex]) != unit.media {
				return nil, imc.ErrCorrupt
			}
			mediaIndex++
		}
	}
	if r.HasMedia {
		remaining := r.Cells
		for _, count := range r.MediaKVCounts {
			if count <= 0 || count > remaining {
				return nil, imc.ErrCorrupt
			}
			remaining -= count
		}
	}
	if err := s.kvState.Commit(ctx, len(target)); err != nil {
		return nil, err
	}
	if draftSize > 0 {
		if err := s.draftKVState.Commit(ctx, len(draft)); err != nil {
			return nil, err
		}
		s.pendingH = r.PendingH
	}
	// An own-KV drafter still needs an empty store when the saved snapshot
	// has no usable draft. The existing resume path handles target-only fallback.
	if _, ok := m.draft.(draftKVExternalizer); ok && s.draftKVState == nil {
		s.draftKVState = &imcCheckpoint{}
	}
	s.snapshotBytes = imcSnapshotBytes(s.kvState, s.draftKVState, s.pendingH)
	s.allocatedContext = c.totalTokensCached
	s.highWaterContext = c.totalTokensCached
	return &s, nil
}
