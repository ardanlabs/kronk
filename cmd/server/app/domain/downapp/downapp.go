// Package downapp serves model files over HTTP using a Hugging Face compatible
// URL scheme so clients on a local network can download models without
// internet access.
package downapp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
)

type app struct {
	log        *logger.Logger
	modelsPath string
	libs       *libs.Libs
}

func newApp(cfg Config) *app {
	return &app{
		log:        cfg.Log,
		modelsPath: cfg.ModelsPath,
		libs:       cfg.Libs,
	}
}

// bundleListResponse is the JSON shape returned by /download/libs.
type bundleListResponse struct {
	Bundles []libs.PeerBundle `json:"bundles"`
}

// handleListBundles returns the list of installed library bundles. Size
// and SHA256 are populated only when the cached zip already exists; the
// first download for any bundle will trigger a lazy build on the GET of
// the .zip endpoint.
func (a *app) handleListBundles(w http.ResponseWriter, r *http.Request) {
	if a.libs == nil {
		http.Error(w, "libs not configured", http.StatusServiceUnavailable)
		return
	}

	tags, err := a.libs.List()
	if err != nil {
		a.log.Error(r.Context(), "download-libs", "status", "list error", "err", err)
		http.Error(w, "list error", http.StatusInternalServerError)
		return
	}

	out := bundleListResponse{Bundles: make([]libs.PeerBundle, 0, len(tags))}

	for _, t := range tags {
		entry := libs.PeerBundle{
			Arch:      t.Arch,
			OS:        t.OS,
			Processor: t.Processor,
			Version:   t.Version,
		}

		// Best-effort enrichment: if the cached zip already exists, surface
		// its size and digest so clients can render a determinate progress
		// bar without an extra request.
		if art, err := a.libs.BundleArtifacts(t.Arch, t.OS, t.Processor); err == nil {
			entry.Size = art.Size
			entry.SHA256 = art.SHA256
		}

		out.Bundles = append(out.Bundles, entry)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_ = json.NewEncoder(w).Encode(out)
}

// handleBundleZip serves the cached bundle zip for the requested triple,
// building it on demand if necessary. The sha256 digest is returned in the
// X-Bundle-SHA256 header so clients can verify integrity without an extra
// round trip. The triple is parsed from the path components after
// "libs/", which the catch-all handle splits into opSys, arch, processor.
func (a *app) handleBundleZip(w http.ResponseWriter, r *http.Request, opSys string, arch string, processor string) {
	if a.libs == nil {
		http.Error(w, "libs not configured", http.StatusServiceUnavailable)
		return
	}

	if opSys == "" || arch == "" || processor == "" {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	if !libs.IsSupported(arch, opSys, processor) {
		http.Error(w, "unsupported combination", http.StatusBadRequest)
		return
	}

	art, err := a.libs.BuildBundleZip(arch, opSys, processor)
	if err != nil {
		a.log.Info(r.Context(), "download-libs", "status", "build error", "os", opSys, "arch", arch, "processor", processor, "err", err)
		// readVersionFile error → bundle not installed → 404. Anything
		// else → internal error.
		if strings.Contains(err.Error(), "bundle not installed") {
			http.Error(w, "bundle not installed", http.StatusNotFound)
			return
		}
		http.Error(w, "build error", http.StatusInternalServerError)
		return
	}

	f, err := os.Open(art.ZipPath)
	if err != nil {
		a.log.Error(r.Context(), "download-libs", "status", "open zip", "path", art.ZipPath, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("X-Bundle-SHA256", art.SHA256)
	w.Header().Set("X-Bundle-Version", art.Version)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(art.ZipPath)))
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Expose-Headers", "X-Bundle-SHA256, X-Bundle-Version")

	a.log.Info(r.Context(), "download-libs", "status", "serving zip", "os", opSys, "arch", arch, "processor", processor, "version", art.Version, "size", info.Size())

	http.ServeContent(w, r, filepath.Base(art.ZipPath), info.ModTime(), f)
}

// handle serves model and library bundle files. It supports several URL
// patterns; the first matching prefix wins:
//
//	/download/libs                                         → JSON bundle list
//	/download/libs/:os/:arch/:processor/bundle.zip         → bundle zip
//	/download/:org/:repo/resolve/main/:file                → modelsPath/:org/:repo/:file
//	/download/:org/:repo/raw/main/:file                    → modelsPath/:org/:repo/sha/:file
func (a *app) handle(w http.ResponseWriter, r *http.Request) {
	// The route is registered as /download/{path...} so strip the prefix.
	trimmed := strings.TrimPrefix(r.URL.Path, "/download/")

	// Library bundle endpoints share /download because that is the public
	// surface gated by DownloadEnabled. Dispatch them here so we do not
	// need separate route registrations that would conflict with the
	// catch-all in the stdlib mux's GET/HEAD method-pattern resolver.
	switch {
	case trimmed == "libs":
		a.handleListBundles(w, r)
		return

	case strings.HasPrefix(trimmed, "libs/"):
		rest := strings.TrimPrefix(trimmed, "libs/")
		segs := strings.Split(rest, "/")
		if len(segs) == 4 && segs[3] == "bundle.zip" {
			a.handleBundleZip(w, r, segs[0], segs[1], segs[2])
			return
		}
		http.Error(w, "invalid libs path", http.StatusBadRequest)
		return
	}

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

	// If the exact file doesn't exist and the request is for a projection file,
	// look for the Kronk-renamed mmproj file in the same directory.
	if _, err := os.Stat(filePath); os.IsNotExist(err) && strings.Contains(fileName, "mmproj") {
		if found := findMmproj(filepath.Dir(filePath)); found != "" {
			filePath = found
		}
	}

	a.log.Info(r.Context(), "download", "status", "resolved path", "org", org, "repo", repo, "action", action, "file", fileName, "filePath", filePath)

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

// findMmproj searches a directory for a file whose name starts with "mmproj".
// This handles the case where Kronk renamed the projection file from its
// original HuggingFace name to the Kronk naming convention.
func findMmproj(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "mmproj") {
			return filepath.Join(dir, e.Name())
		}
	}

	return ""
}
