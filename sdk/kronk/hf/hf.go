// Package hf provides HuggingFace API helpers and URL utilities used by
// the rest of Kronk.
package hf

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// RepoFile represents a file available in a HuggingFace repository.
type RepoFile struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	SizeStr  string `json:"size_str"`
}

// ParseInput parses a HuggingFace URL or path into owner, repo, and
// filename components. The input can be a full URL or a short form like
// owner/repo/file.gguf.
func ParseInput(input string) (owner, repo, filename string, err error) {
	input = strings.TrimSpace(input)

	for _, prefix := range []string{
		"https://huggingface.co/",
		"http://huggingface.co/",
		"https://hf.co/",
		"http://hf.co/",
		"huggingface.co/",
		"hf.co/",
	} {
		if strings.HasPrefix(strings.ToLower(input), prefix) {
			input = input[len(prefix):]
			break
		}
	}

	parts := strings.Split(input, "/")
	if len(parts) < 2 {
		return "", "", "", fmt.Errorf("parse-input: invalid input %q, expected owner/repo format", input)
	}

	owner = parts[0]
	repo = parts[1]

	if len(parts) > 3 && (parts[2] == "resolve" || parts[2] == "blob") {
		filename = strings.Join(parts[4:], "/")
	} else if len(parts) > 3 && parts[2] == "tree" {
		// tree/main URLs point at a folder, not a specific file.
		filename = strings.Join(parts[4:], "/")
	} else if len(parts) > 2 {
		filename = strings.Join(parts[2:], "/")
	}

	return owner, repo, filename, nil
}

// treeEntry represents a file entry returned by the HuggingFace tree API.
type treeEntry struct {
	Type string `json:"type"`
	Path string `json:"path"`
	Size int64  `json:"size"`
	LFS  *struct {
		Size int64 `json:"size"`
	} `json:"lfs"`
}

// RepoFiles fetches files from the HuggingFace tree API for the given
// repository. When recursive is true the full repo tree is fetched; otherwise
// only the immediate contents of path are listed.
func RepoFiles(ctx context.Context, owner, repo, revision, path string, recursive bool) ([]RepoFile, error) {
	if revision == "" {
		revision = "main"
	}

	var apiURL string
	switch {
	case recursive:
		apiURL = fmt.Sprintf("https://huggingface.co/api/models/%s/%s/tree/%s?recursive=true",
			url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(revision))
	case path != "":
		apiURL = fmt.Sprintf("https://huggingface.co/api/models/%s/%s/tree/main/%s",
			url.PathEscape(owner), url.PathEscape(repo), path)
	default:
		apiURL = fmt.Sprintf("https://huggingface.co/api/models/%s/%s/tree/%s",
			url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(revision))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("repo-files: creating request: %w", err)
	}

	if token := os.Getenv("KRONK_HF_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("repo-files: fetching: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("repo-files: unexpected status %d", resp.StatusCode)
	}

	var entries []treeEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("repo-files: decoding: %w", err)
	}

	var files []RepoFile
	for _, e := range entries {
		if e.Type != "file" {
			continue
		}

		size := e.Size
		if e.LFS != nil {
			size = e.LFS.Size
		}

		files = append(files, RepoFile{
			Filename: e.Path,
			Size:     size,
			SizeStr:  FormatFileSize(size),
		})
	}

	return files, nil
}

// FormatFileSize formats a byte count into a human-readable string using
// SI units (KB, MB, GB).
func FormatFileSize(bytes int64) string {
	const (
		kb = 1000
		mb = kb * 1000
		gb = mb * 1000
	)

	switch {
	case bytes >= gb:
		val := float64(bytes) / float64(gb)
		return fmt.Sprintf("%.1f GB", math.Round(val*10)/10)
	case bytes >= mb:
		val := float64(bytes) / float64(mb)
		return fmt.Sprintf("%.1f MB", math.Round(val*10)/10)
	default:
		val := float64(bytes) / float64(kb)
		return fmt.Sprintf("%.1f KB", math.Round(val*10)/10)
	}
}
