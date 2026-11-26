package models

import (
	"fmt"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v2"
)

// migrateCatalogV2 is deliberately the only legacy decoder. All non-artifact
// metadata has already been decoded into the current schema by the caller.
func migrateCatalogV2(data []byte, cat *Catalog) error {
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
	cat.Version = catalogVersion
	return validateDrafts(*cat)
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
