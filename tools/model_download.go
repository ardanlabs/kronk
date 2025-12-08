package tools

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// DownloadModel performs a complete workflow for downloading and installing
// the specified model.
func DownloadModel(ctx context.Context, log Logger, modelFileURL string, projURL string, modelBasePath string) (ModelPath, error) {
	modelFileName, err := extractFileName(modelFileURL)
	if err != nil {
		return ModelPath{}, fmt.Errorf("download-model:unable to extract file name: %w", err)
	}

	modelID := extractModelID(modelFileName)

	log(ctx, fmt.Sprintf("download-model: model-dest[%s] model-url[%s] proj-url[%s] model-id[%s]", modelBasePath, modelFileURL, projURL, modelID))
	log(ctx, "download-model: waiting to check model status...")

	progress := func(src string, currentSize int64, totalSize int64, mibPerSec float64, complete bool) {
		log(ctx, fmt.Sprintf("\x1b[1A\r\x1b[Kdownload-model: Downloading %s... %d MiB of %d MiB (%.2f MiB/s)", src, currentSize/(1024*1024), totalSize/(1024*1024), mibPerSec))
	}

	mp, errOrg := downloadModel(ctx, modelFileURL, projURL, modelBasePath, progress)
	if errOrg != nil {
		log(ctx, "download-model:", "ERROR", errOrg, "model-file-url", modelFileURL)

		if mp, err := FindModel(modelBasePath, modelID); err == nil {
			size, err := fileSize(mp.ModelFile)
			if err != nil {
				return ModelPath{}, fmt.Errorf("download-model:unable to check file size of model: %w", err)
			}

			if size == 0 {
				os.Remove(mp.ModelFile)
				return ModelPath{}, fmt.Errorf("download-model:unable to download file: %w", errOrg)
			}

			log(ctx, fmt.Sprintf("download-model: status[using installed version of model] model-file[%s] proj-file[%s]", mp.ModelFile, mp.ProjFile))
			return mp, nil
		}

		return ModelPath{}, fmt.Errorf("download-model:unable to download model: %w", err)
	}

	switch mp.Downloaded {
	case true:
		log(ctx, fmt.Sprintf("download-model: status[downloaded] model-file[%s] proj-file[%s]", mp.ModelFile, mp.ProjFile))

	default:
		log(ctx, fmt.Sprintf("download-model: status[already exists] model-file[%s] proj-file[%s]", mp.ModelFile, mp.ProjFile))
	}

	return mp, nil
}

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
		return ModelPath{}, fmt.Errorf("download-model:unable to rename projector file: %w", err)
	}

	inf := ModelPath{
		ModelFile:  modelFileName,
		ProjFile:   projFileName,
		Downloaded: downloadedMF && downloadedPF,
	}

	return inf, nil
}

func pullModel(ctx context.Context, modelFileURL string, modelBasePath string, progress ProgressFunc) (string, bool, error) {
	modelFilePath, modelFileName, err := modelFilePathAndName(modelFileURL, modelBasePath)
	if err != nil {
		return "", false, fmt.Errorf("pull-model:unable to extract file-path: %w", err)
	}

	downloaded, err := DownloadFile(ctx, modelFileURL, modelFilePath, progress, SizeIntervalMIB100)
	if err != nil {
		return "", false, fmt.Errorf("pull-model:unable to download model: %w", err)
	}

	return modelFileName, downloaded, nil
}

func modelFilePathAndName(modelFileURL string, modelBasePath string) (string, string, error) {
	mURL, err := url.Parse(modelFileURL)
	if err != nil {
		return "", "", fmt.Errorf("pull-model:unable to parse fileURL: %w", err)
	}

	parts := strings.Split(mURL.Path, "/")
	if len(parts) < 3 {
		return "", "", fmt.Errorf("pull-model:invalid huggingface url: %q", mURL.Path)
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
		return "", fmt.Errorf("extractFileName:parse error: %w", err)
	}

	return path.Base(u.Path), nil
}
