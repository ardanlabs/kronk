package models

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/hf"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"go.yaml.in/yaml/v2"
)

// SchemaVersion is bumped whenever a code change should force a
// rebuild of every persisted catalog entry — typically because the
// detection rules in CapabilitiesFor or ArchitectureClass changed, but
// any future change that affects the shape or content of CatalogEntry
// fields populated by enrichment qualifies. ReconcileCatalog compares
// this code-side constant against the version stamped on catalog.yaml
// and, when the disk is older, re-runs enrichment for every entry. On a
// steady-state reconcile (versions match) only entries with empty
// ModelType / Capabilities are touched, so the catalog stays cheap to
// reconcile even as it grows large.
//
// Bump this when any field populated by enrichment can change for
// existing entries — for example:
//   - CapabilitiesFor's keyword set, endpoint mapping, or modality flags.
//   - ArchitectureClass's hybrid / MoE detection rules.
//   - A new field is added to CatalogEntry that enrichment populates.
//
// History:
//   - v1: initial schema (model_type, capabilities, omni→audio+video on
//     general.architecture).
const SchemaVersion = 1

// Catalog is the on-disk schema for catalog.yaml. It owns the provider
// priority list and the cache of previously-resolved canonical model IDs.
// Schema records the version of the enrichment rules used to populate the
// entries; when it lags behind SchemaVersion the next reconcile rebuilds
// every entry's enriched fields and stamps the new version.
type Catalog struct {
	Schema    int                     `yaml:"schema,omitempty"`
	Providers []string                `yaml:"providers"`
	Models    map[string]CatalogEntry `yaml:"models"`
}

// CatalogEntry is the persisted resolution for a single canonical
// model id ("provider/modelID"). Files and MMProj are family-relative paths.
//
// Family identifies both the source repository on HuggingFace
// (for rebuilding download URLs) and the on-disk folder under
// <modelsPath>/<provider>/<family>/ (for direct path lookups without
// consulting the model index).
//
// FileSizes and MMProjSize are byte sizes that align positionally with
// Files and MMProj. They are populated from os.Stat on local files (or
// HF HEAD when the seeding tool builds the embedded default) so the BUI
// can render total_size for entries that have not been downloaded yet.
type CatalogEntry struct {
	Provider     string              `yaml:"provider"`
	Family       string              `yaml:"family"`
	Revision     string              `yaml:"revision"`
	Files        []string            `yaml:"files"`
	FileSizes    []int64             `yaml:"file_sizes,omitempty"`
	MMProj       string              `yaml:"mmproj,omitempty"`
	MMProjOrig   string              `yaml:"mmproj_orig,omitempty"`
	MMProjSize   int64               `yaml:"mmproj_size,omitempty"`
	ModelType    string              `yaml:"model_type,omitempty"`
	Capabilities CatalogCapabilities `yaml:"capabilities,omitempty"`
	ResolvedAt   time.Time           `yaml:"resolved_at"`
}

// Resolution is the result of a Resolve call. It holds both the persisted
// metadata and any locally-known on-disk paths. MMProj is the local
// (renamed) projection filename used for on-disk lookup; MMProjOrig is
// the HuggingFace source filename used for building DownloadProj URLs.
type Resolution struct {
	CanonicalID  string
	Provider     string
	Family       string
	Revision     string
	Files        []string
	MMProj       string
	MMProjOrig   string
	DownloadURLs []string
	DownloadProj string
	LocalPaths   []string
	LocalProj    string
	FromLocal    bool
	FromCache    bool
}

// Resolver maps a model ID (bare or provider/id) to download URLs and
// on-disk paths. It uses a YAML cache file ("catalog.yaml") for
// previously-seen IDs and falls back to the HuggingFace API for new ones.
type Resolver struct {
	filePath string
	mu       sync.Mutex
	hfClient hf.Client
	models   *Models
}

// NewResolver constructs a Resolver using the default HuggingFace client.
// filePath is the location of catalog.yaml on disk.
func NewResolver(m *Models, filePath string) *Resolver {
	return NewResolverWithClient(m, filePath, hf.NewDefaultClient())
}

// NewResolverWithClient constructs a Resolver with a caller-supplied HF
// client. Used by tests.
func NewResolverWithClient(m *Models, filePath string, client hf.Client) *Resolver {
	return &Resolver{
		filePath: filePath,
		hfClient: client,
		models:   m,
	}
}

// FilePath returns the path of the catalog.yaml file.
func (r *Resolver) FilePath() string {
	return r.filePath
}

// Load reads the resolver file from disk. If the file does not exist a
// zero-value Catalog is returned (with no providers); callers
// should normally seed it from sdk/tools/defaults first.
func (r *Resolver) Load() (Catalog, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.loadLocked()
}

func (r *Resolver) loadLocked() (Catalog, error) {
	data, err := os.ReadFile(r.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return Catalog{Models: map[string]CatalogEntry{}}, nil
		}
		return Catalog{}, fmt.Errorf("resolver-load: read: %w", err)
	}

	var rm Catalog
	if err := yaml.Unmarshal(data, &rm); err != nil {
		return Catalog{}, fmt.Errorf("resolver-load: unmarshal: %w", err)
	}

	if rm.Models == nil {
		rm.Models = map[string]CatalogEntry{}
	}

	return rm, nil
}

// Save writes the resolver file to disk.
func (r *Resolver) Save(rm Catalog) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.saveLocked(rm)
}

func (r *Resolver) saveLocked(rm Catalog) error {
	if rm.Models == nil {
		rm.Models = map[string]CatalogEntry{}
	}

	data, err := yaml.Marshal(rm)
	if err != nil {
		return fmt.Errorf("resolver-save: marshal: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(r.filePath), 0755); err != nil {
		return fmt.Errorf("resolver-save: mkdir: %w", err)
	}

	if err := os.WriteFile(r.filePath, data, 0644); err != nil {
		return fmt.Errorf("resolver-save: write: %w", err)
	}

	return nil
}

// Providers returns the configured provider priority list. If the resolver
// file has no providers (or cannot be loaded) the fallback list is used.
func (r *Resolver) Providers() []string {
	rm, err := r.Load()
	if err != nil || len(rm.Providers) == 0 {
		return []string{"unsloth", "ggml-org", "bartowski", "mradermacher", "gpustack"}
	}
	return rm.Providers
}

// Resolve maps an id to a Resolution. The id may be bare ("Qwen3-0.6B-Q8_0")
// or include an explicit provider ("unsloth/Qwen3-0.6B-Q8_0"). Resolution
// proceeds in order: resolver file (fast cache) → local disk → HF API
// across the provider list. Local and HF hits are persisted to the
// resolver file so subsequent lookups become cache hits.
func (r *Resolver) Resolve(ctx context.Context, id string) (Resolution, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Resolution{}, fmt.Errorf("resolve: empty id")
	}

	// Accept inputs that include the ".gguf" file extension (e.g.
	// "ggml-org/embeddinggemma-300m-qat-Q8_0.gguf") and treat them as
	// canonical ids.
	id = strings.TrimSuffix(id, ".gguf")

	provider, modelID := splitProviderID(id)

	// 1. Resolver file cache. The fast path — look up by canonical
	//    provider/modelID, or by bare modelID if no provider was given.
	rm, err := r.Load()
	if err != nil {
		return Resolution{}, fmt.Errorf("resolve: %w", err)
	}

	online := hasNetwork()

	if cached, ok := r.lookupCache(rm, provider, modelID); ok {
		// Self-heal: pre-MMProjOrig entries (or those persisted from a
		// local-disk discovery before HF was reachable) carry the local
		// renamed projection name but no HF source name, so DownloadProj
		// would be empty. When online, fall through to HF so the entry
		// can be repaired with the canonical mmproj source name. When
		// offline, return what we have.
		needsRepair := cached.MMProj != "" && cached.DownloadProj == ""
		if !needsRepair || !online {
			cached.FromCache = true
			r.attachLocal(&cached)
			return cached, nil
		}
	}

	// 2. HuggingFace search. If a provider was given, search only that
	//    provider; otherwise walk the priority list. The HF lookup is
	//    the only path that can produce a correct DownloadProj URL —
	//    the on-disk projection file has been renamed and the original
	//    HF filename cannot be recovered from the local layout.
	providers := []string{provider}
	if provider == "" {
		providers = rm.Providers
		if len(providers) == 0 {
			providers = r.Providers()
		}
	}

	if online {
		for _, p := range providers {
			res, ok, err := r.resolveAtProvider(ctx, p, modelID)
			if err != nil {
				return Resolution{}, fmt.Errorf("resolve: provider %q: %w", p, err)
			}
			if !ok {
				continue
			}

			// Persist the new entry. MMProj records the local-renamed name
			// (mmproj-<modelID>.gguf) so attachLocal can find the file on
			// disk; MMProjOrig records the HuggingFace source filename so
			// DownloadProj URLs can be reconstructed from cache hits without
			// another HF round-trip.
			entry := r.buildEntry(res.Provider, res.Family, res.Revision, res.Files, res.MMProj)
			entry.MMProjOrig = res.MMProjOrig
			rm.Models[res.CanonicalID] = entry
			if err := r.Save(rm); err != nil {
				return Resolution{}, fmt.Errorf("resolve: persist: %w", err)
			}

			r.attachLocal(&res)
			return res, nil
		}
	}

	// 3. Offline fallback. When HF is unreachable but the model is on
	//    disk, return what we know: provider/family/files/LocalPaths.
	//    DownloadProj is left empty because the HF projection source
	//    name cannot be recovered from the renamed on-disk file.
	if local, ok := r.lookupLocal(provider, modelID); ok {
		local.FromLocal = true
		local.DownloadURLs = buildDownloadURLs(local.Provider, local.Family, local.Revision, local.Files)

		// Persist what we know so subsequent online Resolves can self-heal
		// and fill in MMProjOrig.
		rm.Models[local.CanonicalID] = r.buildEntry(local.Provider, local.Family, local.Revision, local.Files, local.MMProj)
		if err := r.Save(rm); err != nil {
			return Resolution{}, fmt.Errorf("resolve: persist local: %w", err)
		}

		return local, nil
	}

	if !online {
		return Resolution{}, fmt.Errorf("resolve: model %q not found locally and no network available", id)
	}

	return Resolution{}, fmt.Errorf("resolve: model %q not found in any of %v", id, providers)
}

// =============================================================================

// splitProviderID separates "provider/modelID" inputs. For bare ids the
// provider is empty.
func splitProviderID(id string) (provider, modelID string) {
	if before, after, ok := strings.Cut(id, "/"); ok {
		return before, after
	}

	return "", id
}

// canonicalID joins provider and modelID using "/".
func canonicalID(provider, modelID string) string {
	return provider + "/" + modelID
}

// buildEntry assembles a CatalogEntry stamped with the current time. When
// the resolver has access to a Models instance and any of the listed files
// exist on disk under <modelsPath>/<provider>/<family>/, FileSizes and
// MMProjSize are populated from os.Stat. Files that aren't on disk yet
// produce zero entries (callers expecting sizes for un-downloaded entries
// must fill them via HF HEAD elsewhere).
func (r *Resolver) buildEntry(provider, family, revision string, files []string, mmproj string) CatalogEntry {
	entry := CatalogEntry{
		Provider:   provider,
		Family:     family,
		Revision:   revision,
		Files:      files,
		MMProj:     mmproj,
		ResolvedAt: time.Now().UTC(),
	}

	if r.models == nil {
		return entry
	}

	dir := filepath.Join(r.models.modelsPath, provider, family)

	if len(files) > 0 {
		sizes := make([]int64, len(files))
		var any bool
		for i, f := range files {
			if fi, err := os.Stat(filepath.Join(dir, filepath.Base(f))); err == nil {
				sizes[i] = fi.Size()
				any = true
			}
		}
		if any {
			entry.FileSizes = sizes
		}
	}

	if mmproj != "" {
		if fi, err := os.Stat(filepath.Join(dir, filepath.Base(mmproj))); err == nil {
			entry.MMProjSize = fi.Size()
		}
	}

	return entry
}

// enrichCatalogEntry populates a single catalog entry's ModelType and
// Capabilities from GGUF head bytes and writes the result back. Used
// after a fresh download so the persisted entry carries the architecture
// class and capability flags without waiting for the next reconcile.
// Best-effort: when GGUFHead can't source the bytes the entry is left
// untouched.
func (r *Resolver) enrichCatalogEntry(ctx context.Context, canonical string, log applog.Logger) error {
	if r.models == nil {
		return nil
	}

	rm, err := r.Load()
	if err != nil {
		return fmt.Errorf("enrich-catalog-entry: load: %w", err)
	}

	entry, ok := rm.Models[canonical]
	if !ok {
		return nil
	}

	updated, changed := r.models.enrichEntry(ctx, entry, log)
	if !changed {
		return nil
	}

	rm.Models[canonical] = updated

	if err := r.Save(rm); err != nil {
		return fmt.Errorf("enrich-catalog-entry: save: %w", err)
	}

	return nil
}

// refreshSizes re-stats the on-disk files for a catalog entry and writes
// the updated FileSizes/MMProjSize back to catalog.yaml. Used after a
// fresh download so the persisted entry reflects actual byte counts.
func (r *Resolver) refreshSizes(canonical string) error {
	rm, err := r.Load()
	if err != nil {
		return fmt.Errorf("refresh-sizes: load: %w", err)
	}

	entry, ok := rm.Models[canonical]
	if !ok {
		return nil
	}

	updated := r.buildEntry(entry.Provider, entry.Family, entry.Revision, entry.Files, entry.MMProj)

	// Preserve the original ResolvedAt — refreshing sizes shouldn't
	// rewrite the resolution timestamp.
	updated.ResolvedAt = entry.ResolvedAt

	rm.Models[canonical] = updated

	if err := r.Save(rm); err != nil {
		return fmt.Errorf("refresh-sizes: save: %w", err)
	}

	return nil
}

// =============================================================================

// lookupLocal checks the local index for a model with the matching bare ID.
// When provider is empty any provider on disk wins; when set, only entries
// whose owning org matches are accepted.
func (r *Resolver) lookupLocal(provider, modelID string) (Resolution, bool) {
	if r.models == nil {
		return Resolution{}, false
	}

	mp, err := r.models.FullPath(modelID)
	if err != nil || len(mp.ModelFiles) == 0 {
		return Resolution{}, false
	}

	// Derive the owning provider/repo from the on-disk layout
	// (<modelsPath>/<provider>/<repo>/<file>).
	first := mp.ModelFiles[0]
	rel := strings.TrimPrefix(first, r.models.modelsPath)
	rel = strings.TrimPrefix(rel, string(filepath.Separator))
	parts := strings.Split(rel, string(filepath.Separator))

	if len(parts) < 2 {
		return Resolution{}, false
	}

	localProvider := parts[0]
	localFamily := parts[1]

	if provider != "" && !strings.EqualFold(provider, localProvider) {
		return Resolution{}, false
	}

	files := make([]string, len(mp.ModelFiles))
	for i, f := range mp.ModelFiles {
		files[i] = filepath.Base(f)
	}
	sort.Strings(files)

	res := Resolution{
		CanonicalID: canonicalID(localProvider, modelID),
		Provider:    localProvider,
		Family:      localFamily,
		Revision:    "main",
		Files:       files,
		LocalPaths:  append([]string(nil), mp.ModelFiles...),
	}

	sort.Strings(res.LocalPaths)
	if mp.ProjFile != "" {
		res.MMProj = filepath.Base(mp.ProjFile)
		res.LocalProj = mp.ProjFile
	}

	return res, true
}

// lookupCache checks the persisted resolver file for a matching entry.
func (r *Resolver) lookupCache(rm Catalog, provider, modelID string) (Resolution, bool) {
	if provider != "" {
		key := canonicalID(provider, modelID)
		if entry, ok := rm.Models[key]; ok {
			return entryToResolution(key, entry), true
		}
		return Resolution{}, false
	}

	// Bare ID: look for any entry whose modelID half matches.
	for key, entry := range rm.Models {
		_, m := splitProviderID(key)
		if m == modelID {
			return entryToResolution(key, entry), true
		}
	}

	return Resolution{}, false
}

func entryToResolution(canonical string, entry CatalogEntry) Resolution {
	res := Resolution{
		CanonicalID:  canonical,
		Provider:     entry.Provider,
		Family:       entry.Family,
		Revision:     entry.Revision,
		Files:        append([]string(nil), entry.Files...),
		MMProj:       entry.MMProj,
		MMProjOrig:   entry.MMProjOrig,
		DownloadURLs: buildDownloadURLs(entry.Provider, entry.Family, entry.Revision, entry.Files),
	}

	// DownloadProj is built from the HuggingFace source name (MMProjOrig),
	// not the local-renamed name (MMProj). Pre-MMProjOrig entries leave
	// MMProjOrig empty so the resolver can detect the gap and self-heal.
	if entry.MMProjOrig != "" {
		res.DownloadProj = buildDownloadURL(entry.Provider, entry.Family, entry.Revision, entry.MMProjOrig)
	}

	return res
}

// attachLocal fills LocalPaths/LocalProj for any files already on disk at
// the canonical layout (<modelsPath>/<provider>/<family>/<file>).
func (r *Resolver) attachLocal(res *Resolution) {
	if r.models == nil {
		return
	}

	dir := filepath.Join(r.models.modelsPath, res.Provider, res.Family)

	var local []string
	for _, f := range res.Files {
		p := filepath.Join(dir, filepath.Base(f))
		if _, err := os.Stat(p); err == nil {
			local = append(local, p)
		}
	}
	if len(local) == len(res.Files) {
		res.LocalPaths = local
	}

	if res.MMProj != "" {
		p := filepath.Join(dir, filepath.Base(res.MMProj))
		if _, err := os.Stat(p); err == nil {
			res.LocalProj = p
		}
	}
}

// resolveAtProvider searches a single provider for the model. The bool is
// true on a successful match; false (with nil error) means the provider
// has no matching repo and the caller should try the next one.
func (r *Resolver) resolveAtProvider(ctx context.Context, provider, modelID string) (Resolution, bool, error) {
	searchTerm := stripQuantSuffix(modelID)

	repos, err := r.hfClient.SearchModels(ctx, provider, searchTerm)
	if err != nil {
		if isNotFound(err) {
			return Resolution{}, false, nil
		}
		return Resolution{}, false, err
	}

	for _, ownerRepo := range repos {
		owner, repo, ok := splitOwnerRepo(ownerRepo)
		if !ok || !strings.EqualFold(owner, provider) {
			continue
		}

		meta, err := r.hfClient.ModelMeta(ctx, owner, repo, "main")
		if err != nil {
			if isNotFound(err) {
				continue
			}
			return Resolution{}, false, err
		}

		files, mmproj, ok := selectFiles(meta.Siblings, modelID)
		if !ok {
			continue
		}

		res := Resolution{
			CanonicalID:  canonicalID(provider, modelID),
			Provider:     provider,
			Family:       repo,
			Revision:     "main",
			Files:        files,
			MMProj:       localProjName(mmproj, files),
			MMProjOrig:   mmproj,
			DownloadURLs: buildDownloadURLs(provider, repo, "main", files),
		}

		if mmproj != "" {
			res.DownloadProj = buildDownloadURL(provider, repo, "main", mmproj)
		}

		return res, true, nil
	}

	return Resolution{}, false, nil
}

// =============================================================================

// buildDownloadURL composes a HuggingFace resolve URL for a single file.
func buildDownloadURL(owner, repo, revision, file string) string {
	if revision == "" {
		revision = "main"
	}

	return fmt.Sprintf(
		"https://huggingface.co/%s/%s/resolve/%s/%s",
		url.PathEscape(owner),
		url.PathEscape(repo),
		url.PathEscape(revision),
		file,
	)
}

func buildDownloadURLs(owner, repo, revision string, files []string) []string {
	urls := make([]string, len(files))
	for i, f := range files {
		urls[i] = buildDownloadURL(owner, repo, revision, f)
	}

	return urls
}

// splitOwnerRepo splits "owner/repo" returning ok=false when the input
// is malformed.
func splitOwnerRepo(s string) (owner, repo string, ok bool) {
	i := strings.Index(s, "/")
	if i <= 0 || i == len(s)-1 {
		return "", "", false
	}

	return s[:i], s[i+1:], true
}

func isNotFound(err error) bool {
	return err != nil && (err == hf.ErrNotFound || strings.Contains(err.Error(), hf.ErrNotFound.Error()))
}

// =============================================================================

// persistURLResolution records a resolution entry to the resolver file
// derived from one or more HuggingFace download URLs. Used after URL-based
// downloads so catalog.yaml stays current regardless of the input
// shape passed to Download.
func (m *Models) persistURLResolution(modelURLs []string, projURL string) error {
	if len(modelURLs) == 0 {
		return nil
	}

	provider, repo, revision, files, ok := hf.ParseURLs(modelURLs)
	if !ok {
		return nil
	}

	var mmproj, mmprojOrig string
	if projURL != "" {
		if _, _, _, file, parsed := hf.ParseURL(hf.NormalizeDownloadURL(projURL)); parsed {
			mmproj = localProjName(file, files)
			mmprojOrig = file
		}
	}

	rfile, err := defaults.CatalogFile("", m.basePath)
	if err != nil {
		return fmt.Errorf("persist-url: resolver-file: %w", err)
	}

	r := NewResolver(m, rfile)

	rm, err := r.Load()
	if err != nil {
		return fmt.Errorf("persist-url: load: %w", err)
	}

	if rm.Models == nil {
		rm.Models = map[string]CatalogEntry{}
	}

	modelID := extractModelID(files[0])
	canonical := canonicalID(provider, modelID)

	entry := r.buildEntry(provider, repo, revision, files, mmproj)
	entry.MMProjOrig = mmprojOrig
	rm.Models[canonical] = entry

	if err := r.Save(rm); err != nil {
		return fmt.Errorf("persist-url: save: %w", err)
	}

	return nil
}

// localProjName returns the on-disk projection filename that downloadModel
// produces by renaming the HuggingFace source file to "mmproj-<modelID>.gguf".
// Returns "" when there is no projection.
func localProjName(hfMMProj string, modelFiles []string) string {
	if hfMMProj == "" || len(modelFiles) == 0 {
		return ""
	}

	first := modelFiles[0]
	ext := filepath.Ext(first)
	modelID := strings.TrimSuffix(filepath.Base(first), ext)

	return fmt.Sprintf("mmproj-%s%s", modelID, ext)
}
