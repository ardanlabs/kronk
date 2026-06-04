package model

import (
	"context"
	"sync"
	"testing"
)

// noopLog returns a no-op applog-compatible logger for use in unit tests.
func noopLog(ctx context.Context, msg string, args ...any) {}

// newFingerprintTestModel constructs a minimal *Model wired with a fixed
// template script. It is the smallest fixture required to exercise
// imcRenderFingerprint, imcCommitSession, and imcResetSession.
func newFingerprintTestModel(templateScript string) *Model {
	m := &Model{
		cfg: Config{
			PtrIncrementalCache: new(true),
		},
		imcSessions: make([]*imcSession, 1),
		log:         noopLog,
	}
	m.template.Script = templateScript
	m.cacheCond = sync.NewCond(&m.cacheMu)
	return m
}

// TestIMCRenderFingerprintToolsReorderDeterministic verifies the fingerprint
// is stable across maps whose tool fields were inserted in different orders.
// This documents the dependency on encoding/json's deterministic map-key
// ordering — a future stdlib change here would surface as a test failure.
func TestIMCRenderFingerprintToolsReorderDeterministic(t *testing.T) {
	m := newFingerprintTestModel("template-a")

	msgs := []D{
		{"role": "system", "content": "you are helpful"},
		{"role": "user", "content": "hello"},
	}

	// Two equivalent tool definitions, fields inserted in different orders.
	toolsA := []D{
		{
			"type": "function",
			"function": D{
				"name":        "get_weather",
				"description": "fetch weather",
				"parameters": D{
					"type": "object",
					"properties": D{
						"city": D{"type": "string"},
						"unit": D{"type": "string"},
					},
				},
			},
		},
	}
	toolsB := []D{
		{
			"function": D{
				"parameters": D{
					"properties": D{
						"unit": D{"type": "string"},
						"city": D{"type": "string"},
					},
					"type": "object",
				},
				"description": "fetch weather",
				"name":        "get_weather",
			},
			"type": "function",
		},
	}

	dA := D{"tools": toolsA}
	dB := D{"tools": toolsB}

	hashA, okA := m.imcRenderFingerprint(dA, msgs)
	hashB, okB := m.imcRenderFingerprint(dB, msgs)

	if !okA || !okB {
		t.Fatalf("imcRenderFingerprint failed: okA=%v okB=%v", okA, okB)
	}

	if hashA != hashB {
		t.Errorf("fingerprint changed for reordered tools maps:\n  hashA = %q\n  hashB = %q", hashA, hashB)
	}

	// Sanity: repeated calls return the same value.
	hashA2, _ := m.imcRenderFingerprint(dA, msgs)
	if hashA != hashA2 {
		t.Errorf("non-deterministic across calls: got %q then %q", hashA, hashA2)
	}
}

// TestIMCRenderFingerprintChangesWithInputs verifies the fingerprint
// flips for each piece of render-affecting state. This documents what
// the fingerprint covers (template, preserve_thinking, messages, tools,
// assistant tool-call metadata) — a superset of what hashMessages
// covers today.
func TestIMCRenderFingerprintChangesWithInputs(t *testing.T) {
	baseMsgs := []D{
		{"role": "system", "content": "sys"},
		{"role": "user", "content": "u1"},
		{
			"role":    "assistant",
			"content": "",
			"tool_calls": []D{
				{
					"id":   "call_1",
					"type": "function",
					"function": D{
						"name":      "get_weather",
						"arguments": `{"city":"Austin"}`,
					},
				},
			},
		},
		{
			"role":         "tool",
			"name":         "get_weather",
			"tool_call_id": "call_1",
			"content":      "72F",
		},
	}
	baseTools := []D{{"type": "function", "function": D{"name": "get_weather"}}}
	baseD := D{"tools": baseTools, "preserve_thinking": true}

	m := newFingerprintTestModel("template-a")
	base, ok := m.imcRenderFingerprint(baseD, baseMsgs)
	if !ok {
		t.Fatal("base fingerprint failed")
	}

	tests := []struct {
		name string
		mut  func() (*Model, D, []D)
	}{
		{
			name: "template change",
			mut: func() (*Model, D, []D) {
				m2 := newFingerprintTestModel("template-b")
				return m2, baseD, baseMsgs
			},
		},
		{
			name: "preserve_thinking flips",
			mut: func() (*Model, D, []D) {
				d := D{"tools": baseTools, "preserve_thinking": false}
				return m, d, baseMsgs
			},
		},
		{
			name: "tools changed",
			mut: func() (*Model, D, []D) {
				d := D{"tools": []D{{"type": "function", "function": D{"name": "different"}}}, "preserve_thinking": true}
				return m, d, baseMsgs
			},
		},
		{
			name: "tools removed",
			mut: func() (*Model, D, []D) {
				return m, D{"preserve_thinking": true}, baseMsgs
			},
		},
		{
			name: "assistant tool_call id changed",
			mut: func() (*Model, D, []D) {
				msgs := cloneMessages(baseMsgs)
				calls, _ := msgs[2]["tool_calls"].([]D)
				calls[0]["id"] = "call_2"
				return m, baseD, msgs
			},
		},
		{
			name: "assistant tool_call arguments changed",
			mut: func() (*Model, D, []D) {
				msgs := cloneMessages(baseMsgs)
				calls, _ := msgs[2]["tool_calls"].([]D)
				fn, _ := calls[0]["function"].(D)
				fn["arguments"] = `{"city":"Seattle"}`
				return m, baseD, msgs
			},
		},
		{
			name: "tool message name changed",
			mut: func() (*Model, D, []D) {
				msgs := cloneMessages(baseMsgs)
				msgs[3]["name"] = "different_name"
				return m, baseD, msgs
			},
		},
		{
			name: "tool_call_id changed",
			mut: func() (*Model, D, []D) {
				msgs := cloneMessages(baseMsgs)
				msgs[3]["tool_call_id"] = "call_other"
				return m, baseD, msgs
			},
		},
		{
			name: "message content changed",
			mut: func() (*Model, D, []D) {
				msgs := cloneMessages(baseMsgs)
				msgs[1]["content"] = "u-different"
				return m, baseD, msgs
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mm, dd, msgs := tt.mut()
			h, ok := mm.imcRenderFingerprint(dd, msgs)
			if !ok {
				t.Fatal("fingerprint failed")
			}
			if h == base {
				t.Errorf("fingerprint did not change: got %q (== base)", h)
			}
		})
	}
}

// cloneMessages returns a deep enough copy of msgs that test mutations to
// one msg don't bleed into the other test cases. Slices of maps and nested
// []D under "tool_calls" are duplicated; primitive values are reused.
func cloneMessages(msgs []D) []D {
	out := make([]D, len(msgs))
	for i, m := range msgs {
		nm := D{}
		for k, v := range m {
			switch vv := v.(type) {
			case []D:
				cp := make([]D, len(vv))
				for j, item := range vv {
					nitem := D{}
					for ik, iv := range item {
						if dv, ok := iv.(D); ok {
							ndv := D{}
							for dk, dvv := range dv {
								ndv[dk] = dvv
							}
							nitem[ik] = ndv
						} else {
							nitem[ik] = iv
						}
					}
					cp[j] = nitem
				}
				nm[k] = cp
			default:
				nm[k] = vv
			}
		}
		out[i] = nm
	}
	return out
}

// TestIMCCommitAndResetRenderInputHash verifies that imcCommitSession stores
// the render-input fingerprint and imcResetSession clears it. The lifecycle
// is what makes the snapshot-skip predicate safe across session evictions.
func TestIMCCommitAndResetRenderInputHash(t *testing.T) {
	m := newFingerprintTestModel("template-x")

	session := &imcSession{
		kvState: ramSessionStore(),
		slotID:  0,
		seqID:   0,
		pending: true,
	}
	m.imcSessions[0] = session

	m.imcCommitSession(session, "h1", 500, 3, nil, false, nil, "syshash", 50, "render-hash-1")

	if session.cachedRenderInputHash != "render-hash-1" {
		t.Errorf("after commit: cachedRenderInputHash = %q, want %q",
			session.cachedRenderInputHash, "render-hash-1")
	}
	if session.pending {
		t.Error("after commit: pending should be false")
	}

	// A subsequent commit (e.g. extend) refreshes the fingerprint.
	m.imcCommitSession(session, "h2", 700, 4, nil, false, nil, "syshash", 50, "render-hash-2")
	if session.cachedRenderInputHash != "render-hash-2" {
		t.Errorf("after re-commit: cachedRenderInputHash = %q, want %q",
			session.cachedRenderInputHash, "render-hash-2")
	}

	// Reset clears it.
	m.cacheMu.Lock()
	imcResetSession(session)
	m.cacheMu.Unlock()

	if session.cachedRenderInputHash != "" {
		t.Errorf("after reset: cachedRenderInputHash = %q, want empty", session.cachedRenderInputHash)
	}
}

// TestIMCCommitEmptyRenderHashDisqualifiesSkip is a regression guard: a
// committed session with an empty cachedRenderInputHash must never qualify
// for the pure-hit snapshot skip predicate. The startSlot block uses
// session.cachedRenderInputHash != "" as one of its required guards.
func TestIMCCommitEmptyRenderHashDisqualifiesSkip(t *testing.T) {
	m := newFingerprintTestModel("template-x")
	session := &imcSession{
		kvState: ramSessionStore(),
		slotID:  0,
		seqID:   0,
		pending: true,
	}
	m.imcSessions[0] = session

	m.imcCommitSession(session, "h", 100, 2, nil, false, nil, "", 0, "")

	if session.cachedRenderInputHash != "" {
		t.Errorf("cachedRenderInputHash = %q, want empty (pre-rollout sentinel)",
			session.cachedRenderInputHash)
	}
}
