package backend

import (
	"context"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
)

// ModelPath describes the on-disk layout of a single installed model.
// It is the canonical value carried across backends so cross-backend
// code (pool loaders, server handlers) can locate model files without
// knowing which backend produced them.
type ModelPath struct {
	ModelFiles           []string `yaml:"model_files"`
	ProjFile             string   `yaml:"proj_file"`
	MTPFile              string   `yaml:"mtp_file,omitempty"`
	Downloaded           bool     `yaml:"downloaded"`
	Validated            bool     `yaml:"validated"`
	TokenizerFingerprint string   `yaml:"tokenizer_fingerprint,omitempty"`
	FileSizes            []int64  `yaml:"file_sizes,omitempty"`
}

// Catalog manages models for a single backend: locating, listing,
// downloading, and removing the on-disk files. Backend-specific
// concerns (analysis, VRAM estimation, runtime config) stay on the
// concrete catalog type; only operations shared across every backend
// live here.
type Catalog interface {
	// Path returns the directory models for this backend live in.
	Path() string

	// BasePath returns the kronk base directory the catalog was
	// constructed with.
	BasePath() string

	// BuildIndex rebuilds the on-disk index of installed models. When
	// checkSHA is true every file is validated; when false previously
	// validated entries are trusted (used at startup for speed).
	BuildIndex(log applog.Logger, checkSHA bool) error

	// Download fetches a model identified by source — a canonical id,
	// HuggingFace URL, or any other input the backend accepts — and
	// returns its on-disk layout.
	Download(ctx context.Context, log applog.Logger, source string) (ModelPath, error)

	// FullPath returns the on-disk layout of an already-installed
	// model. An error is returned when the model is not present.
	FullPath(modelID string) (ModelPath, error)

	// Remove deletes the supplied model from disk.
	Remove(mp ModelPath, log applog.Logger) error
}
