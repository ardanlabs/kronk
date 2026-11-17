package imc

import (
	"crypto/sha256"
	"encoding/json"
	"time"
)

// Key identifies one logical conversation, not a RAM pool entry or batch slot.
// Both fields are required. Isolation is managed by the supplied Store.
// Model is a stable model namespace, not proof of checkpoint compatibility.
// The model generates a unique ID for a new conversation and preserves it
// across extensions and reloads. Rebinding RAM to unrelated work needs a new ID.
type Key struct {
	Model string
	ID    string
}

// Ref identifies the exact revision selected by discovery or a history page.
// Revision is positive; there is no implicit "latest" or historical-open mode.
// A replaced revision may no longer be openable, but an existing handle pins it.
type Ref struct {
	Key      Key
	Revision uint64
}

// Prefix identifies the cached input boundary using the model's canonical
// SHA-256 encoding (currently kronk-imc-prefix-v1). Length counts text tokens or
// logical text/media units, not physical KV cells. Hash excludes checkpoint
// bytes and the newly generated assistant text. Length must be positive.
type Prefix struct {
	Length int
	Hash   [sha256.Size]byte
}

// Summary is lightweight discovery and conversation-listing metadata. It never
// includes checkpoint bytes, complete messages, or media. CreatedAt is preserved
// across saves; SavedAt is the completion time of the captured response in UTC,
// supplied by the model and preserved by the backend. Revision, not wall-clock
// order, determines whether Save may replace an existing snapshot.
type Summary struct {
	Ref           Ref
	CreatedAt     time.Time
	SavedAt       time.Time
	Preview       string // Optional bounded display text; sensitive conversation data.
	Prefix        Prefix
	Compatibility [sha256.Size]byte // Model-computed checkpoint compatibility identity.
}

// Record describes the input checkpoint and the separately retained final
// assistant text of one successful request. Version identifies the model-owned
// JSON schemas and prefix-hash encoding; unsupported versions return ErrVersion.
// The model currently writes version 1.
// Record contains metadata only; checkpoint and media bytes belong to Snapshot.
// No native handles, slot IDs, locks, RAM capacity, or reservations are persisted.
type Record struct {
	Version uint32
	Summary Summary

	// Request is a JSON object containing input messages (without appending
	// AssistantText), tools, and effective render/request settings. It is the
	// model's recoverable chat representation, not a promise to preserve the
	// untouched original HTTP or Responses API document.
	Request json.RawMessage

	// AssistantText is only the final visible assistant text. No output token
	// IDs, streaming deltas, full response envelope, or completion checkpoint
	// are saved. It has not been decoded into the input checkpoint. Empty text
	// is valid.
	// Tool-only and reasoning-only output is not represented by this field;
	// faithful replay of such output requires a later explicit contract decision.
	AssistantText string

	// Restore is model-owned JSON interpreted according to Version. It must
	// retain exact prefix tokens/media identities, message/render fingerprints,
	// logical positions and physical KV counts, native media chunk descriptors,
	// attention/M-RoPE flags, sampler history/seed, and applicable MTP hidden
	// state. Summary.Compatibility identifies the model/runtime setup. It describes
	// exactly the saved target and draft, not a newer live sequence. Media
	// units correspond to Snapshot.Media in resolved input order.
	// Storage preserves this document without interpreting private model or
	// native types.
	Restore json.RawMessage
}

// Snapshot supplies a Record and its complete uncompressed payloads to Save.
// All data is borrowed read-only for the duration of Save. Storage derives
// payload sizes from these slices; there are no separate blob descriptors.
// Each successful save is self-contained and does not depend on the previous
// save having succeeded. The model assigns revisions and protects the snapshot
// from mutation until Save returns.
type Snapshot struct {
	Record Record
	Target []byte   // Required input-boundary checkpoint of the target sequence.
	Draft  []byte   // Optional matching own-KV draft/MTP state; empty if absent.
	Media  [][]byte // Resolved media in input order, referenced by Record.Restore.
}

// FindQuery bounds automatic prefix discovery. Model is required.
// Compatibility restricts candidates to the current model-computed identity.
// Limit and MaxPrefixLength must be positive; Limit is also bounded by backend
// configuration. SavedAfter is an optional inclusive UTC lower bound; zero
// disables age filtering.
//
// First filter by model, compatibility, age, and MaxPrefixLength. Then
// select at most Limit latest saved sessions, ordered by SavedAt descending
// and ID ascending for ties. The model computes incoming hashes at the returned
// prefix lengths and compares them locally. Limit bounds sessions considered,
// not merely matching results. Older sessions remain listable but are
// intentionally outside this automatic cache search.
type FindQuery struct {
	Model           string
	Compatibility   [sha256.Size]byte
	Limit           int
	MaxPrefixLength int
	SavedAfter      time.Time
}

// ListQuery requests conversation summaries ordered by SavedAt descending,
// then Model and ID ascending for ties. Empty Model lists all models accessible
// through the supplied Store. Limit must be positive and bounded by backend
// configuration. Cursor is empty for the first page and otherwise is a token
// returned by that backend for the same filters and limit. Invalid or mismatched
// cursors return an error.
//
// Pagination is a live view, not a snapshot transaction: concurrent saves and
// deletion can move or remove entries between pages. Listing does not filter
// native compatibility, so an application
// can browse conversations even when their checkpoints are no longer reusable.
type ListQuery struct {
	Model  string
	Limit  int
	Cursor string
}

// Page is a bounded page of latest saved conversation summaries. NextCursor is
// empty when there are no further entries. Each summary's Ref can be passed to
// Open; concurrent replacement or deletion may require refreshing the page.
type Page struct {
	Sessions   []Summary
	NextCursor string
}
