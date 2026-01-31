// Package libs provides llama.cpp library support.
package libs

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/ardanlabs/kronk/sdk/tools/defaults"
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

// Options represents the configuration options for Libs.
type Options struct {
	BasePath     string
	Arch         download.Arch
	OS           download.OS
	Processor    download.Processor
	AllowUpgrade bool
	Version      string
}

// Option is a function that configures Options.
type Option func(*Options)

// WithBasePath sets the base path for library installation.
func WithBasePath(basePath string) Option {
	return func(o *Options) {
		o.BasePath = basePath
	}
}

// WithArch sets the architecture.
func WithArch(arch download.Arch) Option {
	return func(o *Options) {
		o.Arch = arch
	}
}

// WithOS sets the operating system.
func WithOS(opSys download.OS) Option {
	return func(o *Options) {
		o.OS = opSys
	}
}

// WithProcessor sets the processor/hardware type.
func WithProcessor(processor download.Processor) Option {
	return func(o *Options) {
		o.Processor = processor
	}
}

// WithAllowUpgrade sets whether library upgrades are allowed.
func WithAllowUpgrade(allow bool) Option {
	return func(o *Options) {
		o.AllowUpgrade = allow
	}
}

// WithVersion sets a specific version to download instead of latest.
func WithVersion(version string) Option {
	return func(o *Options) {
		o.Version = version
	}
}

// =============================================================================

// Libs manages the library system.
type Libs struct {
	path         string
	arch         download.Arch
	os           download.OS
	processor    download.Processor
	allowUpgrade bool
	version      string
}

// New constructs a Libs with system defaults and applies any provided options.
func New(opts ...Option) (*Libs, error) {
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

	options := Options{
		BasePath:     "",
		Arch:         arch,
		OS:           opSys,
		Processor:    processor,
		AllowUpgrade: true,
	}

	for _, opt := range opts {
		opt(&options)
	}

	basePath := defaults.BaseDir(options.BasePath)

	lib := Libs{
		path:         filepath.Join(basePath, localFolder),
		arch:         options.Arch,
		os:           options.OS,
		processor:    options.Processor,
		allowUpgrade: options.AllowUpgrade,
		version:      options.Version,
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
	if !hasNetwork() {
		vt, err := lib.InstalledVersion()
		if err != nil {
			return VersionTag{}, fmt.Errorf("download: no network available: %w", err)
		}

		log(ctx, "download-libraries: no network available, using current version")
		return vt, nil
	}

	log(ctx, "download-libraries: check libraries version information", "arch", lib.arch, "os", lib.os, "processor", lib.processor)

	tag, err := lib.VersionInformation()
	if err != nil {
		if tag.Version == "" {
			return VersionTag{}, fmt.Errorf("download-libraries: error retrieving version info: %w", err)
		}

		log(ctx, "download-libraries: unable to check latest verion, using installed version", "arch", lib.arch, "os", lib.os, "processor", lib.processor, "latest", tag.Latest, "current", tag.Version)
		return tag, nil
	}

	if lib.version != "" {
		tag.Latest = lib.version
	}

	log(ctx, "download-libraries: check llama.cpp installation", "arch", lib.arch, "os", lib.os, "processor", lib.processor, "latest", tag.Latest, "current", tag.Version)

	if isTagMatch(tag, lib) {
		log(ctx, "download-libraries: already installed", "latest", tag.Latest, "current", tag.Version)
		return tag, nil
	}

	if !lib.allowUpgrade {
		log(ctx, "download-libraries: bypassing upgrade", "latest", tag.Latest, "current", tag.Version)
		return tag, nil
	}

	log(ctx, "download-libraries waiting to start download...", "tag", tag.Latest)

	newTag, err := lib.DownloadVersion(ctx, log, tag.Latest)
	if err != nil {
		log(ctx, "download-libraries: llama.cpp installation", "ERROR", err)

		if _, err := lib.InstalledVersion(); err != nil {
			return VersionTag{}, fmt.Errorf("download: failed to install llama: %q: error: %w", lib.path, err)
		}

		log(ctx, "download-libraries: failed to install new version, using current version")
	}

	log(ctx, "download-libraries: updated llama.cpp installed", "old-version", tag.Version, "current", newTag.Version)

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

// DownloadVersion allows you to download a specific version of llama.cpp. This
// function will not check for existing versions or any of the workflow provided
// by Download.
func (lib *Libs) DownloadVersion(ctx context.Context, log Logger, version string) (VersionTag, error) {
	tempPath := filepath.Join(lib.path, "temp")

	progress := func(src string, currentSize int64, totalSize int64, mibPerSec float64, complete bool) {
		log(ctx, fmt.Sprintf("\r\x1b[Kdownload-libraries: Downloading %s... %d MiB of %d MiB (%.2f MiB/s)", src, currentSize/(1024*1024), totalSize/(1024*1024), mibPerSec))
	}

	pr := downloader.NewProgressReader(progress, downloader.SizeIntervalMIB10)

	err := download.GetWithContext(ctx, lib.arch.String(), lib.os.String(), lib.processor.String(), version, tempPath, pr)
	if err != nil {
		os.RemoveAll(tempPath)
		return VersionTag{}, fmt.Errorf("download-version: unable to install llama.cpp: %w", err)
	}

	if err := lib.swapTempForLib(tempPath); err != nil {
		os.RemoveAll(tempPath)
		return VersionTag{}, fmt.Errorf("download-version: unable to swap temp for lib: %w", err)
	}

	if err := lib.createVersionFile(version); err != nil {
		return VersionTag{}, fmt.Errorf("download-version: unable to create version file: %w", err)
	}

	return lib.VersionInformation()
}

// =============================================================================

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

func hasNetwork() bool {
	conn, err := net.DialTimeout("tcp", "8.8.8.8:53", 5*time.Second)
	if err != nil {
		return false
	}

	conn.Close()

	return true
}
