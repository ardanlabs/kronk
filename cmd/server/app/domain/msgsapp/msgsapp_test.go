package msgsapp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

func TestMessagesRejectsMissingMessagesBeforeModelAcquisition(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"test","max_tokens":1}`))

	resp := (&app{}).messages(t.Context(), req)
	appErr, ok := resp.(*errs.Error)
	if !ok {
		t.Fatalf("messages: got %T, want *errs.Error", resp)
	}
	if !appErr.Code.Equal(errs.InvalidArgument) {
		t.Errorf("Code: got %s, want %s", appErr.Code, errs.InvalidArgument)
	}
}

func TestToAnthropicStopReason(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "stop", input: model.FinishReasonStop, want: "end_turn"},
		{name: "tool call", input: model.FinishReasonTool, want: "tool_use"},
		{name: "token limit", input: model.FinishReasonLength, want: "max_tokens"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toAnthropicStopReason(tt.input); got != tt.want {
				t.Errorf("toAnthropicStopReason(%q): got %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
