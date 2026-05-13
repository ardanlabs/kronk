package models

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/hf"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
)

// ResolveSource maps a model source to a Resolution containing the
// canonical id, provider, family, revision, full download URL(s),
// companion projection URL, and any locally-known on-disk paths. The
// resolver may persist a new entry to catalog.yaml as a side effect of
// a successful network lookup; this matches Download's behaviour.
//
// Use this when you want to preview what Download would fetch — for
// example to drive a "Resolve" button in the BUI before initiating a
// pull.
//
// Accepted input forms (whitespace is trimmed):
//
//   - Bare id:            "Qwen3-0.6B-Q8_0"
//   - Canonical id:       "unsloth/Qwen3-0.6B-Q8_0" (with or without ".gguf")
//   - Provider/repo:tag:  "unsloth/Qwen3.6-35B-A3B-GGUF:UD-Q4_K_XL"
//   - Owner/repo/file:    "unsloth/Qwen3-0.6B-GGUF/Qwen3-0.6B-Q8_0.gguf"
//   - hf.co/ shorthand:   "hf.co/unsloth/Qwen3-0.6B-GGUF/..."
//   - HF resolve URL:     "https://huggingface.co/owner/repo/resolve/main/file.gguf"
//   - HF blob URL:        "https://huggingface.co/owner/repo/blob/main/file.gguf"
//   - HF tree URL:        "https://huggingface.co/owner/repo/tree/main"
//   - HF repo URL:        "https://huggingface.co/owner/repo"
//
// When the input identifies a repository without selecting a specific
// file (e.g. "owner/repo", a tree URL, or a repo root URL), the resolver
// is not run; instead the returned Resolution carries RepoFiles listing
// every GGUF in the repo so the caller can present a picker.
func (m *Models) ResolveSource(ctx context.Context, source string) (Resolution, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return Resolution{}, fmt.Errorf("resolve-source: empty source")
	}

	rfile, err := defaults.CatalogFile("", m.basePath)
	if err != nil {
		return Resolution{}, fmt.Errorf("resolve-source: file: %w", err)
	}

	// Use hf.ParseInput when the source carries any HuggingFace shape
	// (URL with scheme, hf.co/ prefix, or owner/repo[/file]). It accepts
	// resolve, blob, tree, and bare repo URLs. Bare ids ("Qwen3-0.6B-Q8_0")
	// are passed straight through to the resolver.
	id := source
	if needsParse(source) {
		owner, repo, file, perr := hf.ParseInput(source)
		if perr != nil || owner == "" || repo == "" {
			return Resolution{}, fmt.Errorf("resolve-source: parse %q: %w", source, perr)
		}

		// No filename — the input only identifies the repository. Return
		// the GGUF file list so the caller can pick one.
		if file == "" {
			files, ferr := listRepoGGUFs(ctx, owner, repo)
			if ferr != nil {
				return Resolution{}, fmt.Errorf("resolve-source: list %s/%s: %w", owner, repo, ferr)
			}

			return Resolution{
				Provider:  owner,
				Family:    repo,
				RepoFiles: files,
			}, nil
		}

		// The user pinned a specific repo (owner/repo/file). Preserve
		// that pin by routing through the "provider/repo:tag" form so
		// the resolver does not search HF and accidentally land in a
		// sibling repo (e.g. ".../Qwen3.6-35B-A3B-MTP-GGUF") that
		// happens to publish the same quant basename. When the file
		// lacks a recognisable quant suffix, fall back to the bare
		// canonical id — this only affects unusual repos whose GGUFs
		// don't follow the standard quant naming.
		modelID := extractModelID(file)
		if tag := extractQuantTag(modelID); tag != "" {
			id = fmt.Sprintf("%s/%s:%s", owner, repo, tag)
		} else {
			id = fmt.Sprintf("%s/%s", owner, modelID)
		}
	}

	res, err := NewResolver(m, rfile).Resolve(ctx, id)
	if err != nil {
		return Resolution{}, fmt.Errorf("resolve-source: %w", err)
	}

	return res, nil
}

// needsParse reports whether source contains a HuggingFace URL or a
// multi-segment shorthand that hf.ParseInput should handle. Bare ids
// (no slash, no scheme) and canonical ids ("provider/modelID") go
// straight to the resolver.
func needsParse(source string) bool {
	lower := strings.ToLower(source)
	switch {
	case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"):
		return true
	case strings.HasPrefix(lower, "huggingface.co/"), strings.HasPrefix(lower, "hf.co/"):
		return true
	}

	// "owner/repo/file..." has at least two slashes; "provider/modelID"
	// has exactly one and stays on the resolver path.
	return strings.Count(source, "/") >= 2
}

// listRepoGGUFs returns the GGUF files in a HuggingFace repository at
// the default revision. Used by ResolveSource when the input does not
// pin a specific file.
func listRepoGGUFs(ctx context.Context, owner, repo string) ([]hf.RepoFile, error) {
	all, err := hf.RepoFiles(ctx, owner, repo, "main", "", true)
	if err != nil {
		return nil, err
	}

	var ggufs []hf.RepoFile
	for _, f := range all {
		if strings.HasSuffix(strings.ToLower(f.Filename), ".gguf") {
			ggufs = append(ggufs, f)
		}
	}

	return ggufs, nil
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
func (m *Models) ReconcileCatalog(ctx context.Context, log applog.Logger) error {
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
func (m *Models) enrichEntry(ctx context.Context, entry CatalogEntry, log applog.Logger) (CatalogEntry, bool) {
	data, err := m.GGUFHead(ctx, entry)
	if err != nil {
		log(ctx, "enrich-entry: gguf-head", "provider", entry.Provider, "family", entry.Family, "ERROR", err)
		return entry, false
	}

	metadata, err := gguf.ParseMetadata(data)
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
func (m *Models) RemoveCatalogEntry(ctx context.Context, canonicalID string, log applog.Logger) error {
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
	//    <modelsPath>/<provider>/<family>/. Each model file owns three
	//    on-disk artifacts that must be cleaned together: the GGUF body,
	//    the <dir>/sha/<base> hash file, and the <dir>/sha/<base>.verified
	//    sentinel produced by CheckModel after a successful full re-hash.
	//    Missing companions are normal and not errors.
	dir := filepath.Join(m.modelsPath, entry.Provider, entry.Family)
	removeCompanions := func(p string) {
		shaFile := filepath.Join(filepath.Dir(p), "sha", filepath.Base(p))
		if err := os.Remove(shaFile); err != nil && !os.IsNotExist(err) {
			log(ctx, "remove-catalog-entry: sha", "path", shaFile, "ERROR", err)
		}
		if err := model.RemoveVerifiedSentinel(p); err != nil {
			log(ctx, "remove-catalog-entry: verified-sentinel", "path", p, "ERROR", err)
		}
	}

	for _, f := range entry.Files {
		p := filepath.Join(dir, filepath.Base(f))
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			log(ctx, "remove-catalog-entry: file", "path", p, "ERROR", err)
		}
		removeCompanions(p)
	}
	if entry.MMProj != "" {
		p := filepath.Join(dir, filepath.Base(entry.MMProj))
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			log(ctx, "remove-catalog-entry: mmproj", "path", p, "ERROR", err)
		}
		removeCompanions(p)
	}

	// Best-effort cleanup of the now-empty sha subdir, then the
	// family/provider directories. os.Remove fails (silently here) if
	// anything is still inside, which is the right behaviour when other
	// models share the same family.
	_ = os.Remove(filepath.Join(dir, "sha"))
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
