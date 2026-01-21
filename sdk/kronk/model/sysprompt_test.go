package model

import (
	"testing"
)

func TestExtractSystemPrompt(t *testing.T) {
	tests := []struct {
		name    string
		d       D
		want    string
		wantOK  bool
	}{
		{
			name: "with system message",
			d: D{
				"messages": []D{
					{"role": "system", "content": "You are a helpful assistant."},
					{"role": "user", "content": "Hello"},
				},
			},
			want:   "You are a helpful assistant.",
			wantOK: true,
		},
		{
			name: "no system message",
			d: D{
				"messages": []D{
					{"role": "user", "content": "Hello"},
				},
			},
			want:   "",
			wantOK: false,
		},
		{
			name: "empty messages",
			d: D{
				"messages": []D{},
			},
			want:   "",
			wantOK: false,
		},
		{
			name:   "no messages key",
			d:      D{},
			want:   "",
			wantOK: false,
		},
		{
			name: "system message with empty content",
			d: D{
				"messages": []D{
					{"role": "system", "content": ""},
					{"role": "user", "content": "Hello"},
				},
			},
			want:   "",
			wantOK: false,
		},
		{
			name: "user message first",
			d: D{
				"messages": []D{
					{"role": "user", "content": "Hello"},
					{"role": "system", "content": "You are a helpful assistant."},
				},
			},
			want:   "",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := extractSystemPrompt(tt.d)
			if got != tt.want {
				t.Errorf("extractSystemPrompt() got = %q, want %q", got, tt.want)
			}
			if ok != tt.wantOK {
				t.Errorf("extractSystemPrompt() ok = %v, want %v", ok, tt.wantOK)
			}
		})
	}
}

func TestHashSystemPrompt(t *testing.T) {
	hash1 := hashSystemPrompt("You are a helpful assistant.")
	hash2 := hashSystemPrompt("You are a helpful assistant.")
	hash3 := hashSystemPrompt("You are a different assistant.")

	if hash1 != hash2 {
		t.Error("same content should produce same hash")
	}

	if hash1 == hash3 {
		t.Error("different content should produce different hash")
	}

	if len(hash1) != 64 {
		t.Errorf("hash should be 64 hex chars (SHA-256), got %d", len(hash1))
	}
}

func TestRemoveSystemMessage(t *testing.T) {
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
			name: "keeps non-system first message",
			d: D{
				"messages": []D{
					{"role": "user", "content": "Hello"},
					{"role": "assistant", "content": "Hi"},
				},
			},
			wantMsgCount: 2,
		},
		{
			name: "empty messages unchanged",
			d: D{
				"messages": []D{},
			},
			wantMsgCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := removeSystemMessage(tt.d)
			msgs, ok := result["messages"].([]D)
			if !ok {
				t.Fatal("messages should be []D")
			}
			if len(msgs) != tt.wantMsgCount {
				t.Errorf("got %d messages, want %d", len(msgs), tt.wantMsgCount)
			}

			originalMsgs := tt.d["messages"].([]D)
			if len(msgs) != len(originalMsgs) && len(originalMsgs) > 0 {
				if msgs[0]["role"] == "system" {
					t.Error("system message should have been removed")
				}
			}
		})
	}
}
