package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// cacheResult contains the results of cache processing.
type cacheResult struct {
	modifiedD D         // D with cached messages removed if cache was used
	cacheIdx  llama.Pos // KV position where cached content ends; new tokens start here
	err       error     // Any error that occurred

	// Token-v2 plans render the complete request twice and split the actual
	// token sequence at the stable-render boundary. The tail is consumed
	// directly by startSlot; it is never rendered or tokenized independently.
	imcTokenPlan           bool
	imcSamplerPromptTokens []llama.Token
	imcMediaSamplerTokens  []llama.Token
	imcTailTokens          []llama.Token
	imcMatchKind           string
	imcPromptPlan          promptPlan

	// IMC session-routing field. Sessions externalize their KV state
	// via SessionStore between requests, so the matched session may run
	// on any free execution slot — the slot is chosen by the scheduler
	// at startSlot. imcSessionID identifies the matched session pool
	// entry (used by imcReleaseReservation lookup and log correlation).
	imcSessionID    int    // Session-pool index (== imcSession.id) of the matched session.
	imcExpectedHash string // Expected cachedMsgsHash for stale detection at startSlot

	// Pure-hit snapshot-skip state. Token-v2 exact matches retain an exclusive
	// reservation through restore and generation, so the externalized bytes
	// can be reused without a redundant post-restore serialization.
	imcExpectedCachedMsgs  int    // Expected cachedMsgCount at startSlot for the matched session.
	imcExpectedTokens      int    // Expected physical KV cells at startSlot for the matched session.
	imcExpectedPosition    int    // Expected next logical position at startSlot.
	imcExpectedRenderHash  string // Expected cachedRenderInputHash at startSlot (set on hits; carried forward on builds/extends so commit can refresh the session field).
	imcExpectedPromptPlan  promptPlan
	imcReadOnlyReservation bool // True when the session is reserved for restore/use without metadata or snapshot mutation.
	imcMediaAnchorAdvance  bool // True when text after a media anchor should be atomically committed as a larger snapshot.
	imcNewLogicalPosition  int  // Next logical position after a media-anchor advance.
	imcPureHitSkipSnapshot bool // True when startSlot may skip the post-restore snapshot.
	imcPromoteCheckpoint   bool // True when the selected rolling user boundary must be retained before extension commit.
	imcCheckpointTokens    int  // Exact target token boundary at which to publish a progressive reusable snapshot.

	// imcSession is the matched session pointer; the SessionStore on it
	// is the authoritative source of the cached prefix bytes restored
	// into the chosen slot's sequence at startSlot.
	imcSession *imcSession

	// IMC extension fields — tokens to decode on top of the cached KV state.
	imcNewCacheTokens    []llama.Token // New tokens to extend the cache (decoded at startSlot)
	imcNewTotalCached    int           // Total cached KV positions after extension
	imcNewCachedMsgCount int           // New cachedMsgCount after extension
	imcNewMsgsHash       string        // New cachedMsgsHash after extension
	imcNewEndsAtUser     bool          // True when the new rolling snapshot ends at a real user message.
	imcClearSeq          bool          // True if sequence must be cleared before decoding (rebuild from scratch)
	imcNewCachedTokens   []llama.Token // Full token sequence to store in session after decode

	// IMC media cache build — deferred to startSlot because media decoding
	// requires the mtmd pipeline (projection model + embedding decode).
	imcMediaBuild    bool  // True if cache build requires the mtmd pipeline (images/audio in cached messages)
	imcMediaCacheD   D     // Document with cacheable messages + tools for media cache build
	imcMediaKVCounts []int // Media KV position counts to preserve during text-only media extend
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

// =============================================================================

// imcRenderFingerprintInput is the canonical render-input structure hashed by
// imcRenderFingerprint. Fields are exported with stable JSON names so the hash
// is stable across builds. ToolsPresent is emitted explicitly so a request
// with no tools and a request with an explicitly-nil tools value do not
// accidentally collapse to the same fingerprint.
type imcRenderFingerprintInput struct {
	TemplateHash        string `json:"template_hash"`
	AddGenerationPrompt bool   `json:"add_generation_prompt"`
	EnableThinking      bool   `json:"enable_thinking"`
	ReasoningEffort     any    `json:"reasoning_effort,omitempty"`
	PreserveThinkingSet bool   `json:"preserve_thinking_set"`
	PreserveThinking    bool   `json:"preserve_thinking"`
	ChatTemplateKwargs  any    `json:"chat_template_kwargs,omitempty"`
	Messages            []D    `json:"messages"`
	ToolsPresent        bool   `json:"tools_present"`
	Tools               any    `json:"tools,omitempty"`
}

// imcRenderFingerprint computes a SHA-256 fingerprint of the logical inputs
// that determine what the Jinja template will render for an IMC prefix:
// template script, add_generation_prompt=false (the IMC fixed setting),
// reasoning and template options, the cached message slice, and top-level
// tools.
//
// Returned ok=false on marshal failure so callers can disable any optimization
// that depends on a stable fingerprint. Used as the safety guard for
// IMCPureHitSnapshotSkip.
func (m *Model) imcRenderFingerprint(d D, msgs []D) (string, bool) {
	templateSum := sha256.Sum256([]byte(m.template.Script))

	preserve, preserveSet := d["preserve_thinking"].(bool)
	in := imcRenderFingerprintInput{
		TemplateHash:        hex.EncodeToString(templateSum[:]),
		AddGenerationPrompt: false,
		EnableThinking:      d["enable_thinking"] == true,
		ReasoningEffort:     d["reasoning_effort"],
		PreserveThinkingSet: preserveSet,
		PreserveThinking:    preserve,
		ChatTemplateKwargs:  d["chat_template_kwargs"],
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

func exactRenderFingerprintMatches(cached, current string, ok bool) bool {
	return ok && current != "" && cached == current
}
