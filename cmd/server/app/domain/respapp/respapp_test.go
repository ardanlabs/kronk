package respapp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
)

func TestResponsesRejectsTrailingJSONBeforeModelAcquisition(t *testing.T) {
	for _, stream := range []string{"false", "true"} {
		for _, suffix := range []string{`{"second":true}`, `42`, ` trailing-garbage`} {
			body := `{"model":"unavailable","input":"hello","stream":` + stream + `}` + suffix
			r := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
			// No pool: malformed requests must be rejected before model access.
			a := app{}
			resp := a.responses(t.Context(), r)
			err, ok := resp.(*errs.Error)
			if !ok || !err.Code.Equal(errs.InvalidArgument) {
				t.Errorf("stream=%s suffix=%q: got %+v, want InvalidArgument", stream, suffix, resp)
			}
		}
	}
	// Whitespace after one valid object is allowed; model-field validation
	// provides a model-free control that decoding reached the next stage.
	r := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader("{} \n\t"))
	a := app{}
	resp := a.responses(t.Context(), r)
	err, ok := resp.(*errs.Error)
	if !ok || !err.Code.Equal(errs.InvalidArgument) || err.Message != "missing model field" {
		t.Errorf("trailing whitespace: got %+v, want missing model field", resp)
	}
}
