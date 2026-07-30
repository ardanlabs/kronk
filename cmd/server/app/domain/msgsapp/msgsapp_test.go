package msgsapp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
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
