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

// ModelFileInfo returns file information about a model.
type ModelFileInfo struct {
	ModelFile string
	ProjFile  string
}

// FindModel locates the physical location on disk and returns the full path.
func FindModel(modelPath string, modelName string) (ModelFileInfo, error) {
	entries, err := os.ReadDir(modelPath)
	if err != nil {
		return ModelFileInfo{}, fmt.Errorf("reading models directory: %w", err)
	}

	projName := fmt.Sprintf("mmproj-%s", modelName)

	var fi ModelFileInfo

	for _, orgEntry := range entries {
		if !orgEntry.IsDir() {
			continue
		}

		org := orgEntry.Name()

		modelEntries, err := os.ReadDir(fmt.Sprintf("%s/%s", modelPath, org))
		if err != nil {
			continue
		}

		for _, modelEntry := range modelEntries {
			if !modelEntry.IsDir() {
				continue
			}
			model := modelEntry.Name()

			fileEntries, err := os.ReadDir(fmt.Sprintf("%s/%s/%s", modelPath, org, model))
			if err != nil {
				continue
			}

			for _, fileEntry := range fileEntries {
				if fileEntry.IsDir() {
					continue
				}

				if fileEntry.Name() == ".DS_Store" {
					continue
				}

				if fileEntry.Name() == modelName {
					fi.ModelFile = filepath.Join(modelPath, org, model, fileEntry.Name())
					continue
				}

				if fileEntry.Name() == projName {
					fi.ProjFile = filepath.Join(modelPath, org, model, fileEntry.Name())
					continue
				}
			}
		}
	}

	if fi.ModelFile == "" {
		return ModelFileInfo{}, fmt.Errorf("model %q not found", modelName)
	}

	return fi, nil
}

func MustFindModel(modelPath string, modelName string) ModelFileInfo {
	fi, err := FindModel(modelPath, modelName)
	if err != nil {
		panic(err.Error())
	}

	return fi
}

// =============================================================================

// ModelDownloadInfo provides information about the models that were downloaded.
type ModelDownloadInfo struct {
	ModelFile  string
	ProjFile   string
	Downloaded bool
}

// DownloadModel performs a complete workflow for downloading and installing
// the specified model.
func DownloadModel(ctx context.Context, log Logger, modelURL string, projURL string, modelPath string) (ModelDownloadInfo, error) {
	u, _ := url.Parse(modelURL)
	filename := path.Base(u.Path)
	name := strings.TrimSuffix(filename, path.Ext(filename))
	log(ctx, "download-model", "status", "check model installation", "model-path", modelPath, "model-url", modelURL, "proj-url", projURL, "model-name", name)

	f := func(src string, currentSize int64, totalSize int64, mibPerSec float64, complete bool) {
		log(ctx, fmt.Sprintf("\x1b[1A\r\x1b[KDownloading %s... %d MiB of %d MiB (%.2f MiB/s)", src, currentSize/(1024*1024), totalSize/(1024*1024), mibPerSec))
		if complete {
			log(ctx, "download complete")
		}
	}

	info, err := downloadModel(modelURL, projURL, modelPath, f)
	if err != nil {
		return ModelDownloadInfo{}, fmt.Errorf("unable to download model: %w", err)
	}

	switch info.Downloaded {
	case true:
		log(ctx, "download-model", "status", "model downloaded", "model-file", info.ModelFile, "proj-file", info.ProjFile)

	default:
		log(ctx, "download-model", "status", "model already existed", "model-file", info.ModelFile, "proj-file", info.ProjFile)
	}

	return info, nil
}

// =============================================================================

func downloadModel(modelURL string, projURL string, modelPath string, progress ProgressFunc) (ModelDownloadInfo, error) {
	modelFile, downloadedMF, err := pull(modelURL, modelPath, progress)
	if err != nil {
		return ModelDownloadInfo{}, err
	}

	if projURL == "" {
		return ModelDownloadInfo{ModelFile: modelFile, Downloaded: downloadedMF}, nil
	}

	modelFileName := filepath.Base(modelFile)
	profFileName := fmt.Sprintf("mmproj-%s", modelFileName)
	newProjFile := strings.Replace(modelFile, modelFileName, profFileName, 1)

	if _, err := os.Stat(newProjFile); err == nil {
		inf := ModelDownloadInfo{
			ModelFile:  modelFile,
			ProjFile:   newProjFile,
			Downloaded: downloadedMF || false,
		}

		return inf, nil
	}

	projFile, downloadedPF, err := pull(projURL, modelPath, progress)
	if err != nil {
		return ModelDownloadInfo{}, err
	}

	if err := os.Rename(projFile, newProjFile); err != nil {
		return ModelDownloadInfo{}, fmt.Errorf("unable to rename projector file: %w", err)
	}

	inf := ModelDownloadInfo{
		ModelFile:  modelFile,
		ProjFile:   newProjFile,
		Downloaded: downloadedMF || downloadedPF,
	}

	return inf, nil
}

func pull(fileURL string, filePath string, progress ProgressFunc) (string, bool, error) {
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

	// The downloader can check if we have the full file and if it's of the
	// correct size. If we are not given a progress function, we can't check
	// the file size and the existence of the file is all we can do not to
	// start a download.
	if progress == nil {
		if _, err := os.Stat(mFile); err == nil {
			return mFile, false, nil
		}
	}

	downloaded, err := DownloadFile(context.Background(), fileURL, filePath, progress)
	if err != nil {
		return "", false, fmt.Errorf("unable to download model: %w", err)
	}

	return mFile, downloaded, nil
}
