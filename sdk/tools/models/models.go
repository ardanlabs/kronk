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
		modelsPath: modelPath,
	}

	return &m, nil
}

// Path returns the location of the models path.
func (m *Models) Path() string {
	return m.modelsPath
}

// BuildIndex builds the model index for fast model access.
func (m *Models) BuildIndex(log Logger) error {
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
				isValidated := currentIndex[modelID].Validated

				log(ctx, "checking model", "modelID", modelID, "isValidated", isValidated)

				slices.Sort(files)

				mp := Path{
					ModelFiles: files,
					Downloaded: true,
				}

				if projFile, exists := projFiles[modelID]; exists {
					mp.ProjFile = projFile
				}

				validated := true
				if !isValidated {
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
func NormalizeHuggingFaceDownloadURL(url string) string {
	if strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://") {
		return url
	}

	parts := strings.Split(url, "/")
	if len(parts) >= 3 {
		org := parts[0]
		repo := parts[1]
		filename := strings.Join(parts[2:], "/")
		return fmt.Sprintf("https://huggingface.co/%s/%s/resolve/main/%s", org, repo, filename)
	}

	return url
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
func NormalizeHuggingFaceURL(url string) string {
	if strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://") {
		return url
	}

	parts := strings.Split(url, "/")
	if len(parts) >= 3 {
		org := parts[0]
		repo := parts[1]
		filename := strings.Join(parts[2:], "/")
		return fmt.Sprintf("https://huggingface.co/%s/%s/blob/main/%s", org, repo, filename)
	}

	if len(parts) == 2 {
		return fmt.Sprintf("https://huggingface.co/%s", url)
	}

	return url
}
