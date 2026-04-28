package toolapp

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// listCatalog returns all entries from catalog.yaml as cheap summaries.
// No GGUF reads, no HF traffic.
func (a *app) listCatalog(ctx context.Context, r *http.Request) web.Encoder {
	cat, err := a.models.Catalog()
	if err != nil {
		return errs.Errorf(errs.Internal, "load catalog: %s", err)
	}

	downloaded, validated := a.models.IndexState()

	out := make(CatalogListResponse, 0, len(cat.Models))
	for canonical, entry := range cat.Models {
		out = append(out, models.NewSummary(canonical, entry, downloaded, validated))
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})

	return out
}

// showCatalog returns a single catalog entry with GGUF-derived metadata.
// The GGUF head bytes come from the catalog cache when present, the
// downloaded file when not, or HF when neither is available.
func (a *app) showCatalog(ctx context.Context, r *http.Request) web.Encoder {
	id := strings.TrimSpace(web.Param(r, "id"))
	if id == "" {
		return errs.Errorf(errs.InvalidArgument, "id is required")
	}

	entry, ok, err := a.models.CatalogEntry(id)
	if err != nil {
		return errs.Errorf(errs.Internal, "lookup catalog entry: %s", err)
	}
	if !ok {
		return errs.Errorf(errs.NotFound, "catalog entry %q not found", id)
	}

	downloaded, validated := a.models.IndexState()

	detail := CatalogDetailResponse{
		CatalogDetail: models.CatalogDetail{
			CatalogSummary: models.NewSummary(id, entry, downloaded, validated),
			Files:          models.NewFiles(entry),
		},
	}

	// Pull GGUF metadata best-effort; the screen still renders even if
	// the head can't be sourced (offline + nothing cached + nothing
	// downloaded).
	data, ggufErr := a.models.GGUFHead(ctx, entry)
	if ggufErr != nil {
		a.log.Info(ctx, "catalog-show", "id", id, "WARN", "no gguf head", "ERROR", ggufErr)
		return detail
	}

	metadata, perr := models.ParseGGUFMetadata(data)
	if perr != nil {
		a.log.Info(ctx, "catalog-show", "id", id, "WARN", "parse gguf", "ERROR", perr)
		return detail
	}

	detail.ModelMetadata = metadata
	detail.GGUFArch = metadata["general.architecture"]
	detail.ModelType = models.ArchitectureClass(metadata)
	detail.ParameterCount = models.ParameterCount(metadata)
	detail.Parameters = models.ParametersLabel(metadata)
	detail.Template = models.TemplateName(metadata)
	detail.Capabilities = models.CapabilitiesFor(metadata, entry.MMProj != "")

	// Compute the initial VRAM estimate from the same head bytes so
	// the detail screen renders the calculator without a second
	// network round trip. The total file size includes the projection
	// when present.
	totalSize := entry.MMProjSize
	for _, n := range entry.FileSizes {
		totalSize += n
	}

	cfg := models.VRAMConfig{
		ContextWindow:   models.ContextWindow8K,
		BytesPerElement: models.BytesPerElementQ8_0,
		Slots:           models.Slots1,
	}

	if v, vErr := models.BuildVRAMFromBytes(data, totalSize, cfg); vErr != nil {
		a.log.Info(ctx, "catalog-show", "id", id, "WARN", "build vram", "ERROR", vErr)
	} else {
		vr := toVRAMResponse(v, nil)
		detail.Vram = &vr
	}

	return detail
}

// lookupCatalog resolves a HuggingFace URL or shorthand to the list of GGUF
// files in the underlying repository so the VRAM calculator can let the
// user pick a specific shard or quant. Read-only; no catalog mutation.
func (a *app) lookupCatalog(ctx context.Context, r *http.Request) web.Encoder {
	var req LookupRequest
	if err := web.Decode(r, &req); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	input := strings.TrimSpace(req.Input)
	if input == "" {
		return errs.Errorf(errs.InvalidArgument, "input is required")
	}

	owner, repo, _, err := models.ParseHFInput(input)
	if err != nil || owner == "" || repo == "" {
		return errs.Errorf(errs.InvalidArgument, "unable to parse input %q", input)
	}

	all, err := models.FetchHFRepoFiles(ctx, owner, repo, "main", "", true)
	if err != nil {
		return errs.Errorf(errs.Internal, "fetch repo files: %s", err)
	}

	var ggufFiles []HFRepoFile
	for _, f := range all {
		if !strings.HasSuffix(strings.ToLower(f.Filename), ".gguf") {
			continue
		}

		ggufFiles = append(ggufFiles, HFRepoFile{
			Filename: f.Filename,
			Size:     f.Size,
			SizeStr:  f.SizeStr,
		})
	}

	return LookupResponse{RepoFiles: ggufFiles}
}

// reconcileCatalog runs ReconcileCatalog. New on-disk models get added,
// pre-existing entries are re-enriched when the persisted schema version
// lags the code-side constant, and the new schema is stamped on save.
// Steady-state runs (schema match, no missing entries) are nearly free;
// the BUI Catalog Refresh button hits this so users pick up enrichment
// fixes without having to also click Models → Rebuild Index.
func (a *app) reconcileCatalog(ctx context.Context, r *http.Request) web.Encoder {
	if err := a.models.ReconcileCatalog(ctx, a.log.Info); err != nil {
		return errs.Errorf(errs.Internal, "reconcile catalog: %s", err)
	}

	return ReconcileResponse{Status: "reconciled"}
}

// removeCatalog removes a catalog entry along with any downloaded files
// and the GGUF cache file. Removing a model from the model list (via
// /v1/models/{model} DELETE) does NOT touch the catalog; this endpoint
// is the only path that does both.
func (a *app) removeCatalog(ctx context.Context, r *http.Request) web.Encoder {
	id := strings.TrimSpace(web.Param(r, "id"))
	if id == "" {
		return errs.Errorf(errs.InvalidArgument, "id is required")
	}

	a.log.Info(ctx, "catalog-remove", "id", id)

	if err := a.models.RemoveCatalogEntry(ctx, id, a.log.Info); err != nil {
		return errs.Errorf(errs.Internal, "remove catalog entry: %s", err)
	}

	return RemoveResponse{Status: "removed", ID: id}
}
