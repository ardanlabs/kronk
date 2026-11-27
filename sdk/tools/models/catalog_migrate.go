package models

import (
	"fmt"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v2"
)

// migrateCatalog owns the supported upgrade paths to catalogVersion:
// unversioned/v0/v1/v2 convert MTP artifact fields, then v3/v3.1 convert
// drafting capabilities. Earlier versions share the same legacy artifact
// representation, so they do not need separate intermediate files or writes.
// The caller saves only after the entire migration and validation succeed.
func migrateCatalog(data []byte, cat *Catalog) (bool, error) {
	switch cat.Version {
	case "", "0", "1", "2", "3", "3.1", catalogVersion:
	default:
		return false, fmt.Errorf("unsupported catalog version %q", cat.Version)
	}
	if cat.Models == nil {
		cat.Models = map[string]CatalogEntry{}
	}
	migrated := cat.Version != catalogVersion
	switch cat.Version {
	case "", "0", "1", "2":
		if err := migrateCatalogDrafts(data, cat); err != nil {
			return false, fmt.Errorf("migrate draft artifacts: %w", err)
		}
	}
	cat.Version = catalogVersion
	capabilitiesChanged, err := migrateDraftCapabilities(data, cat)
	if err != nil {
		return false, fmt.Errorf("migrate draft capabilities: %w", err)
	}
	if err := validateDrafts(*cat); err != nil {
		return false, err
	}
	return migrated || capabilitiesChanged, nil
}

// migrateCatalogDrafts decodes MTP artifact fields shared by unversioned, v0,
// v1, and v2 catalogs. Other metadata is decoded into the current schema by
// the caller and remains unchanged.
func migrateCatalogDrafts(data []byte, cat *Catalog) error {
	var legacy struct {
		Models map[string]struct {
			MTP      string `yaml:"mtp"`
			Original string `yaml:"mtp_orig"`
			URL      string `yaml:"mtp_url"`
			Size     int64  `yaml:"mtp_size"`
			Checked  bool   `yaml:"mtp_checked"`
		} `yaml:"models"`
	}
	if err := yaml.Unmarshal(data, &legacy); err != nil {
		return err
	}
	for id, old := range legacy.Models {
		entry := cat.Models[id]
		entry.Draft = CatalogDraft{File: old.MTP, Original: old.Original, URL: old.URL, Size: old.Size}
		if old.MTP != "" || old.Original != "" || old.URL != "" || old.Size != 0 {
			entry.Draft.Method = DraftMethodMTP
		}
		entry.DraftChecked = old.Checked
		cat.Models[id] = entry
	}
	return nil
}

// migrateDraftCapabilities also accepts catalogs written during v3 development
// that still contain the MTP-only capability field.
func migrateDraftCapabilities(data []byte, cat *Catalog) (bool, error) {
	var legacy struct {
		Models map[string]struct {
			Capabilities struct {
				MTP *bool `yaml:"mtp"`
			} `yaml:"capabilities"`
		} `yaml:"models"`
	}
	if err := yaml.Unmarshal(data, &legacy); err != nil {
		return false, err
	}
	var changed bool
	for id, entry := range cat.Models {
		previous := entry.Capabilities.Draft
		old := legacy.Models[id].Capabilities.MTP
		entry.Capabilities.MTP = old != nil && *old || !entry.MTPSource.IsZero()
		if entry.MTPSource == MTPSourceCompanion && !entry.Draft.isMTP() {
			entry.Capabilities.MTP = false
		}
		entry.Capabilities = entry.Capabilities.WithDraft(entry.Draft.Method)
		changed = changed || old != nil || previous != entry.Capabilities.Draft
		cat.Models[id] = entry
	}
	return changed, nil
}

func validateDrafts(cat Catalog) error {
	for id, entry := range cat.Models {
		d := entry.Draft
		if d == (CatalogDraft{}) {
			continue
		}
		if d.Method.IsZero() || d.File == "" || filepath.Base(d.File) != d.File ||
			!filepath.IsLocal(d.File) || strings.ContainsAny(d.File, `/\`) ||
			!strings.HasSuffix(strings.ToLower(d.File), ".gguf") || d.Size < 0 {
			return fmt.Errorf("invalid draft artifact for %q", id)
		}
		if d.File == entry.MMProj {
			return fmt.Errorf("draft artifact overlaps projection for %q", id)
		}
		for _, file := range entry.Files {
			if d.File == diskName(entry.Family, file) {
				return fmt.Errorf("draft artifact overlaps model weights for %q", id)
			}
		}
	}
	return nil
}
