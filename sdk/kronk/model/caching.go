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
	modifiedD      D           // D with cached messages removed if cache was used
	cacheIdx       llama.Pos   // Token position where cached content ends; new tokens start here
	cachedMsgCount int         // Number of messages cached (for IMC removal)
	err            error       // Any error that occurred
	cacheSeqID      llama.SeqId // Cache session's sequence ID
	imcSlotID       int         // IMC slot index for routing (distinct from seqID for clarity)
	imcExpectedHash string      // Expected cachedMsgsHash at startSlot time (for stale detection)

	// IMC dedicated slot fields — tokens to decode into slot's sequence.
	imcNewCacheTokens []llama.Token // New tokens to extend the cache (decoded at startSlot)
	imcNewTotalCached int           // Total cached tokens after extension
	imcNewMsgIdx      int           // New lastMsgIdxCached after extension
	imcNewMsgsHash    string        // New cachedMsgsHash after extension
	imcClearSeq          bool          // True if sequence must be cleared before decoding (rebuild from scratch)
	imcNewCachedTokens   []llama.Token // Full token sequence to store in session after decode
	imcTrimPos           llama.Pos     // Position to trim KV cache from (for partial prefix rebuild)
	imcPending           bool          // True if the target slot was pending (caller should retry another slot)
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

// clearCaches clears all cached prompt states.
// This is useful when the model context is reset.
func (m *Model) clearCaches() {
	m.cacheMu.Lock()

	// Clear all IMC slot state.
	for _, slot := range m.imcSlots {
		slot.cachedMsgsHash = ""
		slot.cachedTokens = nil
		slot.totalTokensCached = 0
		slot.lastMsgIdxCached = 0
		slot.lastUsed = time.Time{}
		slot.pending = false
	}

	// Clear SPC session. The KV sequence is already freed after extraction,
	// so only the in-memory session state needs clearing.
	m.spcSession = nil
	m.cacheMu.Unlock()
	m.notifyIMCSlotAvailable()
}

// =============================================================================

// cacheableMessage contains information about a message that can be cached.
type cacheableMessage struct {
	index   int
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
		var content strings.Builder
		for _, part := range c {
			if partMap, ok := part.(map[string]any); ok {
				if partMap["type"] == "text" {
					if text, ok := partMap["text"].(string); ok {
						content.WriteString(text)
					}
				}
			}
		}
		return content.String()

	case []D:
		var content strings.Builder
		for _, part := range c {
			if part["type"] == "text" {
				if text, ok := part["text"].(string); ok {
					content.WriteString(text)
				}
			}
		}
		return content.String()
	}

	return ""
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
