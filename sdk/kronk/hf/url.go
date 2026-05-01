package hf

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// NormalizeDownloadURL converts short format to full HuggingFace download URLs.
// Input:  mradermacher/Qwen2-Audio-7B-GGUF/Qwen2-Audio-7B.Q8_0.gguf
// Output: https://huggingface.co/mradermacher/Qwen2-Audio-7B-GGUF/resolve/main/Qwen2-Audio-7B.Q8_0.gguf
func NormalizeDownloadURL(rawURL string) string {
	if strings.HasPrefix(rawURL, "https://") || strings.HasPrefix(rawURL, "http://") {
		return strings.Replace(rawURL, "/blob/", "/resolve/", 1)
	}

	rawURL = StripHostPrefix(rawURL)

	parts := strings.Split(rawURL, "/")
	if len(parts) >= 3 {
		org := parts[0]
		repo := parts[1]
		filename := strings.Join(parts[2:], "/")
		return fmt.Sprintf("https://huggingface.co/%s/%s/resolve/main/%s", org, repo, filename)
	}

	return rawURL
}

// NormalizeURL converts short format URLs to full HuggingFace URLs.
// Input:  unsloth/Llama-3.3-70B-Instruct-GGUF
// Output: https://huggingface.co/unsloth/Llama-3.3-70B-Instruct-GGUF
//
// Input:  mradermacher/Qwen2-Audio-7B-GGUF/Qwen2-Audio-7B.Q8_0.gguf
// Output: https://huggingface.co/mradermacher/Qwen2-Audio-7B-GGUF/blob/main/Qwen2-Audio-7B.Q8_0.gguf
//
// Input:  unsloth/Llama-3.3-70B-Instruct-GGUF/Llama-3.3-70B-Instruct-Q8_0/Llama-3.3-70B-Instruct-Q8_0-00001-of-00002.gguf
// Output: https://huggingface.co/unsloth/Llama-3.3-70B-Instruct-GGUF/blob/main/Llama-3.3-70B-Instruct-Q8_0/Llama-3.3-70B-Instruct-Q8_0-00001-of-00002.gguf
func NormalizeURL(rawURL string) string {
	if strings.HasPrefix(rawURL, "https://") || strings.HasPrefix(rawURL, "http://") {
		return rawURL
	}

	rawURL = StripHostPrefix(rawURL)

	parts := strings.Split(rawURL, "/")
	if len(parts) >= 3 {
		org := parts[0]
		repo := parts[1]
		filename := strings.Join(parts[2:], "/")
		return fmt.Sprintf("https://huggingface.co/%s/%s/blob/main/%s", org, repo, filename)
	}

	if len(parts) == 2 {
		return fmt.Sprintf("https://huggingface.co/%s", rawURL)
	}

	return rawURL
}

// StripHostPrefix removes bare host prefixes (without scheme) from URLs.
func StripHostPrefix(s string) string {
	lower := strings.ToLower(s)
	for _, prefix := range []string{"huggingface.co/", "hf.co/"} {
		if strings.HasPrefix(lower, prefix) {
			return s[len(prefix):]
		}
	}
	return s
}

// BuildURL composes a HuggingFace resolve URL.
func BuildURL(owner, repo, revision, file string) string {
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

// ParseURL parses a HuggingFace resolve URL into its provider/repo/
// revision/file components. It accepts only fully-qualified URLs of the
// form https://huggingface.co/{owner}/{repo}/resolve/{revision}/{file...}.
func ParseURL(rawURL string) (provider, repo, revision, file string, ok bool) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", "", "", false
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 5 || parts[2] != "resolve" {
		return "", "", "", "", false
	}

	return parts[0], parts[1], parts[3], strings.Join(parts[4:], "/"), true
}

// ParseURLs parses a batch of model URLs that must share the same
// provider/repo/revision and returns the sorted list of repo-relative
// file names.
func ParseURLs(urls []string) (provider, repo, revision string, files []string, ok bool) {
	files = make([]string, 0, len(urls))

	for i, u := range urls {
		p, rp, rev, f, parsed := ParseURL(NormalizeDownloadURL(u))
		if !parsed {
			return "", "", "", nil, false
		}

		if i == 0 {
			provider, repo, revision = p, rp, rev
		} else if p != provider || rp != repo || rev != revision {
			return "", "", "", nil, false
		}

		files = append(files, f)
	}

	sort.Strings(files)

	return provider, repo, revision, files, true
}
