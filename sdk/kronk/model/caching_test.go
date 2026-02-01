package model

import (
	"testing"
)

func TestFindCacheableMessage(t *testing.T) {
	tests := []struct {
		name        string
		messages    []D
		targetRole  string
		wantIndex   int
		wantContent string
		wantOK      bool
	}{
		{
			name: "find system message at index 0",
			messages: []D{
				{"role": "system", "content": "You are a helpful assistant."},
				{"role": "user", "content": "Hello"},
			},
			targetRole:  RoleSystem,
			wantIndex:   0,
			wantContent: "You are a helpful assistant.",
			wantOK:      true,
		},
		{
			name: "find user message at index 0",
			messages: []D{
				{"role": "user", "content": "Hello, this is my first message."},
				{"role": "assistant", "content": "Hi there!"},
			},
			targetRole:  RoleUser,
			wantIndex:   0,
			wantContent: "Hello, this is my first message.",
			wantOK:      true,
		},
		{
			name: "find user message at index 1 (after system)",
			messages: []D{
				{"role": "system", "content": "System prompt"},
				{"role": "user", "content": "Hello user"},
				{"role": "assistant", "content": "Hi there!"},
			},
			targetRole:  RoleUser,
			wantIndex:   1,
			wantContent: "Hello user",
			wantOK:      true,
		},
		{
			name: "no system message found",
			messages: []D{
				{"role": "user", "content": "Hello"},
				{"role": "assistant", "content": "Hi"},
			},
			targetRole:  RoleSystem,
			wantIndex:   0,
			wantContent: "",
			wantOK:      false,
		},
		{
			name:        "empty messages",
			messages:    []D{},
			targetRole:  RoleSystem,
			wantIndex:   0,
			wantContent: "",
			wantOK:      false,
		},
		{
			name: "message with empty content skipped",
			messages: []D{
				{"role": "system", "content": ""},
				{"role": "system", "content": "Valid system"},
				{"role": "user", "content": "Hello"},
			},
			targetRole:  RoleSystem,
			wantIndex:   1,
			wantContent: "Valid system",
			wantOK:      true,
		},
		{
			name: "message with empty role skipped",
			messages: []D{
				{"role": "", "content": "Hello"},
				{"role": "user", "content": "Valid user"},
			},
			targetRole:  RoleUser,
			wantIndex:   1,
			wantContent: "Valid user",
			wantOK:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, ok := findCacheableMessage(tt.messages, tt.targetRole)
			if ok != tt.wantOK {
				t.Errorf("findCacheableMessage() ok = %v, want %v", ok, tt.wantOK)
			}
			if ok {
				if info.index != tt.wantIndex {
					t.Errorf("findCacheableMessage() index = %d, want %d", info.index, tt.wantIndex)
				}
				if info.content != tt.wantContent {
					t.Errorf("findCacheableMessage() content = %q, want %q", info.content, tt.wantContent)
				}
				if info.role != tt.targetRole {
					t.Errorf("findCacheableMessage() role = %q, want %q", info.role, tt.targetRole)
				}
			}
		})
	}
}

func TestHashMessage(t *testing.T) {
	info1 := cacheableMessage{index: 0, role: "system", content: "You are a helpful assistant."}
	info2 := cacheableMessage{index: 0, role: "system", content: "You are a helpful assistant."}
	info3 := cacheableMessage{index: 0, role: "system", content: "You are a different assistant."}
	info4 := cacheableMessage{index: 0, role: "user", content: "You are a helpful assistant."}

	hash1 := hashMessage(info1)
	hash2 := hashMessage(info2)
	hash3 := hashMessage(info3)
	hash4 := hashMessage(info4)

	if hash1 != hash2 {
		t.Error("same role+content should produce same hash")
	}

	if hash1 == hash3 {
		t.Error("different content should produce different hash")
	}

	if hash1 == hash4 {
		t.Error("same content with different role should produce different hash")
	}

	if len(hash1) != 64 {
		t.Errorf("hash should be 64 hex chars (SHA-256), got %d", len(hash1))
	}

	// Index should not affect hash.
	info5 := cacheableMessage{index: 5, role: "system", content: "You are a helpful assistant."}
	hash5 := hashMessage(info5)
	if hash1 != hash5 {
		t.Error("different index should not affect hash")
	}
}

func TestHashMessages(t *testing.T) {
	msgs1 := []D{
		{"role": "system", "content": "You are a helpful assistant."},
		{"role": "user", "content": "Hello"},
	}
	msgs2 := []D{
		{"role": "system", "content": "You are a helpful assistant."},
		{"role": "user", "content": "Hello"},
	}
	msgs3 := []D{
		{"role": "system", "content": "You are a helpful assistant."},
		{"role": "user", "content": "Hello"},
		{"role": "assistant", "content": "Hi there!"},
	}
	msgs4 := []D{
		{"role": "system", "content": "Different system"},
		{"role": "user", "content": "Hello"},
	}

	hash1 := hashMessages(msgs1)
	hash2 := hashMessages(msgs2)
	hash3 := hashMessages(msgs3)
	hash4 := hashMessages(msgs4)

	if hash1 != hash2 {
		t.Error("same messages should produce same hash")
	}

	if hash1 == hash3 {
		t.Error("different message count should produce different hash")
	}

	if hash1 == hash4 {
		t.Error("different content should produce different hash")
	}

	// Verify prefix matching: msgs1 is a prefix of msgs3, but hashes differ.
	if hash1 == hashMessages(msgs3[:2]) {
		// This SHOULD be equal - prefix should match.
		t.Log("prefix hash matches as expected")
	}

	prefixHash := hashMessages(msgs3[:2])
	if hash1 != prefixHash {
		t.Error("prefix of messages should produce same hash as original prefix")
	}
}

func TestRemoveMessagesAtIndices(t *testing.T) {
	tests := []struct {
		name          string
		d             D
		indices       []int
		wantMsgCount  int
		wantFirstRole string
		wantUnchanged bool
	}{
		{
			name: "removes first message (index 0)",
			d: D{
				"messages": []D{
					{"role": "system", "content": "System prompt"},
					{"role": "user", "content": "Hello"},
				},
			},
			indices:       []int{0},
			wantMsgCount:  1,
			wantFirstRole: "user",
		},
		{
			name: "removes second message (index 1)",
			d: D{
				"messages": []D{
					{"role": "system", "content": "System prompt"},
					{"role": "user", "content": "Hello"},
					{"role": "assistant", "content": "Hi"},
				},
			},
			indices:       []int{1},
			wantMsgCount:  2,
			wantFirstRole: "system",
		},
		{
			name: "removes multiple messages",
			d: D{
				"messages": []D{
					{"role": "system", "content": "System prompt"},
					{"role": "user", "content": "Hello"},
					{"role": "assistant", "content": "Hi"},
				},
			},
			indices:       []int{0, 1},
			wantMsgCount:  1,
			wantFirstRole: "assistant",
		},
		{
			name: "empty indices returns unchanged",
			d: D{
				"messages": []D{
					{"role": "user", "content": "Hello"},
				},
			},
			indices:       []int{},
			wantMsgCount:  1,
			wantUnchanged: true,
		},
		{
			name: "empty messages returns unchanged",
			d: D{
				"messages": []D{},
			},
			indices:       []int{0},
			wantMsgCount:  0,
			wantUnchanged: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := removeMessagesAtIndices(tt.d, tt.indices)
			msgs, ok := result["messages"].([]D)
			if !ok {
				t.Fatal("messages should be []D")
			}

			if len(msgs) != tt.wantMsgCount {
				t.Errorf("got %d messages, want %d", len(msgs), tt.wantMsgCount)
			}

			switch {
			case tt.wantUnchanged:
				originalMsgs := tt.d["messages"].([]D)
				if len(msgs) != len(originalMsgs) {
					t.Error("expected unchanged D")
				}

			case len(msgs) > 0:
				if msgs[0]["role"] != tt.wantFirstRole {
					t.Errorf("first message role = %q, want %q", msgs[0]["role"], tt.wantFirstRole)
				}
			}
		})
	}
}
