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
	return m.BuildIndexWithPath(log, Path{})
}

// BuildIndexWithPath builds the model index for fast model access. If a
// non-empty downloadedPath is provided, its model-projector association is
// preserved in the index regardless of filename patterns.
func (m *Models) BuildIndexWithPath(log Logger, downloadedPath Path) error {
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

	// Build a map of known model-projector associations from the downloaded path.
	knownProjFiles := make(map[string]string)
	if len(downloadedPath.ModelFiles) > 0 && downloadedPath.ProjFile != "" {
		modelID := extractModelID(downloadedPath.ModelFiles[0])
		knownProjFiles[modelID] = downloadedPath.ProjFile
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
			availableProjs := make(map[string]string) // modelID from projector name → projector path

			for _, fileEntry := range fileEntries {
				if fileEntry.IsDir() {
					continue
				}

				name := fileEntry.Name()

				if name == ".DS_Store" {
					continue
				}

				// Collect projector files for fallback matching.
				// Prefix format: mmproj-ModelName-Q8_0.gguf
				if strings.HasPrefix(name, "mmproj") {
					projModelID := extractModelID(name[7:]) // strip "mmproj-" prefix
					availableProjs[projModelID] = filepath.Join(m.modelsPath, org, modelFamily, name)
					continue
				}
				// Infix format: ModelName.mmproj-Q8_0.gguf
				if idx := strings.Index(name, ".mmproj"); idx > 0 {
					baseName := name[:idx]                                 // "ModelName"
					quantPart := strings.TrimPrefix(name[idx:], ".mmproj") // "-Q8_0.gguf"
					projModelID := extractModelID(baseName + quantPart)    // "ModelName-Q8_0"
					availableProjs[projModelID] = filepath.Join(m.modelsPath, org, modelFamily, name)
					continue
				}

				modelID := extractModelID(fileEntry.Name())
				filePath := filepath.Join(m.modelsPath, org, modelFamily, fileEntry.Name())
				modelfiles[modelID] = append(modelfiles[modelID], filePath)
			}

			ctx := context.Background()

			// Apply projector associations with priority:
			//
			// 1. New download (knownProjFiles)
			// 2. Previous index (if file still exists)
			// 3. Fallback: filename pattern matching
			for modelID := range modelfiles {
				projFromDownload, hasDownload := knownProjFiles[modelID]
				projFromIndex := currentIndex[modelID].ProjFile
				projFromFilename, hasFilename := availableProjs[modelID]

				switch {
				case hasDownload:
					projFiles[modelID] = projFromDownload
					log(ctx, "proj association", "modelID", modelID, "source", "download", "projFile", filepath.Base(projFromDownload))

				case projFromIndex != "":
					if _, err := os.Stat(projFromIndex); err == nil {
						projFiles[modelID] = projFromIndex
						log(ctx, "proj association", "modelID", modelID, "source", "index", "projFile", filepath.Base(projFromIndex))
					} else {
						log(ctx, "proj association", "modelID", modelID, "source", "index", "ERROR", "projector file missing", "projFile", projFromIndex)
					}

				case hasFilename:
					projFiles[modelID] = projFromFilename
					log(ctx, "proj association", "modelID", modelID, "source", "filename", "projFile", filepath.Base(projFromFilename))
				}
			}

			validated := true
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
