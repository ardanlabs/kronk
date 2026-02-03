package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// cacheResult contains the results of cache processing.
type cacheResult struct {
	modifiedD     D           // D with cached messages removed if cache was used
	newDataCached string      // The new data to cache when there is new data
	media         [][]byte    // Media from templating (set when caching occurs)
	nPast         llama.Pos   // Starting position for new tokens (cumulative from both caches)
	cached        bool        // True if any cache is being used
	err           error       // Any error that occurred
	imcID         string      // IMC session ID
	imcSeqID      llama.SeqId // IMC session's cache sequence
}

// processCache checks if the system prompt or incremental messages are
// being cached and updates to the caches are necessary. The behavior depends
// on which cache modes are enabled:
//
//   - SystemPromptCache (SPC): Caches the first message with role="system".
//   - IncrementalCache (IMC): Caches all messages except the last one.
//
// SPC and IMC are mutually exclusive
// IMC includes the system prompt in its cache.
//
// Returns a cacheResult containing:
//   - modifiedD: D with cached messages removed
//   - newDataCached: The templated prompt (only set when this function handles templating)
//   - media: Media bytes from templating (only set when this function handles templating)
//   - nPast: cumulative starting position from all cache hits
//   - cached: true if any message was cached and can be reused
//   - err: any error that occurred during cache update
//
// This function is thread-safe and handles concurrent requests appropriately.
func (m *Model) processCache(ctx context.Context, d D) cacheResult {
	if !m.cfg.SystemPromptCache && !m.cfg.IncrementalCache {
		return cacheResult{modifiedD: d}
	}

	if m.cfg.SystemPromptCache {
		return m.processSPC(ctx, d)
	}

	return m.processIMC(ctx, d)
}

// processSPC orchestrates the system prompt caching flow. It examines
// the first message and either caches it (if it's a system message) or reuses
// an existing cache (if the client omitted the system message on a follow-up
// request). The system message is always removed from d after processing.
func (m *Model) processSPC(ctx context.Context, d D) cacheResult {
	messages, ok := d["messages"].([]D)
	if !ok || len(messages) == 0 {
		return cacheResult{modifiedD: d}
	}

	role, ok := messages[0]["role"].(string)
	if !ok {
		return cacheResult{modifiedD: d}
	}

	// We need to use this function because we don't know what
	// format that first message is in. Input messages can be
	// in different formats.
	content := extractMessageContent(messages[0])
	if content == "" {
		return cacheResult{modifiedD: d}
	}

	sysMsg := cacheableMessage{
		index:   0,
		role:    role,
		content: content,
	}

	var totalNPast llama.Pos
	var anyCached bool

	switch role {
	case RoleSystem:
		result := m.performSPC(ctx, d, messages, sysMsg)
		if result.err != nil {
			return result
		}

		// Mark we cached the system prompt and where those system prompt tokens
		// end in the cache.
		if result.cached {
			totalNPast = result.nPast
			anyCached = true
		}

		// Remove the system prompt from the messages.
		d = removeMessagesAtIndices(d, []int{0})

	default:
		// No system message in the input, but cache exists. This means the
		// client did not send the system message again on their next request.

		m.cacheMu.RLock()
		cachedTokens := m.sysPromptTokens
		m.cacheMu.RUnlock()

		if cachedTokens > 0 {
			m.log(ctx, "cache", "status", "cache hit (system prompt excluded on this request)", "tokens", cachedTokens)
			totalNPast = llama.Pos(cachedTokens)
			anyCached = true
		}
	}

	return cacheResult{
		modifiedD: d,
		nPast:     totalNPast,
		cached:    anyCached,
	}
}

// performSPC performs the actual caching of a system prompt message.
// It checks for cache hits, and on a miss, templates the system message, tokenizes
// it, and decodes the tokens into sequence 0 for reuse on subsequent requests.
func (m *Model) performSPC(ctx context.Context, d D, messages []D, msgInfo cacheableMessage) cacheResult {
	// Default sequence id for caching.
	const seqID llama.SeqId = 0

	if msgInfo.role != RoleSystem {
		m.log(ctx, "cache", "status", "no system prompt message provided", "role", msgInfo.role)
		return cacheResult{modifiedD: d}
	}

	contentLen := len(msgInfo.content)

	// -------------------------------------------------------------------------
	// Check for cache hit (fast path with read lock).
	// We check the length of the system prompt string first to see if this is a
	// new system prompt before checking with the more accurate hash compare.
	// It's possible a new system prompt is of the same length as the old one.

	m.cacheMu.RLock()
	currentHash := m.sysPromptHash
	currentTokens := m.sysPromptTokens
	currentLen := m.sysPromptLen
	m.cacheMu.RUnlock()

	newHash := hashMessage(msgInfo)

	if currentLen == contentLen && currentHash == newHash && currentTokens > 0 {
		m.log(ctx, "cache", "status", "system prompt cache hit", "role", msgInfo.role, "seq", seqID, "tokens", currentTokens)
		return cacheResult{nPast: llama.Pos(currentTokens), cached: true}
	}

	// -------------------------------------------------------------------------
	// Cache miss - template and cache the message.

	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	// Double-check in case another goroutine cached while we waited.
	// Must compute hash here since we skipped it above on length mismatch.
	if m.sysPromptLen == contentLen && m.sysPromptHash == newHash && m.sysPromptTokens > 0 {
		m.log(ctx, "cache", "status", "system prompt cache hit (after lock)", "role", msgInfo.role, "seq", seqID, "tokens", m.sysPromptTokens)
		return cacheResult{nPast: llama.Pos(m.sysPromptTokens), cached: true}
	}

	// We just need to cache the system message, so create a D with
	// this message.
	msgsToCache := D{
		"messages":              []D{messages[0]},
		"add_generation_prompt": false,
	}

	systemMsgPrompt, _, err := m.createPrompt(ctx, msgsToCache)
	if err != nil {
		return cacheResult{modifiedD: d, err: fmt.Errorf("cache: failed to template system prompt: %w", err)}
	}

	tokens := llama.Tokenize(m.vocab, systemMsgPrompt, m.addBOSToken, true)
	nTokens := len(tokens)

	if nTokens == 0 {
		return cacheResult{modifiedD: d, err: fmt.Errorf("cache: system prompt tokenized to zero tokens")}
	}

	if nTokens < m.cfg.CacheMinTokens {
		m.log(ctx, "cache", "status", "system prompt cache skip (too short)", "role", msgInfo.role, "seq", seqID, "tokens", nTokens, "min", m.cfg.CacheMinTokens)
		return cacheResult{modifiedD: d}
	}

	if err := m.addTokensToCache(ctx, tokens, seqID); err != nil {
		return cacheResult{modifiedD: d, err: err}
	}

	m.sysPromptHash = newHash
	m.sysPromptTokens = nTokens
	m.sysPromptLen = contentLen

	m.log(ctx, "cache", "status", "system prompt cached", "role", msgInfo.role, "seq", seqID, "tokens", nTokens, "hash", newHash[:8])

	return cacheResult{
		modifiedD: d,
		nPast:     llama.Pos(nTokens),
		cached:    true,
	}
}

// processIMC orchestrates the incremental message caching flow. It looks up
// the session for the given imcID and delegates to performIMC for the actual
// cache operations. On success, it removes cached messages from d.
func (m *Model) processIMC(ctx context.Context, d D) cacheResult {
	messages, ok := d["messages"].([]D)
	if !ok || len(messages) == 0 {
		return cacheResult{modifiedD: d}
	}

	// They may forget to provide an id, so we will use the "default" id. If
	// multiple people are on the system not providing an id, this won't end
	// well. Hahaha
	imcID, _ := d["imc_id"].(string)
	if imcID == "" {
		imcID = "default"
		m.log(ctx, "cache", "status", "using default imc id", "reason", "IMC turned on but no imc_id was provided")
	}

	result, cachedMsgCount := m.performIMC(ctx, d, messages, imcID)
	if result.err != nil {
		return result
	}

	var totalNPast llama.Pos
	var anyCached bool
	var indicesToRemove []int

	if result.cached {
		totalNPast += result.nPast
		anyCached = true

		// IMC caches messages[0:cachedMsgCount]. Remove all cached messages.
		for i := range cachedMsgCount {
			indicesToRemove = append(indicesToRemove, i)
		}

		// If IMC returned a prompt (first-time cache or extension), use it directly.
		if result.newDataCached != "" {
			return cacheResult{
				modifiedD:     d,
				newDataCached: result.newDataCached,
				media:         result.media,
				nPast:         totalNPast,
				cached:        true,
				imcID:         result.imcID,
				imcSeqID:      result.imcSeqID,
			}
		}

		// IMC hit without prompt - propagate session info.
		return cacheResult{
			modifiedD: removeMessagesAtIndices(d, indicesToRemove),
			nPast:     totalNPast,
			cached:    true,
			imcID:     result.imcID,
			imcSeqID:  result.imcSeqID,
		}
	}

	return cacheResult{
		modifiedD: d,
		nPast:     totalNPast,
		cached:    anyCached,
	}
}

// performIMC implements incremental multi-turn caching (IMC) for agentic
// workflows. It caches all messages except the last one (which triggers
// generation) and extends the cache incrementally on subsequent requests.
//
// Each unique imcID gets its own session with a dedicated cache sequence.
// Returns the cache result and the number of cached messages (for removal).
//
// Algorithm:
//  1. Look up or create session for imcID
//  2. If session cache is empty, cache messages[0:len-1]
//  3. If session cache exists, check if messages[0:msgCount] match cached hash
//  4. On prefix match: extend cache with messages[msgCount:len-1]
//  5. On prefix mismatch: rebuild cache from scratch
func (m *Model) performIMC(ctx context.Context, d D, messages []D, imcID string) (cacheResult, int) {
	// We need at least 2 messages to start: one to cache, one to generate from.
	totalMsgs := len(messages)
	if totalMsgs < 2 {
		return cacheResult{modifiedD: d}, 0
	}

	// We will cache all messages but the last one.
	lastMsgIdxToCache := totalMsgs - 1

	// -------------------------------------------------------------------------
	// Look up or create session for this imcID.

	session, isNew := m.getOrCreateIMCSession(ctx, imcID)
	if session == nil {
		// All session slots are in use - bypass IMC gracefully.
		m.log(ctx, "imc", "status", "bypass (slots full)", "imc_id", imcID, "max-possible-ids", m.imcMaxSeqs)
		return cacheResult{modifiedD: d}, 0
	}

	// -------------------------------------------------------------------------
	// Read current session state (fast path with read lock).

	m.cacheMu.RLock()
	currentCachedMsgsHash := session.cachedMsgsHash
	currentTotalTokensCached := session.totalTokensCached
	currentLastMsgIdxCached := session.lastMsgIdxCached
	seqID := session.seqID
	m.cacheMu.RUnlock()

	// -------------------------------------------------------------------------
	// Check for cache hit on existing prefix.

	if !isNew && currentLastMsgIdxCached > 0 && currentTotalTokensCached > 0 {

		// Check if the current number of messages in the input has increased.
		// This means we are still processing the same thread.
		if totalMsgs > currentLastMsgIdxCached {
			newMsgsHash := hashMessages(messages[:currentLastMsgIdxCached])

			// Check if the current messages we have cached are the same with
			// the new input request. If nothing has changed with those existing
			// messages we can extend the cache with new messages
			if newMsgsHash == currentCachedMsgsHash {

				// If there are more messages in this request, then we need
				// to extend the cache. I would expect this to be the case
				// everytime.
				if currentLastMsgIdxCached < lastMsgIdxToCache {
					return m.extendIMCCache(ctx, d, messages, imcID, session, currentLastMsgIdxCached, lastMsgIdxToCache, currentTotalTokensCached), currentLastMsgIdxCached
				}

				// If by some weird circumstance the client sends back the
				// exact same messages as before, we have all that already
				// in the cache. Just return it.

				m.log(ctx, "imc", "status", "cache hit", "imc_id", imcID, "seq", seqID, "current-msg-idx-cached", currentLastMsgIdxCached, "current-total-tokens-cached", currentTotalTokensCached)

				return cacheResult{
					nPast:    llama.Pos(currentTotalTokensCached),
					cached:   true,
					imcID:    imcID,
					imcSeqID: seqID,
				}, currentLastMsgIdxCached
			}
		}

		// Prefix mismatch - need to rebuild cache.
		m.log(ctx, "imc", "status", "prefix mismatch", "imc_id", imcID, "current-last-msg-idx-cached", currentLastMsgIdxCached, "total-msgs", totalMsgs)
	}

	// -------------------------------------------------------------------------
	// Cache miss - build cache from scratch.

	result := m.buildInitialIMCCache(ctx, d, messages, imcID, session, lastMsgIdxToCache)

	return result, lastMsgIdxToCache
}

// getOrCreateIMCSession looks up an existing session by imcID or creates a new one.
// Returns nil if all session slots are in use (graceful bypass).
// Returns the session and true if it was newly created.
func (m *Model) getOrCreateIMCSession(ctx context.Context, imcID string) (*imcSession, bool) {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	// Check if session already exists.
	if session, exists := m.imcSessions[imcID]; exists {
		session.lastUsed = time.Now()
		return session, false
	}

	// Check if we have room for a new session.
	if int(m.imcNextSeq) >= m.imcMaxSeqs {
		// All slots are in use. Could implement LRU eviction here in the future.
		return nil, false
	}

	// Create new session with next available sequence.
	session := imcSession{
		seqID:    m.imcNextSeq,
		lastUsed: time.Now(),
	}

	m.imcSessions[imcID] = &session
	m.imcNextSeq++

	m.log(ctx, "imc", "status", "session created", "imc_id", imcID, "seq", session.seqID, "total-sessions", len(m.imcSessions))

	return &session, true
}

// extendIMCCache extends the existing cache with new messages from
// messages[currentLastMsgIdxCached:lastMsgIdxToCache].
func (m *Model) extendIMCCache(ctx context.Context, d D, messages []D, imcID string, session *imcSession, currentLastMsgIdxCached, lastMsgIdxToCache, currentTotalTokensCached int) cacheResult {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	// Double-check session state hasn't changed. If it has, fall back to
	// doing a full rebuild. This could happen if two people are using the
	// imc id.
	if session.lastMsgIdxCached != currentLastMsgIdxCached || session.totalTokensCached != currentTotalTokensCached {
		m.cacheMu.Unlock()
		result := m.buildInitialIMCCache(ctx, d, messages, imcID, session, lastMsgIdxToCache)
		m.cacheMu.Lock()

		return result
	}

	msgs := messages[:lastMsgIdxToCache]

	// We need to add all the messages we want to cache. We don't want the
	// assistant tag produced.
	msgsToCache := D{
		"messages":              msgs,
		"add_generation_prompt": false,
	}

	// Copy tools if present (affects template output).
	if tools, ok := d["tools"]; ok {
		msgsToCache["tools"] = tools
	}

	m.log(ctx, "imc", "status", "create prompt to be cached", "imc_id", imcID, "number-of-messages", len(msgs))

	// Create the prompt we want to cache.
	promptToCache, _, err := m.createPrompt(ctx, msgsToCache)
	if err != nil {
		return cacheResult{modifiedD: d, err: fmt.Errorf("imc: failed to template prefix: %w", err)}
	}

	// If the amount of data hasn't changed, we have nothing to extend. This
	// should only happen if the client sends of the exact same input messages
	// twice back to back.
	if len(promptToCache) <= session.amtOfDataCached {
		m.log(ctx, "imc", "status", "extend (no content)", "imc_id", imcID, "current-amt-of-data-cached", session.amtOfDataCached, "new-amt-of-data-to-cache", len(promptToCache))
		return cacheResult{nPast: llama.Pos(session.totalTokensCached), cached: true}
	}

	// Extract the data that is missing in the cache.
	extensionData := promptToCache[session.amtOfDataCached:]
	extensionTokens := llama.Tokenize(m.vocab, extensionData, false, true)
	numOfExtTokens := len(extensionTokens)

	if numOfExtTokens == 0 {
		m.log(ctx, "imc", "status", "extend (no data to extend)", "imc_id", imcID)
		return cacheResult{nPast: llama.Pos(session.totalTokensCached), cached: true}
	}

	// Add the new tokens into the cache.
	if err := m.extendTokensInCache(ctx, extensionTokens, session.seqID, session.totalTokensCached); err != nil {
		return cacheResult{modifiedD: d, err: err}
	}

	m.log(ctx, "imc", "status", "create prompt for all messages", "imc_id", imcID)

	// Create a prompt for the entire set of input messages.
	fullPrompt, media, err := m.createPrompt(ctx, d)
	if err != nil {
		return cacheResult{modifiedD: d, err: fmt.Errorf("imc: failed to template full message: %w", err)}
	}

	// Verify the full prompt still has the same beginning chars that are
	// currently in the cache. If not, the model has a template that is
	// non-deterministic (gpt, glm). This means we can't use the cache :(.
	// The client should switch to SPC.
	if !strings.HasPrefix(fullPrompt, promptToCache) {
		m.log(ctx, "imc", "status", "prefix (mismatch rebuild)", "imc_id", imcID, "data-to-cache-len", len(promptToCache), "all-data-len", len(fullPrompt))

		// Rebuild cache from scratch to ensure consistency.
		m.cacheMu.Unlock()
		result := m.buildInitialIMCCache(ctx, d, messages, imcID, session, lastMsgIdxToCache)
		m.cacheMu.Lock()

		return result
	}

	// Update session state.
	newTotalTokens := currentTotalTokensCached + numOfExtTokens
	newHash := hashMessages(messages[:lastMsgIdxToCache])
	session.cachedMsgsHash = newHash
	session.totalTokensCached = newTotalTokens
	session.lastMsgIdxCached = lastMsgIdxToCache
	session.amtOfDataCached = len(promptToCache)
	session.lastUsed = time.Now()

	// Extract the new data that was cached.
	newDataCached := fullPrompt[len(promptToCache):]

	m.log(ctx, "imc", "status", "cache extended", "imc_id", imcID, "seq", session.seqID,
		"idx", fmt.Sprintf("cur[%d] -> new[%d]", currentLastMsgIdxCached, lastMsgIdxToCache),
		"tokens", fmt.Sprintf("cur[%d] -> new[%d] (+%d)", currentTotalTokensCached, newTotalTokens, numOfExtTokens),
		"amt-new-data-cached", len(newDataCached))

	return cacheResult{
		modifiedD:     d,
		newDataCached: newDataCached,
		media:         media,
		nPast:         llama.Pos(newTotalTokens),
		cached:        true,
		imcID:         imcID,
		imcSeqID:      session.seqID,
	}
}

// buildInitialIMCCache builds the cache from scratch for messages[0:lastMsgIdxToCache].
func (m *Model) buildInitialIMCCache(ctx context.Context, d D, messages []D, imcID string, session *imcSession, lastMsgIdxToCache int) cacheResult {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	// Double-check in case another goroutine built the cache while we waited.
	if session.lastMsgIdxCached > 0 && session.totalTokensCached > 0 && session.lastMsgIdxCached <= len(messages) {
		prefixHash := hashMessages(messages[:session.lastMsgIdxCached])
		if prefixHash == session.cachedMsgsHash {
			m.log(ctx, "imc", "status", "cache hit (after-lock)", "imc_id", imcID, "last-msg-idx-cached", session.lastMsgIdxCached, "total-tokens-cached", session.totalTokensCached)
			return cacheResult{nPast: llama.Pos(session.totalTokensCached), cached: true}
		}
	}

	// Clear existing cache sequence.
	llama.MemorySeqRm(m.mem, session.seqID, -1, -1)

	// We need to cache all the message from the beginning to the target
	// message which should represent the second to last message. We don't
	// want the assistant tag produced.
	msgsToCache := messages[:lastMsgIdxToCache]
	prefixD := D{
		"messages":              msgsToCache,
		"add_generation_prompt": false,
	}

	// Copy tools if present (affects template output).
	if tools, ok := d["tools"]; ok {
		prefixD["tools"] = tools
	}

	dataToCache, _, err := m.createPrompt(ctx, prefixD)
	if err != nil {
		return cacheResult{modifiedD: d, err: fmt.Errorf("imc: failed to template messages: %w", err)}
	}

	tokens := llama.Tokenize(m.vocab, dataToCache, m.addBOSToken, true)
	nTokens := len(tokens)

	if nTokens == 0 {
		return cacheResult{modifiedD: d, err: fmt.Errorf("imc: messages tokenized to zero tokens")}
	}

	if nTokens < m.cfg.CacheMinTokens {
		m.log(ctx, "imc", "status", "skip (too short)", "imc_id", imcID, "last-msg-index-to-cache", lastMsgIdxToCache, "tokens", nTokens, "cache-min-tokens", m.cfg.CacheMinTokens)
		return cacheResult{modifiedD: d}
	}

	// Decode tokens into cache sequence.
	if err := m.addTokensToCache(ctx, tokens, session.seqID); err != nil {
		return cacheResult{modifiedD: d, err: err}
	}

	// Update session state.
	newHash := hashMessages(msgsToCache)
	session.cachedMsgsHash = newHash
	session.totalTokensCached = nTokens
	session.lastMsgIdxCached = lastMsgIdxToCache
	session.amtOfDataCached = len(dataToCache)
	session.lastUsed = time.Now()

	m.log(ctx, "imc", "status", "built", "imc_id", imcID, "seq", session.seqID, "msgs", lastMsgIdxToCache, "tokens", nTokens, "prompt-len", len(dataToCache), "hash", newHash[:8])

	// Extract the new data that was cached.
	newDataCached, media, err := m.extractUncachedData(ctx, d, dataToCache)
	if err != nil {
		return cacheResult{modifiedD: d, err: err}
	}

	return cacheResult{
		modifiedD:     d,
		newDataCached: newDataCached,
		media:         media,
		nPast:         llama.Pos(nTokens),
		cached:        true,
		imcID:         imcID,
		imcSeqID:      session.seqID,
	}
}

// extendTokensInCache decodes additional tokens into an existing cache sequence.
// Unlike addTokensToCache, this does NOT clear the sequence first.
// startPos is the position offset for the new tokens (i.e., existing token count).
func (m *Model) extendTokensInCache(ctx context.Context, tokens []llama.Token, seqID llama.SeqId, startPos int) error {
	nBatch := int(m.ctxParams.NBatch)
	nTokens := len(tokens)

	if nBatch <= 0 {
		nBatch = m.cfg.NBatch
	}

	m.log(ctx, "cache", "status", "started (extending tokens in cache)", "seq", seqID, "tokens", nTokens, "nbatch", nBatch)

	m.decodeMu.Lock()
	defer m.decodeMu.Unlock()

	// Create batch with explicit sequence ID.
	nCtx := llama.NCtx(m.lctx)
	batch := llama.BatchInit(int32(nCtx), 0, 1)
	defer llama.BatchFree(batch)

	seqIDs := []llama.SeqId{seqID}

	for i := 0; i < nTokens; i += nBatch {
		batchClear(&batch)

		end := min(i+nBatch, nTokens)

		for j := i; j < end; j++ {
			// Position is offset by existing tokens in cache.
			pos := llama.Pos(startPos + j)

			// Only compute logits for the last token of the last chunk.
			logits := j == nTokens-1

			batchAdd(&batch, tokens[j], pos, seqIDs, logits)
		}

		if _, err := llama.Decode(m.lctx, batch); err != nil {
			return fmt.Errorf("imc: failed to decode extension tokens at pos %d: %w", i, err)
		}
	}

	m.log(ctx, "cache", "status", "finished (extending tokens in cache)", "seq", seqID, "tokens", nTokens, "nbatch", nBatch)

	return nil
}

// addTokensToCache decodes tokens into the specified sequence for caching.
// Uses explicit sequence ID assignment to ensure tokens go into the correct
// cache sequence (important for multi-user IMC).
func (m *Model) addTokensToCache(ctx context.Context, tokens []llama.Token, seqID llama.SeqId) error {
	llama.MemorySeqRm(m.mem, seqID, -1, -1)

	nBatch := int(m.ctxParams.NBatch)
	nTokens := len(tokens)

	// Defensive check: log if NBatch seems invalid.
	if nBatch <= 0 {
		m.log(ctx, "decode-tokens-to-seq", "ERROR", "invalid-nbatch",
			"nbatch", nBatch,
			"ctxParams.NBatch", m.ctxParams.NBatch,
			"cfg.NBatch", m.cfg.NBatch)

		nBatch = m.cfg.NBatch // Fall back to config value
	}

	m.log(ctx, "cache", "status", "started (adding tokens to cache)", "seq", seqID, "tokens", nTokens, "nbatch", nBatch)

	// Lock to prevent concurrent decode with batch engine.
	m.decodeMu.Lock()
	defer m.decodeMu.Unlock()

	// Create batch with explicit sequence ID. llama.BatchGetOne uses seq 0 by
	// default, which breaks multi-user IMC where each session has its own seq.
	nCtx := llama.NCtx(m.lctx)
	batch := llama.BatchInit(int32(nCtx), 0, 1)
	defer llama.BatchFree(batch)

	seqIDs := []llama.SeqId{seqID}

	for i := 0; i < nTokens; i += nBatch {
		batchClear(&batch)

		end := min(i+nBatch, nTokens)
		for j := i; j < end; j++ {
			// Only compute logits for the last token of the last chunk.
			logits := j == nTokens-1
			batchAdd(&batch, tokens[j], llama.Pos(j), seqIDs, logits)
		}

		if _, err := llama.Decode(m.lctx, batch); err != nil {
			return fmt.Errorf("cache: failed to decode tokens at pos %d: %w", i, err)
		}
	}

	m.log(ctx, "cache", "status", "finished (adding tokens to cache)", "seq", seqID, "tokens", nTokens)

	return nil
}

// extractUncachedData returns the un-cached data from the messages plus the
// generation prompt.
func (m *Model) extractUncachedData(ctx context.Context, d D, dataCached string) (string, [][]byte, error) {
	// Create a prompt for the entire set of input messages.
	fullPrompt, media, err := m.createPrompt(ctx, d)
	if err != nil {
		return "", nil, fmt.Errorf("imc: failed to template full prompt: %w", err)
	}

	// Verify the full prompt has the same beginning data as the ones we
	// stored in the cache. If not, the model has a template that is
	// non-deterministic (gpt, glm). This means we can't use the cache :(.
	// The client should switch to SPC.
	if !strings.HasPrefix(fullPrompt, dataCached) {
		return "", nil, fmt.Errorf("imc: prefix mismatch, data-cached-len[%d] all-data-len[%d]", len(dataCached), len(fullPrompt))
	}

	// Extract the remaining data from the full prompt we don't have cached.
	uncachedData := fullPrompt[len(dataCached):]

	m.log(ctx, "imc", "uncached-data-len", len(uncachedData), "data-cached-len", len(dataCached), "all-data-len", len(fullPrompt))

	return uncachedData, media, nil
}

// copySystemPromptToSeq copies the SPC cache (seq 0) to the target sequence.
// For batch engine slot restoration after completion.
func (m *Model) copySystemPromptToSeq(seqID llama.SeqId) error {
	if !m.cfg.SystemPromptCache {
		return nil
	}

	m.cacheMu.RLock()
	sysTokens := m.sysPromptTokens
	m.cacheMu.RUnlock()

	if sysTokens > 0 {
		return m.copyCachesToSeq(seqID, 0)
	}

	return nil
}

// copyCachesToSeq copies cached KV state from a cache sequence to the target
// sequence. For SPC, uses seq 0. For IMC, the srcSeqID must be passed explicitly.
func (m *Model) copyCachesToSeq(dstSeqID llama.SeqId, srcSeqID llama.SeqId) error {
	if !m.cfg.SystemPromptCache && !m.cfg.IncrementalCache {
		return nil
	}

	if err := llama.MemorySeqCp(m.mem, srcSeqID, dstSeqID, -1, -1); err != nil {
		return fmt.Errorf("copy-cache: failed to copy seq %d to %d: %w", srcSeqID, dstSeqID, err)
	}

	return nil
}

// clearCaches clears all cached prompt states.
// This is useful when the model context is reset.
func (m *Model) clearCaches() {
	m.cacheMu.Lock()
	m.sysPromptHash = ""
	m.sysPromptTokens = 0
	m.sysPromptLen = 0

	// Clear all IMC sessions.
	for id := range m.imcSessions {
		delete(m.imcSessions, id)
	}

	m.imcNextSeq = 0
	m.cacheMu.Unlock()
}

// =============================================================================

// cacheableMessage contains information about a message that can be cached.
type cacheableMessage struct {
	index   int
	role    string
	content string
}

// findCacheableMessage finds the first message with the specified role.
// Returns the message info and true if found.
func findCacheableMessage(messages []D, targetRole string) (cacheableMessage, bool) {
	for i, msg := range messages {
		role, ok := msg["role"].(string)
		if !ok || role != targetRole {
			continue
		}

		// Handle content as string or array (OpenAI multi-part format).
		var content string
		switch c := msg["content"].(type) {
		case string:
			content = c

		case []any:
			// Extract text from array of content parts.
			for _, part := range c {
				if partMap, ok := part.(map[string]any); ok {
					if partMap["type"] == "text" {
						if text, ok := partMap["text"].(string); ok {
							content += text
						}
					}
				}
			}

		case []D:
			// Extract text from array of D content parts.
			for _, part := range c {
				if part["type"] == "text" {
					if text, ok := part["text"].(string); ok {
						content += text
					}
				}
			}
		}

		if content == "" {
			continue
		}

		return cacheableMessage{index: i, role: role, content: content}, true
	}

	return cacheableMessage{}, false
}

// hashMessage computes a SHA-256 hash of a message.
// Includes the role in the hash to differentiate between same content with different roles.
func hashMessage(cm cacheableMessage) string {
	data := fmt.Sprintf("%s:%s", cm.role, cm.content)
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}

// hashMessages computes a SHA-256 hash of a slice of messages.
// Used by IMC to validate that the cached prefix matches the current request.
func hashMessages(messages []D) string {
	h := sha256.New()

	for i, msg := range messages {
		role, _ := msg["role"].(string)
		content := extractMessageContent(msg)
		fmt.Fprintf(h, "%d:%s:%s|", i, role, content)
	}

	return hex.EncodeToString(h.Sum(nil))
}

// extractMessageContent extracts the text content from a message.
// Handles both string content and array content (OpenAI multi-part format).
func extractMessageContent(msg D) string {
	switch c := msg["content"].(type) {
	case string:
		return c

	case []any:
		var content string
		for _, part := range c {
			if partMap, ok := part.(map[string]any); ok {
				if partMap["type"] == "text" {
					if text, ok := partMap["text"].(string); ok {
						content += text
					}
				}
			}
		}
		return content

	case []D:
		var content string
		for _, part := range c {
			if part["type"] == "text" {
				if text, ok := part["text"].(string); ok {
					content += text
				}
			}
		}
		return content
	}

	return ""
}

// removeMessagesAtIndices returns D with messages at the specified indices removed.
// If no messages remain after removal, adds a default user message prompting the
// agent to greet the user. Mutates d in place.
func removeMessagesAtIndices(d D, indices []int) D {
	messages, ok := d["messages"].([]D)
	if !ok || len(messages) == 0 || len(indices) == 0 {
		return d
	}

	// Build a set of indices to remove for O(1) lookup.
	removeSet := make(map[int]bool, len(indices))
	for _, idx := range indices {
		removeSet[idx] = true
	}

	// Build new messages slice excluding removed indices.
	newMessages := make([]D, 0, len(messages)-len(indices))
	for i, msg := range messages {
		if !removeSet[i] {
			newMessages = append(newMessages, msg)
		}
	}

	// If no messages remain, add a prompt for the agent to greet the user.
	if len(newMessages) == 0 {
		newMessages = []D{
			{"role": RoleUser, "content": "Tell the user you are ready to help them."},
		}
	}

	d["messages"] = newMessages

	return d
}
