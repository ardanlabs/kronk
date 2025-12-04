// Package install provides functions for installing and upgrading llama.cpp.
package install

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/hybridgroup/yzma/pkg/download"
)

const versionFile = "version.json"

type tag struct {
	TagName string `json:"tag_name"`
}

// Version provides information about what is installed and what is the
// latest version of llama.cpp available.
type Version struct {
	Latest  string
	Current string
}

// InstalledVersion retrieves the current version of llama.cpp installed.
func InstalledVersion(libPath string) (string, error) {
	versionInfoPath := filepath.Join(libPath, versionFile)

	d, err := os.ReadFile(versionInfoPath)
	if err != nil {
		return "unknown", fmt.Errorf("unable to read version info file: %w", err)
	}

	var tag tag
	if err := json.Unmarshal(d, &tag); err != nil {
		return "unknown", fmt.Errorf("unable to parse version info file: %w", err)
	}

	return tag.TagName, nil
}

// VersionInformation retrieves the current version of llama.cpp that is
// published on GitHub and the current installed version.
func VersionInformation(libPath string) (Version, error) {
	currentVersion, _ := InstalledVersion(libPath)

	version, err := download.LlamaLatestVersion()
	if err != nil {
		return Version{}, fmt.Errorf("unable to get latest version of llama.cpp: %w", err)
	}

	return Version{Latest: version, Current: currentVersion}, nil
}

// Libraries installs or upgrades to the latest version of llama.cpp at the
// specified libPath.
func Libraries(libPath string, processor download.Processor, allowUpgrade bool) (Version, error) {
	tempPath := filepath.Join(libPath, "temp")

	if err := download.InstallLibraries(tempPath, processor, allowUpgrade); err != nil {
		os.RemoveAll(tempPath)
		return Version{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	if err := swapTempForLib(libPath, tempPath); err != nil {
		os.RemoveAll(tempPath)
		return Version{}, fmt.Errorf("unable to swap temp for lib: %w", err)
	}

	return VersionInformation(libPath)
}

func swapTempForLib(libPath string, tempPath string) error {
	entries, err := os.ReadDir(libPath)
	if err != nil {
		return fmt.Errorf("unable to read libPath: %w", err)
	}

	for _, entry := range entries {
		if entry.Name() == "temp" {
			continue
		}

		os.Remove(filepath.Join(libPath, entry.Name()))
	}

	tempEntries, err := os.ReadDir(tempPath)
	if err != nil {
		return fmt.Errorf("unable to read temp: %w", err)
	}

	for _, entry := range tempEntries {
		src := filepath.Join(tempPath, entry.Name())
		dst := filepath.Join(libPath, entry.Name())
		if err := os.Rename(src, dst); err != nil {
			return fmt.Errorf("unable to move %s: %w", entry.Name(), err)
		}
	}

	os.RemoveAll(tempPath)

	return nil
}

// =============================================================================

// FindModel locates the physical location on disk and returns the full path.
func FindModel(modelName string, modelPath string) (string, error) {
	entries, err := os.ReadDir(modelPath)
	if err != nil {
		return "", fmt.Errorf("reading models directory: %w", err)
	}

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
					return filepath.Join(modelPath, org, model, fileEntry.Name()), nil
				}
			}
		}
	}

	return "", fmt.Errorf("model %q not found", modelName)
}

// =============================================================================

// Info provides information about the models that were installed.
type Info struct {
	ModelFile  string
	ProjFile   string
	Downloaded bool
}

// Model installs the model at the specified URL to the specified path with
// progress tracking. The name of the file and a flag that indicates if an
// actual download occurred is returned.
func Model(modelURL string, projURL string, modelPath string, progress ProgressFunc) (Info, error) {
	modelFile, downloadedMF, err := pull(modelURL, modelPath, progress)
	if err != nil {
		return Info{}, err
	}

	if projURL == "" {
		return Info{ModelFile: modelFile, Downloaded: downloadedMF}, nil
	}

	modelFileName := filepath.Base(modelFile)
	profFileName := fmt.Sprintf("mmproj-%s", modelFileName)
	newProjFile := strings.Replace(modelFile, modelFileName, profFileName, 1)

	if _, err := os.Stat(newProjFile); err == nil {
		inf := Info{
			ModelFile:  modelFile,
			ProjFile:   newProjFile,
			Downloaded: downloadedMF || false,
		}

		return inf, nil
	}

	projFile, downloadedPF, err := pull(projURL, modelPath, progress)
	if err != nil {
		return Info{}, err
	}

	if err := os.Rename(projFile, newProjFile); err != nil {
		return Info{}, fmt.Errorf("unable to rename projector file: %w", err)
	}

	inf := Info{
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

	downloaded, err := pullFile(context.Background(), fileURL, filePath, progress)
	if err != nil {
		return "", false, fmt.Errorf("unable to download model: %w", err)
	}

	return mFile, downloaded, nil
}
