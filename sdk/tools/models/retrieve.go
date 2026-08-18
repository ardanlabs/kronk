package models

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/sdk/tools/backend"
	"go.yaml.in/yaml/v2"
)

// ErrModelNotFound is returned when a requested model is not installed.
var ErrModelNotFound = errors.New("model not found")

// File provides information about a model.
type File struct {
	ID                   string
	OwnedBy              string
	ModelFamily          string
	TokenizerFingerprint string
	Size                 int64
	Modified             time.Time
	Validated            bool
	HasProjection        bool
	HasMTP               bool
}

// Files returns all the models in the model directory.
func (m *Models) Files() ([]File, error) {
	var list []File

	index := m.loadIndex()

	for indexID, mp := range index {
		if len(mp.ModelFiles) == 0 {
			continue
		}

		var totalSize int64
		var modified time.Time

		for _, f := range mp.ModelFiles {
			info, err := os.Stat(f)
			if err != nil {
				return nil, fmt.Errorf("stat: %w", err)
			}

			totalSize += info.Size()
			if info.ModTime().After(modified) {
				modified = info.ModTime()
			}
		}

		modelPath := strings.TrimPrefix(mp.ModelFiles[0], m.modelsPath)
		modelPath = strings.TrimPrefix(modelPath, string(filepath.Separator))
		parts := strings.Split(modelPath, string(filepath.Separator))

		var ownedBy string
		var modelFamily string

		if len(parts) > 0 {
			ownedBy = parts[0]
		}

		if len(parts) > 1 {
			modelFamily = parts[1]
		}

		_, modelID := splitProviderID(indexID)
		if modelID == "" {
			// TODO: Remove the legacy bare-key fallback after 2026-09-18.
			modelID = indexID
		}

		mf := File{
			ID:                   modelID,
			OwnedBy:              ownedBy,
			ModelFamily:          modelFamily,
			TokenizerFingerprint: mp.TokenizerFingerprint,
			Size:                 totalSize,
			Modified:             modified,
			Validated:            mp.Validated,
			HasProjection:        mp.ProjFile != "",
			HasMTP:               mp.MTPFile != "",
		}

		list = append(list, mf)
	}

	slices.SortFunc(list, func(a, b File) int {
		aID := canonicalID(a.OwnedBy, a.ID)
		bID := canonicalID(b.OwnedBy, b.ID)
		return strings.Compare(strings.ToLower(aID), strings.ToLower(bID))
	})

	return list, nil
}

// retrieveFile finds the model and returns the model file information.
func (m *Models) retrieveFile(modelID string) (File, error) {
	if modelID == "" {
		return File{}, fmt.Errorf("retrieve-file: missing model id")
	}
	if _, err := ParseModelID(modelID); err != nil {
		return File{}, fmt.Errorf("retrieve-file: %w", err)
	}

	key, mp, exists := lookupIndex(m.loadIndex(), modelID)
	if !exists {
		return File{}, fmt.Errorf("retrieve-file: unable to retrieve path: %w: %q", ErrModelNotFound, modelID)
	}

	if len(mp.ModelFiles) == 0 {
		return File{}, fmt.Errorf("retrieve-file: no model files found")
	}

	var totalSize int64
	var modified time.Time

	for _, f := range mp.ModelFiles {
		info, err := os.Stat(f)
		if err != nil {
			return File{}, fmt.Errorf("stat: %w", err)
		}

		totalSize += info.Size()
		if info.ModTime().After(modified) {
			modified = info.ModTime()
		}
	}

	modelPath := strings.TrimPrefix(mp.ModelFiles[0], m.modelsPath)
	modelPath = strings.TrimPrefix(modelPath, string(filepath.Separator))
	parts := strings.Split(modelPath, string(filepath.Separator))

	var ownedBy string
	var modelFamily string

	if len(parts) > 0 {
		ownedBy = parts[0]
	}

	if len(parts) > 1 {
		modelFamily = parts[1]
	}

	mf := File{
		ID:          key,
		OwnedBy:     ownedBy,
		ModelFamily: modelFamily,
		Size:        totalSize,
		Modified:    modified,
	}

	return mf, nil
}

// =============================================================================

// FileInfo provides all the model details.
type FileInfo struct {
	ID          string
	Object      string
	ModelFamily string
	Size        int64
	Created     int64
	OwnedBy     string
}

// FileInformation provides details for the specified model.
func (m *Models) FileInformation(modelID string) (FileInfo, error) {
	mf, err := m.retrieveFile(modelID)
	if err != nil {
		return FileInfo{}, fmt.Errorf("retrieve-info: unable to get model file information: %w", err)
	}

	mi := FileInfo{
		ID:          modelID,
		Object:      "model",
		ModelFamily: mf.ModelFamily,
		Size:        mf.Size,
		Created:     mf.Modified.UnixMilli(),
		OwnedBy:     mf.OwnedBy,
	}

	return mi, nil
}

// =============================================================================

// Path returns file path information about a model. It is an alias for
// backend.ModelPath so cross-backend code can consume the same value
// type returned by every backend's Catalog implementation.
type Path = backend.ModelPath

// FullPath locates the physical location on disk and returns the full path.
//
// The index is keyed by canonical provider/modelID. A profile-qualified
// provider/modelID/profile resolves to the same physical model.
func (m *Models) FullPath(modelID string) (Path, error) {
	if _, err := ParseModelID(modelID); err != nil {
		return Path{}, fmt.Errorf("retrieve-path: %w", err)
	}

	_, mp, exists := lookupIndex(m.loadIndex(), modelID)
	if exists {
		return mp, nil
	}

	return Path{}, fmt.Errorf("retrieve-path: %w: %q", ErrModelNotFound, modelID)
}

// LookupFile resolves a model identifier to its catalog File entry using the
// same rules as FullPath.
func (m *Models) LookupFile(modelID string) (File, bool) {
	id, err := ParseModelID(modelID)
	if err != nil {
		return File{}, false
	}

	files, err := m.Files()
	if err != nil {
		return File{}, false
	}

	byID := make(map[string]File, len(files))
	for _, f := range files {
		byID[canonicalID(f.OwnedBy, f.ID)] = f
	}

	_, _, exists := lookupIndex(m.loadIndex(), modelID)
	if !exists {
		return File{}, false
	}

	f, exists := byID[id.Base()]
	return f, exists
}

// MustFullPath finds a model and panics if the model was not found. This
// should only be used for testing.
func (m *Models) MustFullPath(modelID string) Path {
	fi, err := m.FullPath(modelID)
	if err != nil {
		panic(err.Error())
	}

	return fi
}

// fullPathLookupKeys returns the physical index keys for modelID.
func fullPathLookupKeys(modelID string) []string {
	id, err := ParseModelID(modelID)
	if err != nil {
		return nil
	}

	// TODO: Remove the bare model key after 2026-09-18. It keeps indexes
	// created before provider-qualified keys readable during the transition.
	return []string{id.Base(), id.Model}
}

func lookupIndex(index map[string]Path, modelID string) (string, Path, bool) {
	for _, key := range fullPathLookupKeys(modelID) {
		if mp, exists := index[key]; exists {
			return key, mp, true
		}
	}

	return "", Path{}, false
}

// =============================================================================

// loadIndex returns the catalog index. Existing callers treat a missing or
// unreadable index as an empty catalog.
func (m *Models) loadIndex() map[string]Path {
	index, err := m.readIndex()
	if err != nil {
		return make(map[string]Path)
	}

	return index
}

// readIndex returns the catalog index and preserves read or decoding errors
// for operations that cannot safely represent them as an empty catalog.
func (m *Models) readIndex() (map[string]Path, error) {
	m.biMutex.Lock()
	defer m.biMutex.Unlock()

	indexPath := filepath.Join(m.modelsPath, indexFile)

	data, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf("read index: %w", err)
	}

	var index map[string]Path
	if err := yaml.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("decode index: %w", err)
	}

	return index, nil
}
