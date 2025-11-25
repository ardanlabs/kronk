package kronk

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

// VersionInfo provides information about what is installed and what is the
// latest version of llamacpp available.
type VersionInfo struct {
	Latest  string
	Current string
}

// InstallLlama installs or upgrades to the latest version of llamacpp at the
// specified libPath.
func InstallLlama(libPath string, processor download.Processor, allowUpgrade bool) (VersionInfo, error) {
	if err := download.InstallLibraries(libPath, processor, allowUpgrade); err != nil {
		file := filepath.Join(libPath, "libmtmd.dylib")

		if _, err := os.Stat(file); err == nil {
			return VersionInfo{}, nil
		}

		return VersionInfo{}, fmt.Errorf("unable to install llamacpp: %w", err)
	}

	return RetrieveVersionInfo(libPath)
}

// RetrieveVersionInfo retrieves the version information for the llamacpp libs
// and the current version installed.
func RetrieveVersionInfo(libPath string) (VersionInfo, error) {
	versionInfoPath := filepath.Join(libPath, versionFile)

	version, err := download.LlamaLatestVersion()
	if err != nil {
		return VersionInfo{}, fmt.Errorf("unable to get latest version of llamacpp: %w", err)
	}

	d, err := os.ReadFile(versionInfoPath)
	if err != nil {
		return VersionInfo{Current: "unknown", Latest: version}, nil
	}

	var tag tag
	if err := json.Unmarshal(d, &tag); err != nil {
		return VersionInfo{Current: "unknown", Latest: version}, fmt.Errorf("unable to parse version info file: %w", err)
	}

	return VersionInfo{Latest: version, Current: tag.TagName}, nil
}

func InstallModel(modelURL string, modelPath string) (string, error) {
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
