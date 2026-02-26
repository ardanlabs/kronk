// Package downapp serves model files over HTTP using a Hugging Face compatible
// URL scheme so clients on a local network can download models without
// internet access.
package downapp

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
)

type app struct {
	log        *logger.Logger
	modelsPath string
}

func newApp(cfg Config) *app {
	return &app{
		log:        cfg.Log,
		modelsPath: cfg.ModelsPath,
	}
}

// handle serves model files. It supports two URL patterns:
//
//	/download/:org/:repo/resolve/main/:file  →  modelsPath/:org/:repo/:file
//	/download/:org/:repo/raw/main/:file      →  modelsPath/:org/:repo/sha/:file
func (a *app) handle(w http.ResponseWriter, r *http.Request) {
	// The route is registered as /download/{path...} so strip the prefix.
	trimmed := strings.TrimPrefix(r.URL.Path, "/download/")

	// Parse: :org/:repo/:action/main/:file...
	parts := strings.Split(trimmed, "/")
	if len(parts) < 5 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	org := parts[0]
	repo := parts[1]
	action := parts[2]
	// parts[3] is "main"
	fileName := strings.Join(parts[4:], "/")

	if org == "" || repo == "" || fileName == "" {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	if parts[3] != "main" {
		http.Error(w, "only 'main' branch is supported", http.StatusBadRequest)
		return
	}

	var filePath string

	switch action {
	case "resolve":
		filePath = filepath.Join(a.modelsPath, org, repo, fileName)
	case "raw":
		filePath = filepath.Join(a.modelsPath, org, repo, "sha", fileName)
	default:
		http.Error(w, fmt.Sprintf("unsupported action: %s", action), http.StatusBadRequest)
		return
	}

	// Prevent directory traversal.
	absModels, err := filepath.Abs(a.modelsPath)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	absFile, err := filepath.Abs(filePath)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if !strings.HasPrefix(absFile, absModels) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	// Open and serve the file.
	f, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			a.log.Info(r.Context(), "download", "status", "file not found", "path", filePath)
			http.Error(w, "file not found", http.StatusNotFound)
			return
		}
		a.log.Error(r.Context(), "download", "status", "open error", "path", filePath, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if info.IsDir() {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}

	a.log.Info(r.Context(), "download", "status", "serving file", "org", org, "repo", repo, "file", fileName, "size", info.Size())

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(fileName)))

	http.ServeContent(w, r, fileName, info.ModTime(), f)
}
