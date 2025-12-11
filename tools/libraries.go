package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ardanlabs/kronk"
	"github.com/ardanlabs/kronk/defaults"
	"github.com/hybridgroup/yzma/pkg/download"
)

const versionFile = "version.json"

// VersionTag represents information about the installed version of llama.cpp.
type VersionTag struct {
	Version   string `json:"version"`
	Arch      string `json:"arch"`
	OS        string `json:"os"`
	Processor string `json:"processor"`
	Latest    string `json:"-"`
}

func isTagMatch(tag VersionTag, cfg LibConfig) bool {
	return tag.Latest == tag.Version && tag.Arch == cfg.Arch.String() && tag.OS == cfg.OS.String() && tag.Processor == cfg.Processor.String()
}

// =============================================================================

// LibConfig contains all the required parameters to download llama.cpp.
type LibConfig struct {
	LibPath      string
	Arch         download.Arch
	OS           download.OS
	Processor    download.Processor
	LlamaLog     kronk.LogLevel
	AllowUpgrade bool
}

// NewLibConfig constructs a valid library config for downloading based on raw
// values that would come from configuration. It sets defaults for the specified
// values when the parameters are empty.
// libPath     : represents the path the llama.cpp libraries will/are installed in.
// archStr     : string representation of a `download.Arch`.
// osStr       : string representation of a `download.OS`.
// procStr     : string representation of a `download.Processor`.
// llamaLog    : int representation of `kronk.LogSilent` or `kronk.LogNormal`.
// allowUpgrade: true or false to determine to upgrade libraries when available.
func NewLibConfig(libPath string, archStr string, osStr string, procStr string, llamaLog int, allowUpgrade bool) (LibConfig, error) {
	arch, err := defaults.Arch(archStr)
	if err != nil {
		return LibConfig{}, err
	}

	opSys, err := defaults.OS(osStr)
	if err != nil {
		return LibConfig{}, err
	}

	processor, err := defaults.Processor(procStr)
	if err != nil {
		return LibConfig{}, err
	}

	log, err := defaults.LlamaLog(llamaLog)
	if err != nil {
		return LibConfig{}, err
	}

	libPath = defaults.LibsDir(libPath)

	cfg := LibConfig{
		LibPath:      libPath,
		Arch:         arch,
		OS:           opSys,
		Processor:    processor,
		LlamaLog:     log,
		AllowUpgrade: allowUpgrade,
	}

	return cfg, nil
}

// DownloadLibraries performs a complete workflow for downloading and installing
// the latest version of llama.cpp.
func DownloadLibraries(ctx context.Context, log kronk.Logger, libCfg LibConfig) (VersionTag, error) {
	log(ctx, "download-libraries:", "status", "check libraries version information", "arch", libCfg.Arch, "os", libCfg.OS, "processor", libCfg.Processor)

	tag, err := VersionInformation(libCfg.LibPath)
	if err != nil {
		if tag.Version == "" {
			return VersionTag{}, fmt.Errorf("download-libraries:error retrieving version info: %w", err)
		}

		log(ctx, "download-libraries:", "status", "unable to check latest verion, using installed version", "arch", libCfg.Arch, "os", libCfg.OS, "processor", libCfg.Processor, "latest", tag.Latest, "current", tag.Version)
		return tag, nil
	}

	log(ctx, "download-libraries:", "status", "check llama.cpp installation", "arch", libCfg.Arch, "os", libCfg.OS, "processor", libCfg.Processor, "latest", tag.Latest, "current", tag.Version)

	if isTagMatch(tag, libCfg) {
		log(ctx, "download-libraries:", "status", "already installed", "latest", tag.Latest, "current", tag.Version)
		return tag, nil
	}

	if !libCfg.AllowUpgrade {
		log(ctx, "download-libraries:", "status", "bypassing upgrade", "latest", tag.Latest, "current", tag.Version)
		return tag, nil
	}

	log(ctx, "download-libraries: waiting to start download...")

	newTag, err := downloadLibs(ctx, log, libCfg, tag.Latest)
	if err != nil {
		log(ctx, "download-libraries:", "status", "llama.cpp installation", "ERROR", err)

		if _, err := InstalledVersion(libCfg.LibPath); err != nil {
			return VersionTag{}, fmt.Errorf("download-libraries:failed to install llama: %q: error: %w", libCfg.LibPath, err)
		}

		log(ctx, "download-libraries:", "status", "failed to install new version, using current version")
	}

	log(ctx, "download-libraries:", "status", "updated llama.cpp installed", "old-version", tag.Version, "current", newTag.Version)

	return newTag, nil
}

// InstalledVersion retrieves the current version of llama.cpp installed.
func InstalledVersion(libPath string) (VersionTag, error) {
	versionInfoPath := filepath.Join(libPath, versionFile)

	d, err := os.ReadFile(versionInfoPath)
	if err != nil {
		return VersionTag{}, fmt.Errorf("installed-version:unable to read version info file: %w", err)
	}

	var tag VersionTag
	if err := json.Unmarshal(d, &tag); err != nil {
		return VersionTag{}, fmt.Errorf("installed-version:unable to parse version info file: %w", err)
	}

	return tag, nil
}

// VersionInformation retrieves the current version of llama.cpp that is
// published on GitHub and the current installed version.
func VersionInformation(libPath string) (VersionTag, error) {
	tag, _ := InstalledVersion(libPath)

	version, err := download.LlamaLatestVersion()
	if err != nil {
		return tag, fmt.Errorf("version-information:unable to get latest version of llama.cpp: %w", err)
	}

	tag.Latest = version

	return tag, nil
}

// =============================================================================

func downloadLibs(ctx context.Context, log kronk.Logger, cfg LibConfig, version string) (VersionTag, error) {
	tempPath := filepath.Join(cfg.LibPath, "temp")

	progress := func(src string, currentSize int64, totalSize int64, mibPerSec float64, complete bool) {
		log(ctx, fmt.Sprintf("\x1b[1A\r\x1b[Kdownload-libraries: Downloading %s... %d MiB of %d MiB (%.2f MiB/s)", src, currentSize/(1024*1024), totalSize/(1024*1024), mibPerSec))
	}

	pr := NewProgressReader(progress, SizeIntervalMIB10)

	err := download.GetWithContext(ctx, cfg.Arch.String(), cfg.OS.String(), cfg.Processor.String(), version, tempPath, pr)
	if err != nil {
		os.RemoveAll(tempPath)
		return VersionTag{}, fmt.Errorf("download-libs:unable to install llama.cpp: %w", err)
	}

	if err := swapTempForLib(cfg.LibPath, tempPath); err != nil {
		os.RemoveAll(tempPath)
		return VersionTag{}, fmt.Errorf("download-libs:unable to swap temp for lib: %w", err)
	}

	if err := createVersionFile(cfg, version); err != nil {
		return VersionTag{}, fmt.Errorf("download-libs:unable to create version file: %w", err)
	}

	return VersionInformation(cfg.LibPath)
}

func swapTempForLib(libPath string, tempPath string) error {
	entries, err := os.ReadDir(libPath)
	if err != nil {
		return fmt.Errorf("swap:unable to read libPath: %w", err)
	}

	for _, entry := range entries {
		if entry.Name() == "temp" {
			continue
		}

		os.Remove(filepath.Join(libPath, entry.Name()))
	}

	tempEntries, err := os.ReadDir(tempPath)
	if err != nil {
		return fmt.Errorf("swap:unable to read temp: %w", err)
	}

	for _, entry := range tempEntries {
		src := filepath.Join(tempPath, entry.Name())
		dst := filepath.Join(libPath, entry.Name())
		if err := os.Rename(src, dst); err != nil {
			return fmt.Errorf("swap:unable to move %s: %w", entry.Name(), err)
		}
	}

	os.RemoveAll(tempPath)

	return nil
}

func createVersionFile(cfg LibConfig, version string) error {
	versionInfoPath := filepath.Join(cfg.LibPath, versionFile)

	f, err := os.Create(versionInfoPath)
	if err != nil {
		return fmt.Errorf("create-version-file:creating version info file: %w", err)
	}
	defer f.Close()

	t := VersionTag{
		Version:   version,
		Arch:      cfg.Arch.String(),
		OS:        cfg.OS.String(),
		Processor: cfg.Processor.String(),
	}

	d, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("create-version-file:marshalling version info: %w", err)
	}

	if _, err := f.Write(d); err != nil {
		return fmt.Errorf("create-version-file:writing version info: %w", err)
	}

	return nil
}
