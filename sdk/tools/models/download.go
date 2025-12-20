package models

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/tools/downloader"
)

var indexFile = "index.yaml"

// Download performs a complete workflow for downloading and installing
// the specified model.
func Download(ctx context.Context, log kronk.Logger, modelFileURL string, projURL string, modelBasePath string) (Path, error) {
	defer func() {
		if err := BuildIndex(modelBasePath); err != nil {
			log(ctx, "download-model: unable to create index", "ERROR", err)
		}
	}()

	modelFileName, err := extractFileName(modelFileURL)
	if err != nil {
		return Path{}, fmt.Errorf("download-model: unable to extract file name: %w", err)
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

		if mp, err := RetrievePath(modelBasePath, modelID); err == nil {
			size, err := fileSize(mp.ModelFile)
			if err != nil {
				return Path{}, fmt.Errorf("download-model: unable to check file size of model: %w", err)
			}

			if size == 0 {
				os.Remove(mp.ModelFile)
				return Path{}, fmt.Errorf("download-model: unable to download file: %w", errOrg)
			}

			log(ctx, "download-model: status[using installed version of model]")
			return mp, nil
		}

		return Path{}, fmt.Errorf("download-model: unable to download model: %w", err)
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

func downloadModel(ctx context.Context, modelFileURL string, projFileURL string, modelBasePath string, progress downloader.ProgressFunc) (Path, error) {
	modelFileName, downloadedMF, err := pullModel(ctx, modelFileURL, modelBasePath, progress)
	if err != nil {
		return Path{}, err
	}

	if projFileURL == "" {
		return Path{ModelFile: modelFileName, Downloaded: downloadedMF}, nil
	}

	projFileName := createProjFileName(modelFileName)

	if _, err := os.Stat(projFileName); err == nil {
		inf := Path{
			ModelFile:  modelFileName,
			ProjFile:   projFileName,
			Downloaded: downloadedMF,
		}

		return inf, nil
	}

	orjProjFile, downloadedPF, err := pullModel(ctx, projFileURL, modelBasePath, progress)
	if err != nil {
		return Path{}, err
	}

	if err := os.Rename(orjProjFile, projFileName); err != nil {
		return Path{}, fmt.Errorf("download-model: unable to rename projector file: %w", err)
	}

	inf := Path{
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

func pullModel(ctx context.Context, modelFileURL string, modelBasePath string, progress downloader.ProgressFunc) (string, bool, error) {
	modelFilePath, modelFileName, err := modelFilePathAndName(modelFileURL, modelBasePath)
	if err != nil {
		return "", false, fmt.Errorf("pull-model: unable to extract file-path: %w", err)
	}

	downloaded, err := downloader.Download(ctx, modelFileURL, modelFilePath, progress, downloader.SizeIntervalMIB100)
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
