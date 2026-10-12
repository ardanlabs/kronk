package vram

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/hf"
)

// FromHuggingFace fetches GGUF metadata from HuggingFace using HTTP
// Range requests and calculates VRAM requirements. Only the header is
// downloaded, not the entire model file.
//
// The modelURL can be either:
//   - A single file URL: https://huggingface.co/org/repo/resolve/main/model.gguf
//   - A folder URL for split models: https://huggingface.co/org/repo/tree/main/UD-Q5_K_XL
func FromHuggingFace(ctx context.Context, modelURL string, cfg Config) (Result, error) {
	if isFolderURL(modelURL) {
		return fromFolder(ctx, modelURL, cfg)
	}

	modelURL = hf.NormalizeDownloadURL(modelURL)

	metadata, tensors, fileSize, err := fetchHeaderAndTensors(ctx, modelURL)
	if err != nil {
		return Result{}, fmt.Errorf("from-huggingface: failed to fetch GGUF metadata: %w", err)
	}

	return buildFromMetadata(metadata, [][]gguf.TensorInfo{tensors}, fileSize, cfg)
}

// fromFolder handles VRAM calculation for split models hosted in a
// HuggingFace folder. It lists all GGUF files in the folder, sums their
// sizes, and reads metadata from the first split file.
func fromFolder(ctx context.Context, folderURL string, cfg Config) (Result, error) {
	fileURLs, _, err := fetchFolderFiles(ctx, folderURL)
	if err != nil {
		return Result{}, fmt.Errorf("from-folder: %w", err)
	}

	return FromHuggingFaceFiles(ctx, fileURLs, cfg)
}

// FromHuggingFaceFiles computes VRAM requirements from a set of
// pre-resolved HuggingFace file URLs (e.g. from shorthand resolution).
// It reads metadata from the first file and tensor descriptors and sizes
// from every shard so output weights in later shards are included.
func FromHuggingFaceFiles(ctx context.Context, modelURLs []string, cfg Config) (Result, error) {
	if len(modelURLs) == 0 {
		return Result{}, fmt.Errorf("from-huggingface-files: no model URLs provided")
	}

	normalized := make([]string, len(modelURLs))
	for i, u := range modelURLs {
		normalized[i] = hf.NormalizeDownloadURL(u)
	}

	metadata, tensors, firstSize, err := fetchHeaderAndTensors(ctx, normalized[0])
	if err != nil {
		return Result{}, fmt.Errorf("from-huggingface-files: failed to fetch GGUF metadata: %w", err)
	}

	totalSize := firstSize
	shards := [][]gguf.TensorInfo{tensors}
	if len(normalized) > 1 {
		for i := 1; i < len(normalized); i++ {
			_, shardTensors, splitSize, err := fetchHeaderAndTensors(ctx, normalized[i])
			if err != nil {
				return Result{}, fmt.Errorf("from-huggingface-files: failed to determine size for %s: %w", normalized[i], err)
			}
			totalSize += splitSize
			shards = append(shards, shardTensors)
		}
	}

	return buildFromMetadata(metadata, shards, totalSize, cfg)
}

// =============================================================================

// fetchHeaderAndTensors fetches GGUF header, KV metadata, and tensor
// descriptors from a remote URL using HTTP Range requests. Only the
// header sections are downloaded, not the actual tensor data.
func fetchHeaderAndTensors(ctx context.Context, url string) (metadata map[string]string, tensors []gguf.TensorInfo, fileSize int64, err error) {
	data, fileSize, err := gguf.FetchHeaderBytes(ctx, url)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("fetch-header-and-tensors: failed to fetch header data: %w", err)
	}

	metadata, tensors, err = gguf.ParseHeaderAndTensors(data, fileSize)
	if err != nil {
		return nil, nil, 0, err
	}

	return metadata, tensors, fileSize, nil
}

// isFolderURL returns true if the URL points to a HuggingFace folder
// containing split model files rather than a single GGUF file.
func isFolderURL(modelURL string) bool {
	if strings.Contains(modelURL, "/tree/") {
		return true
	}

	lower := strings.ToLower(modelURL)
	if strings.HasSuffix(lower, ".gguf") || strings.Contains(lower, "/resolve/") || strings.Contains(lower, "/blob/") {
		return false
	}

	// Strip HF host prefix and scheme for path segment counting.
	raw := modelURL
	for _, prefix := range []string{
		"https://huggingface.co/",
		"http://huggingface.co/",
		"https://hf.co/",
		"http://hf.co/",
	} {
		if strings.HasPrefix(strings.ToLower(raw), prefix) {
			raw = raw[len(prefix):]
			break
		}
	}
	raw = hf.StripHostPrefix(raw)

	// Shorthand like "owner/repo:TAG" has a colon — not a folder URL.
	if strings.Contains(raw, ":") {
		return false
	}

	// 3+ path segments (owner/repo/subfolder) indicates a folder.
	parts := strings.Split(raw, "/")
	return len(parts) >= 3
}

// fetchFolderFiles lists GGUF files in a HuggingFace folder and returns
// their download URLs (sorted) and total size.
func fetchFolderFiles(ctx context.Context, folderURL string) ([]string, int64, error) {
	owner, repo, revision, folderPath, err := parseFolderURL(folderURL)
	if err != nil {
		return nil, 0, err
	}

	repoFiles, err := hf.RepoFiles(ctx, owner, repo, revision, folderPath, false)
	if err != nil {
		return nil, 0, fmt.Errorf("fetch-folder-files: %w", err)
	}

	var fileURLs []string
	var totalSize int64

	for _, f := range repoFiles {
		if !strings.HasSuffix(strings.ToLower(f.Filename), ".gguf") {
			continue
		}

		downloadURL := hf.BuildURL(owner, repo, revision, f.Filename)
		fileURLs = append(fileURLs, downloadURL)
		totalSize += f.Size
	}

	if len(fileURLs) == 0 {
		return nil, 0, fmt.Errorf("fetch-folder-files: no GGUF files found in folder %s/%s/%s", owner, repo, folderPath)
	}

	slices.Sort(fileURLs)

	return fileURLs, totalSize, nil
}

// parseFolderURL extracts owner, repo, revision, and folder path from a
// HuggingFace folder URL.
//
// Supported formats:
//
//	https://huggingface.co/owner/repo/tree/main/subfolder
//	owner/repo/tree/main/subfolder
//	owner/repo/subfolder (no /tree/main/ prefix)
func parseFolderURL(folderURL string) (owner, repo, revision, folderPath string, err error) {
	raw := strings.TrimSpace(folderURL)
	for _, prefix := range []string{
		"https://huggingface.co/",
		"http://huggingface.co/",
		"https://hf.co/",
		"http://hf.co/",
	} {
		if strings.HasPrefix(strings.ToLower(raw), prefix) {
			raw = raw[len(prefix):]
			break
		}
	}
	raw = hf.StripHostPrefix(raw)

	parts := strings.Split(strings.Trim(raw, "/"), "/")
	if len(parts) < 3 {
		return "", "", "", "", fmt.Errorf("parse-folder-url: invalid folder URL: %s", folderURL)
	}

	owner = parts[0]
	repo = parts[1]
	revision = "main"
	rest := parts[2:]
	if rest[0] == "tree" || rest[0] == "blob" {
		if len(rest) < 3 {
			return "", "", "", "", fmt.Errorf("parse-folder-url: missing folder path in URL: %s", folderURL)
		}
		revision = rest[1]
		rest = rest[2:]
	}
	folderPath = strings.Join(rest, "/")

	return owner, repo, revision, folderPath, nil
}
