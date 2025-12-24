// Package libs provides llama.cpp library support.
package libs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ardanlabs/kronk/sdk/kronk/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/downloader"
	"github.com/hybridgroup/yzma/pkg/download"
)

const (
	versionFile = "version.json"
	localFolder = "libraries"
)

// Logger represents a logger for capturing events.
type Logger func(ctx context.Context, msg string, args ...any)

// VersionTag represents information about the installed version of llama.cpp.
type VersionTag struct {
	Version   string `json:"version"`
	Arch      string `json:"arch"`
	OS        string `json:"os"`
	Processor string `json:"processor"`
	Latest    string `json:"-"`
}

// =============================================================================

// Libs manages the library system.
type Libs struct {
	path         string
	arch         download.Arch
	os           download.OS
	processor    download.Processor
	allowUpgrade bool
}

// New uses defaults based on the system run are running on. It will use CPU
// as the processor.
func New() (*Libs, error) {
	arch, err := defaults.Arch("")
	if err != nil {
		return nil, err
	}

	opSys, err := defaults.OS("")
	if err != nil {
		return nil, err
	}

	processor, err := defaults.Processor("")
	if err != nil {
		return nil, err
	}

	return NewWithSettings("", arch, opSys, processor, true)
}

// NewWithProcessor uses defaults based on the system run are running on, but
// will let you specify the processor.
func NewWithProcessor(processor download.Processor) (*Libs, error) {
	arch, err := defaults.Arch("")
	if err != nil {
		return nil, err
	}

	opSys, err := defaults.OS("")
	if err != nil {
		return nil, err
	}

	return NewWithSettings("", arch, opSys, processor, true)
}

// NewWithSettings constructs a valid library config for downloading based on raw
// values that would come from configuration. It sets defaults for the specified
// values when the parameters are empty.
// basePath    : represents the base path the llama.cpp libraries will/are installed in.
// arch        : represents the architecture.
// opSys       : represents the operating system.
// processor   : represents the hardare.
// allowUpgrade: true or false to determine to upgrade libraries when available.
func NewWithSettings(basePath string, arch download.Arch, opSys download.OS, processor download.Processor, allowUpgrade bool) (*Libs, error) {
	basePath = defaults.BaseDir(basePath)

	lib := Libs{
		path:         filepath.Join(basePath, localFolder),
		arch:         arch,
		os:           opSys,
		processor:    processor,
		allowUpgrade: allowUpgrade,
	}

	return &lib, nil
}

// LibsPath returns the location of the libraries path.
func (lib *Libs) LibsPath() string {
	return lib.path
}

// Arch returns the current architecture being used.
func (lib *Libs) Arch() download.Arch {
	return lib.arch
}

// OS returns the current operating system being used.
func (lib *Libs) OS() download.OS {
	return lib.os
}

// Processor returns the hardware system being used.
func (lib *Libs) Processor() download.Processor {
	return lib.processor
}

// Download performs a complete workflow for downloading and installing
// the latest version of llama.cpp.
func (lib *Libs) Download(ctx context.Context, log Logger) (VersionTag, error) {
	log(ctx, "download-libraries", "status", "check libraries version information", "arch", lib.arch, "os", lib.os, "processor", lib.processor)

	tag, err := lib.VersionInformation()
	if err != nil {
		if tag.Version == "" {
			return VersionTag{}, fmt.Errorf("download-libraries: error retrieving version info: %w", err)
		}

		log(ctx, "download-libraries", "status", "unable to check latest verion, using installed version", "arch", lib.arch, "os", lib.os, "processor", lib.processor, "latest", tag.Latest, "current", tag.Version)
		return tag, nil
	}

	log(ctx, "download-libraries", "status", "check llama.cpp installation", "arch", lib.arch, "os", lib.os, "processor", lib.processor, "latest", tag.Latest, "current", tag.Version)

	if isTagMatch(tag, lib) {
		log(ctx, "download-libraries", "status", "already installed", "latest", tag.Latest, "current", tag.Version)
		return tag, nil
	}

	if !lib.allowUpgrade {
		log(ctx, "download-libraries", "status", "bypassing upgrade", "latest", tag.Latest, "current", tag.Version)
		return tag, nil
	}

	log(ctx, "download-libraries waiting to start download...")

	newTag, err := lib.download(ctx, log, tag.Latest)
	if err != nil {
		log(ctx, "download-libraries", "status", "llama.cpp installation", "ERROR", err)

		if _, err := lib.InstalledVersion(); err != nil {
			return VersionTag{}, fmt.Errorf("download-libraries: failed to install llama: %q: error: %w", lib.path, err)
		}

		log(ctx, "download-libraries", "status", "failed to install new version, using current version")
	}

	log(ctx, "download-libraries", "status", "updated llama.cpp installed", "old-version", tag.Version, "current", newTag.Version)

	return newTag, nil
}

// InstalledVersion retrieves the current version of llama.cpp installed.
func (lib *Libs) InstalledVersion() (VersionTag, error) {
	versionInfoPath := filepath.Join(lib.path, versionFile)

	d, err := os.ReadFile(versionInfoPath)
	if err != nil {
		return VersionTag{}, fmt.Errorf("installed-version: unable to read version info file: %w", err)
	}

	var tag VersionTag
	if err := json.Unmarshal(d, &tag); err != nil {
		return VersionTag{}, fmt.Errorf("installed-version: unable to parse version info file: %w", err)
	}

	return tag, nil
}

// VersionInformation retrieves the current version of llama.cpp that is
// published on GitHub and the current installed version.
func (lib *Libs) VersionInformation() (VersionTag, error) {
	tag, _ := lib.InstalledVersion()

	version, err := download.LlamaLatestVersion()
	if err != nil {
		return tag, fmt.Errorf("version-information: unable to get latest version of llama.cpp: %w", err)
	}

	tag.Latest = version

	return tag, nil
}

// =============================================================================

func (lib *Libs) download(ctx context.Context, log Logger, version string) (VersionTag, error) {
	tempPath := filepath.Join(lib.path, "temp")

	progress := func(src string, currentSize int64, totalSize int64, mibPerSec float64, complete bool) {
		log(ctx, fmt.Sprintf("\x1b[1A\r\x1b[Kdownload-libraries: Downloading %s... %d MiB of %d MiB (%.2f MiB/s)", src, currentSize/(1024*1024), totalSize/(1024*1024), mibPerSec))
	}

	pr := downloader.NewProgressReader(progress, downloader.SizeIntervalMIB10)

	err := download.GetWithContext(ctx, lib.arch.String(), lib.os.String(), lib.processor.String(), version, tempPath, pr)
	if err != nil {
		os.RemoveAll(tempPath)
		return VersionTag{}, fmt.Errorf("download-libs: unable to install llama.cpp: %w", err)
	}

	if err := lib.swapTempForLib(tempPath); err != nil {
		os.RemoveAll(tempPath)
		return VersionTag{}, fmt.Errorf("download-libs: unable to swap temp for lib: %w", err)
	}

	if err := lib.createVersionFile(version); err != nil {
		return VersionTag{}, fmt.Errorf("download-libs: unable to create version file: %w", err)
	}

	return lib.VersionInformation()
}

func (lib *Libs) swapTempForLib(tempPath string) error {
	entries, err := os.ReadDir(lib.path)
	if err != nil {
		return fmt.Errorf("swap-temp-for-lib: unable to read libPath: %w", err)
	}

	for _, entry := range entries {
		if entry.Name() == "temp" {
			continue
		}

		os.Remove(filepath.Join(lib.path, entry.Name()))
	}

	tempEntries, err := os.ReadDir(tempPath)
	if err != nil {
		return fmt.Errorf("swap-temp-for-lib: unable to read temp: %w", err)
	}

	for _, entry := range tempEntries {
		src := filepath.Join(tempPath, entry.Name())
		dst := filepath.Join(lib.path, entry.Name())
		if err := os.Rename(src, dst); err != nil {
			return fmt.Errorf("swap-temp-for-lib: unable to move %s: %w", entry.Name(), err)
		}
	}

	os.RemoveAll(tempPath)

	return nil
}

func (lib *Libs) createVersionFile(version string) error {
	versionInfoPath := filepath.Join(lib.path, versionFile)

	f, err := os.Create(versionInfoPath)
	if err != nil {
		return fmt.Errorf("create-version-file: creating version info file: %w", err)
	}
	defer f.Close()

	t := VersionTag{
		Version:   version,
		Arch:      lib.arch.String(),
		OS:        lib.os.String(),
		Processor: lib.processor.String(),
	}

	d, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("create-version-file: marshalling version info: %w", err)
	}

	if _, err := f.Write(d); err != nil {
		return fmt.Errorf("create-version-file: writing version info: %w", err)
	}

	return nil
}

// =============================================================================

func isTagMatch(tag VersionTag, libs *Libs) bool {
	return tag.Latest == tag.Version && tag.Arch == libs.arch.String() && tag.OS == libs.os.String() && tag.Processor == libs.processor.String()
}
