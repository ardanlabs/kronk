// Package models provides whisper model management backed by the
// github.com/ardanlabs/bucky download primitives. It is the whisper
// counterpart to sdk/tools/models (llama) and is wired into shared
// dispatch code through sdk/tools/backend.
package models

import (
	"context"
	"fmt"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/ardanlabs/bucky/pkg/download"
	"github.com/ardanlabs/kronk/sdk/applog"
	"github.com/ardanlabs/kronk/sdk/tools/backend"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/downloader"
	"github.com/hashicorp/go-getter"
	"go.yaml.in/yaml/v2"
)

var (
	localFolder = "bucky-models"
	indexFile   = ".index.yaml"
)

// Path returns file path information about a whisper model. It is an
// alias for backend.ModelPath so cross-backend code can consume the
// same value type returned by every backend's Catalog implementation.
type Path = backend.ModelPath

// Logger represents a logger for capturing events.
type Logger = applog.Logger

// =============================================================================

// Models manages the whisper model system. Each whisper model is a
// single GGML .bin file stored flat under <basePath>/bucky-models/.
// The on-disk filename convention mirrors the upstream HuggingFace
// mirror (ggml-<name>.bin); the index key strips the "ggml-" prefix
// and ".bin" suffix so callers can look models up by short name
// ("tiny", "base.en", "large-v3").
type Models struct {
	basePath   string
	modelsPath string
	biMutex    sync.Mutex
}

// New constructs the whisper models system using default paths.
func New() (*Models, error) {
	return NewWithPaths("")
}

// NewWithPaths constructs the whisper models system. If basePath is
// empty, the default location is used.
func NewWithPaths(basePath string) (*Models, error) {
	basePath = defaults.BaseDir(basePath)

	modelPath := filepath.Join(basePath, localFolder)

	if err := os.MkdirAll(modelPath, 0o755); err != nil {
		return nil, fmt.Errorf("creating models directory: %w", err)
	}

	m := Models{
		basePath:   basePath,
		modelsPath: modelPath,
	}

	return &m, nil
}

// Path returns the location of the whisper models path.
func (m *Models) Path() string {
	return m.modelsPath
}

// BasePath returns the kronk base directory the system was
// constructed with.
func (m *Models) BasePath() string {
	return m.basePath
}

// BuildIndex builds the whisper model index for fast model access.
// checkSHA is accepted for interface compatibility with
// backend.Catalog but is currently a no-op: whisper models do not
// ship a SHA companion file and the on-disk size is the only check
// performed.
func (m *Models) BuildIndex(log applog.Logger, checkSHA bool) error {
	m.biMutex.Lock()
	defer m.biMutex.Unlock()

	_ = checkSHA

	entries, err := os.ReadDir(m.modelsPath)
	if err != nil {
		return fmt.Errorf("list-models: reading models directory: %w", err)
	}

	index := make(map[string]Path)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".bin") {
			continue
		}

		full := filepath.Join(m.modelsPath, name)

		var size int64
		if info, err := os.Stat(full); err == nil {
			size = info.Size()
		}

		modelID := extractModelID(name)
		index[modelID] = Path{
			ModelFiles: []string{full},
			Downloaded: true,
			Validated:  true,
			FileSizes:  []int64{size},
		}
	}

	indexData, err := yaml.Marshal(&index)
	if err != nil {
		return fmt.Errorf("marshal index: %w", err)
	}

	if err := os.WriteFile(filepath.Join(m.modelsPath, indexFile), indexData, 0o644); err != nil {
		return fmt.Errorf("write index file: %w", err)
	}

	return nil
}

// Download fetches a whisper model identified by source and returns
// its on-disk layout. source may be:
//
//   - A short catalog name ("tiny", "base.en", "large-v3").
//   - A full ggml filename ("ggml-tiny.bin").
//   - A fully qualified download URL accepted by hashicorp/go-getter.
//
// Models already present on disk for the resolved short name are
// returned without a network round-trip.
func (m *Models) Download(ctx context.Context, log applog.Logger, source string) (Path, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return Path{}, fmt.Errorf("download: empty source")
	}

	var (
		downloadURL string
		fileName    string
	)

	switch {
	case isURL(source):
		downloadURL = source
		fileName = filepath.Base(parseURLPath(source))
		if fileName == "" || fileName == "." || fileName == "/" {
			return Path{}, fmt.Errorf("download: cannot derive filename from url %q", source)
		}

	default:
		short := normalizeShortName(source)
		entry, ok := catalog[short]
		if !ok {
			return Path{}, fmt.Errorf("download: unknown model %q (see SupportedModels for the bundled list)", source)
		}
		downloadURL = entry.URL
		fileName = filepath.Base(parseURLPath(entry.URL))
	}

	dest := filepath.Join(m.modelsPath, fileName)
	if info, err := os.Stat(dest); err == nil && info.Size() > 0 {
		log(ctx, "download-model: already installed", "file", fileName)
		mp := Path{
			ModelFiles: []string{dest},
			Downloaded: true,
			Validated:  true,
			FileSizes:  []int64{info.Size()},
		}
		if err := m.refreshIndex(log); err != nil {
			log(ctx, "download-model: refresh index", "ERROR", err)
		}
		if err := m.cacheHeaderFromFile(extractModelID(fileName), dest); err != nil {
			log(ctx, "download-model: cache header", "ERROR", err)
		}
		return mp, nil
	}

	progress := func(src string, currentSize int64, totalSize int64, mbPerSec float64, complete bool) {
		log(ctx, fmt.Sprintf("\r\x1b[Kdownload-model: Downloading %s... %d MB of %d MB (%.2f MB/s)", src, currentSize/(1000*1000), totalSize/(1000*1000), mbPerSec))
	}

	pr := downloader.NewProgressReader(progress, downloader.SizeIntervalMB10)

	if err := download.GetModelWithContext(ctx, downloadURL, m.modelsPath, getter.ProgressTracker(pr)); err != nil {
		return Path{}, fmt.Errorf("download: %w", err)
	}

	info, err := os.Stat(dest)
	if err != nil {
		return Path{}, fmt.Errorf("download: stat installed model: %w", err)
	}

	if err := m.refreshIndex(log); err != nil {
		log(ctx, "download-model: refresh index", "ERROR", err)
	}

	if err := m.cacheHeaderFromFile(extractModelID(fileName), dest); err != nil {
		log(ctx, "download-model: cache header", "ERROR", err)
	}

	return Path{
		ModelFiles: []string{dest},
		Downloaded: true,
		Validated:  true,
		FileSizes:  []int64{info.Size()},
	}, nil
}

// FullPath returns the on-disk layout of an already-installed whisper
// model. modelID may be the short catalog name ("tiny"), the full
// ggml filename ("ggml-tiny.bin"), or the bare basename without
// extension ("ggml-tiny").
func (m *Models) FullPath(modelID string) (Path, error) {
	index := m.loadIndex()

	key := normalizeShortName(modelID)
	if mp, ok := index[key]; ok {
		return mp, nil
	}

	return Path{}, fmt.Errorf("retrieve-path: model %q not found", modelID)
}

// Remove deletes the supplied whisper model from disk. The on-disk
// index is rebuilt after the file has been removed.
func (m *Models) Remove(mp Path, log applog.Logger) error {
	for _, modelFile := range mp.ModelFiles {
		if err := m.removeHeaderCache(extractModelID(modelFile)); err != nil {
			log(context.Background(), "remove: header cache", "ERROR", err)
		}
		if err := os.Remove(modelFile); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove: unable to remove model: %q: %w", modelFile, err)
		}
	}

	if err := m.refreshIndex(log); err != nil {
		return fmt.Errorf("remove: refresh index: %w", err)
	}

	return nil
}

// SupportedModels returns the bundled catalog of well-known whisper
// model short names sorted ascending. It is intended for CLI / UI
// surfaces that need to present the user with a picker.
func SupportedModels() []string {
	out := make([]string, 0, len(catalog))
	for name := range catalog {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// CatalogEntry describes a single bundled whisper model.
type CatalogEntry struct {
	URL   string
	Size  string
	Notes string
}

// Catalog returns a copy of the bundled whisper catalog map keyed by
// short name.
func Catalog() map[string]CatalogEntry {
	out := make(map[string]CatalogEntry, len(catalog))
	maps.Copy(out, catalog)
	return out
}

// =============================================================================

func (m *Models) refreshIndex(log applog.Logger) error {
	// BuildIndex acquires biMutex; do not call it while holding the
	// mutex from a caller. Remove/Download release before invoking.
	return m.BuildIndex(log, false)
}

func (m *Models) loadIndex() map[string]Path {
	m.biMutex.Lock()
	defer m.biMutex.Unlock()

	data, err := os.ReadFile(filepath.Join(m.modelsPath, indexFile))
	if err != nil {
		return map[string]Path{}
	}

	var index map[string]Path
	if err := yaml.Unmarshal(data, &index); err != nil {
		return map[string]Path{}
	}

	return index
}

// normalizeShortName turns input the user typed into the canonical
// short-name key stored in the index ("tiny", "base.en"). It accepts
// the bare short name, the ggml-<name>.bin filename, and the bare
// ggml-<name> form.
func normalizeShortName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimSuffix(name, ".bin")
	name = strings.TrimPrefix(name, "ggml-")
	return name
}

// extractModelID returns the catalog short name for an on-disk
// whisper filename ("ggml-tiny.bin" → "tiny").
func extractModelID(fileName string) string {
	return normalizeShortName(filepath.Base(fileName))
}

func isURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

func parseURLPath(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Path
}

// =============================================================================

// catalog is the curated set of well-known whisper models. URLs are
// pinned to the upstream HuggingFace mirror. The set mirrors the
// table maintained in github.com/ardanlabs/bucky/cmd/model.go so
// short-name input works against both surfaces.
var catalog = map[string]CatalogEntry{
	"tiny":           {URL: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-tiny.bin", Size: "75 MB", Notes: "multilingual, fastest, lowest accuracy"},
	"base":           {URL: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.bin", Size: "142 MB", Notes: "multilingual, fast"},
	"base.en":        {URL: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.en.bin", Size: "142 MB", Notes: "english-only, fast"},
	"small":          {URL: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin", Size: "466 MB", Notes: "multilingual, balanced"},
	"small.en":       {URL: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.en.bin", Size: "466 MB", Notes: "english-only, balanced"},
	"medium":         {URL: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-medium.bin", Size: "1.5 GB", Notes: "multilingual, accurate"},
	"medium.en":      {URL: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-medium.en.bin", Size: "1.5 GB", Notes: "english-only, accurate"},
	"large-v3":       {URL: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3.bin", Size: "2.9 GB", Notes: "multilingual, highest accuracy"},
	"large-v3-turbo": {URL: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3-turbo.bin", Size: "1.5 GB", Notes: "multilingual, near-large accuracy at small/medium speed"},
	"silero-vad":     {URL: "https://huggingface.co/ggml-org/whisper-vad/resolve/main/ggml-silero-v5.1.2.bin", Size: "0.9 MB", Notes: "voice-activity detector, pairs with any whisper model"},
}
