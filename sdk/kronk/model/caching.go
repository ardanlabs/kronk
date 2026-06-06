package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// cacheResult contains the results of cache processing.
type cacheResult struct {
	modifiedD D         // D with cached messages removed if cache was used
	cacheIdx  llama.Pos // KV position where cached content ends; new tokens start here
	err       error     // Any error that occurred

	// IMC session-routing field. Sessions externalize their KV state
	// via SessionStore between requests, so the matched session may run
	// on any free execution slot — the slot is chosen by the scheduler
	// at startSlot. imcSessionID identifies the matched session pool
	// entry (used by imcClearPending lookup and log correlation).
	imcSessionID    int    // Session-pool index (== imcSession.id) of the matched session.
	imcExpectedHash string // Expected cachedMsgsHash for stale detection at startSlot (a concurrent extend may have moved the session forward between processIMC and startSlot)
	imcPending      bool   // True if the matched session was already pending (caller should retry)

	// Pure-hit snapshot-skip state. Populated for every IMC cache-result so
	// imcCommitSession can refresh the session's cachedRenderInputHash;
	// imcPureHitSkipSnapshot is true only on text-only exact pure hits when
	// IMCPureHitSnapshotSkip is enabled and the live session's committed
	// render-input hash equals imcExpectedRenderHash.
	imcExpectedCachedMsgs  int    // Expected cachedMsgCount at startSlot for the matched session.
	imcExpectedTokens      int    // Expected totalTokensCached at startSlot for the matched session.
	imcExpectedRenderHash  string // Expected cachedRenderInputHash at startSlot (set on hits; carried forward on builds/extends so commit can refresh the session field).
	imcPureHitSkipSnapshot bool   // True when startSlot may skip the post-restore snapshot. Always false on extends/media/rebuilds.

	// imcSession is the matched session pointer; the SessionStore on it
	// is the authoritative source of the cached prefix bytes restored
	// into the chosen slot's sequence at startSlot.
	imcSession *imcSession

	// IMC extension fields — tokens to decode on top of the cached KV state.
	imcNewCacheTokens    []llama.Token // New tokens to extend the cache (decoded at startSlot)
	imcNewTotalCached    int           // Total cached KV positions after extension
	imcNewCachedMsgCount int           // New cachedMsgCount after extension
	imcNewMsgsHash       string        // New cachedMsgsHash after extension
	imcClearSeq          bool          // True if sequence must be cleared before decoding (rebuild from scratch)
	imcNewCachedTokens   []llama.Token // Full token sequence to store in session after decode
	imcTrimPos           llama.Pos     // Position to trim KV cache from (for partial prefix rebuild)
	imcSysPromptHash     string        // Hash of system prompt message for the new cache state
	imcSysPromptTokens   int           // Token count of the system prompt in the new cache state

	// IMC media cache build — deferred to startSlot because media decoding
	// requires the mtmd pipeline (projection model + embedding decode).
	imcMediaBuild          bool  // True if cache build requires the mtmd pipeline (images/audio in cached messages)
	imcMediaCacheD         D     // Document with cacheable messages + tools for media cache build
	imcMediaKVCounts       []int // Media KV position counts to preserve during text-only media extend
	imcMediaSkipTextTokens int   // Text tokens already in KV cache to skip during partial media extend
}

// processCache checks if incremental messages are being cached and updates
// the caches as necessary. IMC caches all messages except the last one
// (including the system prompt).
//
// This function is thread-safe and handles concurrent requests appropriately.
func (m *Model) processCache(ctx context.Context, d D, requestStart time.Time) cacheResult {
	if !m.cfg.IncrementalCache() {
		return cacheResult{modifiedD: d}
	}

	return m.processIMC(ctx, d, requestStart)
}

// clearCaches clears all cached prompt states.
// This is useful when the model context is reset.
func (m *Model) clearCaches() {
	m.cacheMu.Lock()

	// Reset all IMC sessions in place (preserving id; seqID is dynamic
	// and is set when a session binds to a slot in startSlot).
	for _, s := range m.imcSessions {
		if s != nil {
			imcResetSession(s)
		}
	}

	m.cacheMu.Unlock()
}

// =============================================================================

// cacheableMessage contains information about a message that can be cached.
type cacheableMessage struct {
	role    string
	content string
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
// Includes raw media bytes (images/audio) in the hash so that different images
// produce different hashes, enabling cache validation for media content.
//
// After prepareMediaContext, media content can be stored as either:
//   - []byte (single media payload — simple case)
//   - []any of strings and []byte parts (interleaved text + media in one
//     message produced by normalizeMediaMessages / toMediaMessage)
func hashMessages(messages []D) string {
	h := sha256.New()

	for i, msg := range messages {
		role, _ := msg["role"].(string)
		content := extractMessageContent(msg)
		fmt.Fprintf(h, "%d:%s:%s|", i, role, content)

		switch c := msg["content"].(type) {
		case []byte:
			fmt.Fprintf(h, "media:%d:", len(c))
			h.Write(c)

		case []any:
			for _, part := range c {
				if b, ok := part.([]byte); ok {
					fmt.Fprintf(h, "media:%d:", len(b))
					h.Write(b)
				}
			}
		}
	}

	return hex.EncodeToString(h.Sum(nil))
}

// extractMessageContent extracts the text content from a message. Handles:
//   - string content (plain text or post-normalization single-text message)
//   - []any content where each part is either a string (post-normalization
//     interleaved parts) or a typed map (raw OpenAI multipart)
//   - []D content where each part is a typed map (raw OpenAI multipart)
//
// Media payloads ([]byte) are intentionally not stringified here; callers that
// need to mix media into the hash use hashMessages, which handles []byte
// content separately.
func extractMessageContent(msg D) string {
	switch c := msg["content"].(type) {
	case string:
		return c

	case []any:
		var content strings.Builder
		for _, part := range c {
			if s, ok := part.(string); ok {
				content.WriteString(s)
				continue
			}
			content.WriteString(textFromPart(part))
		}
		return content.String()

	case []D:
		var content strings.Builder
		for _, part := range c {
			content.WriteString(textFromPart(part))
		}
		return content.String()
	}

	return ""
}

// textFromPart extracts the text value from a multi-part content element.
// The part must be a map with type "text" and a string text field.
func textFromPart(part any) string {
	var m map[string]any

	switch v := part.(type) {
	case map[string]any:
		m = v
	case D:
		m = v
	default:
		return ""
	}

	if m["type"] != "text" {
		return ""
	}

	text, _ := m["text"].(string)

	return text
}

// removeFirstNMessages removes the first n messages from d.
func removeFirstNMessages(d D, n int) D {
	messages, ok := d["messages"].([]D)
	if !ok || len(messages) == 0 || n <= 0 {
		return d
	}

	if n >= len(messages) {
		d["messages"] = []D{
			{"role": RoleUser, "content": "Tell the user you are ready to help them."},
		}
		return d
	}

	newMessages := make([]D, len(messages)-n)
	copy(newMessages, messages[n:])

	// Remove tools and system-level keys from the suffix document. These
	// were already rendered in the cached prefix. Re-rendering them in the
	// suffix causes a duplicate system/tools header mid-conversation, which
	// corrupts the prompt and causes models (e.g., Gemma 4) to loop on
	// tool calls or stop generating prematurely.
	delete(d, "tools")
	delete(d, "tool_choice")

	d["messages"] = newMessages

	return d
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

// =============================================================================

// imcRenderFingerprintInput is the canonical render-input structure hashed by
// imcRenderFingerprint. Fields are exported with stable JSON names so the hash
// is stable across builds. ToolsPresent is emitted explicitly so a request
// with no tools and a request with an explicitly-nil tools value do not
// accidentally collapse to the same fingerprint.
type imcRenderFingerprintInput struct {
	TemplateHash        string `json:"template_hash"`
	AddGenerationPrompt bool   `json:"add_generation_prompt"`
	PreserveThinking    bool   `json:"preserve_thinking"`
	Messages            []D    `json:"messages"`
	ToolsPresent        bool   `json:"tools_present"`
	Tools               any    `json:"tools,omitempty"`
}

// imcRenderFingerprint computes a SHA-256 fingerprint of the logical inputs
// that determine what the Jinja template will render for an IMC prefix:
// template script, add_generation_prompt=false (the IMC fixed setting),
// preserve_thinking, the cacheable message slice (post imcEnsureUserMessage),
// and top-level tools.
//
// Returned ok=false on marshal failure so callers can disable any optimization
// that depends on a stable fingerprint. This is stricter than hashMessages,
// which intentionally ignores tools and assistant tool-call metadata for
// hit-selection scope. Used as the safety guard for IMCPureHitSnapshotSkip.
func (m *Model) imcRenderFingerprint(d D, msgs []D) (string, bool) {
	templateSum := sha256.Sum256([]byte(m.template.Script))

	in := imcRenderFingerprintInput{
		TemplateHash:        hex.EncodeToString(templateSum[:]),
		AddGenerationPrompt: false,
		PreserveThinking:    preserveThinking(d),
		Messages:            msgs,
	}

	if tools, ok := d["tools"]; ok {
		in.ToolsPresent = true
		in.Tools = tools
	}

	b, err := json.Marshal(in)
	if err != nil {
		return "", false
	}

	sum := sha256.Sum256(b)

	return hex.EncodeToString(sum[:]), true
}
