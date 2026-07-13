package launch

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
)

// openCode implements Runner for the OpenCode agent. The Kronk provider
// config is passed via the OPENCODE_CONFIG_CONTENT env var at launch time
// so we never clobber the user's ~/.config/opencode files.
type openCode struct{}

// Run implements Runner. It ensures OpenCode is installed, builds a Kronk
// provider config from the installed models, and execs OpenCode with that
// config injected via the environment.
func (openCode) Run(defaultModel string, chatModels []Model, args []string) error {
	install, err := loadInstall("opencode")
	if err != nil {
		return fmt.Errorf("opencode: %w", err)
	}

	bin, err := ensureInstalled(install)
	if err != nil {
		return err
	}

	content, err := buildOpenCodeConfig(defaultModel, chatModels)
	if err != nil {
		return fmt.Errorf("build opencode config: %w", err)
	}

	cmd := exec.Command(bin, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "OPENCODE_CONFIG_CONTENT="+content)

	return cmd.Run()
}

// buildOpenCodeConfig produces the JSON for OPENCODE_CONFIG_CONTENT: a
// Kronk provider pointed at the local server plus one entry per chat
// model. When a model's resolved context window is known it is forwarded
// as OpenCode's context limit so OpenCode compacts prompts to fit instead
// of overflowing the server's window; when it is unknown the limit is
// omitted and OpenCode's own defaults apply.
func buildOpenCodeConfig(defaultModel string, chatModels []Model) (string, error) {
	if defaultModel == "" || len(chatModels) == 0 {
		return "", fmt.Errorf("a default model and at least one model are required")
	}

	baseURL, err := client.DefaultURL("/v1")
	if err != nil {
		return "", fmt.Errorf("default-url: %w", err)
	}

	options := map[string]any{
		"baseURL": baseURL,
	}

	// When the server requires auth, forward the token so OpenCode's
	// inference calls are authorized too (not just our catalog discovery).
	// Use OpenCode's {env:...} substitution so the token is resolved at
	// runtime from the environment we exec OpenCode with, rather than being
	// baked into the config content.
	if os.Getenv("KRONK_TOKEN") != "" {
		options["apiKey"] = "{env:KRONK_TOKEN}"
	}

	entries := make(map[string]any, len(chatModels))
	for _, m := range chatModels {
		entry := map[string]any{
			"name": m.Name,
		}
		if m.Vision {
			entry["attachment"] = true
			entry["modalities"] = map[string]any{
				"input":  []string{"text", "image"},
				"output": []string{"text"},
			}
		}
		if m.Reasoning {
			entry["reasoning"] = true
		}
		if m.Context > 0 {
			entry["limit"] = map[string]any{
				"context": m.Context,
				"output":  m.Context / 2,
			}
		}
		entries[m.ID] = entry
	}

	config := map[string]any{
		"$schema": "https://opencode.ai/config.json",
		"provider": map[string]any{
			"kronk": map[string]any{
				"npm":     "@ai-sdk/openai-compatible",
				"name":    "Kronk (local)",
				"options": options,
				"models":  entries,
			},
		},
		"model": "kronk/" + defaultModel,
		// Pin the lightweight model to a local one too so OpenCode does not
		// fall back to a non-Kronk provider for small tasks (e.g. titles).
		"small_model": "kronk/" + defaultModel,
		// Guarantee the injected Kronk provider survives regardless of the
		// user's existing config. OpenCode filters providers through the
		// enabled_providers allowlist and the disabled_providers denylist; a
		// user list that omits (or disables) "kronk" would otherwise silently
		// drop the provider and break the launched session. Because launch
		// pins both model and small_model to Kronk anyway, restricting the
		// session to the Kronk provider is the intended behavior.
		"enabled_providers":  []string{"kronk"},
		"disabled_providers": []string{},
	}

	data, err := json.Marshal(config)
	if err != nil {
		return "", err
	}

	return string(data), nil
}
