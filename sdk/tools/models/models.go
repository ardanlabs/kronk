// Package models provides support for tooling around model management.
package models

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"go.yaml.in/yaml/v2"
)

var (
	localFolder = "models"
	indexFile   = ".index.yaml"
)

// Models manages the model system.
type Models struct {
	basePath   string
	modelsPath string
	biMutex    sync.Mutex
}

// New constructs the models system using defaults paths.
func New() (*Models, error) {
	return NewWithPaths("")
}

// NewWithPaths constructs the models system, If the basePath is empty, the
// default location is used.
func NewWithPaths(basePath string) (*Models, error) {
	basePath = defaults.BaseDir(basePath)

	modelPath := filepath.Join(basePath, localFolder)

	if err := os.MkdirAll(modelPath, 0755); err != nil {
		return nil, fmt.Errorf("creating catalogs directory: %w", err)
	}

	m := Models{
		basePath:   basePath,
		modelsPath: modelPath,
	}

	return &m, nil
}

// Path returns the location of the models path.
func (m *Models) Path() string {
	return m.modelsPath
}

// BasePath returns the kronk base directory the system was constructed with.
func (m *Models) BasePath() string {
	return m.basePath
}

// BuildIndex builds the model index for fast model access. When checkSHA is
// true, all models are fully validated with SHA256 checks. When false,
// previously validated models are trusted (used at KMS startup for speed).
func (m *Models) BuildIndex(log Logger, checkSHA bool) error {
	currentIndex := m.loadIndex()

	m.biMutex.Lock()
	defer m.biMutex.Unlock()

	if err := m.removeEmptyDirs(); err != nil {
		return fmt.Errorf("remove-empty-dirs: %w", err)
	}

	entries, err := os.ReadDir(m.modelsPath)
	if err != nil {
		return fmt.Errorf("list-models: reading models directory: %w", err)
	}

	index := make(map[string]Path)

	for _, orgEntry := range entries {
		if !orgEntry.IsDir() {
			continue
		}

		org := orgEntry.Name()

		modelEntries, err := os.ReadDir(fmt.Sprintf("%s/%s", m.modelsPath, org))
		if err != nil {
			continue
		}

		for _, modelEntry := range modelEntries {
			if !modelEntry.IsDir() {
				continue
			}

			modelFamily := modelEntry.Name()

			fileEntries, err := os.ReadDir(fmt.Sprintf("%s/%s/%s", m.modelsPath, org, modelFamily))
			if err != nil {
				continue
			}

			modelfiles := make(map[string][]string)
			projFiles := make(map[string]string)

			for _, fileEntry := range fileEntries {
				if fileEntry.IsDir() {
					continue
				}

				name := fileEntry.Name()

				if name == ".DS_Store" {
					continue
				}

				if strings.HasPrefix(name, "mmproj") {
					modelID := extractModelID(name[7:])
					projFiles[modelID] = filepath.Join(m.modelsPath, org, modelFamily, fileEntry.Name())
					continue
				}

				modelID := extractModelID(fileEntry.Name())
				filePath := filepath.Join(m.modelsPath, org, modelFamily, fileEntry.Name())
				modelfiles[modelID] = append(modelfiles[modelID], filePath)
			}

			ctx := context.Background()

			for modelID, files := range modelfiles {
				prev := currentIndex[modelID]
				isValidated := prev.Validated

				slices.Sort(files)

				// Collect current file sizes and invalidate validation
				// if any size changed (e.g. file was copied/replaced).
				sizes := make([]int64, len(files))
				for i, file := range files {
					if info, err := os.Stat(file); err == nil {
						sizes[i] = info.Size()
					}
				}

				if isValidated && !fileSizesMatch(prev.FileSizes, sizes) {
					isValidated = false
				}

				mp := Path{
					ModelFiles: files,
					Downloaded: true,
					FileSizes:  sizes,
				}

				if projFile, exists := projFiles[modelID]; exists {
					mp.ProjFile = projFile
				}

				validated := isValidated
				if checkSHA {
					validated = true

					for _, file := range files {
						log(ctx, "running check ", "model", path.Base(file))
						if err := model.CheckModel(file, true); err != nil {
							log(ctx, "running check ", "model", path.Base(file), "ERROR", err)
							validated = false
						}
					}

					if mp.ProjFile != "" {
						log(ctx, "running check ", "proj", path.Base(mp.ProjFile))
						if err := model.CheckModel(mp.ProjFile, true); err != nil {
							log(ctx, "running check ", "proj", path.Base(mp.ProjFile), "ERROR", err)
							validated = false
						}
					}
				}

				mp.Validated = validated
				mp.TokenizerFingerprint = tokenizerFingerprintFromFile(files[0])

				index[modelID] = mp
			}
		}
	}

	indexData, err := yaml.Marshal(&index)
	if err != nil {
		return fmt.Errorf("marshal index: %w", err)
	}

	indexPath := filepath.Join(m.modelsPath, indexFile)
	if err := os.WriteFile(indexPath, indexData, 0644); err != nil {
		return fmt.Errorf("write index file: %w", err)
	}

	return nil
}

// MarkValidated sets Validated=true for the specified model in the index,
// without performing a full rebuild. The download flow already verifies SHAs
// for each file via model.CheckModel, so the entry can be trusted as soon as
// the index has been (re)built. Returns an error if the model is not present
// in the index or the index cannot be written.
func (m *Models) MarkValidated(modelID string) error {
	m.biMutex.Lock()
	defer m.biMutex.Unlock()

	indexPath := filepath.Join(m.modelsPath, indexFile)

	data, err := os.ReadFile(indexPath)
	if err != nil {
		return fmt.Errorf("mark-validated: read index: %w", err)
	}

	var index map[string]Path
	if err := yaml.Unmarshal(data, &index); err != nil {
		return fmt.Errorf("mark-validated: unmarshal index: %w", err)
	}

	mp, exists := index[modelID]
	if !exists {
		return fmt.Errorf("mark-validated: model %q not found in index", modelID)
	}

	if mp.Validated {
		return nil
	}

	mp.Validated = true
	index[modelID] = mp

	out, err := yaml.Marshal(&index)
	if err != nil {
		return fmt.Errorf("mark-validated: marshal index: %w", err)
	}

	if err := os.WriteFile(indexPath, out, 0644); err != nil {
		return fmt.Errorf("mark-validated: write index: %w", err)
	}

	return nil
}

// =============================================================================

func (m *Models) removeEmptyDirs() error {
	var dirs []string

	err := filepath.WalkDir(m.modelsPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() && path != m.modelsPath {
			dirs = append(dirs, path)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("walking directory tree: %w", err)
	}

	for i := len(dirs) - 1; i >= 0; i-- {
		entries, err := os.ReadDir(dirs[i])
		if err != nil {
			continue
		}

		if isDirEffectivelyEmpty(entries) {
			// Remove any .DS_Store before removing directory
			dsStore := filepath.Join(dirs[i], ".DS_Store")
			os.Remove(dsStore)
			os.Remove(dirs[i])
		}
	}

	return nil
}

// fileSizesMatch returns true if both slices have the same length and values.
// An empty previous slice (legacy index) is treated as a mismatch to force
// re-validation on first run after upgrade.
func fileSizesMatch(prev, cur []int64) bool {
	if len(prev) == 0 {
		return false
	}

	if len(prev) != len(cur) {
		return false
	}

	for i := range prev {
		if prev[i] != cur[i] {
			return false
		}
	}

	return true
}

// isDirEffectivelyEmpty returns true if directory only contains ignorable files like .DS_Store
func isDirEffectivelyEmpty(entries []os.DirEntry) bool {
	for _, e := range entries {
		if e.Name() != ".DS_Store" {
			return false
		}
	}

	return true
}

// NormalizeHuggingFaceDownloadURL converts short format to full HuggingFace download URLs.
// Input:  mradermacher/Qwen2-Audio-7B-GGUF/Qwen2-Audio-7B.Q8_0.gguf
// Output: https://huggingface.co/mradermacher/Qwen2-Audio-7B-GGUF/resolve/main/Qwen2-Audio-7B.Q8_0.gguf
func NormalizeHuggingFaceDownloadURL(rawURL string) string {
	if strings.HasPrefix(rawURL, "https://") || strings.HasPrefix(rawURL, "http://") {
		return strings.Replace(rawURL, "/blob/", "/resolve/", 1)
	}

	rawURL = stripHFHostPrefix(rawURL)

	parts := strings.Split(rawURL, "/")
	if len(parts) >= 3 {
		org := parts[0]
		repo := parts[1]
		filename := strings.Join(parts[2:], "/")
		return fmt.Sprintf("https://huggingface.co/%s/%s/resolve/main/%s", org, repo, filename)
	}

	return rawURL
}

// NormalizeHuggingFaceURL converts short format URLs to full HuggingFace URLs.
// Input:  unsloth/Llama-3.3-70B-Instruct-GGUF
// Output: https://huggingface.co/unsloth/Llama-3.3-70B-Instruct-GGUF
//
// Input:  mradermacher/Qwen2-Audio-7B-GGUF/Qwen2-Audio-7B.Q8_0.gguf
// Output: https://huggingface.co/mradermacher/Qwen2-Audio-7B-GGUF/blob/main/Qwen2-Audio-7B.Q8_0.gguf
//
// Input:  unsloth/Llama-3.3-70B-Instruct-GGUF/Llama-3.3-70B-Instruct-Q8_0/Llama-3.3-70B-Instruct-Q8_0-00001-of-00002.gguf
// Output: https://huggingface.co/unsloth/Llama-3.3-70B-Instruct-GGUF/blob/main/Llama-3.3-70B-Instruct-Q8_0/Llama-3.3-70B-Instruct-Q8_0-00001-of-00002.gguf
func NormalizeHuggingFaceURL(rawURL string) string {
	if strings.HasPrefix(rawURL, "https://") || strings.HasPrefix(rawURL, "http://") {
		return rawURL
	}

	rawURL = stripHFHostPrefix(rawURL)

	parts := strings.Split(rawURL, "/")
	if len(parts) >= 3 {
		org := parts[0]
		repo := parts[1]
		filename := strings.Join(parts[2:], "/")
		return fmt.Sprintf("https://huggingface.co/%s/%s/blob/main/%s", org, repo, filename)
	}

	if len(parts) == 2 {
		return fmt.Sprintf("https://huggingface.co/%s", rawURL)
	}

	return rawURL
}

// stripHFHostPrefix removes bare host prefixes (without scheme) from URLs.
func stripHFHostPrefix(s string) string {
	lower := strings.ToLower(s)
	for _, prefix := range []string{"huggingface.co/", "hf.co/"} {
		if strings.HasPrefix(lower, prefix) {
			return s[len(prefix):]
		}
	}
	return s
}
