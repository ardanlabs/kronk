package models

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/downloader"
)

// Logger represents a logger for capturing events.
type Logger func(ctx context.Context, msg string, args ...any)

// Download performs a complete workflow for downloading and installing
// the specified model. If you need to set your HuggingFace token, use the
// environment variable KRONK_HF_TOKEN.
func (m *Models) Download(ctx context.Context, log Logger, modelURL string, projURL string) (Path, error) {
	return m.DownloadShards(ctx, log, []string{modelURL}, projURL)
}

// DownloadShards performs a complete workflow for downloading and installing
// the specified model. If you need to set your HuggingFace token, use the
// environment variable KRONK_HF_TOKEN.
func (m *Models) DownloadShards(ctx context.Context, log Logger, modelURLs []string, projURL string) (Path, error) {
	modelFileName, err := extractFileName(modelURLs[0])
	if err != nil {
		return Path{}, fmt.Errorf("download-model: unable to extract file name: %w", err)
	}

	modelID := extractModelID(modelFileName)

	if !hasNetwork() {
		mp, err := m.RetrievePath(modelID)
		if err != nil {
			return Path{}, fmt.Errorf("download-model: no network available: %w", err)
		}

		return mp, nil
	}

	var downloaded bool
	defer func() {
		if downloaded {
			if err := m.BuildIndex(log); err != nil {
				log(ctx, "download-model: unable to create index", "ERROR", err)
			}
		}
	}()

	result := Path{
		ModelFiles: make([]string, len(modelURLs)),
	}

	for i, modelURL := range modelURLs {
		if i > 0 {
			projURL = ""
		}

		log(ctx, fmt.Sprintf("download-model: model-url[%s] proj-url[%s] model-id[%s]", modelURL, projURL, modelID))
		log(ctx, "download-model: waiting to check model status...")

		progress := func(src string, currentSize int64, totalSize int64, mibPerSec float64, complete bool) {
			log(ctx, fmt.Sprintf("\x1b[1A\r\x1b[Kdownload-model: Downloading %s... %d MiB of %d MiB (%.2f MiB/s)", src, currentSize/(1024*1024), totalSize/(1024*1024), mibPerSec))
		}

		mp, errOrg := m.downloadModel(ctx, modelURL, projURL, progress)
		if errOrg != nil {
			log(ctx, "download-model:", "ERROR", errOrg, "model-file-url", modelURL)

			if mp, err := m.RetrievePath(modelID); err == nil && len(mp.ModelFiles) > 0 {
				size, err := fileSize(mp.ModelFiles[0])
				if err != nil {
					return Path{}, fmt.Errorf("download-model: unable to check file size of model: %w", err)
				}

				if size == 0 {
					for _, f := range mp.ModelFiles {
						os.Remove(f)
					}
					return Path{}, fmt.Errorf("download-model: unable to download file: %w", errOrg)
				}

				log(ctx, "download-model: status[using installed version of model files]")
				return mp, nil
			}

			return Path{}, fmt.Errorf("download-model: unable to download model file: %w", errOrg)
		}

		downloaded = mp.Downloaded

		switch mp.Downloaded {
		case true:
			log(ctx, "download-model: status[downloaded]")

		default:
			log(ctx, "download-model: status[already exists]")
		}

		result.ModelFiles[i] = mp.ModelFiles[0]
		if i == 0 {
			result.ProjFile = mp.ProjFile
		}
	}

	result.Downloaded = true

	return result, nil
}

// =============================================================================

func (m *Models) downloadModel(ctx context.Context, modelFileURL string, projFileURL string, progress downloader.ProgressFunc) (Path, error) {
	// Validate the URL is the correct HF download URL.
	if !strings.Contains(modelFileURL, "/resolve/") {
		return Path{}, fmt.Errorf("invalid model download url, missing /resolve/: %s", modelFileURL)
	}

	// If we have a proj file, then check that URL as well.
	if projFileURL != "" {
		if !strings.Contains(projFileURL, "/resolve/") {
			return Path{}, fmt.Errorf("invalid proj download url, missing /resolve/: %s", projFileURL)
		}
	}

	// Check the index to see if this model has already been downloaded and
	// is validated.

	modelFileName, err := extractFileName(modelFileURL)
	if err != nil {
		return Path{}, fmt.Errorf("unable to extract file name: %w", err)
	}

	mp, found := m.loadIndex()[strings.ToLower(extractModelID(modelFileName))]
	if found && mp.Validated {
		mp.Downloaded = false
		return mp, nil
	}

	// -------------------------------------------------------------------------

	// Download the model sha file.
	if _, err := m.pullShaFile(modelFileURL, progress); err != nil {
		return Path{}, fmt.Errorf("pull-model: unable to download sha file: %w", err)
	}

	// Download the model file.
	modelFileName, downloadedMF, err := m.pullFile(ctx, modelFileURL, progress)
	if err != nil {
		return Path{}, err
	}

	// Check the model file matches what is in the sha file.
	if err := model.CheckModel(modelFileName, true); err != nil {
		return Path{}, fmt.Errorf("check-model: %w", err)
	}

	// If there is no proj file we are done.
	if projFileURL == "" {
		return Path{ModelFiles: []string{modelFileName}, Downloaded: downloadedMF}, nil
	}

	// -------------------------------------------------------------------------

	// Download the Sha file for the proj model file and rename the sha to
	// match what the proj file will be called.

	projFileName := createProjFileName(modelFileName)

	orgShaFileName, err := m.pullShaFile(projFileURL, progress)
	if err != nil {
		return Path{}, fmt.Errorf("pull-model: unable to download sha file: %w", err)
	}

	// projFileName:   /Users/bill/.kronk/models/Qwen/Qwen3-8B-GGUF/mmproj-Qwen2-Audio-7B.Q8_0.gguf
	// orgShaFileName: /Users/bill/.kronk/models/Qwen/Qwen3-8B-GGUF/sha/Qwen2-Audio-7B.mmproj-Q8_0.gguf
	// shaFileName:    /Users/bill/.kronk/models/Qwen/Qwen3-8B-GGUF/sha/mmproj-Qwen2-Audio-7B.Q8_0.gguf
	shaFileName := filepath.Join(path.Dir(orgShaFileName), path.Base(projFileName))

	if err := os.Rename(orgShaFileName, shaFileName); err != nil {
		return Path{}, fmt.Errorf("download-model: unable to rename projector sha file: %w", err)
	}

	// -------------------------------------------------------------------------

	// Check if the proj file already exists on disk, and if so check the file
	// against the sha file.
	if _, err := os.Stat(projFileName); err == nil {
		if err := model.CheckModel(projFileName, true); err == nil {
			inf := Path{
				ModelFiles: []string{modelFileName},
				ProjFile:   projFileName,
				Downloaded: downloadedMF,
			}

			return inf, nil
		}
	}

	// Download the proj file.
	orjProjFile, downloadedPF, err := m.pullFile(ctx, projFileURL, progress)
	if err != nil {
		return Path{}, err
	}

	// Rename the proj file to match our naming convention.
	if err := os.Rename(orjProjFile, projFileName); err != nil {
		return Path{}, fmt.Errorf("download-model: unable to rename projector file: %w", err)
	}

	// Check the model file matches what is in the sha file.
	if err := model.CheckModel(projFileName, true); err != nil {
		return Path{}, fmt.Errorf("check-model: %w", err)
	}

	inf := Path{
		ModelFiles: []string{modelFileName},
		ProjFile:   projFileName,
		Downloaded: downloadedMF && downloadedPF,
	}

	return inf, nil
}

func (m *Models) pullShaFile(modelFileURL string, progress downloader.ProgressFunc) (string, error) {
	// modelFileURL: https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf
	// rawFileURL:   https://huggingface.co/Qwen/Qwen3-8B-GGUF/raw/main/Qwen3-8B-Q8_0.gguf
	rawFileURL := strings.Replace(modelFileURL, "resolve", "raw", 1)

	modelFilePath, modelFileName, err := m.modelFilePathAndName(modelFileURL)
	if err != nil {
		return "", err
	}

	// /Users/bill/.kronk/models/Qwen/Qwen3-8B-GGUF
	// /Users/bill/.kronk/models/Qwen/Qwen3-8B-GGUF/sha
	shaDest := filepath.Join(modelFilePath, "sha")
	shaFile := filepath.Join(shaDest, path.Base(modelFileName))

	if !hasNetwork() {
		return shaFile, nil
	}

	if _, err := downloader.Download(context.Background(), rawFileURL, shaDest, progress, 0); err != nil {
		return "", fmt.Errorf("download-sha: %w", err)
	}

	return shaFile, nil
}

func (m *Models) pullFile(ctx context.Context, fileURL string, progress downloader.ProgressFunc) (string, bool, error) {
	modelFilePath, modelFileName, err := m.modelFilePathAndName(fileURL)
	if err != nil {
		return "", false, fmt.Errorf("pull-model: unable to extract file-path: %w", err)
	}

	downloaded, err := downloader.Download(ctx, fileURL, modelFilePath, progress, downloader.SizeIntervalMIB100)
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

	fileName, err := extractFileName(modelFileURL)
	if err != nil {
		return "", "", fmt.Errorf("model-file-path-and-name: unable to extract file name: %w", err)
	}

	modelFilePath := filepath.Join(m.modelsPath, parts[1], parts[2])
	modelFileName := filepath.Join(modelFilePath, fileName)

	// modelFileURL:  https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf
	// parts:         huggingface.co, Qwen, Qwen3-8B-GGUF, resolve, main, Qwen3-8B-Q8_0.gguf
	// fileName:      Qwen3-8B-Q8_0.gguf
	// modelFilePath: /Users/bill/.kronk/models/Qwen/Qwen3-8B-GGUF
	// modelFileName: /Users/bill/.kronk/models/Qwen/Qwen3-8B-GGUF/Qwen3-8B-Q8_0.gguf

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
	profFileName := fmt.Sprintf("mmproj-%s%s", modelID, path.Ext(modelFileName))

	dir, _ := path.Split(modelFileName)
	name := path.Join(dir, profFileName)

	// modelFileName: /Users/bill/.kronk/models/Qwen/Qwen3-8B-GGUF/Qwen3-8B-Q8_0.gguf
	// modelID:       Qwen3-8B-Q8_0
	// profFileName:  mmproj-Qwen3-8B-Q8_0.gguf
	// dir:           /Users/bill/.kronk/models/Qwen/Qwen3-8B-GGUF
	// name:          /Users/bill/.kronk/models/Qwen/Qwen3-8B-GGUF/mmproj-Qwen3-8B-Q8_0.gguf

	return name
}

var shardPattern = regexp.MustCompile(`-\d+-of-\d+$`)

func extractModelID(modelFileName string) string {
	name := strings.TrimSuffix(path.Base(modelFileName), path.Ext(modelFileName))
	name = shardPattern.ReplaceAllString(name, "")

	// modelFileName: /Users/bill/.kronk/models/Qwen/Qwen3-8B-GGUF/Qwen3-8B-Q8_0.gguf
	// name: Qwen3-8B-Q8_0

	// modelFileName: /Users/bill/.kronk/models/unsloth/Llama-3.3-70B-Instruct-GGUF/Llama-3.3-70B-Instruct-Q8_0-00001-of-00002.gguf
	// name: Llama-3.3-70B-Instruct-Q8_0-00001-of-00002
	// name: Llama-3.3-70B-Instruct-Q8_0

	return name
}

func extractFileName(modelFileURL string) (string, error) {
	u, err := url.Parse(modelFileURL)
	if err != nil {
		return "", fmt.Errorf("extract-file-name: parse error: %w", err)
	}

	name := path.Base(u.Path)

	// modelFileURL: https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf
	// name:         Qwen3-8B-Q8_0.gguf

	return name, nil
}

func hasNetwork() bool {
	conn, err := net.DialTimeout("tcp", "8.8.8.8:53", 3*time.Second)
	if err != nil {
		return false
	}

	conn.Close()

	return true
}
