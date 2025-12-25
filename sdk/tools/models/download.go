package models

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/sdk/tools/downloader"
)

// Logger represents a logger for capturing events.
type Logger func(ctx context.Context, msg string, args ...any)

// Download performs a complete workflow for downloading and installing
// the specified model.
func (m *Models) Download(ctx context.Context, log Logger, modelFileURL string, projURL string) (Path, error) {
	if !hasNetwork() {
		return Path{}, fmt.Errorf("download-model: no network available")
	}

	defer func() {
		if err := m.BuildIndex(); err != nil {
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

	mp, errOrg := m.downloadModel(ctx, modelFileURL, projURL, progress)
	if errOrg != nil {
		log(ctx, "download-model:", "ERROR", errOrg, "model-file-url", modelFileURL)

		if mp, err := m.RetrievePath(modelID); err == nil {
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

func (m *Models) downloadModel(ctx context.Context, modelFileURL string, projFileURL string, progress downloader.ProgressFunc) (Path, error) {
	modelFileName, downloadedMF, err := m.pullModel(ctx, modelFileURL, progress)
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

	orjProjFile, downloadedPF, err := m.pullModel(ctx, projFileURL, progress)
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

func (m *Models) pullModel(ctx context.Context, modelFileURL string, progress downloader.ProgressFunc) (string, bool, error) {
	modelFilePath, modelFileName, err := m.modelFilePathAndName(modelFileURL)
	if err != nil {
		return "", false, fmt.Errorf("pull-model: unable to extract file-path: %w", err)
	}

	downloaded, err := downloader.Download(ctx, modelFileURL, modelFilePath, progress, downloader.SizeIntervalMIB100)
	if err != nil {
		return "", false, fmt.Errorf("pull-model: unable to download model: %w", err)
	}

	return modelFileName, downloaded, nil
}

func (m *Models) modelFilePathAndName(modelFileURL string) (string, string, error) {
	mURL, err := url.Parse(modelFileURL)
	if err != nil {
		return "", "", fmt.Errorf("model-file-path-and-name: unable to parse fileURL: %w", err)
	}

	parts := strings.Split(mURL.Path, "/")
	if len(parts) < 3 {
		return "", "", fmt.Errorf("model-file-path-and-name: invalid huggingface url: %q", mURL.Path)
	}

	modelFilePath := filepath.Join(m.modelsPath, parts[1], parts[2])
	modelFileName := filepath.Join(modelFilePath, path.Base(mURL.Path))

	return modelFilePath, modelFileName, nil
}

// =============================================================================

func fileSize(filePath string) (int, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return 0, err
	}

	return int(info.Size()), nil
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

func hasNetwork() bool {
	conn, err := net.DialTimeout("tcp", "8.8.8.8:53", 3*time.Second)
	if err != nil {
		return false
	}

	conn.Close()

	return true
}
