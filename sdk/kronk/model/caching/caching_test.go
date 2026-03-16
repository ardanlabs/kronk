package caching

import (
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestHashMessages(t *testing.T) {
	tests := []struct {
		name     string
		msgs1    []D
		msgs2    []D
		wantSame bool
	}{
		{
			name: "identical messages same hash",
			msgs1: []D{
				{"role": "system", "content": "You are helpful"},
				{"role": "user", "content": "Hello"},
			},
			msgs2: []D{
				{"role": "system", "content": "You are helpful"},
				{"role": "user", "content": "Hello"},
			},
			wantSame: true,
		},
		{
			name: "different content different hash",
			msgs1: []D{
				{"role": "user", "content": "Hello"},
			},
			msgs2: []D{
				{"role": "user", "content": "Goodbye"},
			},
			wantSame: false,
		},
		{
			name: "different role different hash",
			msgs1: []D{
				{"role": "user", "content": "Hello"},
			},
			msgs2: []D{
				{"role": "assistant", "content": "Hello"},
			},
			wantSame: false,
		},
		{
			name: "different order different hash",
			msgs1: []D{
				{"role": "user", "content": "A"},
				{"role": "assistant", "content": "B"},
			},
			msgs2: []D{
				{"role": "assistant", "content": "B"},
				{"role": "user", "content": "A"},
			},
			wantSame: false,
		},
		{
			name:     "empty messages same hash",
			msgs1:    []D{},
			msgs2:    []D{},
			wantSame: true,
		},
		{
			name: "prefix subset different hash",
			msgs1: []D{
				{"role": "user", "content": "Hello"},
			},
			msgs2: []D{
				{"role": "user", "content": "Hello"},
				{"role": "assistant", "content": "Hi"},
			},
			wantSame: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash1 := HashMessages(tt.msgs1)
			hash2 := HashMessages(tt.msgs2)

			if tt.wantSame && hash1 != hash2 {
				t.Errorf("expected same hash, got %s != %s", hash1, hash2)
			}
			if !tt.wantSame && hash1 == hash2 {
				t.Errorf("expected different hash, got same: %s", hash1)
			}
		})
	}
}

func TestExtractMessageContent(t *testing.T) {
	tests := []struct {
		name string
		msg  D
		want string
	}{
		{
			name: "string content",
			msg:  D{"role": "user", "content": "Hello world"},
			want: "Hello world",
		},
		{
			name: "nil content",
			msg:  D{"role": "assistant", "content": nil},
			want: "",
		},
		{
			name: "missing content",
			msg:  D{"role": "user"},
			want: "",
		},
		{
			name: "array content with text parts",
			msg: D{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": "Hello "},
					map[string]any{"type": "text", "text": "world"},
				},
			},
			want: "Hello world",
		},
		{
			name: "array content with mixed types",
			msg: D{
				"role": "user",
				"content": []any{
					map[string]any{"type": "image", "url": "http://..."},
					map[string]any{"type": "text", "text": "caption"},
				},
			},
			want: "caption",
		},
		{
			name: "empty array content",
			msg: D{
				"role":    "user",
				"content": []any{},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractMessageContent(tt.msg)
			if got != tt.want {
				t.Errorf("ExtractMessageContent() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRemoveMessagesAtIndices(t *testing.T) {
	tests := []struct {
		name      string
		messages  []D
		indices   []int
		wantCount int
		wantFirst string
	}{
		{
			name: "remove first message",
			messages: []D{
				{"role": "system", "content": "sys"},
				{"role": "user", "content": "user"},
			},
			indices:   []int{0},
			wantCount: 1,
			wantFirst: "user",
		},
		{
			name: "remove last message",
			messages: []D{
				{"role": "system", "content": "sys"},
				{"role": "user", "content": "user"},
			},
			indices:   []int{1},
			wantCount: 1,
			wantFirst: "sys",
		},
		{
			name: "remove multiple messages",
			messages: []D{
				{"role": "system", "content": "sys"},
				{"role": "user", "content": "user1"},
				{"role": "assistant", "content": "asst"},
				{"role": "user", "content": "user2"},
			},
			indices:   []int{0, 2},
			wantCount: 2,
			wantFirst: "user1",
		},
		{
			name: "remove none",
			messages: []D{
				{"role": "user", "content": "keep"},
			},
			indices:   []int{},
			wantCount: 1,
			wantFirst: "keep",
		},
		{
			name: "remove all",
			messages: []D{
				{"role": "user", "content": "remove"},
			},
			indices:   []int{0},
			wantCount: 1,
			wantFirst: "Tell the user you are ready to help them.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := D{"messages": tt.messages}
			result := RemoveMessagesAtIndices(d, tt.indices)

			msgs, ok := result["messages"].([]D)
			if !ok {
				t.Fatal("result messages not []D")
			}

			if len(msgs) != tt.wantCount {
				t.Errorf("got %d messages, want %d", len(msgs), tt.wantCount)
			}

			if len(msgs) > 0 {
				content, _ := msgs[0]["content"].(string)
				if content != tt.wantFirst {
					t.Errorf("first message content = %q, want %q", content, tt.wantFirst)
				}
			}
		})
	}
}

func TestHashMessage(t *testing.T) {
	msg1 := CacheableMessage{Role: "system", Content: "Hello"}
	msg2 := CacheableMessage{Role: "system", Content: "Hello"}
	msg3 := CacheableMessage{Role: "user", Content: "Hello"}
	msg4 := CacheableMessage{Role: "system", Content: "World"}

	hash1 := HashMessage(msg1)
	hash2 := HashMessage(msg2)
	hash3 := HashMessage(msg3)
	hash4 := HashMessage(msg4)

	if hash1 != hash2 {
		t.Errorf("identical messages should have same hash")
	}

	if hash1 == hash3 {
		t.Errorf("different role should produce different hash")
	}

	if hash1 == hash4 {
		t.Errorf("different content should produce different hash")
	}

	if len(hash1) != 64 {
		t.Errorf("hash length = %d, want 64", len(hash1))
	}
}

func TestTokenPrefixMatch(t *testing.T) {
	tests := []struct {
		name     string
		cached   []llama.Token
		incoming []llama.Token
		want     int
	}{
		{
			name:     "identical sequences",
			cached:   []llama.Token{1, 2, 3, 4, 5},
			incoming: []llama.Token{1, 2, 3, 4, 5},
			want:     5,
		},
		{
			name:     "empty cached",
			cached:   []llama.Token{},
			incoming: []llama.Token{1, 2, 3},
			want:     0,
		},
		{
			name:     "empty incoming",
			cached:   []llama.Token{1, 2, 3},
			incoming: []llama.Token{},
			want:     0,
		},
		{
			name:     "both empty",
			cached:   []llama.Token{},
			incoming: []llama.Token{},
			want:     0,
		},
		{
			name:     "diverge at start",
			cached:   []llama.Token{1, 2, 3},
			incoming: []llama.Token{9, 2, 3},
			want:     0,
		},
		{
			name:     "diverge in middle",
			cached:   []llama.Token{1, 2, 3, 4, 5},
			incoming: []llama.Token{1, 2, 9, 4, 5},
			want:     2,
		},
		{
			name:     "cached shorter than incoming",
			cached:   []llama.Token{1, 2, 3},
			incoming: []llama.Token{1, 2, 3, 4, 5},
			want:     3,
		},
		{
			name:     "incoming shorter than cached",
			cached:   []llama.Token{1, 2, 3, 4, 5},
			incoming: []llama.Token{1, 2, 3},
			want:     3,
		},
		{
			name:     "diverge at last element",
			cached:   []llama.Token{1, 2, 3, 4, 5},
			incoming: []llama.Token{1, 2, 3, 4, 9},
			want:     4,
		},
		{
			name:     "single element match",
			cached:   []llama.Token{42},
			incoming: []llama.Token{42},
			want:     1,
		},
		{
			name:     "single element mismatch",
			cached:   []llama.Token{42},
			incoming: []llama.Token{99},
			want:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TokenPrefixMatch(tt.cached, tt.incoming)
			if got != tt.want {
				t.Errorf("TokenPrefixMatch() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestResultFields(t *testing.T) {
	result := Result{
		ModifiedD: D{"test": "value"},
		IMC: &IMCResult{
			CacheIdx: 1000,
			SeqID:    llama.SeqId(2),
		},
	}
	if result.IMC.SeqID != 2 {
		t.Errorf("SeqID = %d, want 2", result.IMC.SeqID)
	}
	if result.IMC.CacheIdx != 1000 {
		t.Errorf("CacheIdx = %d, want 1000", result.IMC.CacheIdx)
	}
}
