package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hybridgroup/yzma/pkg/download"
)

const versionFile = "version.json"

type tag struct {
	TagName string `json:"tag_name"`
}

// LibVersion provides information about what is installed and what is the
// latest version of llama.cpp available.
type LibVersion struct {
	Latest  string
	Current string
}

// =============================================================================

// DownloadLibraries performs a complete workflow for downloading and installing
// the latest version of llama.cpp.
func DownloadLibraries(ctx context.Context, log Logger, libPath string, processor download.Processor, allowUpgrade bool) (LibVersion, error) {
	orgVI, err := VersionInformation(libPath)
	if err != nil {
		return LibVersion{}, fmt.Errorf("error retrieving version info: %w", err)
	}

	log(ctx, "download-libs", "status", "check llama.cpp installation", "lib-path", libPath, "processor", processor, "latest", orgVI.Latest, "current", orgVI.Current)

	if orgVI.Current == orgVI.Latest {
		log(ctx, "download-libs", "status", "current already installed", "latest", orgVI.Latest, "current", orgVI.Current)
		return orgVI, nil
	}

	log(ctx, "download-libs", "status", "llama.cpp installation", "lib-path", libPath, "processor", processor)

	vi, err := downloadLibraries(libPath, processor, allowUpgrade)
	if err != nil {
		log(ctx, "download-libs", "status", "llama.cpp installation", "ERROR", err)

		if _, err := InstalledVersion(libPath); err != nil {
			return LibVersion{}, fmt.Errorf("failed to install llama: %q: error: %w", libPath, err)
		}

		log(ctx, "download-libs", "status", "failed to install new version, using current version")
	}

	log(ctx, "download-libs", "status", "updated llama.cpp installation", "lib-path", "old-version", orgVI.Current, "current", vi.Current)

	return vi, nil
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
func VersionInformation(libPath string) (LibVersion, error) {
	cv, _ := InstalledVersion(libPath)

	version, err := download.LlamaLatestVersion()
	if err != nil {
		return LibVersion{Latest: "unknown", Current: cv}, fmt.Errorf("unable to get latest version of llama.cpp: %w", err)
	}

	return LibVersion{Latest: version, Current: cv}, nil
}

// =============================================================================

func downloadLibraries(libPath string, processor download.Processor, allowUpgrade bool) (LibVersion, error) {
	cv, _ := InstalledVersion(libPath)
	tempPath := filepath.Join(libPath, "temp")

	if err := download.InstallLibraries(tempPath, processor, allowUpgrade); err != nil {
		os.RemoveAll(tempPath)
		return LibVersion{Latest: "unknown", Current: cv}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	if err := swapTempForLib(libPath, tempPath); err != nil {
		os.RemoveAll(tempPath)
		return LibVersion{Latest: "unknown", Current: cv}, fmt.Errorf("unable to swap temp for lib: %w", err)
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
