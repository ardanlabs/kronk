package models

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
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
//   - v2: MTP drafter companion discovery. Entries persisted before MTP
//     support never recorded a co-located mtp-*.gguf companion; the
//     schema-upgrade reconcile re-scans HuggingFace once per entry to
//     fill in MTP/MTPOrig and stamp mtp_checked.
const SchemaVersion = 2

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
	MTP          string              `yaml:"mtp,omitempty"`
	MTPOrig      string              `yaml:"mtp_orig,omitempty"`
	MTPSize      int64               `yaml:"mtp_size,omitempty"`
	MTPChecked   bool                `yaml:"mtp_checked,omitempty"`
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
	MTP          string
	MTPOrig      string
	DownloadURLs []string
	DownloadProj string
	DownloadMTP  string
	LocalPaths   []string
	LocalProj    string
	LocalMTP     string
	FromLocal    bool
	FromCache    bool

	// MTPChecked carries the persisted entry's mtp_checked flag through a
	// cache hit. It reports whether the entry was built by an HF sibling
	// scan that looked for an MTP drafter companion. Pre-MTP-support
	// entries leave it false so the resolver knows to re-scan once.
	MTPChecked bool

	// RepoFiles is populated only when the input identified a repository
	// without selecting a specific model file (e.g. "owner/repo" or a
	// HuggingFace tree/blob URL with no filename). It lists every GGUF
	// in the repo so the caller can present a picker. When set, the
	// resolver fields above are zero-valued and no resolver lookup was
	// performed.
	RepoFiles []hf.RepoFile
}

// VerifyLocal reports whether every model file (and any projection)
// referenced by the resolution exists on disk at its canonical path and
// has the size recorded in its companion sha pointer file. It returns
// nil when the on-disk copy is complete and an error describing the
// first missing or short file otherwise. Callers (notably the BUI
// Resolve handler) use it to distinguish a fully installed model from
// one whose download was cancelled or truncated, so the UI can offer to
// resume the pull instead of locking the user out with "already
// installed". The check is size-only (no sha256 re-hash) so it is cheap
// enough to run on every Resolve request.
func (r Resolution) VerifyLocal() error {
	mp := Path{
		ModelFiles: append([]string(nil), r.LocalPaths...),
		ProjFile:   r.LocalProj,
		MTPFile:    r.LocalMTP,
	}

	return verifyAllSizes(mp)
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

// Resolve maps an id to a Resolution. The id may be bare ("Qwen3-0.6B-Q8_0"),
// include an explicit provider ("unsloth/Qwen3-0.6B-Q8_0"), or carry a
// HuggingFace-style "provider/repo:tag" quant selector
// ("unsloth/Qwen3.6-35B-A3B-GGUF:UD-Q4_K_XL"). Resolution proceeds in
// order: resolver file (fast cache) → local disk → HF API across the
// provider list. Local and HF hits are persisted to the resolver file
// so subsequent lookups become cache hits.
func (r *Resolver) Resolve(ctx context.Context, id string) (Resolution, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Resolution{}, fmt.Errorf("resolve: empty id")
	}

	// Accept inputs that include the ".gguf" file extension (e.g.
	// "ggml-org/embeddinggemma-300m-qat-Q8_0.gguf") and treat them as
	// canonical ids.
	id = strings.TrimSuffix(id, ".gguf")

	// "provider/repo:tag" form pins both the HuggingFace owner/repo and
	// the desired quant variant — no SearchModels round-trip needed.
	if provider, repo, tag, ok := splitProviderRepoTag(id); ok {
		return r.resolveByTag(ctx, provider, repo, tag)
	}

	provider, modelID := splitProviderID(id)

	// 1. Resolver file cache. The fast path — look up by canonical
	//    provider/modelID, or by bare modelID if no provider was given.
	rm, err := r.Load()
	if err != nil {
		return Resolution{}, fmt.Errorf("resolve: %w", err)
	}

	online := hasNetwork()

	var preferredFamily string
	if cached, ok := r.lookupCache(rm, provider, modelID); ok {
		// Self-heal: pre-MMProjOrig entries (or those persisted from a
		// local-disk discovery before HF was reachable) carry the local
		// renamed projection name but no HF source name, so DownloadProj
		// would be empty. When online, fall through to HF so the entry
		// can be repaired with the canonical mmproj source name. When
		// offline, return what we have. The same self-heal applies to a
		// tracked MTP companion missing its DownloadMTP URL.
		needsRepair := (cached.MMProj != "" && cached.DownloadProj == "") ||
			(cached.MTP != "" && cached.DownloadMTP == "")
		if !needsRepair || !online {
			cached.FromCache = true
			r.attachLocal(&cached)
			return cached, nil
		}

		// Remember the cached repo so the HF search below can prefer it
		// over any sibling repos that happen to surface ahead of it in
		// the search results.
		preferredFamily = cached.Family
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
			res, ok, err := r.resolveAtProvider(ctx, p, modelID, preferredFamily)
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
			entry := r.buildEntry(res.Provider, res.Family, res.Revision, res.Files, res.MMProj, res.MTP)
			entry.MMProjOrig = res.MMProjOrig
			entry.MTPOrig = res.MTPOrig
			entry.MTPChecked = true
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
		entry := r.buildEntry(local.Provider, local.Family, local.Revision, local.Files, local.MMProj, local.MTP)
		entry.MMProjOrig = local.MMProjOrig
		entry.MTPOrig = local.MTPOrig
		rm.Models[local.CanonicalID] = entry
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

// resolveByTag handles the "provider/repo:tag" input shape. It first tries
// the catalog cache by (provider, family, tag); on a miss it calls
// hfClient.ModelMeta directly (no SearchModels needed because the
// repository is already pinned), picks the sibling file matching the tag
// via selectFilesByTag, and persists the resulting entry under the
// canonical id derived from the chosen file's basename.
func (r *Resolver) resolveByTag(ctx context.Context, provider, repo, tag string) (Resolution, error) {
	rm, err := r.Load()
	if err != nil {
		return Resolution{}, fmt.Errorf("resolve: %w", err)
	}

	online := hasNetwork()

	if cached, ok := r.lookupCacheByTag(rm, provider, repo, tag); ok {
		needsRepair := (cached.MMProj != "" && cached.DownloadProj == "") ||
			(cached.MTP != "" && cached.DownloadMTP == "")
		if !needsRepair || !online {
			cached.FromCache = true
			r.attachLocal(&cached)
			return cached, nil
		}
	}

	if !online {
		return Resolution{}, fmt.Errorf("resolve: %s/%s:%s not cached and no network available", provider, repo, tag)
	}

	meta, err := r.hfClient.ModelMeta(ctx, provider, repo, "main")
	if err != nil {
		if isNotFound(err) {
			return Resolution{}, fmt.Errorf("resolve: %s/%s not found", provider, repo)
		}
		return Resolution{}, fmt.Errorf("resolve: %s/%s: %w", provider, repo, err)
	}

	files, mmproj, mtp, ok := selectFilesByTag(meta.Siblings, repo, tag)
	if !ok {
		return Resolution{}, fmt.Errorf("resolve: tag %q not found in %s/%s", tag, provider, repo)
	}

	modelID := catalogModelID(repo, files[0])
	canonical := canonicalID(provider, modelID)

	res := Resolution{
		CanonicalID:  canonical,
		Provider:     provider,
		Family:       repo,
		Revision:     "main",
		Files:        files,
		MMProj:       localProjName(repo, mmproj, files),
		MMProjOrig:   mmproj,
		MTP:          localMTPName(repo, mtp, files),
		MTPOrig:      mtp,
		DownloadURLs: buildDownloadURLs(provider, repo, "main", files),
	}

	if mmproj != "" {
		res.DownloadProj = buildDownloadURL(provider, repo, "main", mmproj)
	}
	if mtp != "" {
		res.DownloadMTP = buildDownloadURL(provider, repo, "main", mtp)
	}

	entry := r.buildEntry(provider, repo, "main", files, res.MMProj, res.MTP)
	entry.MMProjOrig = mmproj
	entry.MTPOrig = mtp
	entry.MTPChecked = true

	if rm.Models == nil {
		rm.Models = map[string]CatalogEntry{}
	}
	rm.Models[canonical] = entry

	if err := r.Save(rm); err != nil {
		return Resolution{}, fmt.Errorf("resolve: persist: %w", err)
	}

	r.attachLocal(&res)

	return res, nil
}

// lookupCacheByTag scans the persisted catalog for an entry whose
// provider+family match the request and whose first file's model id ends
// with the requested tag (separated by "-" or ".").
func (r *Resolver) lookupCacheByTag(rm Catalog, provider, repo, tag string) (Resolution, bool) {
	for key, entry := range rm.Models {
		if !strings.EqualFold(entry.Provider, provider) {
			continue
		}
		if !strings.EqualFold(entry.Family, repo) {
			continue
		}
		if len(entry.Files) == 0 {
			continue
		}
		if !fileMatchesTag(entry.Files[0], tag) {
			continue
		}

		return entryToResolution(key, entry), true
	}

	return Resolution{}, false
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

// splitProviderRepoTag parses "provider/repo:tag" inputs. The tag is a
// quantization selector (e.g. "UD-Q4_K_XL", "Q8_0", "BF16") used to pick
// the matching sibling file in repo. Returns ok=false when the input is
// not in this exact shape — exactly one "/", a non-empty ":tag" suffix,
// and no further "/" or ":" in any segment.
func splitProviderRepoTag(id string) (provider, repo, tag string, ok bool) {
	slash := strings.Index(id, "/")
	if slash <= 0 || slash == len(id)-1 {
		return "", "", "", false
	}

	rest := id[slash+1:]
	if strings.Contains(rest, "/") {
		return "", "", "", false
	}

	colon := strings.Index(rest, ":")
	if colon <= 0 || colon == len(rest)-1 {
		return "", "", "", false
	}

	provider = id[:slash]
	repo = rest[:colon]
	tag = rest[colon+1:]

	if strings.ContainsAny(tag, ":/") {
		return "", "", "", false
	}

	return provider, repo, tag, true
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
func (r *Resolver) buildEntry(provider, family, revision string, files []string, mmproj, mtp string) CatalogEntry {
	entry := CatalogEntry{
		Provider:   provider,
		Family:     family,
		Revision:   revision,
		Files:      files,
		MMProj:     mmproj,
		MTP:        mtp,
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
			if fi, err := os.Stat(filepath.Join(dir, diskName(family, f))); err == nil {
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

	if mtp != "" {
		if fi, err := os.Stat(filepath.Join(dir, filepath.Base(mtp))); err == nil {
			entry.MTPSize = fi.Size()
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

	updated := r.buildEntry(entry.Provider, entry.Family, entry.Revision, entry.Files, entry.MMProj, entry.MTP)

	// Preserve fields buildEntry does not repopulate so a size refresh
	// never drops the HF source names, enrichment, or the resolution
	// timestamp.
	updated.MMProjOrig = entry.MMProjOrig
	updated.MTPOrig = entry.MTPOrig
	updated.ModelType = entry.ModelType
	updated.Capabilities = entry.Capabilities
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
	if mp.MTPFile != "" {
		res.MTP = filepath.Base(mp.MTPFile)
		res.LocalMTP = mp.MTPFile
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
		MTP:          entry.MTP,
		MTPOrig:      entry.MTPOrig,
		MTPChecked:   entry.MTPChecked,
		DownloadURLs: buildDownloadURLs(entry.Provider, entry.Family, entry.Revision, entry.Files),
	}

	// DownloadProj is built from the HuggingFace source name (MMProjOrig),
	// not the local-renamed name (MMProj). Pre-MMProjOrig entries leave
	// MMProjOrig empty so the resolver can detect the gap and self-heal.
	if entry.MMProjOrig != "" {
		res.DownloadProj = buildDownloadURL(entry.Provider, entry.Family, entry.Revision, entry.MMProjOrig)
	}

	// DownloadMTP mirrors DownloadProj: built from the HF source name
	// (MTPOrig), left empty for pre-MTPOrig entries so they self-heal.
	if entry.MTPOrig != "" {
		res.DownloadMTP = buildDownloadURL(entry.Provider, entry.Family, entry.Revision, entry.MTPOrig)
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
		p := filepath.Join(dir, diskName(res.Family, f))
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

	if res.MTP != "" {
		p := filepath.Join(dir, filepath.Base(res.MTP))
		if _, err := os.Stat(p); err == nil {
			res.LocalMTP = p
		}
	}
}

// resolveAtProvider searches a single provider for the model. The bool is
// true on a successful match; false (with nil error) means the provider
// has no matching repo and the caller should try the next one.
//
// preferredFamily, when non-empty, is the HF repo name a prior cache
// entry already resolved to. It is moved to the front of the search
// results so a self-heal repair cannot silently flip the model to a
// different sibling repo.
func (r *Resolver) resolveAtProvider(ctx context.Context, provider, modelID, preferredFamily string) (Resolution, bool, error) {
	searchTerm := stripQuantSuffix(modelID)

	repos, err := r.hfClient.SearchModels(ctx, provider, searchTerm)
	if err != nil {
		if isNotFound(err) {
			return Resolution{}, false, nil
		}
		return Resolution{}, false, err
	}

	repos = orderRepos(repos, modelID, preferredFamily)

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

		files, mmproj, mtp, ok := selectFiles(meta.Siblings, repo, modelID)
		if !ok {
			continue
		}

		res := Resolution{
			CanonicalID:  canonicalID(provider, modelID),
			Provider:     provider,
			Family:       repo,
			Revision:     "main",
			Files:        files,
			MMProj:       localProjName(repo, mmproj, files),
			MMProjOrig:   mmproj,
			MTP:          localMTPName(repo, mtp, files),
			MTPOrig:      mtp,
			DownloadURLs: buildDownloadURLs(provider, repo, "main", files),
		}

		if mmproj != "" {
			res.DownloadProj = buildDownloadURL(provider, repo, "main", mmproj)
		}
		if mtp != "" {
			res.DownloadMTP = buildDownloadURL(provider, repo, "main", mtp)
		}

		return res, true, nil
	}

	return Resolution{}, false, nil
}

// discoverCompanions re-scans the HuggingFace repo backing an existing
// catalog entry to fill in companion files (an mtp-*.gguf drafter and/or a
// missing mmproj projection) with a single ModelMeta round-trip. It serves
// two jobs run during a reconcile:
//
//   - MTP migration: entries persisted before MTP companion support
//     (MTPChecked == false) get the drafter discovered and recorded.
//     MTPChecked is stamped on every successful scan — including when no
//     companion is found — so the work runs at most once per entry.
//
//   - mmproj recovery: when the entry has no mmproj recorded but the
//     renamed projection file is present on disk (the signature of a
//     URL-based download — e.g. an MTP-only pull — that clobbered the
//     projection metadata), the projection source name is recovered so
//     the BUI surfaces it again. Models genuinely without a projection
//     have no on-disk mmproj and so never trip this path.
//
// The scan is skipped entirely when there is nothing to look up. When
// offline or the lookup fails the entry is returned unchanged (ok=false)
// and the work is retried on the next reconcile.
func (r *Resolver) discoverCompanions(ctx context.Context, entry CatalogEntry, log applog.Logger) (CatalogEntry, bool) {
	if len(entry.Files) == 0 {
		return entry, false
	}

	recoverMMProj := entry.MMProj == "" && r.companionOnDisk(entry, projOnDiskName(entry))

	if entry.MTPChecked && !recoverMMProj {
		return entry, false
	}

	if !hasNetwork() {
		return entry, false
	}

	revision := entry.Revision
	if revision == "" {
		revision = "main"
	}

	meta, err := r.hfClient.ModelMeta(ctx, entry.Provider, entry.Family, revision)
	if err != nil {
		log(ctx, "discover-companions: model-meta", "provider", entry.Provider, "family", entry.Family, "ERROR", err)
		return entry, false
	}

	// Companion context: an mtp-*.gguf in this repo is a drafter for the
	// co-resident main model, not a standalone model — unless the repo
	// itself is a dedicated *-MTP-GGUF sibling.
	_, proj, mtpc := classifySiblings(meta.Siblings, repoMatchesRenameRule(entry.Family))
	target := entry.Files[0]

	if !entry.MTPChecked {
		entry.MTPChecked = true
		if mtp := pickMTPCompanion(mtpc, target); mtp != "" {
			entry.MTP = localMTPName(entry.Family, mtp, entry.Files)
			entry.MTPOrig = mtp
			entry.MTPSize = r.companionSize(entry, entry.MTP)
		}
	}

	if recoverMMProj {
		if mmproj := pickF16Mmproj(proj, target); mmproj != "" {
			entry.MMProj = localProjName(entry.Family, mmproj, entry.Files)
			entry.MMProjOrig = mmproj
			entry.MMProjSize = r.companionSize(entry, entry.MMProj)
		}
	}

	return entry, true
}

// projOnDiskName returns the renamed on-disk projection filename a complete
// download would have produced for the entry's model, or "" when the entry
// has no model files.
func projOnDiskName(entry CatalogEntry) string {
	if len(entry.Files) == 0 {
		return ""
	}

	return fmt.Sprintf("mmproj-%s.gguf", catalogModelID(entry.Family, entry.Files[0]))
}

// companionOnDisk reports whether the named companion file exists on disk
// under the entry's canonical model directory.
func (r *Resolver) companionOnDisk(entry CatalogEntry, name string) bool {
	if r.models == nil || name == "" {
		return false
	}

	dir := filepath.Join(r.models.modelsPath, entry.Provider, entry.Family)
	_, err := os.Stat(filepath.Join(dir, filepath.Base(name)))

	return err == nil
}

// companionSize returns the on-disk byte size of the named companion file,
// or 0 when it is not present.
func (r *Resolver) companionSize(entry CatalogEntry, name string) int64 {
	if r.models == nil || name == "" {
		return 0
	}

	dir := filepath.Join(r.models.modelsPath, entry.Provider, entry.Family)
	if fi, err := os.Stat(filepath.Join(dir, filepath.Base(name))); err == nil {
		return fi.Size()
	}

	return 0
}

// orderRepos returns repos sorted so the most likely correct candidate
// is tried first by resolveAtProvider. The ordering rules, highest
// priority first:
//
//  1. Any repo whose name matches preferredFamily (case-insensitive).
//     Used when a cached entry already pinned the right repo and the
//     resolver is only revisiting HF to fill in a missing field.
//  2. Repos that do NOT match any download rename rule (e.g. the
//     "*-MTP-GGUF" siblings). This prevents a bare-name pull like
//     "unsloth/Qwen3.5-0.8B-Q8_0" from silently resolving to a sibling
//     repo whose file would be written to disk under a renamed id.
//  3. Repos that DO match a rename rule come last, unless the requested
//     modelID itself carries the marker (e.g. "mtp-Foo-Q8_0"), in which
//     case those repos are promoted to the front instead.
//
// Sorting is stable so the upstream HF result order is preserved within
// each priority tier.
func orderRepos(repos []string, modelID, preferredFamily string) []string {
	if len(repos) <= 1 {
		return repos
	}

	out := slices.Clone(repos)
	wantMarker := modelIDCarriesRenameMarker(modelID)

	rank := func(ownerRepo string) int {
		_, repo, ok := splitOwnerRepo(ownerRepo)
		if !ok {
			return 3
		}

		if preferredFamily != "" && strings.EqualFold(repo, preferredFamily) {
			return 0
		}

		marked := repoMatchesRenameRule(repo)
		switch {
		case wantMarker && marked:
			return 1
		case wantMarker && !marked:
			return 2
		case !wantMarker && !marked:
			return 1
		default:
			return 2
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		return rank(out[i]) < rank(out[j])
	})

	return out
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
func (m *Models) persistURLResolution(modelURLs []string, projURL, mtpURL string) error {
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
			mmproj = localProjName(repo, file, files)
			mmprojOrig = file
		}
	}

	var mtp, mtpOrig string
	if mtpURL != "" {
		if _, _, _, file, parsed := hf.ParseURL(hf.NormalizeDownloadURL(mtpURL)); parsed {
			mtp = localMTPName(repo, file, files)
			mtpOrig = file
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

	modelID := catalogModelID(repo, files[0])
	canonical := canonicalID(provider, modelID)

	entry := r.buildEntry(provider, repo, revision, files, mmproj, mtp)
	entry.MMProjOrig = mmprojOrig
	entry.MTPOrig = mtpOrig
	if mtpURL != "" {
		entry.MTPChecked = true
	}

	// A URL-based download only carries the companions it was asked to
	// fetch. Preserve any companion already recorded for this model when
	// the current call did not supply it, so pulling just the MTP drafter
	// (projURL == "") does not clobber a previously resolved mmproj, and a
	// projection-only pull does not wipe a tracked MTP companion.
	if prev, ok := rm.Models[canonical]; ok {
		if projURL == "" {
			entry.MMProj = prev.MMProj
			entry.MMProjOrig = prev.MMProjOrig
			entry.MMProjSize = prev.MMProjSize
		}
		if mtpURL == "" {
			entry.MTP = prev.MTP
			entry.MTPOrig = prev.MTPOrig
			entry.MTPSize = prev.MTPSize
			entry.MTPChecked = prev.MTPChecked
		}
	}

	rm.Models[canonical] = entry

	if err := r.Save(rm); err != nil {
		return fmt.Errorf("persist-url: save: %w", err)
	}

	return nil
}

// localProjName returns the on-disk projection filename that downloadModel
// produces by renaming the HuggingFace source file to "mmproj-<modelID>.gguf".
// family is the HF repo segment (e.g. "Qwen3.6-35B-A3B-MTP-GGUF") and is
// threaded in so the rename prefix rules pick up the same prefix the
// downloader applies to model files. Returns "" when there is no projection.
func localProjName(family, hfMMProj string, modelFiles []string) string {
	if hfMMProj == "" || len(modelFiles) == 0 {
		return ""
	}

	return fmt.Sprintf("mmproj-%s.gguf", catalogModelID(family, modelFiles[0]))
}

// localMTPName returns the on-disk MTP drafter filename that downloadModel
// produces by renaming the HuggingFace source file to a canonical
// "mtp-<modelID>.gguf" keyed off the main model. The upstream name (e.g.
// "mtp-gemma-4-26B-A4B-it.gguf") is therefore re-keyed to the requesting
// model id so the index has a deterministic name with no request context.
// Returns "" when there is no MTP companion.
func localMTPName(family, hfMTP string, modelFiles []string) string {
	if hfMTP == "" || len(modelFiles) == 0 {
		return ""
	}

	return fmt.Sprintf("mtp-%s.gguf", catalogModelID(family, modelFiles[0]))
}

// diskName returns the on-disk basename for an upstream HF file name,
// after applying any repo-specific rename rules (e.g. the "mtp-" prefix
// for files coming from a sibling MTP repo).
func diskName(family, file string) string {
	return applyMTPPrefix(family, filepath.Base(file))
}

// catalogModelID returns the model id derived from the on-disk filename
// for a catalog entry — i.e. the key under which the index records the
// model on disk. Use this anywhere catalog metadata (which stores
// upstream HF names) needs to be mapped to the index key.
func catalogModelID(family, file string) string {
	return extractModelID(diskName(family, file))
}
