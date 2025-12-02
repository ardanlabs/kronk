// Package install provides functions for installing and upgrading llama.cpp.
package install

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"

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
		return "", fmt.Errorf("unable to read version info file: %w", err)
	}

	var tag tag
	if err := json.Unmarshal(d, &tag); err != nil {
		return "", fmt.Errorf("unable to parse version info file: %w", err)
	}

	return tag.TagName, nil
}

// VersionInformation retrieves the current version of llama.cpp that is
// published on GitHub and the current installed version.
func VersionInformation(libPath string) (Version, error) {
	currentVersion, err := InstalledVersion(libPath)
	if err != nil {
		return Version{}, fmt.Errorf("unable to get current version of llama.cpp: %w", err)
	}

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

// Model installs the model at the specified URL to the specified path.
func Model(modelURL string, modelPath string) (string, error) {
	u, err := url.Parse(modelURL)
	if err != nil {
		return "", fmt.Errorf("unable to parse modelURL: %w", err)
	}

	file := filepath.Join(modelPath, path.Base(u.Path))

	if _, err := os.Stat(file); err == nil {
		return file, nil
	}

	if err := download.GetModel(modelURL, modelPath); err != nil {
		return "", fmt.Errorf("unable to download model: %w", err)
	}

	return file, nil
}
