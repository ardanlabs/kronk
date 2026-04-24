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
	modifiedD D         // D with cached messages removed if cache was used
	cacheIdx  llama.Pos // KV position where cached content ends; new tokens start here
	err       error     // Any error that occurred

	// Transitional: slot-oriented fields retained until Phase 4/5 remove slot routing.
	cacheSeqID      llama.SeqId // Sequence ID for the cached content
	imcSlotID       int         // Target slot index for IMC routing
	imcExpectedHash string      // Expected cachedMsgsHash for stale detection at startSlot
	imcPending      bool        // True if the target slot was already pending (caller should retry)

	// IMC session pointer. Carries the session reference alongside the
	// slot-oriented fields during the transition period.
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

	// Reset all IMC sessions in place (preserving slotID/seqID).
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
func hashMessages(messages []D) string {
	h := sha256.New()

	for i, msg := range messages {
		role, _ := msg["role"].(string)
		content := extractMessageContent(msg)
		fmt.Fprintf(h, "%d:%s:%s|", i, role, content)

		// Include raw media bytes in hash for vision/audio models.
		// After prepareMediaContext, media content is stored as []byte.
		if b, ok := msg["content"].([]byte); ok {
			fmt.Fprintf(h, "media:%d:", len(b))
			h.Write(b)
		}
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
		var content strings.Builder
		for _, part := range c {
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
