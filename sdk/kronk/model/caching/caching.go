// Package caching provides an abstraction for model inference caching strategies.
//
// Kronk supports two built-in caching strategies:
//
//   - System Prompt Cache (SPC): Caches the system prompt for reuse across requests.
//   - Incremental Message Cache (IMC): Caches all messages except the last one,
//     extending the cache incrementally on each turn.
//
// Custom caching strategies can be injected by SDK consumers via the
// CacheFactory field in model.Config.
package caching

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// D represents a generic document of fields and values.
type D = map[string]any

// =============================================================================
// Cacher Interface
// =============================================================================

// Cacher processes incoming requests for cache hits and manages cache state.
// Implementations must be safe for concurrent use.
type Cacher interface {

	// ProcessCache examines a request and returns cache hit information.
	// The returned Result contains the modified request document (with cached
	// messages removed) and cache metadata for the batch engine.
	ProcessCache(ctx context.Context, d D, requestStart time.Time) Result

	// ClearCaches resets all cached state. Called when the model context is reset.
	ClearCaches()

	// ClearPending clears a slot's pending flag and notifies waiters.
	// Safe to call even if the slot wasn't pending.
	ClearPending(slotID int)

	// CommitSession updates a slot's session metadata after a successful
	// cache build/extend/rebuild. Implementations clear the pending flag
	// and notify waiters internally.
	CommitSession(commit Commit)

	// InvalidateSlot clears all cached data for a slot. Called when cache
	// corruption is detected (e.g., failed hybrid restore).
	InvalidateSlot(slotID int)

	// SnapshotSlot returns a point-in-time copy of a slot's metadata.
	// Returns false if the slotID is out of range.
	SnapshotSlot(slotID int) (SlotSnapshot, bool)

	// RestoreSPCToSeq restores the externalized SPC KV state into the
	// destination sequence. Returns an error if no cached state is available
	// or the restore fails.
	RestoreSPCToSeq(dstSeqID llama.SeqId) error

	// HasCachedSlot returns true if the given slot has cached content.
	// Used by the batch engine to decide whether to clear IMC metadata
	// for non-cacheable requests.
	HasCachedSlot(slotID int) bool

	// SetSlotMRoPE sets the M-RoPE flag for a slot after a media cache build.
	SetSlotMRoPE(slotID int, useMRoPE bool)
}

// =============================================================================
// Deps Interface
// =============================================================================

// Deps provides the model operations that cache implementations need.
// The model.Model type implements this interface.
type Deps interface {

	// CreatePrompt templates a set of messages into a prompt string.
	// Returns the prompt and any media byte slices extracted from messages.
	CreatePrompt(ctx context.Context, d D) (string, [][]byte, error)

	// TokenizeString converts a prompt string into tokens.
	TokenizeString(prompt string) []llama.Token

	// DecodeTokensIntoCache decodes tokens into a KV cache sequence
	// starting at startPos.
	DecodeTokensIntoCache(ctx context.Context, tokens []llama.Token, seqID llama.SeqId, startPos int) error

	// ClearSequence removes all KV cache entries for a sequence.
	ClearSequence(seqID llama.SeqId)

	// ExtractKVState extracts the KV state for a sequence into a byte buffer.
	// Returns the state bytes and the number of bytes extracted.
	ExtractKVState(seqID llama.SeqId) ([]byte, int, error)

	// RestoreKVState restores KV state from a byte buffer into a sequence.
	// Returns the number of bytes read.
	RestoreKVState(data []byte, dstSeqID llama.SeqId) (int, error)

	// MediaMarkerTokens returns the number of tokens in the media marker
	// string. Computed once per model lifetime.
	MediaMarkerTokens(ctx context.Context) int

	// Log logs a message with key-value pairs.
	Log(ctx context.Context, msg string, args ...any)
}

// =============================================================================
// Config
// =============================================================================

// Strategy selects which caching strategy to use.
type Strategy int

const (
	// StrategyNone disables caching.
	StrategyNone Strategy = iota

	// StrategySPC enables System Prompt Caching.
	StrategySPC

	// StrategyIMC enables Incremental Message Caching.
	StrategyIMC
)

// Config holds configuration for cache construction.
type Config struct {
	Strategy      Strategy      // Which caching strategy to use
	NumSlots      int           // Number of IMC slots (NSeqMax)
	SPCSeqID      llama.SeqId   // Dedicated SPC cache sequence ID
	MinTokens     int           // Minimum token count for caching
	SlotTimeout   time.Duration // Timeout for waiting for a slot
	SupportsMedia bool          // Whether the model supports media (has projection file)
}

// Factory creates a Cacher from the given dependencies and configuration.
// SDK consumers can provide a custom Factory to inject their own caching
// strategy into the model.
type Factory func(deps Deps, cfg Config) Cacher

// =============================================================================
// Result Types
// =============================================================================

// Result contains the results of cache processing.
type Result struct {
	ModifiedD D          // D with cached messages removed if cache was used
	Err       error      // Any error that occurred
	SPC       *SPCResult // nil when SPC not used
	IMC       *IMCResult // nil when IMC not used
}

// SPCResult holds the System Prompt Cache lookup result.
type SPCResult struct {
	CacheIdx llama.Pos // KV position where cached content ends
}

// IMCResult holds the Incremental Message Cache lookup result.
type IMCResult struct {
	SlotID         int         // IMC slot index for routing
	SeqID          llama.SeqId // Cache session's sequence ID
	CacheIdx       llama.Pos   // KV position where cached content ends
	ExpectedHash   string      // Expected cachedMsgsHash at startSlot time (for stale detection)
	CachedMsgCount int         // Number of messages cached (for IMC removal)
	Pending        bool        // True if the target slot was pending (caller should retry)

	// Extension/rebuild work.
	NewCacheTokens    []llama.Token // New tokens to extend the cache (decoded at startSlot)
	NewTotalCached    int           // Total cached KV positions after extension
	NewCachedMsgCount int           // New cachedMsgCount after extension
	NewMsgsHash       string        // New cachedMsgsHash after extension
	ClearSeq          bool          // True if sequence must be cleared before decoding (rebuild from scratch)
	NewCachedTokens   []llama.Token // Full token sequence to store in session after decode
	TrimPos           llama.Pos     // Position to trim KV cache from (for partial prefix rebuild)

	// Media cache build (deferred to startSlot).
	MediaBuild          bool  // True if cache build requires the mtmd pipeline (images/audio in cached messages)
	MediaCacheD         D     // Document with cacheable messages + tools for media cache build
	MediaKVCounts       []int // Media KV position counts to preserve during text-only media extend
	MediaSkipTextTokens int   // Text tokens already in KV cache to skip during partial media extend
}

// Commit holds the data needed to update a slot's session after a successful
// cache build/extend/rebuild.
type Commit struct {
	SlotID         int
	Hash           string
	TotalCached    int
	CachedMsgCount int
	CachedTokens   []llama.Token
	HasMedia       bool
	UseMRoPE       bool
	MediaKVCounts  []int
}

// SlotSnapshot holds a point-in-time copy of a slot's metadata.
// Used by the batch engine for stale-check and metadata snapshots.
type SlotSnapshot struct {
	SlotID            int
	SeqID             llama.SeqId
	CachedMsgsHash    string
	CachedTokens      []llama.Token
	TotalTokensCached int
	CachedMsgCount    int
	Pending           bool
	Empty             bool
	HasMedia          bool
	UseMRoPE          bool
	MediaKVCounts     []int
}

// =============================================================================
// New — Default Factory
// =============================================================================

// New creates a Cacher using the built-in strategy for the given config.
// This is the default Factory.
func New(deps Deps, cfg Config) Cacher {
	switch cfg.Strategy {
	case StrategySPC:
		return NewSPCCache(deps, cfg)
	case StrategyIMC:
		return NewIMCCache(deps, cfg)
	default:
		return NewNoop()
	}
}

// =============================================================================
// Shared Helper Types and Functions
// =============================================================================

// RoleSystem is the role string for system messages.
const RoleSystem = "system"

// CacheableMessage contains information about a message that can be cached.
type CacheableMessage struct {
	Index   int
	Role    string
	Content string
}

// HashMessage computes a SHA-256 hash of a message.
// Includes the role in the hash to differentiate between same content with different roles.
func HashMessage(cm CacheableMessage) string {
	data := fmt.Sprintf("%s:%s", cm.Role, cm.Content)
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}

// HashMessages computes a SHA-256 hash of a slice of messages.
// Used by IMC to validate that the cached prefix matches the current request.
// Includes raw media bytes (images/audio) in the hash so that different images
// produce different hashes, enabling cache validation for media content.
func HashMessages(messages []D) string {
	h := sha256.New()

	for i, msg := range messages {
		role, _ := msg["role"].(string)
		content := ExtractMessageContent(msg)
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

// ExtractMessageContent extracts the text content from a message.
// Handles both string content and array content (OpenAI multi-part format).
func ExtractMessageContent(msg D) string {
	switch c := msg["content"].(type) {
	case string:
		return c

	case []any:
		var content strings.Builder
		for _, part := range c {
			content.WriteString(TextFromPart(part))
		}
		return content.String()

	}

	return ""
}

// TextFromPart extracts the text value from a multi-part content element.
// The part must be a map with type "text" and a string text field.
func TextFromPart(part any) string {
	var m map[string]any

	switch v := part.(type) {
	case map[string]any:
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

// RemoveFirstNMessages removes the first n messages from d.
func RemoveFirstNMessages(d D, n int) D {
	messages, ok := d["messages"].([]D)
	if !ok || len(messages) == 0 || n <= 0 {
		return d
	}

	if n >= len(messages) {
		d["messages"] = []D{
			{"role": "user", "content": "Tell the user you are ready to help them."},
		}
		return d
	}

	newMessages := make([]D, len(messages)-n)
	copy(newMessages, messages[n:])
	d["messages"] = newMessages

	return d
}

// RemoveMessagesAtIndices returns D with messages at the specified indices removed.
// If no messages remain after removal, adds a default user message prompting the
// agent to greet the user. Mutates d in place.
func RemoveMessagesAtIndices(d D, indices []int) D {
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
			{"role": "user", "content": "Tell the user you are ready to help them."},
		}
	}

	d["messages"] = newMessages

	return d
}

// TokenPrefixMatch returns the number of tokens that match between two slices,
// starting from the beginning. Used to find the longest common prefix between
// a slot's cached tokens and a new request's tokens.
func TokenPrefixMatch(cached, incoming []llama.Token) int {
	n := min(len(cached), len(incoming))
	for i := range n {
		if cached[i] != incoming[i] {
			return i
		}
	}
	return n
}

// MessageHasMedia checks if a single message D contains media content.
// Handles both OpenAI structured format (image_url, video_url, input_audio)
// and pre-converted []byte media. Plain strings are NOT treated as media;
// media detection for strings is left to prepareContext via detectMediaType.
func MessageHasMedia(msg D) bool {
	content, ok := msg["content"]
	if !ok {
		return false
	}

	switch c := content.(type) {
	case []byte:
		return true

	case []any:
		if slices.ContainsFunc(c, partHasMediaType) {
			return true
		}

	case []D:
		for _, part := range c {
			if partHasMediaType(part) {
				return true
			}
		}
	}

	return false
}

// partHasMediaType checks if a content part has a media type field.
func partHasMediaType(part any) bool {
	var m map[string]any

	switch v := part.(type) {
	case map[string]any:
		m = v
	default:
		return false
	}

	switch m["type"] {
	case "image_url", "video_url", "input_audio":
		return true
	}

	return false
}
