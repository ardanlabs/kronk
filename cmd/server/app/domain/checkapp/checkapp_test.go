package checkapp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/checkapp"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
)

func TestLivenessMachine(t *testing.T) {
	app := web.NewApp(func(context.Context, string, ...any) {})
	checkapp.Routes(app, checkapp.Config{Build: "fixture"})
	w := httptest.NewRecorder()
	app.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/liveness", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("liveness status: %d", w.Code)
	}
	var response struct {
		OS       string `json:"os"`
		MaxProcs int    `json:"GOMAXPROCS"`
		Host     string `json:"host"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.OS != runtime.GOOS || response.MaxProcs <= 0 || response.Host == "" {
		t.Fatalf("missing/wrong server machine identity: %+v", response)
	}
}
