package models

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// DefaultHFClient is the production implementation of HFClient. It talks
// to https://huggingface.co and honours KRONK_HF_TOKEN.
type DefaultHFClient struct {
	BaseURL string // defaults to https://huggingface.co
	HTTP    *http.Client
}

// NewDefaultHFClient constructs the production HF client.
func NewDefaultHFClient() *DefaultHFClient {
	return &DefaultHFClient{
		BaseURL: "https://huggingface.co",
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}
}

// ModelMeta implements HFClient.
func (c *DefaultHFClient) ModelMeta(ctx context.Context, owner, repo, revision string) (HFModelMeta, error) {
	u := fmt.Sprintf("%s/api/models/%s/%s", c.baseURL(), url.PathEscape(owner), url.PathEscape(repo))
	if revision != "" && revision != "main" {
		u += "?revision=" + url.QueryEscape(revision)
	}

	body, err := c.do(ctx, u)
	if err != nil {
		return HFModelMeta{}, err
	}

	var raw struct {
		ID       string `json:"id"`
		Gated    any    `json:"gated"`
		Siblings []struct {
			RFilename string `json:"rfilename"`
		} `json:"siblings"`
	}

	if err := json.Unmarshal(body, &raw); err != nil {
		return HFModelMeta{}, fmt.Errorf("hf-model-meta: decode: %w", err)
	}

	siblings := make([]string, 0, len(raw.Siblings))
	for _, s := range raw.Siblings {
		siblings = append(siblings, s.RFilename)
	}

	return HFModelMeta{
		ID:       raw.ID,
		Siblings: siblings,
		Gated:    isGated(raw.Gated),
	}, nil
}

// SearchModels implements HFClient. It searches HuggingFace for models
// owned by `author` matching `query`, restricted to GGUF repos.
func (c *DefaultHFClient) SearchModels(ctx context.Context, author, query string) ([]string, error) {
	q := url.Values{}
	q.Set("author", author)
	if query != "" {
		q.Set("search", query)
	}
	q.Set("filter", "gguf")
	q.Set("limit", "10")

	u := fmt.Sprintf("%s/api/models?%s", c.baseURL(), q.Encode())

	body, err := c.do(ctx, u)
	if err != nil {
		return nil, err
	}

	var entries []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("hf-search: decode: %w", err)
	}

	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.ID)
	}

	if len(out) == 0 {
		return nil, ErrHFNotFound
	}

	return out, nil
}

func (c *DefaultHFClient) baseURL() string {
	if c.BaseURL == "" {
		return "https://huggingface.co"
	}

	return c.BaseURL
}

func (c *DefaultHFClient) httpClient() *http.Client {
	if c.HTTP == nil {
		return http.DefaultClient
	}

	return c.HTTP
}

// do issues a GET request, attaching KRONK_HF_TOKEN when set, and maps
// HTTP error codes to the resolver's typed errors.
func (c *DefaultHFClient) do(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("hf-request: build: %w", err)
	}
	if tok := os.Getenv("KRONK_HF_TOKEN"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("hf-request: do: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// fall through
	case http.StatusNotFound:
		return nil, ErrHFNotFound
	case http.StatusTooManyRequests:
		if os.Getenv("KRONK_HF_TOKEN") == "" {
			return nil, fmt.Errorf("%w: HuggingFace is rate-limiting requests; set KRONK_HF_TOKEN to authenticate", ErrHFThrottled)
		}
		return nil, fmt.Errorf("%w: HuggingFace is rate-limiting requests", ErrHFThrottled)
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, fmt.Errorf("hf-request: status %d for %s — set KRONK_HF_TOKEN", resp.StatusCode, u)
	default:
		return nil, fmt.Errorf("hf-request: unexpected status %d for %s", resp.StatusCode, u)
	}

	body := make([]byte, 0, 4096)
	buf := make([]byte, 4096)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			body = append(body, buf[:n]...)
		}
		if rerr != nil {
			break
		}
	}

	return body, nil
}

func isGated(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x != "" && !strings.EqualFold(x, "false")
	default:
		return false
	}
}
