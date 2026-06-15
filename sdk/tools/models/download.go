package models

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/hf"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/downloader"
)

// Logger represents a logger for capturing events.
type Logger = applog.Logger

// downloadFn is the package-level downloader function. It defaults to
// downloader.Download and is overridable by tests so download paths
// can be exercised hermetically without network. Test-only seam — do
// not reassign in production code.
var downloadFn = downloader.Download

// hasNetworkFn is the package-level network probe used by download
// flows. It defaults to hasNetwork and is overridable by tests for the
// same reason as downloadFn.
var hasNetworkFn = hasNetwork

// =============================================================================
// Public API

// Download performs a complete workflow for downloading and installing the
// specified model. The input may be:
//
//   - A direct HuggingFace URL ("https://huggingface.co/.../Qwen3-0.6B-Q8_0.gguf")
//   - A canonical model id ("unsloth/Qwen3-0.6B-Q8_0")
//   - A bare model id ("Qwen3-0.6B-Q8_0")
//   - A "provider/repo:tag" quant selector ("unsloth/Qwen3.6-35B-A3B-GGUF:UD-Q4_K_XL")
//
// In every case the projection file (when applicable) is located
// automatically through the resolver. Split (multi-file) models are
// supported transparently — when a model id is supplied, the resolver
// returns every shard URL plus the companion projection file. When a
// full URL is supplied, the file at that URL is downloaded as-is and
// the projection is resolved best-effort by deriving the canonical id
// from the URL.
//
// The resolver checks local disk first, then the resolver-file cache at
// <basePath>/catalog.yaml (seeded from the embedded default on
// first use), then walks the configured HuggingFace provider list.
//
// Successful downloads — whether triggered by URL or by id — are persisted
// to the resolver file so subsequent lookups become cache hits.
//
// To take full control of which files are downloaded — including pinning
// a specific projection file or downloading no projection at all — use
// DownloadURLs.
//
// Set KRONK_HF_TOKEN to access gated models.
func (m *Models) Download(ctx context.Context, log applog.Logger, modelSource string) (Path, error) {
	if isURL(modelSource) {
		return m.downloadByURL(ctx, log, modelSource)
	}

	return m.downloadByID(ctx, log, modelSource)
}

// DownloadURLs performs a complete workflow using explicit URLs for both
// the model file(s) and the projection file. This is the full-control
// API: the caller specifies exactly which files to fetch and the
// resolver is not consulted.
//
// modelURLs may contain a single URL or every shard URL of a split
// model. All model URLs must be fully qualified HuggingFace download
// URLs. projURL may be an empty string when the model has no projection
// file or when the caller does not want one downloaded; when supplied,
// it must also be a fully qualified URL. mtpURL behaves the same way for
// a companion MTP draft file.
//
// For the default workflow, use Download.
//
// Set KRONK_HF_TOKEN to access gated models.
func (m *Models) DownloadURLs(ctx context.Context, log applog.Logger, modelURLs []string, projURL, mtpURL string) (Path, error) {
	if len(modelURLs) == 0 {
		return Path{}, fmt.Errorf("download-urls: no model URLs provided")
	}

	for _, u := range modelURLs {
		if !isURL(u) {
			return Path{}, fmt.Errorf("download-urls: model URL must be fully qualified: %q", u)
		}
	}

	if projURL != "" && !isURL(projURL) {
		return Path{}, fmt.Errorf("download-urls: projection URL must be fully qualified: %q", projURL)
	}

	if mtpURL != "" && !isURL(mtpURL) {
		return Path{}, fmt.Errorf("download-urls: mtp URL must be fully qualified: %q", mtpURL)
	}

	mp, err := m.downloadSplits(ctx, log, modelURLs, projURL, mtpURL)
	if err != nil {
		return mp, err
	}

	if perr := m.persistURLResolution(modelURLs, projURL, mtpURL); perr != nil {
		log(ctx, "download: unable to persist resolver entry", "ERROR", perr)
	}

	m.cacheGGUFHeadBestEffort(ctx, log, modelURLs[0], mp)

	return mp, nil
}

// =============================================================================
// Internal entry points

// downloadByURL downloads the file at modelURL as-is and best-effort
// resolves a companion projection file by deriving the canonical id
// from the URL. When the projection lookup fails the model is still
// downloaded; only the projection is skipped.
func (m *Models) downloadByURL(ctx context.Context, log applog.Logger, modelURL string) (Path, error) {
	projURL, mtpURL := m.lookupCompanionsForURL(ctx, modelURL)

	mp, err := m.downloadSplits(ctx, log, []string{modelURL}, projURL, mtpURL)
	if err != nil {
		return mp, err
	}

	if perr := m.persistURLResolution([]string{modelURL}, projURL, mtpURL); perr != nil {
		log(ctx, "download: unable to persist resolver entry", "ERROR", perr)
	}

	m.cacheGGUFHeadBestEffort(ctx, log, modelURL, mp)

	return mp, nil
}

// downloadByID resolves a bare model id ("Qwen3-0.6B-Q8_0") or canonical
// id ("unsloth/Qwen3-0.6B-Q8_0") through the resolver and downloads the
// resulting files (including any companion mmproj).
func (m *Models) downloadByID(ctx context.Context, log applog.Logger, modelSource string) (Path, error) {
	rfile, err := defaults.CatalogFile("", m.basePath)
	if err != nil {
		return Path{}, fmt.Errorf("download: resolver-file: %w", err)
	}

	r := NewResolver(m, rfile)

	res, err := r.Resolve(ctx, modelSource)
	if err != nil {
		return Path{}, fmt.Errorf("download: resolve %q: %w", modelSource, err)
	}

	// Already on disk — no download needed. attachLocal/lookupLocal only
	// populate LocalPaths when every expected file is present, so the
	// resolver already knows the on-disk layout via Family and we can
	// build the Path directly without consulting the model index.
	//
	// attachLocal uses os.Stat only, which cannot distinguish a complete
	// file from a partial download left behind by a cancelled pull. Run
	// verifyAllSizes against the sha pointer files before short-circuiting;
	// on a size mismatch fall through to the regular download path so the
	// truncated shard (or projection) gets re-fetched.
	if len(res.LocalPaths) > 0 {
		mp := Path{
			ModelFiles: append([]string(nil), res.LocalPaths...),
			ProjFile:   res.LocalProj,
			MTPFile:    res.LocalMTP,
			Downloaded: true,
		}

		if vErr := verifyAllSizes(mp); vErr != nil {
			log(ctx, "download-model: on-disk copy is incomplete, re-downloading", "provider", res.Provider, "family", res.Family, "ERROR", vErr)
		} else {
			log(ctx, "download-model: already installed", "provider", res.Provider, "family", res.Family)

			if res.LocalProj != "" {
				log(ctx, "download-projection: already installed", "provider", res.Provider, "family", res.Family)
			}

			if res.LocalMTP != "" {
				log(ctx, "download-mtp: already installed", "provider", res.Provider, "family", res.Family)
			}

			return mp, nil
		}
	}

	if len(res.DownloadURLs) == 0 {
		return Path{}, fmt.Errorf("download: resolve %q: resolver returned no download URLs", modelSource)
	}

	mp, err := m.downloadSplits(ctx, log, res.DownloadURLs, res.DownloadProj, res.DownloadMTP)
	if err != nil {
		return Path{}, fmt.Errorf("download: download %q: %w", modelSource, err)
	}

	// Files are on disk now — re-persist the catalog entry so FileSizes
	// and MMProjSize get filled by os.Stat. Best-effort.
	if err := r.refreshSizes(res.CanonicalID); err != nil {
		log(ctx, "download: unable to refresh catalog sizes", "ERROR", err)
	}

	// Opportunistically populate the GGUF head cache from the freshly
	// downloaded file so the BUI catalog detail screen renders without
	// an HF round-trip on first view. Best-effort.
	if len(mp.ModelFiles) > 0 {
		modelID := extractModelID(filepath.Base(mp.ModelFiles[0]))
		if err := m.CacheGGUFHeadFromFile(res.Provider, res.Family, modelID, mp.ModelFiles[0]); err != nil {
			log(ctx, "download: unable to cache gguf head", "ERROR", err)
		}
	}

	// Enrich the persisted catalog entry with ModelType and Capabilities
	// while the GGUF head is hot in cache. Best-effort.
	if err := r.enrichCatalogEntry(ctx, res.CanonicalID, log); err != nil {
		log(ctx, "download: unable to enrich catalog entry", "ERROR", err)
	}

	return mp, nil
}

// lookupCompanionsForURL parses a HuggingFace URL into provider/<modelID>
// and asks the resolver for the matching projection and MTP companion
// files. Returns empty strings when the URL cannot be parsed or no
// companion is found.
func (m *Models) lookupCompanionsForURL(ctx context.Context, modelURL string) (projURL, mtpURL string) {
	provider, _, _, file, ok := hf.ParseURL(hf.NormalizeDownloadURL(modelURL))
	if !ok || provider == "" || file == "" {
		return "", ""
	}

	rfile, err := defaults.CatalogFile("", m.basePath)
	if err != nil {
		return "", ""
	}

	canonical := fmt.Sprintf("%s/%s", provider, extractModelID(file))

	res, err := NewResolver(m, rfile).Resolve(ctx, canonical)
	if err != nil {
		return "", ""
	}

	return res.DownloadProj, res.DownloadMTP
}

// cacheGGUFHeadBestEffort populates the GGUF head cache from the first
// downloaded model file so the BUI catalog detail screen renders
// without an HF round-trip. Best-effort — failures are logged but do
// not affect the download outcome.
func (m *Models) cacheGGUFHeadBestEffort(ctx context.Context, log applog.Logger, sourceURL string, mp Path) {
	if len(mp.ModelFiles) == 0 {
		return
	}

	provider, family, _, _, ok := hf.ParseURL(hf.NormalizeDownloadURL(sourceURL))
	if !ok {
		return
	}

	modelID := extractModelID(filepath.Base(mp.ModelFiles[0]))
	if err := m.CacheGGUFHeadFromFile(provider, family, modelID, mp.ModelFiles[0]); err != nil {
		log(ctx, "download: unable to cache gguf head", "ERROR", err)
	}
}

// isURL reports whether input is a direct HTTP(S) URL.
func isURL(input string) bool {
	return strings.HasPrefix(input, "https://") || strings.HasPrefix(input, "http://")
}

// =============================================================================
// Orchestration: split download + index/validation lifecycle

// downloadSplits performs a complete workflow for downloading and installing
// the specified model. If you need to set your HuggingFace token, use the
// environment variable KRONK_HF_TOKEN.
func (m *Models) downloadSplits(ctx context.Context, log applog.Logger, modelURLs []string, projURL, mtpURL string) (result Path, retErr error) {
	if len(modelURLs) == 0 {
		return Path{}, fmt.Errorf("download-splits: no model URLs provided")
	}

	mLoc0, err := newLocator(hf.NormalizeDownloadURL(modelURLs[0]))
	if err != nil {
		return Path{}, fmt.Errorf("download-splits: unable to derive model id: %w", err)
	}
	modelID := mLoc0.ModelID

	if !hasNetworkFn() {
		mp, err := m.FullPath(modelID)
		if err != nil {
			return Path{}, fmt.Errorf("download-splits: no network available: %w", err)
		}

		return mp, nil
	}

	defer m.markValidatedAfterSplits(ctx, log, modelID, &retErr)

	result = Path{
		ModelFiles: make([]string, len(modelURLs)),
	}

	projURL = hf.NormalizeDownloadURL(projURL)

	var projLoc *Locator
	if projURL != "" {
		pl, err := newLocator(projURL)
		if err != nil {
			return Path{}, fmt.Errorf("download-splits: unable to parse proj url: %w", err)
		}
		projLoc = &pl
	}

	mtpURL = hf.NormalizeDownloadURL(mtpURL)

	var mtpLoc *Locator
	if mtpURL != "" {
		ml, err := newLocator(mtpURL)
		if err != nil {
			return Path{}, fmt.Errorf("download-splits: unable to parse mtp url: %w", err)
		}
		mtpLoc = &ml
	}

	for i, modelURL := range modelURLs {
		modelURL = hf.NormalizeDownloadURL(modelURL)

		mLoc, err := newLocator(modelURL)
		if err != nil {
			return Path{}, fmt.Errorf("download-splits: unable to parse model url[%d]: %w", i, err)
		}

		// Only the first shard carries companions — drop after.
		pLoc := projLoc
		dLoc := mtpLoc
		if i > 0 {
			pLoc = nil
			dLoc = nil
		}

		logProjURL := ""
		if pLoc != nil {
			logProjURL = pLoc.RawURL
		}

		log(ctx, fmt.Sprintf("download-model: model-url[%s] proj-url[%s] model-id[%s] file[%d/%d]", mLoc.RawURL, logProjURL, modelID, i+1, len(modelURLs)))
		log(ctx, "download-model: waiting to check model status...")

		progress := func(src string, currentSize int64, totalSize int64, mbPerSec float64, complete bool) {
			log(ctx, fmt.Sprintf("\r\x1b[Kdownload-model: Downloading %s... %d MB of %d MB (%.2f MB/s)", src, currentSize/(1000*1000), totalSize/(1000*1000), mbPerSec))
		}

		mp, errOrg := m.downloadModel(ctx, log, mLoc, pLoc, dLoc, progress)
		if errOrg != nil {
			log(ctx, "download-model:", "ERROR", errOrg, "model-file-url", mLoc.RawURL)

			// Only fall back to the previously installed copy when every shard
			// (and any projection) on disk still matches its sha pointer. A
			// partial split would otherwise be reported as "downloaded" and
			// the next pull would short-circuit on the index instead of
			// retrying the truncated shard.
			if mp, err := m.FullPath(modelID); err == nil && len(mp.ModelFiles) > 0 {
				if vErr := verifyAllSizes(mp); vErr == nil {
					log(ctx, "download-model: using installed version of model files")
					return mp, nil
				}
				log(ctx, "download-model: previously installed copy is incomplete", "ERROR", verifyAllSizes(mp))

				// Don't blow away partial files here. A subsequent pull can
				// re-attempt the affected shard via the getter.
			}

			return Path{}, fmt.Errorf("download-model: unable to download model file: %w", errOrg)
		}

		switch mp.Downloaded {
		case true:
			log(ctx, "download-model: download complete")

		default:
			log(ctx, "download-model: already installed")
		}

		if len(mp.ModelFiles) >= len(modelURLs) {
			for j := i + 1; j < len(modelURLs); j++ {
				log(ctx, fmt.Sprintf("download-model: model-url[%s] proj-url[] model-id[%s]", hf.NormalizeDownloadURL(modelURLs[j]), modelID))
				log(ctx, "download-model: already installed")
			}
			result.ModelFiles = mp.ModelFiles[:len(modelURLs)]
			result.ProjFile = mp.ProjFile
			result.MTPFile = mp.MTPFile
			break
		}

		result.ModelFiles[i] = mp.ModelFiles[0]
		if i == 0 {
			result.ProjFile = mp.ProjFile
			result.MTPFile = mp.MTPFile
		}
	}

	result.Downloaded = true

	return result, nil
}

// markValidatedAfterSplits rebuilds the index and, when the split download
// completed cleanly and every recorded shard still matches its sha pointer,
// stamps the model entry as Validated so subsequent loads skip re-hashing.
// This is the deferred tail of downloadSplits — pulled out so the orchestration
// loop reads as a straight line.
func (m *Models) markValidatedAfterSplits(ctx context.Context, log applog.Logger, modelID string, retErr *error) {
	if err := m.BuildIndex(log, false); err != nil {
		log(ctx, "download-model: unable to create index", "ERROR", err)
		return
	}

	// Only mark validated when every shard (and any projection) for the
	// model finished and matches its sha pointer. Marking validated after
	// a partial split would let the next pull short-circuit on the index
	// and never notice the missing bytes.
	if *retErr != nil {
		log(ctx, "download-model: skipping mark-validated due to error", "model-id", modelID, "ERROR", *retErr)
		return
	}

	if err := m.verifySizesFromIndex(modelID); err != nil {
		log(ctx, "download-model: skipping mark-validated, model is incomplete", "model-id", modelID, "ERROR", err)
		return
	}

	// downloadModel performs SHA validation on every model and projection
	// file as it is fetched, so the freshly downloaded entry can be marked
	// as validated without a full index rebuild.
	if err := m.MarkValidated(modelID); err != nil {
		log(ctx, "download-model: unable to mark model validated", "ERROR", err)
	}
}

// =============================================================================
// Per-model download — state-machine style

// downloadModel pulls a single model file plus any same-repo companions
// (a projection and/or an MTP drafter) using the supplied Locators. The
// high-level flow:
//
//  1. checkValidatedIndex  → fast path on validated, complete index entry
//  2. pull(body) + verify  → download the model file, sha-check it
//  3. downloadCompanion(proj) → projection, when projLoc is set
//  4. downloadCompanion(mtp)  → MTP drafter, when mtpLoc is set
//
// Each companion runs the same reuse-by-URL-name / reuse-by-SHA / fall-
// through-pull state machine (see downloadCompanion). Downloaded is true
// when the model body OR any companion body was freshly fetched.
func (m *Models) downloadModel(ctx context.Context, log applog.Logger, mLoc Locator, projLoc *Locator, mtpLoc *Locator, progress downloader.ProgressFunc) (Path, error) {
	if !strings.Contains(mLoc.RawURL, "/resolve/") {
		return Path{}, fmt.Errorf("download-model: invalid model download url, missing /resolve/: %s", mLoc.RawURL)
	}
	if projLoc != nil && !strings.Contains(projLoc.RawURL, "/resolve/") {
		return Path{}, fmt.Errorf("download-model: invalid proj download url, missing /resolve/: %s", projLoc.RawURL)
	}
	if mtpLoc != nil && !strings.Contains(mtpLoc.RawURL, "/resolve/") {
		return Path{}, fmt.Errorf("download-model: invalid mtp download url, missing /resolve/: %s", mtpLoc.RawURL)
	}

	if mp, hit := m.checkValidatedIndex(ctx, log, mLoc, projLoc, mtpLoc); hit {
		return mp, nil
	}

	modelFileName, downloadedMF, err := m.downloadModelFile(ctx, mLoc, progress)
	if err != nil {
		return Path{}, err
	}

	mp := Path{ModelFiles: []string{modelFileName}, Downloaded: downloadedMF}

	if projLoc != nil {
		projFileName, fetched, err := m.downloadCompanion(ctx, log, *projLoc, modelFileName, companionProj, progress)
		if err != nil {
			return Path{}, err
		}
		mp.ProjFile = projFileName
		if fetched {
			mp.Downloaded = true
		}
	}

	if mtpLoc != nil {
		mtpFileName, fetched, err := m.downloadCompanion(ctx, log, *mtpLoc, modelFileName, companionMTP, progress)
		if err != nil {
			return Path{}, err
		}
		mp.MTPFile = mtpFileName
		if fetched {
			mp.Downloaded = true
		}
	}

	return mp, nil
}

// companionKind identifies a same-repo companion artifact and carries the
// naming/logging knobs that differ between a projection and an MTP
// drafter. The download state machine is otherwise identical for both.
type companionKind struct {
	namePrefix string // on-disk filename prefix, e.g. "mmproj-" / "mtp-"
	shaPrefix  string // sha-scan match prefix, e.g. "mmproj" / "mtp"
	label      string // log/error label, e.g. "proj" / "mtp"
}

var (
	companionProj = companionKind{namePrefix: "mmproj-", shaPrefix: "mmproj", label: "proj"}
	companionMTP  = companionKind{namePrefix: "mtp-", shaPrefix: "mtp", label: "mtp"}
)

// downloadCompanion pulls a single same-repo companion file (projection or
// MTP drafter) into the canonical "<prefix><modelID>.gguf" name next to the
// model body. It applies two optimizations before falling through to a full
// body pull:
//
//  1. tryReuseCompanionFromURLName → a prior pull left the upstream-named file
//  2. tryReuseCompanionFromSHA     → an identical file exists under another id
//
// The returned bool reports whether the companion body was freshly fetched
// (false on a reuse hit) so the caller can maintain the Downloaded flag.
func (m *Models) downloadCompanion(ctx context.Context, log applog.Logger, loc Locator, modelFileName string, kind companionKind, progress downloader.ProgressFunc) (string, bool, error) {
	dstFileName := createCompanionFileName(modelFileName, kind.namePrefix)

	if path, hit, err := m.tryReuseCompanionFromURLName(ctx, log, kind, loc, dstFileName); err != nil {
		return "", false, err
	} else if hit {
		return path, false, nil
	}

	// Pull the companion's sha pointer first so we can compare against any
	// existing local file's sha and skip the body download on a match.
	shaFileName := filepath.Join(filepath.Dir(dstFileName), "sha", filepath.Base(dstFileName))

	orgShaFileName, _, err := m.pull(context.Background(), loc, pullSha, progress)
	if err != nil {
		return "", false, fmt.Errorf("download-model: unable to download sha file: %w", err)
	}

	if path, hit, err := m.tryReuseCompanionFromSHA(ctx, log, kind, orgShaFileName, dstFileName, shaFileName); err != nil {
		return "", false, err
	} else if hit {
		return path, false, nil
	}

	// Rename the downloaded sha file to match our naming convention.
	if err := os.Rename(orgShaFileName, shaFileName); err != nil {
		return "", false, fmt.Errorf("download-model: unable to rename %s sha file: %w", kind.label, err)
	}

	orgFile, _, err := m.pull(ctx, loc, pullBody, progress)
	if err != nil {
		return "", false, err
	}

	if err := os.Rename(orgFile, dstFileName); err != nil {
		return "", false, fmt.Errorf("download-model: unable to rename %s file: %w", kind.label, err)
	}

	if err := model.CheckModel(dstFileName, true); err != nil {
		return "", false, fmt.Errorf("download-model: unable to check model: %w", err)
	}

	return dstFileName, true, nil
}

// checkValidatedIndex returns an existing Path when the index already has a
// validated entry for this model AND every recorded file still passes the
// size check (size mismatch = caller may have deleted the file or a prior
// split download truncated a shard). On a miss the second return is false
// and the caller falls through to the regular pull path.
func (m *Models) checkValidatedIndex(ctx context.Context, log applog.Logger, mLoc Locator, projLoc *Locator, mtpLoc *Locator) (Path, bool) {
	mp, found := m.loadIndex()[mLoc.ModelID]
	if !found || !mp.Validated {
		return Path{}, false
	}

	hasFile := false
	for _, mf := range mp.ModelFiles {
		if filepath.Base(mf) == mLoc.DiskFile {
			hasFile = true
			break
		}
	}
	if !hasFile {
		return Path{}, false
	}

	// Re-verify every recorded file (model splits and any projection) is
	// still present on disk AND matches the size from its sha pointer.
	for _, mf := range mp.ModelFiles {
		if err := model.CheckModel(mf, false); err != nil {
			log(ctx, "download-model: index entry stale, re-downloading", "model-file", mf, "ERROR", err)
			return Path{}, false
		}
	}

	if projLoc != nil {
		if mp.ProjFile == "" {
			return Path{}, false
		}
		if err := model.CheckModel(mp.ProjFile, false); err != nil {
			log(ctx, "download-model: index entry stale, re-downloading projection", "proj-file", mp.ProjFile, "ERROR", err)
			return Path{}, false
		}
	}

	if mtpLoc != nil {
		if mp.MTPFile == "" {
			return Path{}, false
		}
		if err := model.CheckModel(mp.MTPFile, false); err != nil {
			log(ctx, "download-model: index entry stale, re-downloading mtp", "mtp-file", mp.MTPFile, "ERROR", err)
			return Path{}, false
		}
	}

	mp.Downloaded = false
	return mp, true
}

// downloadModelFile fetches the model body and its sha pointer, verifies
// the body against the pointer, and returns the on-disk path of the body.
// The boolean indicates whether the body was freshly downloaded (false
// when the getter no-op'd because the file already existed at the expected
// size).
func (m *Models) downloadModelFile(ctx context.Context, mLoc Locator, progress downloader.ProgressFunc) (string, bool, error) {
	if _, _, err := m.pull(context.Background(), mLoc, pullSha, progress); err != nil {
		return "", false, fmt.Errorf("download-model: unable to download sha file: %w", err)
	}

	modelFileName, downloaded, err := m.pull(ctx, mLoc, pullBody, progress)
	if err != nil {
		return "", false, err
	}

	if err := model.CheckModel(modelFileName, true); err != nil {
		return "", false, fmt.Errorf("download-model: unable to check model: %w", err)
	}

	return modelFileName, downloaded, nil
}

// tryReuseCompanionFromURLName is optimization 1: if a previous download
// landed the companion under its upstream basename, copy it into place
// under the canonical "<prefix><modelID>" name instead of re-fetching the
// body. Misses (returns hit=false) when the URL-name file is absent or
// fails its sha re-check. On a hit it returns the canonical companion path.
func (m *Models) tryReuseCompanionFromURLName(ctx context.Context, log applog.Logger, kind companionKind, loc Locator, dstFileName string) (string, bool, error) {
	urlFileName := loc.DiskFile
	urlFilePath := filepath.Join(filepath.Dir(dstFileName), urlFileName)
	urlShaFilePath := filepath.Join(filepath.Dir(dstFileName), "sha", urlFileName)
	shaFileName := filepath.Join(filepath.Dir(dstFileName), "sha", filepath.Base(dstFileName))

	if _, err := os.Stat(urlFilePath); err != nil {
		return "", false, nil
	}

	log(ctx, "download-model: found existing companion file by URL name, copying", "kind", kind.label, "src", urlFileName, "dst", filepath.Base(dstFileName))

	if err := copyFile(urlFilePath, dstFileName); err != nil {
		return "", false, fmt.Errorf("download-model: unable to copy %s file: %w", kind.label, err)
	}

	if _, err := os.Stat(urlShaFilePath); err == nil {
		if err := copyFile(urlShaFilePath, shaFileName); err != nil {
			return "", false, fmt.Errorf("download-model: unable to copy %s sha file: %w", kind.label, err)
		}
	}

	if err := model.CheckModel(dstFileName, true); err != nil {
		// Copied file failed verification — fall through to regular download.
		return "", false, nil
	}

	log(ctx, "download-model: skipping companion download, using existing file", "kind", kind.label)
	return dstFileName, true, nil
}

// tryReuseCompanionFromSHA is optimization 2: after pulling the companion's
// sha pointer, scan the local sha directory for any existing companion file
// (matching kind.shaPrefix) whose pointer contents are identical. On a
// match, copy that file into place instead of fetching the body — useful
// when the user has the same companion installed under another model id.
func (m *Models) tryReuseCompanionFromSHA(ctx context.Context, log applog.Logger, kind companionKind, orgShaFileName, dstFileName, shaFileName string) (string, bool, error) {
	existing, existingSha, found := m.findMatchingCompanionBySha(orgShaFileName, kind.shaPrefix)
	if !found {
		return "", false, nil
	}

	log(ctx, "download-model: found existing companion file by SHA match, copying", "kind", kind.label, "src", filepath.Base(existing), "dst", filepath.Base(dstFileName))

	if err := copyFile(existing, dstFileName); err != nil {
		return "", false, fmt.Errorf("download-model: unable to copy %s file: %w", kind.label, err)
	}
	if err := copyFile(existingSha, shaFileName); err != nil {
		return "", false, fmt.Errorf("download-model: unable to copy %s sha file: %w", kind.label, err)
	}

	os.Remove(orgShaFileName)

	if err := model.CheckModel(dstFileName, true); err != nil {
		// Copied file failed verification — fall through to regular download.
		return "", false, nil
	}

	log(ctx, "download-model: skipping companion download, using existing file", "kind", kind.label)
	return dstFileName, true, nil
}

// =============================================================================
// Pull (collapsed pullFile + pullShaFile)

// pullKind selects which artifact of a HuggingFace file pull operates on.
// pullBody fetches the model bytes at the resolve URL. pullSha fetches the
// LFS sha pointer at the matching raw URL into a "sha" sibling directory
// and returns its on-disk path.
type pullKind int

const (
	pullBody pullKind = iota
	pullSha
)

// pull is the single artifact fetcher behind both model-body and sha
// downloads. The artifact kind controls (a) which URL transform to apply,
// (b) which destination directory to write to, and (c) the progress
// interval. The sha variant additionally short-circuits to the expected
// on-disk path when there is no network, so partial-offline flows can
// proceed against locally-present pointer files.
func (m *Models) pull(ctx context.Context, loc Locator, kind pullKind, progress downloader.ProgressFunc) (string, bool, error) {
	modelDir := loc.ModelDir(m)
	modelPath := loc.ModelPath(m)

	var destDir, destFile, srcURL string
	var interval int64

	switch kind {
	case pullSha:
		destDir = filepath.Join(modelDir, "sha")
		destFile = filepath.Join(destDir, filepath.Base(modelPath))
		srcURL = strings.Replace(loc.RawURL, "resolve", "raw", 1)
		interval = 0

		// Offline: trust the sha already on disk; downloads of the body
		// will pick it up via the standard CheckModel path.
		if !hasNetworkFn() {
			return destFile, false, nil
		}

	case pullBody:
		destDir = modelDir
		destFile = modelPath
		srcURL = loc.RawURL
		interval = downloader.SizeIntervalMB100

	default:
		return "", false, fmt.Errorf("pull: unknown kind %d", kind)
	}

	src, err := withDestFilename(srcURL, filepath.Base(destFile))
	if err != nil {
		return "", false, fmt.Errorf("pull: %w", err)
	}

	downloaded, err := downloadFn(ctx, src, destDir, progress, interval)
	if err != nil {
		return "", false, fmt.Errorf("pull: unable to download: %w", err)
	}

	return destFile, downloaded, nil
}

// =============================================================================
// Locator — URL → on-disk-name derivation, done once per file.

// Locator captures every name that derives from a single HuggingFace
// download URL: the owner/repo, the upstream filename, the on-disk
// filename after any rename rules, and the model id used for index
// lookups. One parse — downstream code reads fields rather than
// re-parsing the URL in five places.
type Locator struct {
	RawURL       string // the normalized HF resolve URL passed in
	Owner        string // first path segment ("Qwen")
	Repo         string // second path segment ("Qwen3-8B-GGUF")
	UpstreamFile string // url basename ("Qwen3-8B-Q8_0.gguf")
	DiskFile     string // upstream after applyRenamePrefix ("mtp-..." for MTP repos)
	ModelID      string // extractModelID(DiskFile) — matches the index key
}

// newLocator parses a HuggingFace download URL into a Locator. Accepts
// the optional kronk "/download" prefix used by mirror-download URLs.
func newLocator(rawURL string) (Locator, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return Locator{}, fmt.Errorf("locator: parse: %w", err)
	}

	urlPath := strings.TrimPrefix(u.Path, "/download")
	parts := strings.Split(urlPath, "/")
	if len(parts) < 3 {
		return Locator{}, fmt.Errorf("locator: invalid huggingface url: %q", u.Path)
	}

	// urlPath like "/owner/repo/resolve/main/file": after the leading
	// empty segment, owner=parts[1], repo=parts[2], file=path.Base.
	owner := parts[1]
	repo := parts[2]
	upstream := path.Base(u.Path)
	disk := applyRenamePrefix(repo, upstream)

	return Locator{
		RawURL:       rawURL,
		Owner:        owner,
		Repo:         repo,
		UpstreamFile: upstream,
		DiskFile:     disk,
		ModelID:      extractModelID(disk),
	}, nil
}

// ModelDir returns the on-disk directory where this model's files live:
// <modelsPath>/<owner>/<repo>.
func (l Locator) ModelDir(m *Models) string {
	return filepath.Join(m.modelsPath, l.Owner, l.Repo)
}

// ModelPath returns the on-disk path of the model body file, after any
// rename rules (e.g. "mtp-" prefix) have been applied.
func (l Locator) ModelPath(m *Models) string {
	return filepath.Join(l.ModelDir(m), l.DiskFile)
}

// =============================================================================
// Index/size verification

// verifyAllSizes confirms every shard (and the projection, if any) of the
// supplied model entry exists on disk and its size matches the value
// recorded in the companion sha pointer file. CheckModel is called with
// checkSHA=false so the (very expensive) sha256 re-hash is skipped — for
// detecting an interrupted download a size mismatch is all we need.
func verifyAllSizes(mp Path) error {
	for _, mf := range mp.ModelFiles {
		if err := model.CheckModel(mf, false); err != nil {
			return fmt.Errorf("verify-sizes: model-file[%s]: %w", filepath.Base(mf), err)
		}
	}

	if mp.ProjFile != "" {
		if err := model.CheckModel(mp.ProjFile, false); err != nil {
			return fmt.Errorf("verify-sizes: proj-file[%s]: %w", filepath.Base(mp.ProjFile), err)
		}
	}

	if mp.MTPFile != "" {
		if err := model.CheckModel(mp.MTPFile, false); err != nil {
			return fmt.Errorf("verify-sizes: mtp-file[%s]: %w", filepath.Base(mp.MTPFile), err)
		}
	}

	return nil
}

// verifySizesFromIndex resolves the model id back to its on-disk paths and
// runs verifyAllSizes against them. Used by downloadSplits to gate the
// "mark validated" defer so a partially-completed split does not get
// stamped as valid in the index.
func (m *Models) verifySizesFromIndex(modelID string) error {
	mp, err := m.FullPath(modelID)
	if err != nil {
		return fmt.Errorf("verify-sizes-from-index: %w", err)
	}

	if len(mp.ModelFiles) == 0 {
		return fmt.Errorf("verify-sizes-from-index: no model files recorded for %q", modelID)
	}

	return verifyAllSizes(mp)
}

// =============================================================================
// Naming helpers

// createCompanionFileName returns the canonical on-disk path for a
// same-repo companion of modelFileName, re-keyed to the model id with the
// supplied prefix ("mmproj-" or "mtp-"). The companion lands next to the
// model body so a single family directory holds the model, its projection,
// and its MTP drafter.
func createCompanionFileName(modelFileName, prefix string) string {
	modelID := extractModelID(modelFileName)
	companionName := fmt.Sprintf("%s%s%s", prefix, modelID, filepath.Ext(modelFileName))

	dir := filepath.Dir(modelFileName)
	name := filepath.Join(dir, companionName)

	// modelFileName: /Users/bill/.kronk/models/Qwen/Qwen3-8B-GGUF/Qwen3-8B-Q8_0.gguf
	// prefix:        mmproj-
	// modelID:       Qwen3-8B-Q8_0
	// companionName: mmproj-Qwen3-8B-Q8_0.gguf
	// dir:           /Users/bill/.kronk/models/Qwen/Qwen3-8B-GGUF
	// name:          /Users/bill/.kronk/models/Qwen/Qwen3-8B-GGUF/mmproj-Qwen3-8B-Q8_0.gguf

	return name
}

// =============================================================================
// Repo-based file rename rules
//
// Some HuggingFace sibling repos publish a GGUF whose upstream filename
// collides with another repo (e.g. unsloth/Qwen3.6-35B-A3B-MTP-GGUF and
// unsloth/Qwen3.6-35B-A3B-GGUF both ship "Qwen3.6-35B-A3B-UD-Q2_K_XL.gguf").
// To keep on-disk paths and model ids unambiguous, the downloader writes
// files from rule-matching repos with a prefix applied to the filename.

// renamePrefixRule prepends prefix to a filename when the upstream repo
// segment matches repoPattern AND the filename does not already encode
// the prefix marker (case-insensitive substring check). The marker
// keeps the rule idempotent: a re-download never produces "mtp-mtp-".
type renamePrefixRule struct {
	repoPattern *regexp.Regexp
	prefix      string // includes any separator, e.g. "mtp-"
	marker      string // lowercase substring; if present in fileName the rename is a no-op
}

var renamePrefixRules = []renamePrefixRule{
	{
		repoPattern: regexp.MustCompile(`(?i)(^|[-_])mtp([-_]|$)`),
		prefix:      "mtp-",
		marker:      "mtp",
	},
}

// applyRenamePrefix walks renamePrefixRules and returns fileName with
// the first matching rule's prefix prepended. When no rule matches, or
// the marker is already present in fileName, the original name is
// returned unchanged.
func applyRenamePrefix(repoSegment, fileName string) string {
	lower := strings.ToLower(fileName)
	for _, r := range renamePrefixRules {
		if !r.repoPattern.MatchString(repoSegment) {
			continue
		}
		if strings.Contains(lower, r.marker) {
			return fileName
		}
		return r.prefix + fileName
	}
	return fileName
}

// repoMatchesRenameRule reports whether repoSegment matches any active
// rename-prefix rule (e.g. the unsloth "*-MTP-GGUF" sibling repos). The
// catalog resolver uses this to de-prioritize sibling repos during HF
// search so a bare model id never silently resolves to the renamed
// variant.
func repoMatchesRenameRule(repoSegment string) bool {
	for _, r := range renamePrefixRules {
		if r.repoPattern.MatchString(repoSegment) {
			return true
		}
	}
	return false
}

// modelIDCarriesRenameMarker reports whether modelID already encodes a
// rename-rule marker (e.g. starts with "mtp-"). When true, the resolver
// should prefer sibling repos that match the same marker rather than
// de-prioritize them.
func modelIDCarriesRenameMarker(modelID string) bool {
	lower := strings.ToLower(modelID)
	for _, r := range renamePrefixRules {
		if strings.HasPrefix(lower, r.marker+"-") || strings.HasPrefix(lower, r.marker+"_") {
			return true
		}
	}
	return false
}

// applyMTPPrefix is the MTP-specific convenience wrapper around
// applyRenamePrefix. Kept as a named function because callers in catalog
// helpers read more clearly when the intent is spelled out.
func applyMTPPrefix(repoSegment, fileName string) string {
	return applyRenamePrefix(repoSegment, fileName)
}

var splitPattern = regexp.MustCompile(`-\d+-of-\d+$`)

func extractModelID(modelFileName string) string {
	name := strings.TrimSuffix(filepath.Base(modelFileName), filepath.Ext(modelFileName))
	name = splitPattern.ReplaceAllString(name, "")

	// modelFileName: /Users/bill/.kronk/models/Qwen/Qwen3-8B-GGUF/Qwen3-8B-Q8_0.gguf
	// name: Qwen3-8B-Q8_0

	// modelFileName: /Users/bill/.kronk/models/unsloth/Llama-3.3-70B-Instruct-GGUF/Llama-3.3-70B-Instruct-Q8_0-00001-of-00002.gguf
	// name: Llama-3.3-70B-Instruct-Q8_0-00001-of-00002
	// name: Llama-3.3-70B-Instruct-Q8_0

	return name
}

// withDestFilename returns rawURL with a "?filename=<destName>" query
// parameter attached when destName differs from the URL's own basename.
// go-getter consumes this hint client-side (it strips the parameter
// before issuing the HTTP request) and writes the response body to the
// requested filename from byte zero — avoiding the bad UX of a
// multi-GB download appearing under its upstream name and only being
// renamed after the transfer completes. When destName already matches
// the URL basename the original URL is returned unchanged.
func withDestFilename(rawURL, destName string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("with-dest-filename: parse: %w", err)
	}

	if path.Base(u.Path) == destName {
		return rawURL, nil
	}

	q := u.Query()
	q.Set("filename", destName)
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// =============================================================================
// Misc helpers

// hasNetwork reports whether Kronk can reach huggingface.co, the host model
// downloads actually target. It issues a real HTTP request through a client
// that honors HTTP_PROXY/HTTPS_PROXY, so the probe exercises exactly the path
// the subsequent download will take and succeeds in proxy-only environments
// where a raw outbound TCP dial is blocked. Setting KRONK_SKIP_NETWORK_CHECK
// bypasses the probe for unusual setups.
func hasNetwork() bool {
	if os.Getenv("KRONK_SKIP_NETWORK_CHECK") != "" {
		return true
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, "https://huggingface.co", nil)
	if err != nil {
		return false
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}

	resp.Body.Close()

	return true
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// findMatchingCompanionBySha scans the local sha directory for a companion
// sha pointer (basename starting with prefix, e.g. "mmproj" or "mtp")
// whose contents match newShaFile, returning the matching companion body
// and its sha pointer. Used to reuse an identical companion already on
// disk under another model id instead of re-downloading it.
func (m *Models) findMatchingCompanionBySha(newShaFile, prefix string) (companionFile, shaFile string, found bool) {
	newShaContent, err := os.ReadFile(newShaFile)
	if err != nil {
		return "", "", false
	}

	shaDir := filepath.Dir(newShaFile)

	entries, err := os.ReadDir(shaDir)
	if err != nil {
		return "", "", false
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}

		existingShaPath := filepath.Join(shaDir, name)
		if existingShaPath == newShaFile {
			continue
		}

		existingShaContent, err := os.ReadFile(existingShaPath)
		if err != nil {
			continue
		}

		if string(existingShaContent) == string(newShaContent) {
			existingFile := filepath.Join(filepath.Dir(shaDir), name)
			if _, err := os.Stat(existingFile); err == nil {
				return existingFile, existingShaPath, true
			}
		}
	}

	return "", "", false
}
