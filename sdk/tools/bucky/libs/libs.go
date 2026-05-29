// Package libs provides whisper.cpp library support backed by the
// github.com/ardanlabs/bucky download primitives. It is the whisper
// counterpart to sdk/tools/libs (llama) and is wired into shared
// dispatch code through sdk/tools/backend.
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

	"github.com/ardanlabs/bucky/pkg/download"
	"github.com/ardanlabs/kronk/sdk/applog"
	"github.com/ardanlabs/kronk/sdk/tools/backend"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/downloader"
	"github.com/hashicorp/go-getter"
)

const (
	versionFile = "version.json"
	localFolder = "bucky-libraries"

	// defaultVersion is the well-known working version of whisper.cpp used
	// when no explicit version is provided and AllowUpgrade is false.
	// This pin is owned by Kronk and intentionally overrides whatever
	// github.com/ardanlabs/bucky/pkg/download.DefaultWhisperVersion ships
	// with, because the bucky module may not be bumped in lockstep with
	// upstream whisper.cpp releases.
	defaultVersion = "v1.8.6"
)

// ErrReadOnly is returned by mutating operations on a Libs instance
// whose install path is a user-supplied directory that does not
// contain a version.json file. Such paths are treated as user-managed
// builds that Kronk will load from but never modify.
var ErrReadOnly = errors.New("libs: install path is read-only (no version.json)")

// Logger represents a logger for capturing events.
type Logger = applog.Logger

// VersionTag represents information about the installed version of
// whisper.cpp. It is an alias for backend.VersionTag so cross-backend
// code that dispatches by kind can consume the same value type
// returned by every backend's LibsManager implementation.
type VersionTag = backend.VersionTag

// Combination represents a single supported (architecture, operating
// system, processor) triple for a precompiled whisper.cpp library
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
//  3. An empty or non-existent directory — treated as the libraries
//     root. Installs land in a subfolder of the form
//     <root>/<os>/<arch>/<processor>/.
//
// An empty string falls back to the Kronk default libraries root.
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

// WithProcessor sets the processor / hardware type.
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
// true, Download will track the latest whisper.cpp release published
// by bucky-builder. The default is false, which pins to the
// well-known default version.
func WithAllowUpgrade(allow bool) Option {
	return func(o *Options) {
		o.AllowUpgrade = allow
	}
}

// =============================================================================

// Libs manages the whisper.cpp library system. Each Libs instance
// points at exactly one install directory containing a whisper.cpp
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
// version.json to back-fill the (arch, os, processor) triple for
// fields the caller did not explicitly set.
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

	// rocm has no upstream whisper.cpp bundle (see combinations.go),
	// so a host that auto-detects (or is forced into) rocm would
	// otherwise resolve to a non-existent install directory and bucky
	// would degrade to "no library found". Fall back to vulkan when it
	// is supported on this triple — it works on every ROCm-capable AMD
	// GPU via the RADV ICD shipped by mesa-vulkan-drivers, so the
	// substitution is transparent and keeps GPU-accelerated
	// transcription working on both the `:rocm` and `:all` container
	// images and native ROCm installs. The IsSupported(rocm) guard
	// auto-disables this shim if upstream ever publishes a rocm
	// whisper bundle.
	processorOpt := options.Processor
	if processorOpt == "rocm" {
		if !IsSupported(arch, opSys, "rocm") {
			if IsSupported(arch, opSys, "vulkan") {
				processorOpt = "vulkan"
			} else {
				processorOpt = "cpu"
			}
		}
	}

	processor, err := resolveProcessor(processorOpt, tag.Processor)
	if err != nil {
		return nil, err
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
// system, processor) triple that the upstream whisper.cpp build
// matrix publishes through bucky's download package.
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
// whisper.cpp. The version that gets installed is selected according to
// the following matrix, evaluated in order. The first matching row wins:
//
//	# | Override (WithVersion) | AllowUpgrade | On-disk version          | Action
//	--+------------------------+--------------+--------------------------+-----------------------------
//	1 | set                    | any          | any                      | install the override version
//	2 | unset                  | true         | any                      | install latest from bucky-builder
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

	if !hasNetwork() {
		vt, err := lib.InstalledVersion()
		if err != nil {
			return VersionTag{}, fmt.Errorf("download: no network available: %w", err)
		}
		log(ctx, "download-libraries: no network available, using current version", "current", vt.Version)
		return vt, nil
	}

	installed, _ := lib.InstalledVersion()

	// For matrix row 2 we need the latest version published by
	// bucky-builder. For all other rows the network lookup is
	// unnecessary, so skip it.
	var latest string
	if lib.version == "" && lib.AllowUpgrade {
		v, err := download.WhisperLatestVersion()
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

	log(ctx, "download-libraries: check whisper.cpp installation", "arch", lib.arch, "os", lib.os, "processor", lib.processor, "requested", version, "current", installed.Version)

	if installed.Version == version && installed.Arch == lib.arch && installed.OS == lib.os && installed.Processor == lib.processor {
		log(ctx, "download-libraries: already installed", "version", version)
		return installed, nil
	}

	return lib.downloadInto(ctx, log, lib.path, lib.arch, lib.os, lib.processor, version)
}

// DownloadFor downloads the supplied version into the canonical
// install directory for the supplied (arch, os, processor) triple
// under the libraries Root. If version is empty, the
// bucky-baked-in default is used.
func (lib *Libs) DownloadFor(ctx context.Context, log Logger, arch string, opSys string, processor string, version string) (VersionTag, error) {
	if lib.readOnly {
		return VersionTag{}, fmt.Errorf("libs: download-for: %w", ErrReadOnly)
	}
	if !IsSupported(arch, opSys, processor) {
		return VersionTag{}, fmt.Errorf("libs: download-for: unsupported combination arch=%s os=%s processor=%s", arch, opSys, processor)
	}

	if version == "" {
		version = download.DefaultWhisperVersion
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

// downloadInto fetches the supplied whisper.cpp version into path
// using bucky's download package, then writes a version.json
// alongside so subsequent InstalledVersion calls can report the
// installed metadata.
func (lib *Libs) downloadInto(ctx context.Context, log Logger, path string, arch string, opSys string, processor string, version string) (VersionTag, error) {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return VersionTag{}, fmt.Errorf("download-into: unable to create destination: %w", err)
	}

	tempPath := filepath.Join(path, "temp")
	if err := os.MkdirAll(tempPath, 0o755); err != nil {
		return VersionTag{}, fmt.Errorf("download-into: unable to create temp: %w", err)
	}

	progress := func(src string, currentSize int64, totalSize int64, mbPerSec float64, complete bool) {
		log(ctx, fmt.Sprintf("\r\x1b[Kdownload-libraries: Downloading %s... %d MB of %d MB (%.2f MB/s)", src, currentSize/(1000*1000), totalSize/(1000*1000), mbPerSec))
	}

	pr := downloader.NewProgressReader(progress, downloader.SizeIntervalMB10)

	if err := download.GetWithContext(ctx, arch, opSys, processor, version, tempPath, getter.ProgressTracker(pr)); err != nil {
		os.RemoveAll(tempPath)
		return VersionTag{}, fmt.Errorf("download-into: unable to install whisper.cpp: %w", err)
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
//   - latest: the latest version reported by bucky-builder; only
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

// versionGreater reports whether v1 is greater than v2. Versions are
// expected to be whisper.cpp release tags like "v1.8.4". It strips a
// single leading non-digit character (covering "v<num>" tags) and
// compares the suffixes; when both are purely numeric it does a
// numeric comparison, otherwise it falls back to lexicographic
// comparison (which is correct for same-shape dotted versions).
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
