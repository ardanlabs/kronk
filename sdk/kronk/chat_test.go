package kronk

import (
	"errors"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

func TestChatValidatesMessagesBeforeAdmission(t *testing.T) {
	tests := []struct {
		name string
		d    model.D
		want error
	}{
		{name: "missing", d: model.D{}, want: model.ErrMessagesMissing},
		{name: "invalid", d: model.D{"messages": "invalid"}, want: model.ErrMessagesInvalid},
		{
			name: "unsupported tool choice",
			d: model.D{
				"messages":    []model.D{{"role": "user", "content": "hello"}},
				"tool_choice": "required",
			},
			want: model.ErrInvalidRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var krn Kronk

			if _, err := krn.Chat(t.Context(), tt.d); !errors.Is(err, tt.want) {
				t.Errorf("Chat: got %v, want %v", err, tt.want)
			}
			if _, err := krn.ChatStreaming(t.Context(), tt.d); !errors.Is(err, tt.want) {
				t.Errorf("ChatStreaming: got %v, want %v", err, tt.want)
			}
		})
	}
}
