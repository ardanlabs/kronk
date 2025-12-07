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
func DownloadModel(ctx context.Context, log Logger, modelURL string, projURL string, modelPath string) (ModelPath, error) {
	u, _ := url.Parse(modelURL)
	filename := path.Base(u.Path)
	name := strings.TrimSuffix(filename, path.Ext(filename))

	log(ctx, fmt.Sprintf("download-model: model-path[%s] model-url[%s] proj-url[%s] model-name[%s]", modelPath, modelURL, projURL, name))
	log(ctx, "download-model: waiting to start download...")

	progress := func(src string, currentSize int64, totalSize int64, mibPerSec float64, complete bool) {
		log(ctx, fmt.Sprintf("\x1b[1A\r\x1b[Kdownload-model: Downloading %s... %d MiB of %d MiB (%.2f MiB/s)", src, currentSize/(1024*1024), totalSize/(1024*1024), mibPerSec))
	}

	info, err := downloadModel(ctx, modelURL, projURL, modelPath, progress)
	if err != nil {
		return ModelPath{}, fmt.Errorf("unable to download model: %w", err)
	}

	switch info.Downloaded {
	case true:
		log(ctx, fmt.Sprintf("download-model: status[downloaded] model-file[%s] proj-file[%s]", info.ModelFile, info.ProjFile))

	default:
		log(ctx, fmt.Sprintf("download-model: status[already exists] model-file[%s] proj-file[%s]", info.ModelFile, info.ProjFile))
	}

	return info, nil
}

func downloadModel(ctx context.Context, modelURL string, projURL string, modelPath string, progress ProgressFunc) (ModelPath, error) {
	modelFile, downloadedMF, err := pullModel(ctx, modelURL, modelPath, progress)
	if err != nil {
		return ModelPath{}, err
	}

	if projURL == "" {
		return ModelPath{ModelFile: modelFile, Downloaded: downloadedMF}, nil
	}

	modelFileName := filepath.Base(modelFile)
	profFileName := fmt.Sprintf("mmproj-%s", modelFileName)
	newProjFile := strings.Replace(modelFile, modelFileName, profFileName, 1)

	if _, err := os.Stat(newProjFile); err == nil {
		inf := ModelPath{
			ModelFile:  modelFile,
			ProjFile:   newProjFile,
			Downloaded: downloadedMF || false,
		}

		return inf, nil
	}

	projFile, downloadedPF, err := pullModel(ctx, projURL, modelPath, progress)
	if err != nil {
		return ModelPath{}, err
	}

	if err := os.Rename(projFile, newProjFile); err != nil {
		return ModelPath{}, fmt.Errorf("unable to rename projector file: %w", err)
	}

	inf := ModelPath{
		ModelFile:  modelFile,
		ProjFile:   newProjFile,
		Downloaded: downloadedMF && downloadedPF,
	}

	return inf, nil
}

func pullModel(ctx context.Context, fileURL string, filePath string, progress ProgressFunc) (string, bool, error) {
	mURL, err := url.Parse(fileURL)
	if err != nil {
		return "", false, fmt.Errorf("unable to parse fileURL: %w", err)
	}

	parts := strings.Split(mURL.Path, "/")
	if len(parts) < 3 {
		return "", false, fmt.Errorf("invalid huggingface url: %q", mURL.Path)
	}

	filePath = filepath.Join(filePath, parts[1], parts[2])
	mFile := filepath.Join(filePath, path.Base(mURL.Path))

	downloaded, err := DownloadFile(ctx, fileURL, filePath, progress, SizeIntervalMIB100)
	if err != nil {
		return "", false, fmt.Errorf("unable to download model: %w", err)
	}

	return mFile, downloaded, nil
}
