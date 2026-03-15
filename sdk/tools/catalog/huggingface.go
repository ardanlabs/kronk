package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// HFRepoFile is an alias for models.HFRepoFile so existing callers that
// reference catalog.HFRepoFile continue to compile without changes.
type HFRepoFile = models.HFRepoFile

// HFLookupResult contains the information extracted from HuggingFace for
// populating a catalog entry.
type HFLookupResult struct {
	ModelDetails ModelDetails
	RepoFiles    []HFRepoFile
}

// LookupHuggingFace queries the HuggingFace API to retrieve model metadata
// and returns a pre-populated ModelDetails. The input can be:
//   - A full URL: https://huggingface.co/owner/repo/resolve/main/file.gguf
//   - A full URL: https://huggingface.co/owner/repo/blob/main/file.gguf
//   - A short form: owner/repo/file.gguf
//   - A shorthand: owner/repo:Q4_K_M or hf.co/owner/repo:Q4_K_M
//   - A shorthand with revision: owner/repo:Q4_K_M@revision
//
// If filename is empty (only owner/repo provided), the result includes
// RepoFiles listing all available GGUF files in the repository.
func LookupHuggingFace(ctx context.Context, input string) (HFLookupResult, error) {

	// Try shorthand resolution first (e.g. "owner/repo:Q4_K_M").
	resolved, meta, isShorthand, err := resolveHFShorthandInternal(ctx, input)
	if err != nil {
		return HFLookupResult{}, fmt.Errorf("lookup-huggingface: %w", err)
	}
	if isShorthand {
		return lookupFromResolved(ctx, resolved, meta)
	}

	owner, repo, filename, err := models.ParseHFInput(input)
	if err != nil {
		return HFLookupResult{}, fmt.Errorf("lookup-huggingface: %w", err)
	}

	modelMeta, err := fetchHFModelMeta(ctx, owner, repo, "main")
	if err != nil {
		return HFLookupResult{}, fmt.Errorf("lookup-huggingface: %w", err)
	}

	repoFiles, err := models.FetchHFRepoFiles(ctx, owner, repo, "main", "", true)
	if err != nil {
		return HFLookupResult{}, fmt.Errorf("lookup-huggingface: %w", err)
	}

	var ggufFiles []HFRepoFile
	for _, f := range repoFiles {
		if strings.HasSuffix(strings.ToLower(f.Filename), ".gguf") {
			ggufFiles = append(ggufFiles, f)
		}
	}

	md := buildModelDetails(owner, repo, filename, modelMeta, ggufFiles)

	return HFLookupResult{
		ModelDetails: md,
		RepoFiles:    ggufFiles,
	}, nil
}

// lookupFromResolved builds an HFLookupResult from shorthand-resolved files.
// It reuses the model metadata already fetched by the resolver to avoid a
// redundant API call, and fetches a recursive file listing for sizes.
func lookupFromResolved(ctx context.Context, resolved HFResolvedFiles, modelMeta hfModelMeta) (HFLookupResult, error) {
	repoFiles, err := models.FetchHFRepoFiles(ctx, resolved.Owner, resolved.Repo, resolved.Revision, "", true)
	if err != nil {
		return HFLookupResult{}, fmt.Errorf("lookup-from-resolved: %w", err)
	}

	var ggufFiles []HFRepoFile
	for _, f := range repoFiles {
		if strings.HasSuffix(strings.ToLower(f.Filename), ".gguf") {
			ggufFiles = append(ggufFiles, f)
		}
	}

	// Build a size lookup using the full repo-relative path so we can
	// match files in subdirectories (the recursive tree listing returns
	// paths like "subdir/file.gguf").
	sizeByPath := make(map[string]string, len(ggufFiles))
	for _, f := range ggufFiles {
		sizeByPath[f.Filename] = f.SizeStr
	}

	if len(resolved.ModelFiles) == 0 {
		return HFLookupResult{}, fmt.Errorf("lookup-from-resolved: no resolved model files")
	}

	// Use the first resolved model file as the primary filename for details.
	filename := baseName(resolved.ModelFiles[0])

	md := buildModelDetails(resolved.Owner, resolved.Repo, filename, modelMeta, ggufFiles)

	// Override Files to include all resolved model files and projection.
	modelFiles := make([]File, len(resolved.ModelFiles))
	for i, mf := range resolved.ModelFiles {
		relPath := repoRelativePath(mf, resolved.Owner, resolved.Repo)
		modelFiles[i] = File{URL: mf, Size: sizeByPath[relPath]}
	}
	md.Files.Models = modelFiles

	if resolved.ProjFile != "" {
		relPath := repoRelativePath(resolved.ProjFile, resolved.Owner, resolved.Repo)
		md.Files.Proj = File{URL: resolved.ProjFile, Size: sizeByPath[relPath]}
	}

	return HFLookupResult{
		ModelDetails: md,
		RepoFiles:    ggufFiles,
	}, nil
}

// =============================================================================

type hfModelMeta struct {
	ID          string   `json:"id"`
	Author      string   `json:"author"`
	Gated       any      `json:"gated"`
	PipelineTag string   `json:"pipeline_tag"`
	Tags        []string `json:"tags"`
	CreatedAt   string   `json:"createdAt"`
	CardData    struct {
		License string `json:"license"`
	} `json:"cardData"`
	GGUF struct {
		Total         int64  `json:"total"`
		Architecture  string `json:"architecture"`
		ContextLength int    `json:"context_length"`
	} `json:"gguf"`
	Siblings []struct {
		RFilename string `json:"rfilename"`
	} `json:"siblings"`
}

func (m hfModelMeta) isGated() bool {
	switch v := m.Gated.(type) {
	case bool:
		return v
	case string:
		return v != "" && v != "false"
	default:
		return false
	}
}

func fetchHFModelMeta(ctx context.Context, owner, repo, revision string) (hfModelMeta, error) {
	apiURL := fmt.Sprintf("https://huggingface.co/api/models/%s/%s", url.PathEscape(owner), url.PathEscape(repo))
	if revision != "" && revision != "main" {
		apiURL += "?revision=" + url.QueryEscape(revision)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return hfModelMeta{}, fmt.Errorf("fetch-hf-model-meta: creating request: %w", err)
	}

	if token := os.Getenv("KRONK_HF_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return hfModelMeta{}, fmt.Errorf("fetch-hf-model-meta: fetching: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return hfModelMeta{}, fmt.Errorf("fetch-hf-model-meta: authentication required for %s/%s (status %d) — set KRONK_HF_TOKEN", owner, repo, resp.StatusCode)
		case http.StatusNotFound:
			return hfModelMeta{}, fmt.Errorf("fetch-hf-model-meta: repository %s/%s not found", owner, repo)
		default:
			return hfModelMeta{}, fmt.Errorf("fetch-hf-model-meta: unexpected status %d for %s/%s", resp.StatusCode, owner, repo)
		}
	}

	var meta hfModelMeta
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return hfModelMeta{}, fmt.Errorf("fetch-hf-model-meta: decoding: %w", err)
	}

	return meta, nil
}

func buildModelDetails(owner, repo, filename string, meta hfModelMeta, ggufFiles []HFRepoFile) ModelDetails {
	category := mapPipelineTag(meta.PipelineTag)
	endpoint := mapEndpoint(meta.PipelineTag)

	id := strings.TrimSuffix(filename, ".gguf")

	if filename == "" && len(ggufFiles) > 0 {
		id = ""
	}

	var modelFiles []File
	if filename != "" {
		var size string
		for _, f := range ggufFiles {
			if f.Filename == filename {
				size = f.SizeStr
				break
			}
		}
		modelFiles = []File{
			{
				URL:  fmt.Sprintf("%s/%s/%s", owner, repo, filename),
				Size: size,
			},
		}
	}

	isReasoning := false
	isTooling := false
	for _, tag := range meta.Tags {
		lower := strings.ToLower(tag)
		if lower == "reasoning" {
			isReasoning = true
		}
	}

	lowerRepo := strings.ToLower(repo)
	if strings.Contains(lowerRepo, "instruct") || strings.Contains(lowerRepo, "coder") {
		isTooling = true
	}

	isStreaming := category == "Text-Generation" || category == "Image-Text-to-Text" || category == "Audio-Text-to-Text"

	var created time.Time
	if meta.CreatedAt != "" {
		if t, err := time.Parse(time.RFC3339, meta.CreatedAt); err == nil {
			created = t
		}
	}

	contextWindow := meta.GGUF.ContextLength

	md := ModelDetails{
		ID:           id,
		Category:     category,
		OwnedBy:      owner,
		ModelFamily:  repo,
		Architecture: ClassifyArchitecture(meta.GGUF.Architecture),
		GGUFArch:     meta.GGUF.Architecture,
		WebPage:      fmt.Sprintf("https://huggingface.co/%s/%s", owner, repo),
		GatedModel:   meta.isGated(),
		Files: Files{
			Models: modelFiles,
		},
		Capabilities: Capabilities{
			Endpoint:  endpoint,
			Streaming: isStreaming,
			Reasoning: isReasoning,
			Tooling:   isTooling,
		},
		Metadata: Metadata{
			Created:     created,
			Collections: fmt.Sprintf("collections/%s", owner),
		},
	}

	if contextWindow > 0 {
		md.BaseModelConfig.ContextWindow = contextWindow
	}

	return md
}

func mapPipelineTag(tag string) string {
	switch strings.ToLower(tag) {
	case "text-generation":
		return "Text-Generation"
	case "feature-extraction", "sentence-similarity":
		return "Embedding"
	case "image-text-to-text":
		return "Image-Text-to-Text"
	case "audio-text-to-text", "automatic-speech-recognition":
		return "Audio-Text-to-Text"
	case "text-classification":
		return "Rerank"
	default:
		return "Text-Generation"
	}
}

func mapEndpoint(tag string) string {
	switch strings.ToLower(tag) {
	case "feature-extraction", "sentence-similarity":
		return "embeddings"
	case "text-classification":
		return "rerank"
	default:
		return "chat_completion"
	}
}

// ClassifyArchitecture maps a GGUF general.architecture value (returned by
// the HuggingFace API) to one of the three model types used by the catalog:
// Dense, MoE, or Hybrid. An empty string is returned for unrecognised
// architectures so the user can set the value manually.
func ClassifyArchitecture(arch string) string {
	switch strings.ToLower(arch) {

	// MoE — models with expert routing.
	case "qwen2moe", "qwen3moe", "mixtral", "dbrx", "deepseek2", "arctic", "olmoe":
		return "MoE"

	// Hybrid — models mixing attention with recurrent/SSM/convolution layers.
	case "jamba", "falcon-h1", "bamba", "lfm2", "plamo2", "rwkv6qwen2":
		return "Hybrid"

	// Dense — standard transformer models. We list the common ones explicitly
	// and fall through to a default of "Dense" for any other recognised value
	// so that only a truly empty/unknown architecture returns "".
	case "llama", "qwen2", "qwen3", "phi3", "phi4", "gemma", "gemma2", "gemma3",
		"starcoder2", "command-r", "cohere2", "gpt2", "bloom", "falcon",
		"mpt", "refact", "stablelm", "internlm2", "minicpm", "minicpm3",
		"olmo", "olmo2", "granite", "exaone", "nemotron", "glm4", "chatglm":
		return "Dense"
	}

	if arch != "" {
		return "Dense"
	}

	return ""
}
