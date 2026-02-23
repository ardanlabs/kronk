package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"
)

// HFLookupResult contains the information extracted from HuggingFace for
// populating a catalog entry.
type HFLookupResult struct {
	ModelDetails ModelDetails
	RepoFiles    []HFRepoFile
}

// HFRepoFile represents a GGUF file available in a HuggingFace repository.
type HFRepoFile struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	SizeStr  string `json:"size_str"`
}

// LookupHuggingFace queries the HuggingFace API to retrieve model metadata
// and returns a pre-populated ModelDetails. The input can be:
//   - A full URL: https://huggingface.co/owner/repo/resolve/main/file.gguf
//   - A full URL: https://huggingface.co/owner/repo/blob/main/file.gguf
//   - A short form: owner/repo/file.gguf
//
// If filename is empty (only owner/repo provided), the result includes
// RepoFiles listing all available GGUF files in the repository.
func LookupHuggingFace(ctx context.Context, input string) (HFLookupResult, error) {
	owner, repo, filename, err := parseHFInput(input)
	if err != nil {
		return HFLookupResult{}, fmt.Errorf("lookup-huggingface: %w", err)
	}

	modelMeta, err := fetchHFModelMeta(ctx, owner, repo)
	if err != nil {
		return HFLookupResult{}, fmt.Errorf("lookup-huggingface: %w", err)
	}

	repoFiles, err := fetchHFRepoFiles(ctx, owner, repo)
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

// =============================================================================

func parseHFInput(input string) (owner, repo, filename string, err error) {
	input = strings.TrimSpace(input)

	if strings.HasPrefix(input, "https://huggingface.co/") || strings.HasPrefix(input, "http://huggingface.co/") {
		input = strings.TrimPrefix(input, "https://huggingface.co/")
		input = strings.TrimPrefix(input, "http://huggingface.co/")
	}

	parts := strings.Split(input, "/")
	if len(parts) < 2 {
		return "", "", "", fmt.Errorf("parse-hf-input: invalid input %q, expected owner/repo format", input)
	}

	owner = parts[0]
	repo = parts[1]

	if len(parts) > 3 && (parts[2] == "resolve" || parts[2] == "blob") {
		filename = strings.Join(parts[4:], "/")
	} else if len(parts) > 2 {
		filename = strings.Join(parts[2:], "/")
	}

	return owner, repo, filename, nil
}

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

func fetchHFModelMeta(ctx context.Context, owner, repo string) (hfModelMeta, error) {
	url := fmt.Sprintf("https://huggingface.co/api/models/%s/%s", owner, repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return hfModelMeta{}, fmt.Errorf("fetch-hf-model-meta: creating request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return hfModelMeta{}, fmt.Errorf("fetch-hf-model-meta: fetching: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return hfModelMeta{}, fmt.Errorf("fetch-hf-model-meta: unexpected status %d for %s/%s", resp.StatusCode, owner, repo)
	}

	var meta hfModelMeta
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return hfModelMeta{}, fmt.Errorf("fetch-hf-model-meta: decoding: %w", err)
	}

	return meta, nil
}

type hfTreeFile struct {
	Type string `json:"type"`
	Path string `json:"path"`
	Size int64  `json:"size"`
	LFS  *struct {
		Size int64 `json:"size"`
	} `json:"lfs"`
}

func fetchHFRepoFiles(ctx context.Context, owner, repo string) ([]HFRepoFile, error) {
	url := fmt.Sprintf("https://huggingface.co/api/models/%s/%s/tree/main", owner, repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch-hf-repo-files: creating request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch-hf-repo-files: fetching: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch-hf-repo-files: unexpected status %d", resp.StatusCode)
	}

	var treeFiles []hfTreeFile
	if err := json.NewDecoder(resp.Body).Decode(&treeFiles); err != nil {
		return nil, fmt.Errorf("fetch-hf-repo-files: decoding: %w", err)
	}

	var files []HFRepoFile
	for _, tf := range treeFiles {
		if tf.Type != "file" {
			continue
		}

		size := tf.Size
		if tf.LFS != nil {
			size = tf.LFS.Size
		}

		files = append(files, HFRepoFile{
			Filename: tf.Path,
			Size:     size,
			SizeStr:  formatFileSize(size),
		})
	}

	return files, nil
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
		ID:          id,
		Category:    category,
		OwnedBy:     owner,
		ModelFamily: repo,
		WebPage:     fmt.Sprintf("https://huggingface.co/%s/%s", owner, repo),
		GatedModel:  meta.isGated(),
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

func formatFileSize(bytes int64) string {
	const (
		kib = 1024
		mib = kib * 1024
		gib = mib * 1024
	)

	switch {
	case bytes >= gib:
		val := float64(bytes) / float64(gib)
		return fmt.Sprintf("%.1f GB", math.Round(val*10)/10)
	case bytes >= mib:
		val := float64(bytes) / float64(mib)
		return fmt.Sprintf("%.1f MB", math.Round(val*10)/10)
	default:
		val := float64(bytes) / float64(kib)
		return fmt.Sprintf("%.1f KiB", math.Round(val*10)/10)
	}
}
