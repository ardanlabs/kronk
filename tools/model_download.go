package tools

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ardanlabs/kronk"
	"go.yaml.in/yaml/v2"
)

var indexFile = "index.yaml"

// DownloadModel performs a complete workflow for downloading and installing
// the specified model.
func DownloadModel(ctx context.Context, log kronk.Logger, modelFileURL string, projURL string, modelBasePath string) (ModelPath, error) {
	defer func() {
		if err := buildIndex(modelBasePath); err != nil {
			log(ctx, "download-model: unable to create index", "ERROR", err)
		}
	}()

	modelFileName, err := extractFileName(modelFileURL)
	if err != nil {
		return ModelPath{}, fmt.Errorf("download-model: unable to extract file name: %w", err)
	}

	modelID := extractModelID(modelFileName)

	log(ctx, fmt.Sprintf("download-model: model-url[%s] proj-url[%s] model-id[%s]", modelFileURL, projURL, modelID))
	log(ctx, "download-model: waiting to check model status...")

	progress := func(src string, currentSize int64, totalSize int64, mibPerSec float64, complete bool) {
		log(ctx, fmt.Sprintf("\x1b[1A\r\x1b[Kdownload-model: Downloading %s... %d MiB of %d MiB (%.2f MiB/s)", src, currentSize/(1024*1024), totalSize/(1024*1024), mibPerSec))
	}

	mp, errOrg := downloadModel(ctx, modelFileURL, projURL, modelBasePath, progress)
	if errOrg != nil {
		log(ctx, "download-model:", "ERROR", errOrg, "model-file-url", modelFileURL)

		if mp, err := RetrieveModelPath(modelBasePath, modelID); err == nil {
			size, err := fileSize(mp.ModelFile)
			if err != nil {
				return ModelPath{}, fmt.Errorf("download-model: unable to check file size of model: %w", err)
			}

			if size == 0 {
				os.Remove(mp.ModelFile)
				return ModelPath{}, fmt.Errorf("download-model: unable to download file: %w", errOrg)
			}

			log(ctx, "download-model: status[using installed version of model]")
			return mp, nil
		}

		return ModelPath{}, fmt.Errorf("download-model: unable to download model: %w", err)
	}

	switch mp.Downloaded {
	case true:
		log(ctx, "download-model: status[downloaded]")

	default:
		log(ctx, "download-model: status[already exists]")
	}

	return mp, nil
}

// =============================================================================

func downloadModel(ctx context.Context, modelFileURL string, projFileURL string, modelBasePath string, progress ProgressFunc) (ModelPath, error) {
	modelFileName, downloadedMF, err := pullModel(ctx, modelFileURL, modelBasePath, progress)
	if err != nil {
		return ModelPath{}, err
	}

	if projFileURL == "" {
		return ModelPath{ModelFile: modelFileName, Downloaded: downloadedMF}, nil
	}

	projFileName := createProjFileName(modelFileName)

	if _, err := os.Stat(projFileName); err == nil {
		inf := ModelPath{
			ModelFile:  modelFileName,
			ProjFile:   projFileName,
			Downloaded: downloadedMF,
		}

		return inf, nil
	}

	orjProjFile, downloadedPF, err := pullModel(ctx, projFileURL, modelBasePath, progress)
	if err != nil {
		return ModelPath{}, err
	}

	if err := os.Rename(orjProjFile, projFileName); err != nil {
		return ModelPath{}, fmt.Errorf("download-model: unable to rename projector file: %w", err)
	}

	inf := ModelPath{
		ModelFile:  modelFileName,
		ProjFile:   projFileName,
		Downloaded: downloadedMF && downloadedPF,
	}

	return inf, nil
}

func fileSize(filePath string) (int, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return 0, err
	}

	return int(info.Size()), nil
}

func pullModel(ctx context.Context, modelFileURL string, modelBasePath string, progress ProgressFunc) (string, bool, error) {
	modelFilePath, modelFileName, err := modelFilePathAndName(modelFileURL, modelBasePath)
	if err != nil {
		return "", false, fmt.Errorf("pull-model: unable to extract file-path: %w", err)
	}

	downloaded, err := DownloadFile(ctx, modelFileURL, modelFilePath, progress, SizeIntervalMIB100)
	if err != nil {
		return "", false, fmt.Errorf("pull-model: unable to download model: %w", err)
	}

	return modelFileName, downloaded, nil
}

func modelFilePathAndName(modelFileURL string, modelBasePath string) (string, string, error) {
	mURL, err := url.Parse(modelFileURL)
	if err != nil {
		return "", "", fmt.Errorf("model-file-path-and-name: unable to parse fileURL: %w", err)
	}

	parts := strings.Split(mURL.Path, "/")
	if len(parts) < 3 {
		return "", "", fmt.Errorf("model-file-path-and-name: invalid huggingface url: %q", mURL.Path)
	}

	modelFilePath := filepath.Join(modelBasePath, parts[1], parts[2])
	modelFileName := filepath.Join(modelFilePath, path.Base(mURL.Path))

	return modelFilePath, modelFileName, nil
}

func createProjFileName(modelFileName string) string {
	modelID := extractModelID(modelFileName)
	profFileName := fmt.Sprintf("mmproj-%s", modelID)

	return strings.Replace(modelFileName, modelID, profFileName, 1)
}

func extractModelID(modelFileName string) string {
	return strings.TrimSuffix(path.Base(modelFileName), path.Ext(modelFileName))
}

func extractFileName(modelFileURL string) (string, error) {
	u, err := url.Parse(modelFileURL)
	if err != nil {
		return "", fmt.Errorf("extract-file-name: parse error: %w", err)
	}

	return path.Base(u.Path), nil
}

var biMutex sync.Mutex

func buildIndex(modelBasePath string) error {
	biMutex.Lock()
	defer biMutex.Unlock()

	entries, err := os.ReadDir(modelBasePath)
	if err != nil {
		return fmt.Errorf("list-models: reading models directory: %w", err)
	}

	index := make(map[string]ModelPath)

	for _, orgEntry := range entries {
		if !orgEntry.IsDir() {
			continue
		}

		org := orgEntry.Name()

		modelEntries, err := os.ReadDir(fmt.Sprintf("%s/%s", modelBasePath, org))
		if err != nil {
			continue
		}

		for _, modelEntry := range modelEntries {
			if !modelEntry.IsDir() {
				continue
			}

			modelFamily := modelEntry.Name()

			fileEntries, err := os.ReadDir(fmt.Sprintf("%s/%s/%s", modelBasePath, org, modelFamily))
			if err != nil {
				continue
			}

			modelfiles := make(map[string]string)
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
					projFiles[modelID] = filepath.Join(modelBasePath, org, modelFamily, fileEntry.Name())
					continue
				}

				modelID := extractModelID(fileEntry.Name())
				modelfiles[modelID] = filepath.Join(modelBasePath, org, modelFamily, fileEntry.Name())
			}

			for modelID, modelFile := range modelfiles {
				mp := ModelPath{
					ModelFile:  modelFile,
					Downloaded: true,
				}

				if projFile, exists := projFiles[modelID]; exists {
					mp.ProjFile = projFile
				}

				modelID = strings.ToLower(modelID)
				index[modelID] = mp
			}
		}
	}

	indexData, err := yaml.Marshal(&index)
	if err != nil {
		return fmt.Errorf("marshal index: %w", err)
	}

	indexPath := filepath.Join(modelBasePath, indexFile)
	if err := os.WriteFile(indexPath, indexData, 0644); err != nil {
		return fmt.Errorf("write index file: %w", err)
	}

	return nil
}
