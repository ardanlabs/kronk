package backend

import (
	"context"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
)

// Combination is a single supported (architecture, operating system,
// processor) triple a backend can install a precompiled native library
// bundle for.
type Combination struct {
	Arch      string `json:"arch"`
	OS        string `json:"os"`
	Processor string `json:"processor"`
}

// VersionTag is the version metadata recorded for an installed library
// bundle. Latest is populated only by lookup-style calls that consult
// the upstream catalog; installed-version reads leave it blank.
type VersionTag struct {
	Version   string `json:"version"`
	Arch      string `json:"arch"`
	OS        string `json:"os"`
	Processor string `json:"processor"`
	Latest    string `json:"-"`
}

// LibsManager installs and inventories the precompiled native library
// bundle a backend depends on (llama.cpp, whisper.cpp, …).
//
// All triple arguments are passed as plain strings so callers do not
// have to import a backend's underlying typed enums. The supported
// values are whatever SupportedCombinations reports for that backend.
type LibsManager interface {
	// LibsPath returns the directory the active install lives in. This
	// is the path the runtime should load shared libraries from.
	LibsPath() string

	// Arch returns the active architecture identifier.
	Arch() string

	// OS returns the active operating system identifier.
	OS() string

	// Processor returns the active processor / hardware identifier.
	Processor() string

	// ReadOnly reports whether the active install path is a
	// user-managed directory mutations are not permitted on.
	ReadOnly() bool

	// SupportedCombinations returns every (arch, os, processor) triple
	// the backend can install a precompiled bundle for.
	SupportedCombinations() []Combination

	// IsSupported reports whether the supplied triple is part of
	// SupportedCombinations.
	IsSupported(arch string, opSys string, processor string) bool

	// InstalledVersion returns the version metadata of the install
	// covering the active triple. An error is returned when nothing is
	// installed at that location.
	InstalledVersion() (VersionTag, error)

	// InstalledFor returns the version metadata of the install matching
	// the supplied triple under the libraries root.
	InstalledFor(arch string, opSys string, processor string) (VersionTag, error)

	// List returns one VersionTag per installed (arch, os, processor)
	// bundle the backend knows about.
	List() ([]VersionTag, error)

	// Download resolves the right version for the active triple
	// following the backend's own version-selection policy and lays it
	// down on disk. The returned tag describes what is now installed.
	Download(ctx context.Context, log applog.Logger) (VersionTag, error)

	// DownloadFor lays down a bundle for the supplied triple. An empty
	// version means "use the backend's default".
	DownloadFor(ctx context.Context, log applog.Logger, arch string, opSys string, processor string, version string) (VersionTag, error)

	// Remove deletes the install for the supplied triple. Removing a
	// triple that is not installed must not return an error.
	Remove(arch string, opSys string, processor string) error
}
