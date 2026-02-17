package mcpapp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// WebSearchInput defines the input parameters for the web_search tool.
type WebSearchInput struct {
	Query      string `json:"query" jsonschema:"Search query"`
	Count      int    `json:"count,omitempty" jsonschema:"Number of results to return (default 10, max 20)"`
	Country    string `json:"country,omitempty" jsonschema:"Country code for search context (e.g. US, GB, DE)"`
	Freshness  string `json:"freshness,omitempty" jsonschema:"Filter by freshness: pd (past day), pw (past week), pm (past month), py (past year)"`
	SafeSearch string `json:"safesearch,omitempty" jsonschema:"Safe search filter: off, moderate, strict (default moderate)"`
}

func (a *App) webSearch(ctx context.Context, req *mcp.CallToolRequest, input WebSearchInput) (*mcp.CallToolResult, any, error) {
	a.log.Info(ctx, "web_search", "status", "request", "query", input.Query, "count", input.Count)

	if input.Query == "" {
		a.log.Info(ctx, "web_search", "status", "error", "msg", "empty query")
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "error: query parameter is required"},
			},
			IsError: true,
		}, nil, nil
	}

	if input.Count == 0 {
		input.Count = 10
	}

	if input.Count > 20 {
		input.Count = 20
	}

	results, err := a.searchBrave(ctx, input)
	if err != nil {
		a.log.Error(ctx, "web_search", "status", "error", "msg", err)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("error: search failed: %v", err)},
			},
			IsError: true,
		}, nil, nil
	}

	a.log.Info(ctx, "web_search", "status", "completed", "results", len(results))

	text := formatResults(results)

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: text},
		},
	}, nil, nil
}

// =============================================================================
// Brave Search API

type searchResult struct {
	Title       string
	URL         string
	Description string
}

// braveSearchResponse represents the Brave Web Search API response.
type braveSearchResponse struct {
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"results"`
	} `json:"web"`
}

func (a *App) searchBrave(ctx context.Context, input WebSearchInput) ([]searchResult, error) {
	params := url.Values{}
	params.Set("q", input.Query)
	params.Set("count", fmt.Sprintf("%d", input.Count))

	if input.Country != "" {
		params.Set("country", input.Country)
	}

	if input.Freshness != "" {
		params.Set("freshness", input.Freshness)
	}

	if input.SafeSearch != "" {
		params.Set("safesearch", input.SafeSearch)
	}

	reqURL := "https://api.search.brave.com/res/v1/web/search?" + params.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("X-Subscription-Token", a.braveAPIKey)

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search returned status %d: %s", resp.StatusCode, string(body))
	}

	var braveResp braveSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&braveResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	results := make([]searchResult, len(braveResp.Web.Results))
	for i, r := range braveResp.Web.Results {
		results[i] = searchResult{
			Title:       r.Title,
			URL:         r.URL,
			Description: r.Description,
		}
	}

	return results, nil
}

// =============================================================================

func formatResults(results []searchResult) string {
	if len(results) == 0 {
		return "No results found."
	}

	var b strings.Builder

	for i, result := range results {
		fmt.Fprintf(&b, "Result %d:\n", i+1)
		fmt.Fprintf(&b, "Title: %s\n", result.Title)
		fmt.Fprintf(&b, "URL: %s\n", result.URL)
		fmt.Fprintf(&b, "Description: %s\n", result.Description)
		b.WriteString("\n")
	}

	return b.String()
}
