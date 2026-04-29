package models

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ardanlabs/kronk/sdk/tools/defaults"
)

// ResolveSource maps a model source (full HuggingFace URL, canonical id
// "provider/family", or bare id) to a Resolution containing the canonical
// id, provider, family, revision, full download URL(s), companion
// projection URL, and any locally-known on-disk paths. The resolver may
// persist a new entry to catalog.yaml as a side effect of a successful
// network lookup; this matches Download's behaviour.
//
// Use this when you want to preview what Download would fetch — for
// example to drive a "Resolve" button in the BUI before initiating a
// pull.
func (m *Models) ResolveSource(ctx context.Context, source string) (Resolution, error) {
	rfile, err := defaults.CatalogFile("", m.basePath)
	if err != nil {
		return Resolution{}, fmt.Errorf("resolve-source: file: %w", err)
	}

	res, err := NewResolver(m, rfile).Resolve(ctx, source)
	if err != nil {
		return Resolution{}, fmt.Errorf("resolve-source: %w", err)
	}

	return res, nil
}

// Catalog returns the persisted catalog (catalog.yaml). The Models receiver
// is used to resolve the on-disk path from m.basePath.
func (m *Models) Catalog() (Catalog, error) {
	rfile, err := defaults.CatalogFile("", m.basePath)
	if err != nil {
		return Catalog{}, fmt.Errorf("models-catalog: file: %w", err)
	}

	r := NewResolver(m, rfile)

	cat, err := r.Load()
	if err != nil {
		return Catalog{}, fmt.Errorf("models-catalog: load: %w", err)
	}

	return cat, nil
}

// CatalogEntry returns a single entry from catalog.yaml by canonical id
// ("provider/modelID"). Returns ok=false when the entry is absent.
func (m *Models) CatalogEntry(canonicalID string) (CatalogEntry, bool, error) {
	cat, err := m.Catalog()
	if err != nil {
		return CatalogEntry{}, false, err
	}

	entry, ok := cat.Models[canonicalID]
	return entry, ok, nil
}

// ReconcileCatalog walks the local model index and adds any on-disk model
// that is not yet recorded in catalog.yaml. This handles the upgrade case
// where a user runs a build that introduces (or extends) the catalog while
// already having models on disk: the embedded seed only ships curated
// entries, so user-downloaded models would otherwise be invisible to the
// catalog screen until something explicitly resolved them.
//
// New entries derive their provider/family/files from the on-disk layout
// (<modelsPath>/<provider>/<family>/<file>) using the same logic as the
// resolver's local-disk lookup.
//
// A second pass populates ModelType and Capabilities by reading the GGUF
// head bytes (via GGUFHead's cache → local-file → HF Range lookup) so the
// list page can filter by architecture class and capabilities without
// paying GGUF I/O on every list call. The pass scope depends on the
// schema version stamped on catalog.yaml:
//
//   - When cat.Schema < SchemaVersion every entry is re-enriched (the
//     enrichment rules in code have changed since the entries were
//     written) and the new version is stamped on save.
//   - Otherwise only entries that are missing ModelType or Capabilities
//     are touched, keeping reconcile cheap as the catalog grows.
//
// Enrichment is best-effort throughout — when GGUFHead can't source the
// bytes (offline + nothing cached + nothing downloaded) the entry is
// left untouched and tried again next reconcile.
func (m *Models) ReconcileCatalog(ctx context.Context, log Logger) error {
	rfile, err := defaults.CatalogFile("", m.basePath)
	if err != nil {
		return fmt.Errorf("reconcile-catalog: file: %w", err)
	}

	r := NewResolver(m, rfile)

	cat, err := r.Load()
	if err != nil {
		return fmt.Errorf("reconcile-catalog: load: %w", err)
	}

	if cat.Models == nil {
		cat.Models = map[string]CatalogEntry{}
	}

	files, err := m.Files()
	if err != nil {
		return fmt.Errorf("reconcile-catalog: files: %w", err)
	}

	var changed int

	for _, mf := range files {
		if mf.OwnedBy == "" || mf.ModelFamily == "" {
			continue
		}

		canonical := canonicalID(mf.OwnedBy, mf.ID)
		if _, ok := cat.Models[canonical]; ok {
			continue
		}

		local, ok := r.lookupLocal(mf.OwnedBy, mf.ID)
		if !ok {
			continue
		}

		cat.Models[canonical] = r.buildEntry(local.Provider, local.Family, local.Revision, local.Files, local.MMProj)

		log(ctx, "reconcile-catalog: added", "id", canonical)
		changed++
	}

	// Second pass: enrich entries from the GGUF head bytes. Scope depends
	// on whether the persisted schema version lags the code-side constant.
	// When it does, every entry is re-enriched so detection-rule fixes
	// take effect on upgrade. Otherwise we only touch entries that are
	// missing ModelType / Capabilities — the steady-state path.
	schemaUpgrade := cat.Schema < SchemaVersion

	for canonical, entry := range cat.Models {
		if !schemaUpgrade && entry.ModelType != "" && entry.Capabilities.Endpoint != "" {
			continue
		}

		updated, ok := m.enrichEntry(ctx, entry, log)
		if !ok {
			continue
		}

		cat.Models[canonical] = updated
		changed++
	}

	if schemaUpgrade {
		log(ctx, "reconcile-catalog: schema upgrade", "from", cat.Schema, "to", SchemaVersion)
		cat.Schema = SchemaVersion
		changed++
	}

	if changed == 0 {
		return nil
	}

	if err := r.Save(cat); err != nil {
		return fmt.Errorf("reconcile-catalog: save: %w", err)
	}

	return nil
}

// enrichEntry populates a catalog entry's ModelType and Capabilities by
// reading the GGUF head bytes through GGUFHead's cache → local-file → HF
// Range lookup. Returns the (possibly modified) entry and a boolean that
// is true when the entry actually changed. Failures are logged and treated
// as a no-op so an offline reconcile leaves entries untouched.
func (m *Models) enrichEntry(ctx context.Context, entry CatalogEntry, log Logger) (CatalogEntry, bool) {
	data, err := m.GGUFHead(ctx, entry)
	if err != nil {
		log(ctx, "enrich-entry: gguf-head", "provider", entry.Provider, "family", entry.Family, "ERROR", err)
		return entry, false
	}

	metadata, err := ParseGGUFMetadata(data)
	if err != nil {
		log(ctx, "enrich-entry: parse-gguf", "provider", entry.Provider, "family", entry.Family, "ERROR", err)
		return entry, false
	}

	modelType := ArchitectureClass(metadata)
	capabilities := CapabilitiesFor(metadata, entry.MMProj != "")

	if entry.ModelType == modelType && entry.Capabilities == capabilities {
		return entry, false
	}

	entry.ModelType = modelType
	entry.Capabilities = capabilities

	return entry, true
}

// RemoveCatalogEntry deletes the catalog entry, its GGUF cache, and any
// downloaded files for the given canonical id. This is the catalog-level
// removal contract: removing a model from the model list alone (via
// Models.Remove) does NOT touch the catalog. Removing from the catalog
// here removes both.
func (m *Models) RemoveCatalogEntry(ctx context.Context, canonicalID string, log Logger) error {
	rfile, err := defaults.CatalogFile("", m.basePath)
	if err != nil {
		return fmt.Errorf("remove-catalog-entry: file: %w", err)
	}

	r := NewResolver(m, rfile)

	cat, err := r.Load()
	if err != nil {
		return fmt.Errorf("remove-catalog-entry: load: %w", err)
	}

	entry, ok := cat.Models[canonicalID]
	if !ok {
		return fmt.Errorf("remove-catalog-entry: %q not found", canonicalID)
	}

	// 1. Best-effort remove of any downloaded files under
	//    <modelsPath>/<provider>/<family>/.
	dir := filepath.Join(m.modelsPath, entry.Provider, entry.Family)
	for _, f := range entry.Files {
		p := filepath.Join(dir, filepath.Base(f))
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			log(ctx, "remove-catalog-entry: file", "path", p, "ERROR", err)
		}
	}
	if entry.MMProj != "" {
		p := filepath.Join(dir, filepath.Base(entry.MMProj))
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			log(ctx, "remove-catalog-entry: mmproj", "path", p, "ERROR", err)
		}
	}

	// Best-effort cleanup of the empty family/provider directories.
	_ = os.Remove(dir)
	_ = os.Remove(filepath.Dir(dir))

	// 2. Remove the GGUF cache for this entry.
	if len(entry.Files) > 0 {
		modelID := extractModelID(entry.Files[0])
		if err := m.RemoveGGUFHeadCache(entry.Provider, entry.Family, modelID); err != nil {
			log(ctx, "remove-catalog-entry: gguf-cache", "ERROR", err)
		}
	}

	// 3. Remove the entry itself and persist.
	delete(cat.Models, canonicalID)

	if err := r.Save(cat); err != nil {
		return fmt.Errorf("remove-catalog-entry: save: %w", err)
	}

	// 4. Rebuild the index so the model list view is consistent.
	if err := m.BuildIndex(log, false); err != nil {
		log(ctx, "remove-catalog-entry: rebuild-index", "ERROR", err)
	}

	return nil
}
