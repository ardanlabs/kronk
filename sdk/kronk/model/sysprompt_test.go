package model

import (
	"testing"
)

func TestExtractFirstMessage(t *testing.T) {
	tests := []struct {
		name        string
		d           D
		wantRole    string
		wantContent string
		wantOK      bool
	}{
		{
			name: "with system message",
			d: D{
				"messages": []D{
					{"role": "system", "content": "You are a helpful assistant."},
					{"role": "user", "content": "Hello"},
				},
			},
			wantRole:    "system",
			wantContent: "You are a helpful assistant.",
			wantOK:      true,
		},
		{
			name: "with user message first",
			d: D{
				"messages": []D{
					{"role": "user", "content": "Hello, this is my first message."},
					{"role": "assistant", "content": "Hi there!"},
				},
			},
			wantRole:    "user",
			wantContent: "Hello, this is my first message.",
			wantOK:      true,
		},
		{
			name: "with assistant message first",
			d: D{
				"messages": []D{
					{"role": "assistant", "content": "Welcome!"},
					{"role": "user", "content": "Thanks"},
				},
			},
			wantRole:    "assistant",
			wantContent: "Welcome!",
			wantOK:      true,
		},
		{
			name: "empty messages",
			d: D{
				"messages": []D{},
			},
			wantRole:    "",
			wantContent: "",
			wantOK:      false,
		},
		{
			name:        "no messages key",
			d:           D{},
			wantRole:    "",
			wantContent: "",
			wantOK:      false,
		},
		{
			name: "first message with empty content",
			d: D{
				"messages": []D{
					{"role": "system", "content": ""},
					{"role": "user", "content": "Hello"},
				},
			},
			wantRole:    "",
			wantContent: "",
			wantOK:      false,
		},
		{
			name: "first message with empty role",
			d: D{
				"messages": []D{
					{"role": "", "content": "Hello"},
				},
			},
			wantRole:    "",
			wantContent: "",
			wantOK:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, ok := extractFirstMessage(tt.d)
			if info.role != tt.wantRole {
				t.Errorf("extractFirstMessage() role = %q, want %q", info.role, tt.wantRole)
			}
			if info.content != tt.wantContent {
				t.Errorf("extractFirstMessage() content = %q, want %q", info.content, tt.wantContent)
			}
			if ok != tt.wantOK {
				t.Errorf("extractFirstMessage() ok = %v, want %v", ok, tt.wantOK)
			}
		})
	}
}

func TestHashFirstMessage(t *testing.T) {
	info1 := firstMessageInfo{role: "system", content: "You are a helpful assistant."}
	info2 := firstMessageInfo{role: "system", content: "You are a helpful assistant."}
	info3 := firstMessageInfo{role: "system", content: "You are a different assistant."}
	info4 := firstMessageInfo{role: "user", content: "You are a helpful assistant."}

	hash1 := hashFirstMessage(info1)
	hash2 := hashFirstMessage(info2)
	hash3 := hashFirstMessage(info3)
	hash4 := hashFirstMessage(info4)

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
}

func TestRemoveFirstMessage(t *testing.T) {
	tests := []struct {
		name         string
		d            D
		wantMsgCount int
	}{
		{
			name: "removes system message",
			d: D{
				"messages": []D{
					{"role": "system", "content": "System prompt"},
					{"role": "user", "content": "Hello"},
				},
			},
			wantMsgCount: 1,
		},
		{
			name: "removes user message",
			d: D{
				"messages": []D{
					{"role": "user", "content": "Hello"},
					{"role": "assistant", "content": "Hi"},
				},
			},
			wantMsgCount: 1,
		},
		{
			name: "empty messages unchanged",
			d: D{
				"messages": []D{},
			},
			wantMsgCount: 0,
		},
		{
			name: "single message removed",
			d: D{
				"messages": []D{
					{"role": "user", "content": "Hello"},
				},
			},
			wantMsgCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := removeFirstMessage(tt.d)
			msgs, ok := result["messages"].([]D)
			if !ok {
				t.Fatal("messages should be []D")
			}
			if len(msgs) != tt.wantMsgCount {
				t.Errorf("got %d messages, want %d", len(msgs), tt.wantMsgCount)
			}

			originalMsgs := tt.d["messages"].([]D)
			if len(originalMsgs) > 0 && len(msgs) > 0 {
				if msgs[0]["role"] == originalMsgs[0]["role"] && msgs[0]["content"] == originalMsgs[0]["content"] {
					t.Error("first message should have been removed")
				}
			}
		})
	}
}
