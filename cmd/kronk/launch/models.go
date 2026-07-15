package launch

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
)

// Model is a chat-capable model installed on the Kronk server, distilled
// from the server's model listing into just what the launcher needs to
// configure an agent.
type Model struct {
	ID        string
	Name      string
	Vision    bool
	Reasoning bool

	// Variant reports whether ID carries a profile suffix (e.g.
	// "<base>/AGENT"). Profile variants are declared in the user's
	// model_config.yaml and typically carry the large context window an
	// agent needs, so they are preferred as the default model.
	Variant bool

	// Context is the model's resolved server-side context window, or 0 when
	// it could not be determined. When known it is forwarded to the agent so
	// the agent compacts prompts to fit instead of overflowing the server.
	Context int
}

// modelListEntry is the subset of the Kronk GET /v1/kronk/models response
// the launcher needs. These ids are the filename-based ids the inference
// endpoint and model_config.yaml actually key on (including profile
// variants like "<base>/AGENT"), which is why the launcher discovers
// models here rather than from /v1/kronk/catalog (whose "owner/repo" ids
// do not resolve to the per-model config and fall back to the default
// context window).
type modelListEntry struct {
	ID            string `json:"id"`
	ModelFamily   string `json:"model_family"`
	Validated     bool   `json:"validated"`
	HasProjection bool   `json:"has_projection"`
}

// modelListResponse is the GET /v1/kronk/models envelope.
type modelListResponse struct {
	Data []modelListEntry `json:"data"`
}

// catalogEntry is the subset of the Kronk GET /v1/kronk/catalog response
// the launcher needs. The catalog is consulted only for per-model
// capabilities (chat vs embedding/rerank, vision, reasoning); its ids are
// not used to configure the agent. It is defined locally so this package
// does not depend on the SDK.
type catalogEntry struct {
	ID            string `json:"id"`
	Downloaded    bool   `json:"downloaded"`
	Validated     bool   `json:"validated"`
	HasProjection bool   `json:"has_projection"`
	Capabilities  struct {
		Endpoint  string `json:"endpoint"`
		Images    bool   `json:"images"`
		Reasoning bool   `json:"reasoning"`
		Embedding bool   `json:"embedding"`
		Rerank    bool   `json:"rerank"`
	} `json:"capabilities"`
}

// modelDetail is the subset of GET /v1/kronk/models/{id} the launcher
// needs: the resolved context window (analysis defaults overlaid with the
// user's model_config.yaml entry).
type modelDetail struct {
	ModelConfig struct {
		ContextWindow *int `json:"context-window"`
	} `json:"model_config"`
}

// capability describes what a base model can do, sourced from the catalog.
type capability struct {
	chat      bool
	vision    bool
	reasoning bool
}

// fetchChatModels queries the running Kronk server and returns the
// installed, chat-capable models (including profile variants) with their
// resolved context windows.
func fetchChatModels(ctx context.Context) ([]Model, error) {
	cln := client.New(
		client.NoopLogger,
		client.WithBearer(os.Getenv("KRONK_TOKEN")),
	)

	listURL, err := client.DefaultURL("/v1/kronk/models")
	if err != nil {
		return nil, fmt.Errorf("default-url: %w", err)
	}

	var list modelListResponse
	if err := cln.Do(ctx, http.MethodGet, listURL, nil, &list); err != nil {
		return nil, fmt.Errorf("query models at %s: %w", listURL, err)
	}

	// The catalog is best-effort: it sharpens chat-vs-non-chat and
	// vision/reasoning detection, but if it is unavailable we still return a
	// usable list using a name-based heuristic.
	caps := fetchCapabilities(ctx, cln)

	models := selectChatModels(list.Data, caps)

	// Best-effort: attach each model's resolved context window so the agent
	// can be told the real limit. Each lookup is a separate (and non-trivial)
	// server call, so they run concurrently with a small worker pool to keep
	// launch responsive when many models are installed. Failures are non-fatal.
	attachContextWindows(ctx, cln, models)

	return models, nil
}

// contextWindowWorkers bounds how many context-window lookups run at once, so a
// large installed-model set does not open a burst of connections to the server.
const contextWindowWorkers = 8

// attachContextWindows fills in each model's resolved context window in place,
// fetching them concurrently. Lookups are best-effort: a model whose window
// cannot be determined keeps Context == 0 and the agent falls back to its own
// default.
func attachContextWindows(ctx context.Context, cln *client.Client, models []Model) {
	if len(models) == 0 {
		return
	}

	sem := make(chan struct{}, contextWindowWorkers)
	var wg sync.WaitGroup

	for i := range models {
		wg.Add(1)
		sem <- struct{}{}

		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()

			if cw, ok := fetchContextWindow(ctx, cln, models[i].ID); ok {
				models[i].Context = cw
			}
		}(i)
	}

	wg.Wait()
}

// fetchCapabilities loads the catalog and returns per-base-model
// capabilities keyed by the bare model name (the segment after the last
// "/"). It returns nil when the catalog cannot be loaded so callers fall
// back to name-based heuristics.
func fetchCapabilities(ctx context.Context, cln *client.Client) map[string]capability {
	catURL, err := client.DefaultURL("/v1/kronk/catalog")
	if err != nil {
		return nil
	}

	var entries []catalogEntry
	if err := cln.Do(ctx, http.MethodGet, catURL, nil, &entries); err != nil {
		return nil
	}

	caps := make(map[string]capability, len(entries))
	for _, e := range entries {
		if !e.Downloaded || !e.Validated {
			continue
		}
		caps[baseName(e.ID)] = capability{
			chat:      !catalogIsNonChat(e),
			vision:    e.Capabilities.Images || e.HasProjection,
			reasoning: e.Capabilities.Reasoning,
		}
	}

	return caps
}

// fetchContextWindow returns the resolved context window for a single
// model. The id is path-escaped so profile variants (which contain "/")
// are passed as a single path segment.
func fetchContextWindow(ctx context.Context, cln *client.Client, id string) (int, bool) {
	base, err := client.DefaultURL("/v1/kronk/models")
	if err != nil {
		return 0, false
	}

	detURL := base + "/" + url.PathEscape(id)

	var detail modelDetail
	if err := cln.Do(ctx, http.MethodGet, detURL, nil, &detail); err != nil {
		return 0, false
	}

	if detail.ModelConfig.ContextWindow == nil || *detail.ModelConfig.ContextWindow <= 0 {
		return 0, false
	}

	return *detail.ModelConfig.ContextWindow, true
}

// selectChatModels keeps the validated, chat-capable models from the
// server's model listing and maps them into the launcher's Model type.
// caps (from the catalog) is preferred for the chat/vision/reasoning
// decision; when a model is absent from caps (or caps is nil because the
// catalog was unavailable) a name-based heuristic is used instead. The
// result is sorted by id for a stable default-model choice.
func selectChatModels(entries []modelListEntry, caps map[string]capability) []Model {
	var out []Model
	for _, e := range entries {
		if !e.Validated {
			continue
		}

		base := baseSegment(e.ID)

		vision := e.HasProjection
		reasoning := false

		if c, ok := caps[base]; ok {
			if !c.chat {
				continue
			}
			vision = c.vision
			reasoning = c.reasoning
		} else if looksNonChat(e.ID, e.ModelFamily) {
			continue
		}

		out = append(out, Model{
			ID:        e.ID,
			Name:      e.ID,
			Vision:    vision,
			Reasoning: reasoning,
			Variant:   strings.Contains(e.ID, "/"),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})

	return out
}

// catalogIsNonChat reports whether a catalog entry is an embedding or
// rerank model that should not be offered to a chat agent.
//
// When the server reports a non-chat endpoint we trust it. Otherwise we
// fall back to the capability booleans and, finally, a name-based
// heuristic: the server derives capabilities from the GGUF architecture,
// so an embedding/rerank model built on a chat-style architecture (e.g.
// Qwen3-Embedding on the qwen3 arch) can be tagged as chat with
// embedding=false.
func catalogIsNonChat(e catalogEntry) bool {
	if e.Capabilities.Endpoint != "" && e.Capabilities.Endpoint != "chat_completion" {
		return true
	}

	if e.Capabilities.Embedding || e.Capabilities.Rerank {
		return true
	}

	return looksNonChat(e.ID, "")
}

// looksNonChat is the name-based fallback used when catalog capabilities
// are unavailable. It flags obvious embedding/rerank models by id or model
// family.
func looksNonChat(id, family string) bool {
	s := strings.ToLower(id + " " + family)
	return strings.Contains(s, "embed") || strings.Contains(s, "rerank")
}

// baseName returns the segment of a model id after the last "/", i.e. the
// bare model name for an "owner/name" catalog id.
func baseName(id string) string {
	if i := strings.LastIndex(id, "/"); i >= 0 {
		return id[i+1:]
	}
	return id
}

// baseSegment returns the segment of a model id before the first "/", i.e.
// the base model name for a "name/VARIANT" listing id.
func baseSegment(id string) string {
	if before, _, ok := strings.Cut(id, "/"); ok {
		return before
	}
	return id
}
