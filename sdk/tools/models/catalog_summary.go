package models

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
)

// CatalogFile is one downloadable artifact (model or projection).
type CatalogFile struct {
	URL  string `json:"url"`
	Size int64  `json:"size"`
}

// CatalogFiles groups the model files and a single projection (if any).
type CatalogFiles struct {
	Model []CatalogFile `json:"model"`
	Proj  CatalogFile   `json:"proj"`
}

// CatalogCapabilities describes what the model can do, derived from
// GGUF metadata + presence of a projection file.
type CatalogCapabilities struct {
	Endpoint  string `json:"endpoint"             yaml:"endpoint,omitempty"`
	Images    bool   `json:"images"               yaml:"images,omitempty"`
	Audio     bool   `json:"audio"                yaml:"audio,omitempty"`
	Video     bool   `json:"video"                yaml:"video,omitempty"`
	Streaming bool   `json:"streaming"            yaml:"streaming,omitempty"`
	Reasoning bool   `json:"reasoning"            yaml:"reasoning,omitempty"`
	Tooling   bool   `json:"tooling"              yaml:"tooling,omitempty"`
	Embedding bool   `json:"embedding"            yaml:"embedding,omitempty"`
	Rerank    bool   `json:"rerank"               yaml:"rerank,omitempty"`
}

// CatalogSummary is the cheap per-entry payload. It only consults
// catalog.yaml and the local model index — no GGUF reads. ModelType and
// Capabilities are read from the persisted catalog entry (populated when
// the entry was added/refreshed) so the list page can filter by them
// without paying GGUF I/O on every call.
type CatalogSummary struct {
	ID             string              `json:"id"`
	OwnedBy        string              `json:"owned_by"`
	ModelFamily    string              `json:"model_family"`
	Revision       string              `json:"revision"`
	WebPage        string              `json:"web_page"`
	TotalSize      string              `json:"total_size"`
	TotalSizeBytes int64               `json:"total_size_bytes"`
	HasProjection  bool                `json:"has_projection"`
	Downloaded     bool                `json:"downloaded"`
	Validated      bool                `json:"validated"`
	ModelType      string              `json:"model_type,omitempty"`
	Capabilities   CatalogCapabilities `json:"capabilities,omitzero"`
}

// CatalogDetail is the full per-entry payload, layering GGUF-derived
// metadata on top of the summary. ModelType and Capabilities live on the
// embedded CatalogSummary so list and detail share a single source. The
// GGUF bytes come from the catalog cache when present, the downloaded
// file when not, or HF Range otherwise.
type CatalogDetail struct {
	CatalogSummary
	GGUFArch       string            `json:"gguf_arch"`
	Parameters     string            `json:"parameters"`
	ParameterCount int64             `json:"parameter_count"`
	Template       string            `json:"template"`
	Files          CatalogFiles      `json:"files"`
	ModelMetadata  map[string]string `json:"model_metadata,omitempty"`
}

// IndexState returns two maps from the local model index keyed by canonical
// id: downloaded[id] is true when the model is present on disk; validated[id]
// is true when integrity checks have passed.
func (m *Models) IndexState() (downloaded, validated map[string]bool) {
	downloaded = map[string]bool{}
	validated = map[string]bool{}

	files, err := m.Files()
	if err != nil {
		return
	}

	for _, mf := range files {
		canonical := mf.OwnedBy + "/" + mf.ID
		downloaded[canonical] = true
		validated[canonical] = mf.Validated
	}

	return
}

// NewSummary builds a CatalogSummary from a catalog entry plus the local
// index state.
func NewSummary(canonical string, entry CatalogEntry, downloaded, validated map[string]bool) CatalogSummary {
	totalBytes := entry.MMProjSize
	for _, n := range entry.FileSizes {
		totalBytes += n
	}

	return CatalogSummary{
		ID:             canonical,
		OwnedBy:        entry.Provider,
		ModelFamily:    entry.Family,
		Revision:       entry.Revision,
		WebPage:        webPage(entry.Provider, entry.Family),
		TotalSize:      FormatBytes(totalBytes),
		TotalSizeBytes: totalBytes,
		HasProjection:  entry.MMProj != "",
		Downloaded:     downloaded[canonical],
		Validated:      validated[canonical],
		ModelType:      entry.ModelType,
		Capabilities:   entry.Capabilities,
	}
}

// NewFiles builds the URL+size structure used by the detail payload.
func NewFiles(entry CatalogEntry) CatalogFiles {
	out := CatalogFiles{
		Model: make([]CatalogFile, len(entry.Files)),
	}

	for i, f := range entry.Files {
		var size int64
		if i < len(entry.FileSizes) {
			size = entry.FileSizes[i]
		}

		out.Model[i] = CatalogFile{
			URL:  BuildHFURL(entry.Provider, entry.Family, entry.Revision, f),
			Size: size,
		}
	}

	if entry.MMProj != "" {
		// The projection URL is built from the HuggingFace source name
		// (MMProjOrig). Pre-MMProjOrig entries leave the URL empty until
		// the resolver self-heals on the next online Resolve.
		var projURL string
		if entry.MMProjOrig != "" {
			projURL = BuildHFURL(entry.Provider, entry.Family, entry.Revision, entry.MMProjOrig)
		}
		out.Proj = CatalogFile{
			URL:  projURL,
			Size: entry.MMProjSize,
		}
	}

	return out
}

// =============================================================================

// webPage synthesizes the canonical HF model URL.
func webPage(provider, family string) string {
	return fmt.Sprintf("https://huggingface.co/%s/%s", provider, family)
}

// BuildHFURL composes a HuggingFace resolve URL.
func BuildHFURL(owner, repo, revision, file string) string {
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

// FormatBytes renders a byte count as a human-readable string ("1.23 GB",
// "456 MB", etc.). Returns "" for non-positive input.
func FormatBytes(n int64) string {
	const (
		kib = 1024
		mib = 1024 * kib
		gib = 1024 * mib
	)

	switch {
	case n <= 0:
		return ""
	case n >= gib:
		return fmt.Sprintf("%.2f GB", float64(n)/float64(gib))
	case n >= mib:
		return fmt.Sprintf("%.0f MB", float64(n)/float64(mib))
	case n >= kib:
		return fmt.Sprintf("%.0f KB", float64(n)/float64(kib))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// FormatParameterCount turns a raw count into "8B", "300M" style. Returns
// "" for zero/unknown.
func FormatParameterCount(n int64) string {
	switch {
	case n <= 0:
		return ""
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.1fB", float64(n)/1e9)
	case n >= 1_000_000:
		return fmt.Sprintf("%.0fM", float64(n)/1e6)
	default:
		return strconv.FormatInt(n, 10)
	}
}

// TemplateName returns "tokenizer.chat_template" when present, indicating
// the model carries an embedded chat template; otherwise "".
func TemplateName(metadata map[string]string) string {
	if gguf.HasChatTemplate(metadata) {
		return "tokenizer.chat_template"
	}
	return ""
}

// CapabilitiesFor derives a coarse capability set from GGUF metadata and
// projection presence. The mappings are intentionally simple and may need
// refinement as new architectures are added.
//
// Audio / video detection scans a wide hint string built from
// general.architecture, general.name, general.basename, and general.tags
// because each GGUF surfaces the multimodal signal in a different place:
//
//   - Some carry it in the architecture (e.g. "qwen3omni").
//   - Some only carry it in the name / basename (e.g. "Qwen3-Omni-...").
//   - Some report a generic architecture but list "any-to-any", "audio",
//     "video", or "omni" in general.tags (the HuggingFace standard).
//
// "any-to-any" and "omni" both turn on audio and video; the explicit
// "audio" and "video" tokens turn on the matching modality only.
func CapabilitiesFor(metadata map[string]string, hasProjection bool) CatalogCapabilities {
	arch := strings.ToLower(gguf.DetectArchitecture(metadata))

	caps := CatalogCapabilities{
		Streaming: true,
	}

	hasTemplate := gguf.HasChatTemplate(metadata)
	switch {
	case strings.Contains(arch, "embed"):
		caps.Endpoint = "embeddings"
		caps.Embedding = true
		caps.Streaming = false
	case strings.Contains(arch, "rerank") || strings.Contains(arch, "bert"):
		caps.Endpoint = "rerank"
		caps.Rerank = true
		caps.Streaming = false
	default:
		caps.Endpoint = "chat_completion"
		caps.Tooling = hasTemplate
		caps.Reasoning = hasTemplate
	}

	if hasProjection {
		caps.Images = true

		hint := strings.ToLower(strings.Join([]string{
			arch,
			gguf.GeneralName(metadata),
			gguf.GeneralBasename(metadata),
			gguf.GeneralTags(metadata),
		}, " "))

		anyToAny := strings.Contains(hint, "any-to-any") || strings.Contains(hint, "omni")

		if anyToAny || strings.Contains(hint, "audio") {
			caps.Audio = true
		}
		if anyToAny || strings.Contains(hint, "video") {
			caps.Video = true
		}
	}

	return caps
}

// ParameterCount extracts the model's parameter count from GGUF metadata.
// Returns 0 when the metadata value is missing or unparseable.
func ParameterCount(metadata map[string]string) int64 {
	return gguf.ParameterCount(metadata)
}

// ParametersLabel returns a human-readable parameter count for the model,
// preferring "general.parameter_count" when present and falling back to
// "general.size_label" (e.g. "0.6B", "20B") which most modern GGUFs ship
// instead of the numeric count. Returns "" when neither is available.
func ParametersLabel(metadata map[string]string) string {
	if n := ParameterCount(metadata); n > 0 {
		return FormatParameterCount(n)
	}
	return gguf.SizeLabel(metadata)
}

// ArchitectureClass classifies a model as "Dense", "MoE", or "Hybrid"
// based on GGUF metadata. Hybrid wins over MoE when both apply because
// the recurrent state cleanup path is the differentiator that matters
// for the engine.
func ArchitectureClass(metadata map[string]string) string {
	if gguf.IsHybridArchitecture(metadata) {
		return "Hybrid"
	}
	if detectMoE(metadata).IsMoE {
		return "MoE"
	}
	return "Dense"
}
