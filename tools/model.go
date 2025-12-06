package tools

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// FindModelInfo returns file information about a model.
type FindModelInfo struct {
	ModelFile string
	ProjFile  string
}

// FindModel locates the physical location on disk and returns the full path.
func FindModel(modelPath string, modelName string) (FindModelInfo, error) {
	entries, err := os.ReadDir(modelPath)
	if err != nil {
		return FindModelInfo{}, fmt.Errorf("reading models directory: %w", err)
	}

	projName := fmt.Sprintf("mmproj-%s", modelName)

	var fi FindModelInfo

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
		return FindModelInfo{}, fmt.Errorf("model %q not found", modelName)
	}

	return fi, nil
}

func MustFindModel(modelPath string, modelName string) FindModelInfo {
	fi, err := FindModel(modelPath, modelName)
	if err != nil {
		panic(err.Error())
	}

	return fi
}

// =============================================================================

// DownloadModelInfo provides information about the models that were downloaded.
type DownloadModelInfo struct {
	ModelFile  string
	ProjFile   string
	Downloaded bool
}

// DownloadModel performs a complete workflow for downloading and installing
// the specified model.
func DownloadModel(ctx context.Context, log Logger, modelURL string, projURL string, modelPath string) (DownloadModelInfo, error) {
	u, _ := url.Parse(modelURL)
	filename := path.Base(u.Path)
	name := strings.TrimSuffix(filename, path.Ext(filename))
	log(ctx, fmt.Sprintf("download-model: model-path[%s] model-url[%s] proj-url[%s] model-name[%s]", modelPath, modelURL, projURL, name))
	log(ctx, "download-model: Waiting to start downloading...")

	f := func(src string, currentSize int64, totalSize int64, mibPerSec float64, complete bool) {
		log(ctx, fmt.Sprintf("\x1b[1A\r\x1b[Kdownload-model: Downloading %s... %d MiB of %d MiB (%.2f MiB/s)", src, currentSize/(1024*1024), totalSize/(1024*1024), mibPerSec))
	}

	info, err := downloadModel(ctx, modelURL, projURL, modelPath, f)
	if err != nil {
		return DownloadModelInfo{}, fmt.Errorf("unable to download model: %w", err)
	}

	switch info.Downloaded {
	case true:
		log(ctx, fmt.Sprintf("download-model: status[downloaded] model-file[%s] proj-file[%s]", info.ModelFile, info.ProjFile))

	default:
		log(ctx, fmt.Sprintf("download-model: status[already existed] model-file[%s] proj-file[%s]", info.ModelFile, info.ProjFile))
	}

	return info, nil
}

func downloadModel(ctx context.Context, modelURL string, projURL string, modelPath string, progress ProgressFunc) (DownloadModelInfo, error) {
	modelFile, downloadedMF, err := pullModel(ctx, modelURL, modelPath, progress)
	if err != nil {
		return DownloadModelInfo{}, err
	}

	if projURL == "" {
		return DownloadModelInfo{ModelFile: modelFile, Downloaded: downloadedMF}, nil
	}

	modelFileName := filepath.Base(modelFile)
	profFileName := fmt.Sprintf("mmproj-%s", modelFileName)
	newProjFile := strings.Replace(modelFile, modelFileName, profFileName, 1)

	if _, err := os.Stat(newProjFile); err == nil {
		inf := DownloadModelInfo{
			ModelFile:  modelFile,
			ProjFile:   newProjFile,
			Downloaded: downloadedMF || false,
		}

		return inf, nil
	}

	projFile, downloadedPF, err := pullModel(ctx, projURL, modelPath, progress)
	if err != nil {
		return DownloadModelInfo{}, err
	}

	if err := os.Rename(projFile, newProjFile); err != nil {
		return DownloadModelInfo{}, fmt.Errorf("unable to rename projector file: %w", err)
	}

	inf := DownloadModelInfo{
		ModelFile:  modelFile,
		ProjFile:   newProjFile,
		Downloaded: downloadedMF || downloadedPF,
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

	// The downloader can check if we have the full file and if it's of the
	// correct size. If we are not given a progress function, we can't check
	// the file size and the existence of the file is all we can do not to
	// start a download.
	if progress == nil {
		if _, err := os.Stat(mFile); err == nil {
			return mFile, false, nil
		}
	}

	downloaded, err := DownloadFile(ctx, fileURL, filePath, progress)
	if err != nil {
		return "", false, fmt.Errorf("unable to download model: %w", err)
	}

	return mFile, downloaded, nil
}

// =============================================================================

// ListModelInfo provides information about a model.
type ListModelInfo struct {
	Organization string
	ModelName    string
	ModelFile    string
	Size         int64
	Modified     time.Time
}

// ListModels lists all the models in the given directory.
func ListModels(modelPath string) ([]ListModelInfo, error) {
	entries, err := os.ReadDir(modelPath)
	if err != nil {
		return nil, fmt.Errorf("reading models directory: %w", err)
	}

	var list []ListModelInfo

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

				info, err := fileEntry.Info()
				if err != nil {
					continue
				}

				list = append(list, ListModelInfo{
					Organization: org,
					ModelName:    model,
					ModelFile:    fileEntry.Name(),
					Size:         info.Size(),
					Modified:     info.ModTime(),
				})
			}
		}
	}

	return list, nil
}
