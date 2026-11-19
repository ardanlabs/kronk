package chatapi_test

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/apitest"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/mux"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func TestDownloadRejectsPathOutsideModelsRoot(t *testing.T) {
	basePath := t.TempDir()
	manager, err := models.NewWithPaths(basePath)
	if err != nil {
		t.Fatal(err)
	}
	modelsPath := manager.Path()
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

	api := newTransportAPI(t, mux.Config{Models: manager, DownloadEnabled: true})
	t.Run("in-root control", func(t *testing.T) {
		path := filepath.Join(modelsPath, "safe", "repo")
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "model.gguf"), []byte("public model"), 0644); err != nil {
			t.Fatal(err)
		}
		api.Run(t, []apitest.Table{{
			Name: "public model", Method: http.MethodGet,
			URL: "/download/safe/repo/resolve/main/model.gguf", StatusCode: http.StatusOK,
			ResponseCmpFunc: func(_ http.Header, body []byte) string {
				if string(body) != "public model" {
					return fmt.Sprintf("body: got %q, want public model", body)
				}
				return ""
			},
		}}, "download")
	})

	rejectSecret := func(_ http.Header, body []byte) string {
		if strings.Contains(string(body), "secret model") {
			return fmt.Sprintf("body: served file outside models root: %q", body)
		}
		return ""
	}
	api.Run(t, []apitest.Table{
		{
			Name: "unclean parent traversal", Method: http.MethodGet,
			URL:        "/download/../models-private/resolve/main/repo/secret.gguf",
			StatusCode: http.StatusTemporaryRedirect, ResponseCmpFunc: rejectSecret,
		},
		{
			// Encoded dot segments bypass ServeMux's clean-path redirect so the
			// bound download handler must reject the actual root escape.
			Name: "parent traversal", Method: http.MethodGet,
			URL:        "/download/%2e%2e/models-private/resolve/main/repo/secret.gguf",
			StatusCode: http.StatusBadRequest, ResponseCmpFunc: rejectSecret,
		},
	}, "download")

	t.Run("symlink escape", func(t *testing.T) {
		orgPath := filepath.Join(modelsPath, "org")
		if err := os.MkdirAll(orgPath, 0755); err != nil {
			t.Fatalf("create org directory: %v", err)
		}
		if err := os.Symlink(outsidePath, filepath.Join(orgPath, "repo")); err != nil {
			t.Skipf("create symlink: %v", err)
		}

		api.Run(t, []apitest.Table{{
			Name: "symlink escape", Method: http.MethodGet,
			URL:        "/download/org/repo/resolve/main/secret.gguf",
			StatusCode: http.StatusInternalServerError, ResponseCmpFunc: rejectSecret,
		}}, "download")
	})
}
