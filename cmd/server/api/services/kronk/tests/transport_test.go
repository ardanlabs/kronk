package chatapi_test

import (
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/api/services/kronk/build"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/apitest"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/mux"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security/auth"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
)

// newTransportAPI binds the production routes and middleware in open mode for
// model-free HTTP checks with test-owned dependencies. JWT and native inference
// checks use apitest.NewWithModelConfig instead.
func newTransportAPI(t *testing.T, cfg mux.Config) *apitest.Test {
	t.Helper()
	cfg.Log = logger.New(io.Discard, logger.LevelInfo, "TEST", nil)
	cfg.AuthorizationMode = auth.Open
	if cfg.InferenceTimeout == 0 {
		cfg.InferenceTimeout = time.Minute
	}
	return apitest.NewWithHandler(mux.WebAPI(cfg, build.Routes(build.Config{})))
}

func jsonContentType(header http.Header, _ []byte) string {
	if got := header.Get("Content-Type"); got != "application/json" {
		return fmt.Sprintf("Content-Type: got %q, want application/json", got)
	}
	return ""
}
