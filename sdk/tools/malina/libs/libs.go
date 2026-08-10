// Package libs provides stable-diffusion.cpp library support backed by the
// github.com/ardanlabs/malina download primitives. It is the stable-diffusion
// counterpart to sdk/tools/libs (llama) and is wired into shared
// dispatch code through sdk/tools/backend.
package libs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/ardanlabs/kronk/sdk/applog"
	"github.com/ardanlabs/kronk/sdk/tools/backend"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/downloader"
	"github.com/ardanlabs/malina/pkg/download"
	"github.com/hashicorp/go-getter"
)

const (
	versionFile = "version.json"
	localFolder = "malina-libraries"

	// defaultVersion is the well-known working version of stable-diffusion.cpp used
	// when no explicit version is provided and AllowUpgrade is false.
	// This intentionally follows Malina's stable-diffusion.cpp ABI pin.
	defaultVersion = download.DefaultSDVersion
)

// ErrReadOnly is returned by mutating operations on a Libs instance
// whose install path is a user-supplied directory that does not
// contain a version.json file. Such paths are treated as user-managed
// builds that Kronk will load from but never modify.
var ErrReadOnly = errors.New("libs: install path is read-only (no version.json)")

var (
	networkAvailable  = hasNetwork
	latestVersion     = download.SDLatestVersion
	downloadLibraries = download.GetWithContext
)

// Logger represents a logger for capturing events.
type Logger = applog.Logger

// VersionTag represents information about the installed version of
// stable-diffusion.cpp. It is an alias for backend.VersionTag so cross-backend
// code that dispatches by kind can consume the same value type
// returned by every backend's LibsManager implementation.
type VersionTag = backend.VersionTag

// Combination represents a single supported (architecture, operating
// system, processor) triple for a precompiled stable-diffusion.cpp library
// bundle. It is an alias for backend.Combination so the same value
// type travels across every backend that satisfies
// backend.LibsManager.
type Combination = backend.Combination

// =============================================================================

// Options represents the configuration options for Libs.
type Options struct {
	LibPath      string
	BasePath     string
	Arch         string
	OS           string
	Processor    string
	Version      string
	AllowUpgrade bool
	detect       *detectOptions
}

// Option is a function that configures Options.
type Option func(*Options)

// WithBasePath sets the base path for library installation.
func WithBasePath(basePath string) Option {
	return func(o *Options) {
		o.BasePath = basePath
	}
}

// WithLibPath sets the path Kronk should load libraries from. The
// supplied path is interpreted as one of three things:
//
//  1. A directory that already contains a version.json — used directly
//     as the install location and the (arch, os, processor) triple
//     recorded in that file is adopted unless the caller overrides it.
//  2. A non-empty directory without a version.json — treated as a
//     user-managed read-only build. Mutating operations return
//     ErrReadOnly.
//  3. An empty or non-existent directory — used directly as a writable
//     custom install location.
//
// Any non-empty path is authoritative and bypasses automatic host runtime
// compatibility selection. An empty string falls back to the Kronk default
// libraries root, where installs land in
// <root>/<os>/<arch>/<processor>/.
func WithLibPath(libPath string) Option {
	return func(o *Options) {
		o.LibPath = libPath
	}
}

// WithArch sets the architecture.
func WithArch(arch string) Option {
	return func(o *Options) {
		o.Arch = arch
	}
}

// WithOS sets the operating system.
func WithOS(opSys string) Option {
	return func(o *Options) {
		o.OS = opSys
	}
}

// WithProcessor sets the processor / hardware type. Explicit supported
// processors, including ROCm, are authoritative.
func WithProcessor(processor string) Option {
	return func(o *Options) {
		o.Processor = processor
	}
}

// WithVersion sets a specific version to download instead of the
// default.
func WithVersion(version string) Option {
	return func(o *Options) {
		o.Version = version
	}
}

// WithAllowUpgrade sets whether library upgrades are allowed. When
// true, Download will track the latest leejet stable-diffusion.cpp release.
// The default is false, which pins to the
// well-known default version.
func WithAllowUpgrade(allow bool) Option {
	return func(o *Options) {
		o.AllowUpgrade = allow
	}
}

// =============================================================================

// Libs manages the stable-diffusion.cpp library system. Each Libs instance
// points at exactly one install directory containing a stable-diffusion.cpp
// library bundle. The directory is resolved at construction time
// according to the rules described on WithLibPath and may be one of:
//
//   - A per-triple subfolder under the libraries root (the default).
//   - A user-supplied directory that already contains a version.json.
//   - A user-supplied read-only directory (see ReadOnly).
//
// Other installs for different (arch, os, processor) triples on the
// same libraries root are discoverable through List, Remove, and
// InstalledFor.
type Libs struct {
	root         string
	path         string
	arch         string
	os           string
	processor    string
	version      string
	readOnly     bool
	AllowUpgrade bool
}

// New constructs a Libs with system defaults and applies any provided
// options. It resolves the install location and reads any existing
// version.json to back-fill the (arch, os, processor) triple for fields the
// caller did not explicitly set. Under the default root, New validates
// metadata, environment, and detected hardware preferences against Malina's
// published artifacts and the current host. A non-empty library path and an
// explicitly selected supported Malina processor remain authoritative.
func New(opts ...Option) (*Libs, error) {
	var options Options
	for _, opt := range opts {
		opt(&options)
	}

	root, path, readOnly, err := resolvePaths(options.BasePath, options.LibPath)
	if err != nil {
		return nil, err
	}

	// Apply the resolution precedence for each triple field:
	//   1. explicit Option (WithArch/WithOS/WithProcessor)
	//   2. existing version.json at the resolved install path
	//   3. KRONK_* environment variable / runtime detection
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

	explicitTriple := options.Arch != "" || options.OS != "" || options.Processor != ""
	if explicitTriple && !IsSupported(arch, opSys, processor) {
		return nil, fmt.Errorf("libs: unsupported explicit combination arch=%s os=%s processor=%s", arch, opSys, processor)
	}

	// Explicit tuples and caller-supplied paths are authoritative. Ambient
	// detection may safely fall back from an unusable GPU runtime.
	if options.LibPath == "" && !explicitTriple {
		ctx := context.Background()
		var log Logger
		probes := defaultRuntimeProbes()
		if options.detect != nil {
			if options.detect.ctx != nil {
				ctx = options.detect.ctx
			}
			log = options.detect.log
			if options.detect.probes != nil {
				probes = *options.detect.probes
			}
		}

		preferred := processor
		var reason string
		var supported bool
		processor, reason, supported = selectRuntime(ctx, arch, opSys, preferred, probes)
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("libs: detect runtime: %w", err)
		}
		if !supported {
			return nil, fmt.Errorf("libs: no supported automatic runtime for arch=%s os=%s", arch, opSys)
		}
		if log != nil {
			log(ctx, "select malina runtime", "preferred", preferred, "selected", processor, "reason", reason)
		}
	}

	// If the caller did not point at a specific install directory, the
	// final install path is <root>/<os>/<arch>/<processor>/ for the
	// resolved triple.
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
	return lib.arch
}

// OS returns the current operating system being used.
func (lib *Libs) OS() string {
	return lib.os
}

// Processor returns the hardware system being used.
func (lib *Libs) Processor() string {
	return lib.processor
}

// ReadOnly reports whether the resolved install path is a
// user-supplied directory without a version.json. Mutating operations
// will return ErrReadOnly when this is true.
func (lib *Libs) ReadOnly() bool {
	return lib.readOnly
}

// SupportedCombinations returns every (architecture, operating
// system, processor) triple that the upstream stable-diffusion.cpp build
// matrix publishes through malina's download package.
func (lib *Libs) SupportedCombinations() []Combination {
	return SupportedCombinations()
}

// IsSupported reports whether the supplied triple is part of
// SupportedCombinations.
func (lib *Libs) IsSupported(arch string, opSys string, processor string) bool {
	return IsSupported(arch, opSys, processor)
}

// InstalledVersion returns the version metadata of the install
// covering the active triple. An error is returned when nothing is
// installed at that location.
func (lib *Libs) InstalledVersion() (VersionTag, error) {
	return readVersionFile(lib.path)
}

// InstalledFor returns the version metadata of the install matching
// the supplied triple under the libraries Root.
func (lib *Libs) InstalledFor(arch string, opSys string, processor string) (VersionTag, error) {
	if !IsSupported(arch, opSys, processor) {
		return VersionTag{}, fmt.Errorf("libs: installed-for: unsupported combination arch=%s os=%s processor=%s", arch, opSys, processor)
	}
	return readVersionFile(installPathFor(lib.root, arch, opSys, processor))
}

// List walks the libraries Root and returns one VersionTag per
// installed (arch, os, processor) bundle whose version.json could be
// read. Bundles without a readable version.json are skipped silently.
// The returned slice is sorted by (os, arch, processor) for stable
// presentation.
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

// Download performs a complete workflow for downloading and installing
// stable-diffusion.cpp. The version that gets installed is selected according to
// the following matrix, evaluated in order. The first matching row wins:
//
//	# | Override (WithVersion) | AllowUpgrade | On-disk version          | Action
//	--+------------------------+--------------+--------------------------+-----------------------------
//	1 | set                    | any          | any                      | install the override version
//	2 | unset                  | true         | any                      | install latest from stable-diffusion.cpp
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
	log = normalizeLogger(log)
	if err := ctx.Err(); err != nil {
		return VersionTag{}, fmt.Errorf("download-libraries: %w", err)
	}
	if lib.readOnly {
		tag, err := lib.InstalledVersion()
		if err != nil {
			return VersionTag{}, fmt.Errorf("libs: read-only install path has no version.json: %w", ErrReadOnly)
		}
		log(ctx, "download-libraries: read-only install path, treating as fixed", "current", tag.Version)
		return tag, nil
	}

	if !networkAvailable(ctx) {
		if err := ctx.Err(); err != nil {
			return VersionTag{}, fmt.Errorf("download-libraries: network check: %w", err)
		}
		vt, err := lib.InstalledVersion()
		if err != nil {
			return VersionTag{}, fmt.Errorf("download: no network available: %w", err)
		}
		log(ctx, "download-libraries: no network available, using current version", "current", vt.Version)
		return vt, nil
	}
	if err := ctx.Err(); err != nil {
		return VersionTag{}, fmt.Errorf("download-libraries: network check: %w", err)
	}

	installed, _ := lib.InstalledVersion()

	// For matrix row 2 we need the latest version published by
	// leejet stable-diffusion.cpp releases. For all other rows the network lookup is
	// unnecessary, so skip it.
	var latest string
	if lib.version == "" && lib.AllowUpgrade {
		v, err := latestVersion()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return VersionTag{}, fmt.Errorf("download-libraries: retrieve latest version: %w", ctxErr)
		}
		if err != nil {
			if installed.Version == "" {
				return VersionTag{}, fmt.Errorf("download-libraries: error retrieving latest version: %w", err)
			}

			log(ctx, "download-libraries: unable to check latest version, using installed version", "arch", lib.arch, "os", lib.os, "processor", lib.processor, "current", installed.Version)
			return installed, nil
		}
		latest = v
	}

	version := chooseVersion(lib.version, lib.AllowUpgrade, installed.Version, latest, defaultVersion)

	log(ctx, "download-libraries: check stable-diffusion.cpp installation", "arch", lib.arch, "os", lib.os, "processor", lib.processor, "requested", version, "current", installed.Version)

	if installed.Version == version && installed.Arch == lib.arch && installed.OS == lib.os && installed.Processor == lib.processor {
		log(ctx, "download-libraries: already installed", "version", version)
		return installed, nil
	}

	return lib.downloadInto(ctx, log, lib.path, lib.arch, lib.os, lib.processor, version)
}

// DownloadFor downloads the supplied version into the canonical
// install directory for the supplied (arch, os, processor) triple
// under the libraries Root. If version is empty, the Kronk-pinned
// defaultVersion is used unless a newer version is already installed,
// in which case that newer version is kept. This mirrors the llama
// backend's DownloadFor behavior.
func (lib *Libs) DownloadFor(ctx context.Context, log Logger, arch string, opSys string, processor string, version string) (VersionTag, error) {
	log = normalizeLogger(log)
	if err := ctx.Err(); err != nil {
		return VersionTag{}, fmt.Errorf("download-for: %w", err)
	}
	if lib.readOnly {
		return VersionTag{}, fmt.Errorf("libs: download-for: %w", ErrReadOnly)
	}
	if !IsSupported(arch, opSys, processor) {
		return VersionTag{}, fmt.Errorf("libs: download-for: unsupported combination arch=%s os=%s processor=%s", arch, opSys, processor)
	}

	if version == "" {
		installed, _ := lib.InstalledFor(arch, opSys, processor)
		if installed.Version != "" && versionGreater(installed.Version, defaultVersion) {
			version = installed.Version
		} else {
			version = defaultVersion
		}
	}

	return lib.downloadInto(ctx, log, installPathFor(lib.root, arch, opSys, processor), arch, opSys, processor, version)
}

// Remove deletes the install directory matching the supplied triple
// under the libraries Root. Empty parent directories (the arch and os
// folders) are removed as well, but the libraries Root is preserved.
// Removing an install that does not exist is not an error.
func (lib *Libs) Remove(arch string, opSys string, processor string) error {
	if lib.readOnly {
		return fmt.Errorf("libs: remove: %w", ErrReadOnly)
	}
	if !IsSupported(arch, opSys, processor) {
		return fmt.Errorf("libs: remove: unsupported combination arch=%s os=%s processor=%s", arch, opSys, processor)
	}

	path := installPathFor(lib.root, arch, opSys, processor)

	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("libs: remove: %w", err)
	}

	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("libs: remove: %w", err)
	}

	parent := filepath.Dir(path)
	for parent != lib.root && parent != filepath.Dir(parent) {
		entries, err := os.ReadDir(parent)
		if err != nil || len(entries) > 0 {
			break
		}
		if err := os.Remove(parent); err != nil {
			break
		}
		parent = filepath.Dir(parent)
	}

	return nil
}

// =============================================================================

// downloadInto fetches the supplied stable-diffusion.cpp version into path
// using malina's download package, then writes a version.json
// alongside so subsequent InstalledVersion calls can report the
// installed metadata.
func (lib *Libs) downloadInto(ctx context.Context, log Logger, path string, arch string, opSys string, processor string, version string) (VersionTag, error) {
	log = normalizeLogger(log)
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return VersionTag{}, fmt.Errorf("download-into: unable to create parent: %w", err)
	}

	stagePath, err := os.MkdirTemp(parent, "."+filepath.Base(path)+".stage-")
	if err != nil {
		return VersionTag{}, fmt.Errorf("download-into: unable to create staging directory: %w", err)
	}
	preserveStage := false
	defer func() {
		if !preserveStage {
			if err := os.RemoveAll(stagePath); err != nil {
				log(ctx, "download-libraries: unable to remove staging directory", "stage_path", stagePath, "error", err)
			}
		}
	}()

	progress := func(src string, currentSize int64, totalSize int64, mbPerSec float64, complete bool) {
		log(ctx, fmt.Sprintf("\r\x1b[Kdownload-libraries: Downloading %s... %d MB of %d MB (%.2f MB/s)", src, currentSize/(1000*1000), totalSize/(1000*1000), mbPerSec))
	}

	pr := downloader.NewProgressReader(progress, downloader.SizeIntervalMB10)

	err = downloadLibraries(ctx, arch, opSys, processor, version, stagePath, getter.ProgressTracker(pr))
	if ctxErr := ctx.Err(); ctxErr != nil {
		return VersionTag{}, fmt.Errorf("download-into: download libraries: %w", ctxErr)
	}
	if err != nil {
		return VersionTag{}, fmt.Errorf("download-into: unable to install stable-diffusion.cpp: %w", err)
	}

	if err := writeVersionFile(stagePath, version, arch, opSys, processor); err != nil {
		return VersionTag{}, fmt.Errorf("download-into: unable to create version file: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return VersionTag{}, fmt.Errorf("download-into: activate libraries: %w", err)
	}

	preserveStage, err = swapInstall(ctx, log, path, stagePath)
	if err != nil {
		return VersionTag{}, fmt.Errorf("download-into: unable to install staged libraries: %w", err)
	}

	return readVersionFile(path)
}

func swapInstall(ctx context.Context, log Logger, path string, stagePath string) (bool, error) {
	backupPath := ""
	if _, err := os.Stat(path); err == nil {
		backup, err := os.MkdirTemp(filepath.Dir(path), "."+filepath.Base(path)+".backup-")
		if err != nil {
			return false, fmt.Errorf("swap-install: create backup path: %w", err)
		}
		if err := os.Remove(backup); err != nil {
			return false, fmt.Errorf("swap-install: prepare backup path: %w", err)
		}
		backupPath = backup
		if err := os.Rename(path, backupPath); err != nil {
			return false, fmt.Errorf("swap-install: preserve current install: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("swap-install: inspect current install: %w", err)
	}

	if err := os.Rename(stagePath, path); err != nil {
		if backupPath != "" {
			if rollbackErr := os.Rename(backupPath, path); rollbackErr != nil {
				return true, errors.Join(
					fmt.Errorf("swap-install: activate staged install %q: %w", stagePath, err),
					fmt.Errorf("swap-install: restore backup %q: %w", backupPath, rollbackErr),
				)
			}
		}
		return false, fmt.Errorf("swap-install: activate staged install: %w", err)
	}

	if backupPath != "" {
		if err := os.RemoveAll(backupPath); err != nil {
			log(ctx, "download-libraries: unable to remove replaced installation", "backup_path", backupPath, "error", err)
		}
	}
	return false, nil
}

func writeVersionFile(path string, version string, arch string, opSys string, processor string) error {
	tag := VersionTag{
		Version:   version,
		Arch:      arch,
		OS:        opSys,
		Processor: processor,
	}

	data, err := json.Marshal(tag)
	if err != nil {
		return fmt.Errorf("write-version-file: marshalling version info: %w", err)
	}

	if err := os.WriteFile(filepath.Join(path, versionFile), data, 0o644); err != nil {
		return fmt.Errorf("write-version-file: writing version info: %w", err)
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
	if download.VersionIsValid(tag.Version) != nil || !IsSupported(tag.Arch, tag.OS, tag.Processor) {
		return VersionTag{}, errors.New("installed-version: incomplete or unsupported version info")
	}

	return tag, nil
}

// =============================================================================

func installPathFor(root string, arch string, opSys string, processor string) string {
	return filepath.Join(root, opSys, arch, processor)
}

func resolvePaths(basePath string, libPath string) (root string, path string, readOnly bool, err error) {
	defaultRoot := filepath.Join(defaults.BaseDir(basePath), localFolder)

	if libPath == "" {
		return defaultRoot, defaultRoot, false, nil
	}
	libPath = filepath.Clean(libPath)

	if _, err := os.Stat(filepath.Join(libPath, versionFile)); err == nil {
		if _, err := readVersionFile(libPath); err != nil {
			return "", "", false, fmt.Errorf("libs: resolve-paths: invalid version file: %w", err)
		}
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

func resolveArch(opt string, fallback string) (string, error) {
	if opt != "" {
		if _, err := download.ParseArch(opt); err != nil {
			return "", fmt.Errorf("libs: resolve-arch: %w", err)
		}
		return opt, nil
	}
	if fallback != "" {
		if _, err := download.ParseArch(fallback); err == nil {
			return fallback, nil
		}
	}
	a, err := defaults.Arch("")
	if err != nil {
		return "", err
	}
	return a.String(), nil
}

func resolveOS(opt string, fallback string) (string, error) {
	if opt != "" {
		if _, err := download.ParseOS(opt); err != nil {
			return "", fmt.Errorf("libs: resolve-os: %w", err)
		}
		return opt, nil
	}
	if fallback != "" {
		if _, err := download.ParseOS(fallback); err == nil {
			return fallback, nil
		}
	}
	o, err := defaults.OS("")
	if err != nil {
		return "", err
	}
	return o.String(), nil
}

func resolveProcessor(opt string, fallback string) (string, error) {
	if opt != "" {
		if _, err := download.ParseProcessor(opt); err != nil {
			return "", fmt.Errorf("libs: resolve-processor: %w", err)
		}
		return opt, nil
	}
	if fallback != "" {
		if _, err := download.ParseProcessor(fallback); err == nil {
			return fallback, nil
		}
	}
	p, err := defaults.Processor("")
	if err != nil {
		return "", err
	}
	return p.String(), nil
}

// =============================================================================

// chooseVersion implements the Download policy matrix as a pure function.
// See Download for the full matrix and exception rules. Inputs:
//
//   - override: explicit version pin (lib.version), or "" if unset.
//   - allowUpgrade: whether to track the latest published version.
//   - installed: the version currently on disk, or "" if nothing is
//     installed (or version.json is unreadable).
//   - latest: the latest version reported by stable-diffusion.cpp; only
//     consulted when override is unset and allowUpgrade is true.
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

// versionGreater compares the numeric build component in stable-diffusion.cpp
// tags such as master-813-bfbef5b.
func versionGreater(v1, v2 string) bool {
	b1, ok1 := releaseBuild(v1)
	b2, ok2 := releaseBuild(v2)
	if !ok1 || !ok2 {
		return false
	}
	return b1 > b2
}

var releaseTagRE = regexp.MustCompile(`^[^-]+-([0-9]+)(?:-|$)`)

func releaseBuild(version string) (int, bool) {
	match := releaseTagRE.FindStringSubmatch(version)
	if len(match) != 2 {
		return 0, false
	}
	build, err := strconv.Atoi(match[1])
	return build, err == nil
}

func normalizeLogger(log Logger) Logger {
	if log == nil {
		return applog.DiscardLogger
	}
	return log
}

// hasNetwork reports whether Kronk can reach the internet. It issues a real
// HTTP request through a client that honors HTTP_PROXY/HTTPS_PROXY, so the
// probe succeeds in proxy-only environments where a raw outbound TCP dial is
// blocked. Setting KRONK_SKIP_NETWORK_CHECK bypasses the probe for unusual
// setups.
func hasNetwork(ctx context.Context) bool {
	if os.Getenv("KRONK_SKIP_NETWORK_CHECK") != "" {
		return true
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, "https://github.com", nil)
	if err != nil {
		return false
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}

	resp.Body.Close()

	return true
}
