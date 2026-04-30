// Package libs provides llama.cpp library support.
package libs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/downloader"
	"github.com/hybridgroup/yzma/pkg/download"
)

const (
	versionFile = "version.json"
	localFolder = "libraries"

	// defaultVersion is the well-known working version of llama.cpp used
	// when no explicit version is provided and AllowUpgrade is false.
	defaultVersion = "b8981"
)

// ErrReadOnly is returned by mutating operations on a Libs instance whose
// install path is a user-supplied directory that does not contain a
// version.json file. Such paths are treated as user-managed builds that
// Kronk will load from but never modify.
var ErrReadOnly = errors.New("libs: install path is read-only (no version.json)")

// Logger represents a logger for capturing events.
type Logger = applog.Logger

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
	LibPath      string
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

// WithLibPath sets the path Kronk should load libraries from. The supplied
// path is interpreted as one of three things:
//
//  1. A directory that already contains a version.json — used directly as
//     the install location and the (arch, os, processor) triple recorded in
//     that file is adopted unless the caller overrides it.
//  2. A non-empty directory without a version.json — treated as a
//     user-managed read-only build. Mutating operations return ErrReadOnly.
//  3. An empty or non-existent directory — treated as the libraries root.
//     Installs land in a subfolder of the form <root>/<os>/<arch>/<processor>/.
//
// An empty string falls back to the Kronk default libraries root.
func WithLibPath(basePath string) Option {
	return func(o *Options) {
		o.LibPath = basePath
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

// WithVersion sets a specific version to download instead of the default.
func WithVersion(version string) Option {
	return func(o *Options) {
		o.Version = version
	}
}

// =============================================================================

// Libs manages the library system. Each Libs instance points at exactly one
// install directory containing a llama.cpp library bundle. The directory is
// resolved at construction time according to the rules described on
// WithLibPath and may be one of:
//
//   - A per-triple subfolder under the libraries root (the default).
//   - A user-supplied directory that already contains a version.json.
//   - A user-supplied read-only directory (see ReadOnly).
//
// Other installs for different (arch, os, processor) triples on the same
// libraries root are discoverable through List, Remove, and InstalledFor.
type Libs struct {
	root         string
	path         string
	arch         download.Arch
	os           download.OS
	processor    download.Processor
	version      string
	readOnly     bool
	AllowUpgrade bool
	testMode     bool
	testLatest   string
}

// New constructs a Libs with system defaults and applies any provided
// options. It resolves the install location, reads any existing version.json
// to back-fill the (arch, os, processor) triple for fields the caller did not
// explicitly set, and migrates a legacy root-level install (libraries
// directly under <libsRoot>) into <libsRoot>/<os>/<arch>/<processor>/ if one
// is found.
func New(opts ...Option) (*Libs, error) {
	var options Options
	for _, opt := range opts {
		opt(&options)
	}

	root, path, readOnly, err := resolvePaths(options.BasePath, options.LibPath)
	if err != nil {
		return nil, err
	}

	// Migrate a legacy install (libs sitting directly under the root with a
	// version.json next to them) into the new per-triple layout. Only attempt
	// this when the user did not explicitly point at a custom install path.
	if options.LibPath == "" {
		if migrated, err := migrateLegacyRoot(root); err != nil {
			return nil, fmt.Errorf("libs: migrate legacy install: %w", err)
		} else if migrated != "" && path == root {
			// Migration produced a triple folder; if the caller's resolved
			// path was the root itself, switch to the migrated location.
			path = migrated
		}
	}

	// Apply the resolution precedence for each triple field:
	//   1. explicit Option (WithArch/WithOS/WithProcessor)
	//   2. existing version.json at the resolved install path
	//   3. KRONK_* environment variable / runtime detection (defaults package)
	tag, _ := readVersionFile(path)

	arch, err := resolveArch(options.Arch, tag.Arch)
	if err != nil {
		return nil, err
	}

	opSys, err := resolveOS(options.OS, tag.OS)
	if err != nil {
		return nil, err
	}

	processor, err := resolveProcessor(options.Processor, tag.Processor)
	if err != nil {
		return nil, err
	}

	// If the caller did not point at a specific install directory, the final
	// install path is <root>/<os>/<arch>/<processor>/ for the resolved triple.
	if options.LibPath == "" {
		path = installPathFor(root, arch, opSys, processor)
	}

	lib := Libs{
		root:         root,
		path:         path,
		arch:         arch,
		os:           opSys,
		processor:    processor,
		version:      options.Version,
		readOnly:     readOnly,
		AllowUpgrade: options.AllowUpgrade,
	}

	return &lib, nil
}

// LibsPath returns the directory the loaded libraries live in.
func (lib *Libs) LibsPath() string {
	return lib.path
}

// Root returns the libraries root that holds per-triple install
// subdirectories. When the Libs instance was constructed against a
// user-supplied directory containing a version.json (or against a
// read-only user build), Root returns that directory itself.
func (lib *Libs) Root() string {
	return lib.root
}

// Arch returns the current architecture being used.
func (lib *Libs) Arch() string {
	return lib.arch.String()
}

// OS returns the current operating system being used.
func (lib *Libs) OS() string {
	return lib.os.String()
}

// Processor returns the hardware system being used.
func (lib *Libs) Processor() string {
	return lib.processor.String()
}

// ReadOnly reports whether the resolved install path is a user-supplied
// directory without a version.json. Mutating operations will return
// ErrReadOnly when this is true.
func (lib *Libs) ReadOnly() bool {
	return lib.readOnly
}

// SetVersion sets the version to download. An empty string means use the default.
func (lib *Libs) SetVersion(version string) {
	lib.version = version
}

// Download performs a complete workflow for downloading and installing
// llama.cpp. The version that gets installed is selected according to the
// following matrix, evaluated in order. The first matching row wins:
//
//	# | Override (WithVersion) | AllowUpgrade | On-disk version          | Action
//	--+------------------------+--------------+--------------------------+-----------------------------
//	1 | set                    | any          | any                      | install the override version
//	2 | unset                  | true         | any                      | install latest from llama.cpp
//	3 | unset                  | false        | none                     | install defaultVersion
//	4 | unset                  | false        | <= defaultVersion        | install defaultVersion
//	5 | unset                  | false        | >  defaultVersion        | keep on-disk version
//
// Additional rules independent of the matrix:
//   - A read-only install path (user-supplied directory without a
//     version.json) is always honored as-is; nothing is downloaded or
//     mutated. See WithLibPath.
//   - When the network is unreachable the currently installed version is
//     returned. If nothing is installed and no network is available the
//     call fails.
//   - If the desired version is already installed for the active (arch,
//     os, processor) triple, no download occurs.
func (lib *Libs) Download(ctx context.Context, log Logger) (VersionTag, error) {
	if lib.readOnly {
		tag, err := lib.InstalledVersion()
		if err != nil {
			return VersionTag{}, fmt.Errorf("libs: read-only install path has no version.json: %w", ErrReadOnly)
		}
		log(ctx, "download-libraries: read-only install path, treating as fixed", "current", tag.Version)
		return tag, nil
	}

	if !lib.testMode && !hasNetwork() {
		vt, err := lib.InstalledVersion()
		if err != nil {
			return VersionTag{}, fmt.Errorf("download: no network available: %w", err)
		}

		log(ctx, "download-libraries: no network available, using current version", "current", vt.Version)
		return vt, nil
	}

	log(ctx, "download-libraries: check libraries version information", "arch", lib.arch, "os", lib.os, "processor", lib.processor)

	installed, _ := lib.InstalledVersion()

	// For matrix row 2 we need the latest version published by llama.cpp.
	// For all other rows the network lookup is unnecessary, so skip it.
	var latest string
	if lib.version == "" && lib.AllowUpgrade {
		info, err := lib.VersionInformation()
		if err != nil {
			if installed.Version == "" {
				return VersionTag{}, fmt.Errorf("download-libraries: error retrieving version info: %w", err)
			}

			log(ctx, "download-libraries: unable to check latest version, using installed version", "arch", lib.arch, "os", lib.os, "processor", lib.processor, "current", installed.Version)
			return info, nil
		}
		latest = info.Latest
	}

	tag := installed
	tag.Latest = chooseVersion(lib.version, lib.AllowUpgrade, installed.Version, latest, defaultVersion)

	log(ctx, "download-libraries: check llama.cpp installation", "arch", lib.arch, "os", lib.os, "processor", lib.processor, "latest", tag.Latest, "current", tag.Version)

	if isTagMatch(tag, lib) {
		log(ctx, "download-libraries: already installed", "latest", tag.Latest, "current", tag.Version)
		return tag, nil
	}

	log(ctx, "download-libraries: waiting to start download...", "tag", tag.Latest)

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

// InstalledVersion retrieves the current version of llama.cpp installed at
// this Libs instance's resolved install path.
func (lib *Libs) InstalledVersion() (VersionTag, error) {
	return readVersionFile(lib.path)
}

// InstalledFor reads the version metadata for an install matching the
// supplied (arch, os, processor) triple under the libraries Root. It is the
// triple-aware counterpart to InstalledVersion. The triple values must match
// strings recognized by IsSupported.
func (lib *Libs) InstalledFor(arch string, opSys string, processor string) (VersionTag, error) {
	a, o, p, err := parseTriple(arch, opSys, processor)
	if err != nil {
		return VersionTag{}, err
	}
	return readVersionFile(installPathFor(lib.root, a, o, p))
}

// ReadVersionFile reads and parses the version.json file from the supplied
// directory. It is exposed for callers that need to inspect installed
// library metadata in arbitrary locations without constructing a separate
// *Libs instance for each one.
func ReadVersionFile(path string) (VersionTag, error) {
	return readVersionFile(path)
}

// VersionInformation retrieves the current version of llama.cpp that is
// published on GitHub and the current installed version.
func (lib *Libs) VersionInformation() (VersionTag, error) {
	tag, _ := lib.InstalledVersion()

	if lib.testMode {
		tag.Latest = lib.testLatest
		return tag, nil
	}

	version, err := download.LlamaLatestVersion()
	if err != nil {
		return tag, fmt.Errorf("version-information: unable to get latest version of llama.cpp: %w", err)
	}

	tag.Latest = version

	return tag, nil
}

// DownloadVersion allows you to download a specific version of llama.cpp
// into this Libs instance's resolved install path. This function bypasses
// the workflow checks performed by Download (network availability, version
// comparison, AllowUpgrade) and writes unconditionally.
func (lib *Libs) DownloadVersion(ctx context.Context, log Logger, version string) (VersionTag, error) {
	if lib.readOnly {
		return VersionTag{}, fmt.Errorf("libs: download-version: %w", ErrReadOnly)
	}
	return lib.downloadInto(ctx, log, lib.path, lib.arch, lib.os, lib.processor, version)
}

// DownloadInto downloads a specific version of llama.cpp into the supplied
// directory using the supplied (arch, os, processor) triple. This is the
// single canonical install primitive used by both Download (via
// DownloadVersion) and the triple-aware DownloadFor entry point.
//
// The destination directory is created if it does not exist. Existing
// non-temp content in that directory is replaced. On success the returned
// VersionTag reflects the freshly installed version metadata read back from
// the destination. The triple values must match strings recognized by
// IsSupported.
func (lib *Libs) DownloadInto(ctx context.Context, log Logger, path string, arch string, opSys string, processor string, version string) (VersionTag, error) {
	a, o, p, err := parseTriple(arch, opSys, processor)
	if err != nil {
		return VersionTag{}, fmt.Errorf("download-into: %w", err)
	}
	return lib.downloadInto(ctx, log, path, a, o, p, version)
}

// downloadInto is the unexported implementation that operates on the typed
// download.* values. Internal callers that have already parsed a triple use
// this directly to avoid re-parsing.
func (lib *Libs) downloadInto(ctx context.Context, log Logger, path string, arch download.Arch, opSys download.OS, processor download.Processor, version string) (VersionTag, error) {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return VersionTag{}, fmt.Errorf("download-into: unable to create destination: %w", err)
	}

	tempPath := filepath.Join(path, "temp")

	progress := func(src string, currentSize int64, totalSize int64, mbPerSec float64, complete bool) {
		log(ctx, fmt.Sprintf("\r\x1b[Kdownload-libraries: Downloading %s... %d MB of %d MB (%.2f MB/s)", src, currentSize/(1000*1000), totalSize/(1000*1000), mbPerSec))
	}

	pr := downloader.NewProgressReader(progress, downloader.SizeIntervalMB10)

	err := download.GetWithContext(ctx, arch.String(), opSys.String(), processor.String(), version, tempPath, pr)
	if err != nil {
		os.RemoveAll(tempPath)
		return VersionTag{}, fmt.Errorf("download-into: unable to install llama.cpp: %w", err)
	}

	if err := swapTempForLibAt(path, tempPath); err != nil {
		os.RemoveAll(tempPath)
		return VersionTag{}, fmt.Errorf("download-into: unable to swap temp for lib: %w", err)
	}

	if err := writeVersionFile(path, version, arch, opSys, processor); err != nil {
		return VersionTag{}, fmt.Errorf("download-into: unable to create version file: %w", err)
	}

	return readVersionFile(path)
}

// DownloadFor downloads the supplied version into the canonical install
// directory for the supplied (arch, os, processor) triple under the
// libraries Root. This is the triple-aware entry point for installing
// llama.cpp bundles for platforms other than the active one.
//
// If version is empty, the default version is used. If the supplied
// triple is not part of triple is not part of
// SupportedCombinations the call returns an error.
func (lib *Libs) DownloadFor(ctx context.Context, log Logger, arch string, opSys string, processor string, version string) (VersionTag, error) {
	if lib.readOnly {
		return VersionTag{}, fmt.Errorf("libs: download-for: %w", ErrReadOnly)
	}
	if !IsSupported(arch, opSys, processor) {
		return VersionTag{}, fmt.Errorf("libs: download-for: unsupported combination arch=%s os=%s processor=%s", arch, opSys, processor)
	}

	a, o, p, err := parseTriple(arch, opSys, processor)
	if err != nil {
		return VersionTag{}, fmt.Errorf("libs: download-for: %w", err)
	}

	if version == "" {
		installed, _ := lib.InstalledVersion()
		if installed.Version != "" && versionGreater(installed.Version, defaultVersion) {
			version = installed.Version
		} else {
			version = defaultVersion
		}
	}

	return lib.downloadInto(ctx, log, installPathFor(lib.root, a, o, p), a, o, p, version)
}

// List walks the libraries Root and returns one VersionTag per installed
// (arch, os, processor) bundle whose version.json could be read. Bundles
// without a readable version.json are skipped silently. The returned slice
// is sorted by (os, arch, processor) for stable presentation.
func (lib *Libs) List() ([]VersionTag, error) {
	osEntries, err := os.ReadDir(lib.root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("libs: list: %w", err)
	}

	var out []VersionTag

	for _, osEntry := range osEntries {
		if !osEntry.IsDir() {
			continue
		}

		osPath := filepath.Join(lib.root, osEntry.Name())

		archEntries, err := os.ReadDir(osPath)
		if err != nil {
			continue
		}

		for _, archEntry := range archEntries {
			if !archEntry.IsDir() {
				continue
			}

			archPath := filepath.Join(osPath, archEntry.Name())

			procEntries, err := os.ReadDir(archPath)
			if err != nil {
				continue
			}

			for _, procEntry := range procEntries {
				if !procEntry.IsDir() {
					continue
				}

				tag, err := readVersionFile(filepath.Join(archPath, procEntry.Name()))
				if err != nil {
					continue
				}

				out = append(out, tag)
			}
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].OS != out[j].OS {
			return out[i].OS < out[j].OS
		}
		if out[i].Arch != out[j].Arch {
			return out[i].Arch < out[j].Arch
		}
		return out[i].Processor < out[j].Processor
	})

	return out, nil
}

// Remove deletes the install directory matching the supplied (arch, os,
// processor) triple under the libraries Root. Empty parent directories
// (the arch and os folders) are removed as well, but the libraries Root
// is preserved. Removing an install that does not exist is not an error.
// Removing the active install (the one matching LibsPath) is permitted;
// callers are responsible for not loading an install they have just
// removed. The triple values must match strings recognized by
// IsSupported.
func (lib *Libs) Remove(arch string, opSys string, processor string) error {
	if lib.readOnly {
		return fmt.Errorf("libs: remove: %w", ErrReadOnly)
	}

	a, o, p, err := parseTriple(arch, opSys, processor)
	if err != nil {
		return fmt.Errorf("libs: remove: %w", err)
	}

	path := installPathFor(lib.root, a, o, p)

	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("libs: remove: %w", err)
	}

	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("libs: remove: %w", err)
	}

	// Walk up and prune any now-empty parent directories, stopping at
	// the libraries root. Directories that only contain ignorable OS
	// metadata (e.g. macOS .DS_Store) are treated as empty so a Finder
	// visit does not pin them in place; a sibling install with real
	// content still safely blocks the prune.
	parent := filepath.Dir(path)
	for parent != lib.root && parent != filepath.Dir(parent) {
		if !pruneDirIfEmpty(parent) {
			break
		}
		parent = filepath.Dir(parent)
	}

	return nil
}

// pruneDirIfEmpty removes path when it contains nothing but ignorable OS
// metadata files. Returns true if the directory was removed (or was
// already gone). Returns false when the directory still has meaningful
// content or removal failed for any other reason.
func pruneDirIfEmpty(path string) bool {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}

	for _, e := range entries {
		if isIgnorableOSFile(e.Name()) {
			continue
		}
		return false
	}

	if err := os.RemoveAll(path); err != nil {
		return false
	}
	return true
}

// isIgnorableOSFile reports whether name is an OS-generated metadata
// file that should not block pruning of an otherwise empty directory.
func isIgnorableOSFile(name string) bool {
	switch name {
	case ".DS_Store", "Thumbs.db", "desktop.ini":
		return true
	}
	return false
}

// parseTriple validates and parses the supplied (arch, os, processor)
// strings into the underlying typed values used by the install primitive.
// It is the single internal funnel where string-based public APIs convert
// to typed values, keeping the download package import contained to the
// libs package.
func parseTriple(arch string, opSys string, processor string) (download.Arch, download.OS, download.Processor, error) {
	a, err := download.ParseArch(arch)
	if err != nil {
		return download.Arch{}, download.OS{}, download.Processor{}, fmt.Errorf("invalid arch %q: %w", arch, err)
	}
	o, err := download.ParseOS(opSys)
	if err != nil {
		return download.Arch{}, download.OS{}, download.Processor{}, fmt.Errorf("invalid os %q: %w", opSys, err)
	}
	p, err := download.ParseProcessor(processor)
	if err != nil {
		return download.Arch{}, download.OS{}, download.Processor{}, fmt.Errorf("invalid processor %q: %w", processor, err)
	}
	return a, o, p, nil
}

// =============================================================================

func swapTempForLibAt(path string, tempPath string) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("swap-temp-for-lib: unable to read libPath: %w", err)
	}

	for _, entry := range entries {
		if entry.Name() == "temp" {
			continue
		}

		os.Remove(filepath.Join(path, entry.Name()))
	}

	tempEntries, err := os.ReadDir(tempPath)
	if err != nil {
		return fmt.Errorf("swap-temp-for-lib: unable to read temp: %w", err)
	}

	for _, entry := range tempEntries {
		src := filepath.Join(tempPath, entry.Name())
		dst := filepath.Join(path, entry.Name())
		if err := os.Rename(src, dst); err != nil {
			return fmt.Errorf("swap-temp-for-lib: unable to move %s: %w", entry.Name(), err)
		}
	}

	os.RemoveAll(tempPath)

	return nil
}

func writeVersionFile(path string, version string, arch download.Arch, opSys download.OS, processor download.Processor) error {
	versionInfoPath := filepath.Join(path, versionFile)

	f, err := os.Create(versionInfoPath)
	if err != nil {
		return fmt.Errorf("create-version-file: creating version info file: %w", err)
	}
	defer f.Close()

	t := VersionTag{
		Version:   version,
		Arch:      arch.String(),
		OS:        opSys.String(),
		Processor: processor.String(),
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

func readVersionFile(path string) (VersionTag, error) {
	d, err := os.ReadFile(filepath.Join(path, versionFile))
	if err != nil {
		return VersionTag{}, fmt.Errorf("installed-version: unable to read version info file: %w", err)
	}

	var tag VersionTag
	if err := json.Unmarshal(d, &tag); err != nil {
		return VersionTag{}, fmt.Errorf("installed-version: unable to parse version info file: %w", err)
	}

	return tag, nil
}

// =============================================================================

// installPathFor returns the canonical per-triple install directory under
// the supplied libraries root.
func installPathFor(root string, arch download.Arch, opSys download.OS, processor download.Processor) string {
	return filepath.Join(root, opSys.String(), arch.String(), processor.String())
}

// resolvePaths derives the libraries root and the active install path from
// the supplied basePath / libPath options. It also reports whether the
// active install path is a user-supplied read-only directory.
func resolvePaths(basePath string, libPath string) (root string, path string, readOnly bool, err error) {
	defaultRoot := filepath.Join(defaults.BaseDir(basePath), localFolder)

	if libPath == "" {
		return defaultRoot, defaultRoot, false, nil
	}

	// Caller supplied an explicit lib path. If it contains a version.json it
	// is a complete install bundle that we should use as-is. If it contains
	// other files but no version.json it is a user-managed read-only build.
	// Otherwise (empty or non-existent) treat it as a libraries root.
	if _, err := os.Stat(filepath.Join(libPath, versionFile)); err == nil {
		return libPath, libPath, false, nil
	}

	entries, statErr := os.ReadDir(libPath)
	switch {
	case statErr != nil && !os.IsNotExist(statErr):
		return "", "", false, fmt.Errorf("libs: resolve-paths: %w", statErr)
	case statErr == nil && len(entries) > 0:
		return libPath, libPath, true, nil
	}

	return libPath, libPath, false, nil
}

// migrateLegacyRoot moves an install written in the legacy "libs at root"
// layout into the per-triple subdirectory for its own (arch, os, processor)
// triple. It is a no-op when no version.json exists at the root or when the
// triple subfolder already contains a version.json.
//
// Returns the new install path on successful migration, the empty string
// otherwise.
func migrateLegacyRoot(root string) (string, error) {
	rootVersionPath := filepath.Join(root, versionFile)
	tag, err := readVersionFile(root)
	if err != nil {
		return "", nil // no legacy install to migrate.
	}
	if tag.OS == "" || tag.Arch == "" || tag.Processor == "" {
		return "", nil // version.json is too incomplete to migrate safely.
	}

	arch, err := download.ParseArch(tag.Arch)
	if err != nil {
		return "", nil
	}
	opSys, err := download.ParseOS(tag.OS)
	if err != nil {
		return "", nil
	}
	processor, err := download.ParseProcessor(tag.Processor)
	if err != nil {
		return "", nil
	}

	dst := installPathFor(root, arch, opSys, processor)

	// If the destination already has a version.json, the migration has
	// effectively already happened; just clean up the stale root files.
	if _, err := os.Stat(filepath.Join(dst, versionFile)); err == nil {
		return cleanLegacyRoot(root, rootVersionPath, dst)
	}

	if err := os.MkdirAll(dst, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dst, err)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return "", fmt.Errorf("read root: %w", err)
	}

	for _, e := range entries {
		// Skip the per-triple subtree we're migrating into and any peer OS
		// directories that may already exist for cross-triple installs.
		if e.IsDir() {
			if _, parseErr := download.ParseOS(e.Name()); parseErr == nil {
				continue
			}
		}
		if e.Name() == "temp" {
			continue
		}

		src := filepath.Join(root, e.Name())
		if err := os.Rename(src, filepath.Join(dst, e.Name())); err != nil {
			return "", fmt.Errorf("move %s: %w", e.Name(), err)
		}
	}

	return dst, nil
}

func cleanLegacyRoot(root string, rootVersionPath string, _ string) (string, error) {
	// The destination already exists; just remove the duplicate root-level
	// version.json so subsequent calls don't re-detect a legacy install.
	if err := os.Remove(rootVersionPath); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("clean root version.json: %w", err)
	}
	_ = root
	return "", nil
}

// resolveArch returns the architecture to use following the precedence:
// explicit option > version.json fallback > KRONK_ARCH / runtime detection.
func resolveArch(opt download.Arch, fallback string) (download.Arch, error) {
	if opt.String() != "" {
		return opt, nil
	}
	if fallback != "" {
		if a, err := download.ParseArch(fallback); err == nil {
			return a, nil
		}
	}
	return defaults.Arch("")
}

func resolveOS(opt download.OS, fallback string) (download.OS, error) {
	if opt.String() != "" {
		return opt, nil
	}
	if fallback != "" {
		if o, err := download.ParseOS(fallback); err == nil {
			return o, nil
		}
	}
	return defaults.OS("")
}

func resolveProcessor(opt download.Processor, fallback string) (download.Processor, error) {
	if opt.String() != "" {
		return opt, nil
	}
	if fallback != "" {
		if p, err := download.ParseProcessor(fallback); err == nil {
			return p, nil
		}
	}
	return defaults.Processor("")
}

// =============================================================================

func isTagMatch(tag VersionTag, libs *Libs) bool {
	return tag.Latest == tag.Version && tag.Arch == libs.arch.String() && tag.OS == libs.os.String() && tag.Processor == libs.processor.String()
}

// chooseVersion implements the Download policy matrix as a pure function.
// See Download for the full matrix and exception rules. Inputs:
//
//   - override: explicit version pin (lib.version), or "" if unset.
//   - allowUpgrade: whether to track the latest published version.
//   - installed: the version currently on disk, or "" if nothing is
//     installed (or version.json is unreadable).
//   - latest: the latest version reported by llama.cpp; only consulted
//     when override is unset and allowUpgrade is true.
//   - def: the well-known default version baked into Kronk.
//
// Returns the version string that should end up installed.
func chooseVersion(override string, allowUpgrade bool, installed string, latest string, def string) string {
	switch {
	case override != "":
		// Matrix row 1: an explicit override always wins.
		return override
	case allowUpgrade:
		// Matrix row 2: track the latest published version.
		return latest
	case installed != "" && versionGreater(installed, def):
		// Matrix row 5: never downgrade past what is on disk.
		return installed
	default:
		// Matrix rows 3-4: pin to the well-known default version.
		return def
	}
}

// versionGreater reports whether v1 is greater than v2.
// Versions are expected to be llama.cpp build tags like "b8937" or plain
// version strings. It strips a single leading non-digit character (covering
// both "b<num>" build tags and "v<num>" version tags) and compares the
// numeric suffixes; when both are purely numeric it does a numeric
// comparison, otherwise it falls back to lexicographic comparison.
func versionGreater(v1, v2 string) bool {
	if v1 == "" || v2 == "" {
		return false
	}

	stripPrefix := func(s string) string {
		if len(s) > 0 && (s[0] < '0' || s[0] > '9') {
			return s[1:]
		}
		return s
	}

	n1 := stripPrefix(v1)
	n2 := stripPrefix(v2)

	if n1 == n2 {
		return false
	}

	// Try numeric comparison for plain numbers (e.g. "8937" vs "7406").
	if i1, e1 := strconv.Atoi(n1); e1 == nil {
		if i2, e2 := strconv.Atoi(n2); e2 == nil {
			return i1 > i2
		}
	}

	return n1 > n2
}

func hasNetwork() bool {
	conn, err := net.DialTimeout("tcp", "8.8.8.8:53", 5*time.Second)
	if err != nil {
		return false
	}

	conn.Close()

	return true
}
