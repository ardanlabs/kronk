// Package models manages the curated stable-diffusion model bundles supported
// by Kronk's Malina SDK: sd-1.5, sdxl-base-1.0, and flux2-klein-9b.
package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/ardanlabs/kronk/sdk/applog"
	"github.com/ardanlabs/kronk/sdk/tools/backend"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/malina/pkg/download"
	getter "github.com/hashicorp/go-getter"
)

const localFolder = "malina-models"

var (
	// ErrBundleNotFound reports an unknown curated bundle.
	ErrBundleNotFound = errors.New("bundle not found")

	// ErrModelNotFound reports a model that is not installed.
	ErrModelNotFound = errors.New("model not found")

	downloadModelFile = downloadFile
)

// Path contains the files and installation state for a model bundle.
type Path = backend.ModelPath

// Models manages model bundles rooted below a Kronk base directory.
type Models struct {
	basePath   string
	modelsPath string
	mu         sync.Mutex
	index      map[string]backend.ModelPath
}

// New constructs a model catalog at the default base directory.
func New() (*Models, error) { return NewWithPaths("") }

// NewWithPaths constructs a model catalog at basePath.
func NewWithPaths(basePath string) (*Models, error) {
	basePath = defaults.BaseDir(basePath)
	path := filepath.Join(basePath, localFolder)
	if err := os.MkdirAll(path, 0o755); err != nil {
		return nil, fmt.Errorf("creating models directory: %w", err)
	}
	m := Models{basePath: basePath, modelsPath: path, index: map[string]backend.ModelPath{}}
	return &m, nil
}

// Path returns the model root.
func (m *Models) Path() string { return m.modelsPath }

// BasePath returns the Kronk base root.
func (m *Models) BasePath() string { return m.basePath }

// DownloadBundle downloads a curated bundle and returns its manifest.
func (m *Models) DownloadBundle(ctx context.Context, name BundleName) (Manifest, error) {
	return m.DownloadBundleWithProgress(ctx, name, download.ProgressTracker)
}

// DownloadBundleWithProgress downloads a bundle with progress reporting.
func (m *Models) DownloadBundleWithProgress(ctx context.Context, name BundleName, progress getter.ProgressTracker) (Manifest, error) {
	bundle, ok := BundleByName(name)
	if !ok {
		return Manifest{}, fmt.Errorf("%w: %q", ErrBundleNotFound, name)
	}
	return m.downloadBundle(ctx, applog.DiscardLogger, bundle, progress)
}

func (m *Models) downloadBundle(ctx context.Context, log applog.Logger, bundle Bundle, progress getter.ProgressTracker) (Manifest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Manifest{}, fmt.Errorf("download-bundle: %w", err)
	}
	if err := m.buildIndexLocked(); err != nil {
		return Manifest{}, err
	}
	if _, ok := m.index[bundle.Name.String()]; ok {
		return m.loadManifestLocked(bundle)
	}

	stageDir, err := os.MkdirTemp(m.modelsPath, "."+bundle.Name.String()+".stage-")
	if err != nil {
		return Manifest{}, fmt.Errorf("download-bundle: create staging directory: %w", err)
	}
	preserveStage := false
	defer func() {
		if !preserveStage {
			if err := os.RemoveAll(stageDir); err != nil {
				log(ctx, "download-model: unable to remove staging directory", "stage_path", stageDir, "error", err)
			}
		}
	}()

	dir := filepath.Join(m.modelsPath, bundle.Name.String())
	manifest := Manifest{Bundle: bundle.Name, License: bundle.License, Gated: bundle.Gated, Files: make(map[string]string, len(bundle.Files))}
	for _, file := range bundle.Files {
		finalTarget := filepath.Join(dir, file.Filename)
		absoluteTarget, err := filepath.Abs(finalTarget)
		if err != nil {
			return Manifest{}, fmt.Errorf("download-bundle: resolve target: %w", err)
		}
		manifest.Files[string(file.Role)] = absoluteTarget
		if err := downloadModelFile(ctx, file.URL, filepath.Join(stageDir, file.Filename), progress); err != nil {
			return Manifest{}, bundleDownloadError(bundle, file, err)
		}
	}
	if err := validateStagedBundle(bundle, stageDir); err != nil {
		return Manifest{}, err
	}

	if err := ctx.Err(); err != nil {
		return Manifest{}, fmt.Errorf("download-bundle: %w", err)
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Manifest{}, fmt.Errorf("download-bundle: marshal manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(stageDir, ManifestFilename), data, 0o644); err != nil {
		return Manifest{}, fmt.Errorf("download-bundle: write manifest: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return Manifest{}, fmt.Errorf("download-bundle: activate bundle: %w", err)
	}
	preserveStage, err = swapBundle(ctx, log, dir, stageDir)
	if err != nil {
		return Manifest{}, fmt.Errorf("download-bundle: activate bundle: %w", err)
	}
	if err := m.buildIndexLocked(); err != nil {
		return Manifest{}, fmt.Errorf("download-bundle: refresh index: %w", err)
	}
	return m.loadManifestLocked(bundle)
}

func validateStagedBundle(bundle Bundle, stageDir string) error {
	for _, file := range bundle.Files {
		if filepath.Base(file.Filename) != file.Filename {
			return fmt.Errorf("download-bundle: invalid filename %q", file.Filename)
		}
		path := filepath.Join(stageDir, file.Filename)
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("download-bundle: inspect staged file %q: %w", file.Filename, err)
		}
		if !info.Mode().IsRegular() || info.Size() == 0 {
			return fmt.Errorf("download-bundle: staged file %q is not a non-empty regular file", file.Filename)
		}
	}
	return nil
}

func bundleDownloadError(bundle Bundle, file BundleFile, err error) error {
	if bundle.Gated && isAccessDenied(err) {
		return fmt.Errorf("download-bundle %q: %w: accept the Hugging Face license, then set KRONK_HF_TOKEN (or HF_TOKEN) to a token with read access", bundle.Name, err)
	}
	return fmt.Errorf("download-bundle %q: %s: %w", bundle.Name, file.Filename, err)
}

func downloadFile(ctx context.Context, source string, target string, progress getter.ProgressTracker) error {
	separator := "?"
	if strings.Contains(source, "?") {
		separator = "&"
	}
	header := http.Header{}
	token := os.Getenv("KRONK_HF_TOKEN")
	if token == "" {
		token = os.Getenv("HF_TOKEN")
	}
	if token != "" {
		header.Set("Authorization", "Bearer "+token)
	}
	client := getter.Client{
		Ctx:              ctx,
		Src:              source + separator + "archive=false",
		Dst:              target,
		Mode:             getter.ClientModeFile,
		ProgressListener: progress,
		Getters: map[string]getter.Getter{
			"http":  &getter.HttpGetter{Header: header},
			"https": &getter.HttpGetter{Header: header},
		},
	}
	if err := client.Get(); err != nil {
		return fmt.Errorf("download file: %w", err)
	}
	return nil
}

func isAccessDenied(err error) bool {
	return strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "403")
}

// LoadManifest loads a complete named bundle manifest.
func (m *Models) LoadManifest(name BundleName) (Manifest, error) {
	bundle, ok := BundleByName(name)
	if !ok {
		return Manifest{}, fmt.Errorf("load-manifest: %w: %q", ErrBundleNotFound, name)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loadManifestLocked(bundle)
}

func (m *Models) loadManifestLocked(bundle Bundle) (Manifest, error) {
	path := filepath.Join(m.modelsPath, bundle.Name.String(), ManifestFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("load-manifest: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("load-manifest: parse %q: %w", path, err)
	}
	if !m.validManifest(bundle, manifest) {
		return Manifest{}, fmt.Errorf("load-manifest: invalid manifest for bundle %q", bundle.Name)
	}
	return manifest, nil
}

// BuildIndex indexes only valid complete manifests.
func (m *Models) BuildIndex(log applog.Logger, checkSHA bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.buildIndexLocked()
}

func (m *Models) buildIndexLocked() error {
	next := map[string]backend.ModelPath{}
	entries, err := os.ReadDir(m.modelsPath)
	if err != nil {
		return fmt.Errorf("build-index: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name, err := ParseBundleName(entry.Name())
		if err != nil {
			continue
		}
		bundle, ok := BundleByName(name)
		if !ok {
			continue
		}
		if _, err := m.loadManifestLocked(bundle); err != nil {
			continue
		}
		dir := filepath.Join(m.modelsPath, entry.Name())
		mp := backend.ModelPath{Downloaded: true, Validated: true}
		for _, file := range bundle.Files {
			expected := filepath.Join(dir, file.Filename)
			info, err := os.Lstat(expected)
			if err != nil {
				return fmt.Errorf("build-index: inspect validated bundle: %w", err)
			}
			mp.ModelFiles = append(mp.ModelFiles, expected)
			mp.FileSizes = append(mp.FileSizes, info.Size())
		}
		next[bundle.Name.String()] = mp
	}
	m.index = next
	data, err := json.Marshal(next)
	if err != nil {
		return fmt.Errorf("build-index: marshal: %w", err)
	}
	if err := os.WriteFile(filepath.Join(m.modelsPath, "index.json"), data, 0o644); err != nil {
		return fmt.Errorf("build-index: write: %w", err)
	}
	return nil
}

func (m *Models) validManifest(bundle Bundle, manifest Manifest) bool {
	if manifest.Bundle != bundle.Name || len(manifest.Files) != len(bundle.Files) {
		return false
	}
	dir := filepath.Join(m.modelsPath, bundle.Name.String())
	for _, file := range bundle.Files {
		if filepath.Base(file.Filename) != file.Filename {
			return false
		}
		expected := filepath.Join(dir, file.Filename)
		path := manifest.Files[string(file.Role)]
		if path == "" || !samePath(path, expected) {
			return false
		}
		info, err := os.Lstat(expected)
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return false
		}
	}
	return true
}

func samePath(left string, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	return leftErr == nil && rightErr == nil && filepath.Clean(leftAbs) == filepath.Clean(rightAbs)
}

// Download downloads source as a curated bundle name.
func (m *Models) Download(ctx context.Context, log applog.Logger, source string) (Path, error) {
	name, err := ParseBundleName(source)
	if err != nil {
		return Path{}, fmt.Errorf("download: %w: %q", ErrBundleNotFound, source)
	}
	bundle, ok := BundleByName(name)
	if !ok {
		return Path{}, fmt.Errorf("download: %w: %q", ErrBundleNotFound, source)
	}
	if _, err := m.downloadBundle(ctx, log, bundle, download.ProgressTracker); err != nil {
		return Path{}, err
	}
	return m.FullPath(source)
}

// FullPath returns an indexed complete bundle.
func (m *Models) FullPath(modelID string) (Path, error) {
	name, err := ParseBundleName(modelID)
	if err != nil {
		return Path{}, fmt.Errorf("full-path: %w: %s", ErrModelNotFound, modelID)
	}
	bundle, ok := BundleByName(name)
	if !ok {
		return Path{}, fmt.Errorf("full-path: %w: %s", ErrModelNotFound, modelID)
	}
	modelID = bundle.Name.String()

	m.mu.Lock()
	defer m.mu.Unlock()
	if mp, ok := m.index[modelID]; ok {
		return mp, nil
	}
	if err := m.buildIndexLocked(); err != nil {
		return Path{}, err
	}
	if mp, ok := m.index[modelID]; ok {
		return mp, nil
	}
	return Path{}, fmt.Errorf("full-path: %w: %s", ErrModelNotFound, modelID)
}

// Remove deletes a safely contained bundle and rebuilds the index.
func (m *Models) Remove(mp Path, log applog.Logger) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(mp.ModelFiles) == 0 {
		return nil
	}
	if err := m.buildIndexLocked(); err != nil {
		return fmt.Errorf("remove: build index: %w", err)
	}

	var installed backend.ModelPath
	for _, candidate := range m.index {
		if slices.Equal(candidate.ModelFiles, mp.ModelFiles) {
			installed = candidate
			break
		}
	}
	if len(installed.ModelFiles) == 0 {
		return errors.New("remove: model path is not an installed bundle")
	}

	dir := filepath.Clean(filepath.Dir(installed.ModelFiles[0]))
	rel, err := filepath.Rel(m.modelsPath, dir)
	if err != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.Dir(rel) != "." {
		return errors.New("remove: bundle is not a direct child of models path")
	}
	name, err := ParseBundleName(filepath.Base(dir))
	if err != nil {
		return errors.New("remove: bundle is not in catalog")
	}
	if _, ok := BundleByName(name); !ok {
		return errors.New("remove: bundle is not in catalog")
	}
	for _, modelFile := range installed.ModelFiles {
		if filepath.Clean(filepath.Dir(modelFile)) != dir {
			return errors.New("remove: model files do not belong to one bundle")
		}
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove: %w", err)
	}
	if err := m.buildIndexLocked(); err != nil {
		return fmt.Errorf("remove: rebuild index: %w", err)
	}
	return nil
}

func swapBundle(ctx context.Context, log applog.Logger, path string, stagePath string) (bool, error) {
	backupPath := ""
	if _, err := os.Stat(path); err == nil {
		backup, err := os.MkdirTemp(filepath.Dir(path), "."+filepath.Base(path)+".backup-")
		if err != nil {
			return false, fmt.Errorf("swap-bundle: create backup path: %w", err)
		}
		if err := os.Remove(backup); err != nil {
			return false, fmt.Errorf("swap-bundle: prepare backup path: %w", err)
		}
		backupPath = backup
		if err := os.Rename(path, backupPath); err != nil {
			return false, fmt.Errorf("swap-bundle: preserve current bundle: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("swap-bundle: inspect current bundle: %w", err)
	}

	if err := os.Rename(stagePath, path); err != nil {
		if backupPath != "" {
			if rollbackErr := os.Rename(backupPath, path); rollbackErr != nil {
				return true, errors.Join(
					fmt.Errorf("swap-bundle: activate staged bundle %q: %w", stagePath, err),
					fmt.Errorf("swap-bundle: restore backup %q: %w", backupPath, rollbackErr),
				)
			}
		}
		return false, fmt.Errorf("swap-bundle: activate staged bundle: %w", err)
	}

	if backupPath != "" {
		if err := os.RemoveAll(backupPath); err != nil {
			log(ctx, "download-model: unable to remove replaced bundle", "backup_path", backupPath, "error", err)
		}
	}
	return false, nil
}
