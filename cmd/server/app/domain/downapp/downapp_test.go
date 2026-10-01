package downapp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
)

func TestHandleRejectsPathOutsideModelsRoot(t *testing.T) {
	basePath := t.TempDir()
	modelsPath := filepath.Join(basePath, "models")
	outsidePath := filepath.Join(basePath, "models-private", "repo")
	if err := os.MkdirAll(modelsPath, 0755); err != nil {
		t.Fatalf("create models directory: %v", err)
	}
	if err := os.MkdirAll(outsidePath, 0755); err != nil {
		t.Fatalf("create outside directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outsidePath, "secret.gguf"), []byte("secret model"), 0644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	log := logger.New(io.Discard, logger.LevelInfo, "TEST", func(context.Context) string { return "" })
	a := app{log: log, modelsPath: modelsPath}

	t.Run("parent traversal", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/download/../models-private/resolve/main/repo/secret.gguf", nil)
		w := httptest.NewRecorder()

		a.handle(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status: got %d, want %d", w.Code, http.StatusBadRequest)
		}
		if strings.Contains(w.Body.String(), "secret model") {
			t.Fatalf("body: served file outside models root: %q", w.Body.String())
		}
	})

	t.Run("symlink escape", func(t *testing.T) {
		orgPath := filepath.Join(modelsPath, "org")
		if err := os.MkdirAll(orgPath, 0755); err != nil {
			t.Fatalf("create org directory: %v", err)
		}
		if err := os.Symlink(outsidePath, filepath.Join(orgPath, "repo")); err != nil {
			t.Skipf("create symlink: %v", err)
		}

		r := httptest.NewRequest(http.MethodGet, "/download/org/repo/resolve/main/secret.gguf", nil)
		w := httptest.NewRecorder()

		a.handle(w, r)

		if w.Code == http.StatusOK {
			t.Fatalf("status: got %d, want path rejection", w.Code)
		}
		if strings.Contains(w.Body.String(), "secret model") {
			t.Fatalf("body: served symlink target outside models root: %q", w.Body.String())
		}
	})
}

func TestPeerPullEventEndsWithBlankLine(t *testing.T) {
	event := toPeerPullEvent(PeerPullEvent{Status: "downloading"})
	if !strings.HasSuffix(event, "\n\n") {
		t.Errorf("event: got %q, want blank-line terminator", event)
	}
}
