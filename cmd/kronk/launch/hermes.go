package launch

import (
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	yaml "go.yaml.in/yaml/v2"
)

// hermesPlaceholderKey is written as the provider api_key when the Kronk server
// needs no auth. A token-less Kronk server ignores it, and Hermes' "custom"
// provider still reads api_key from config. When a token is required the config
// instead uses "${KRONK_TOKEN}" so Hermes interpolates it from the environment
// at request time (the secret is never persisted to disk).
const hermesPlaceholderKey = "kronk"

// hermes implements Runner for Nous Research's Hermes Agent. Hermes is a
// personal-assistant platform (CLI plus an optional messaging gateway), but its
// core reads a single primary model from ~/.hermes/config.yaml. To keep the
// launch experience simple and local, Run configures the "custom"
// OpenAI-compatible provider pointing at the local Kronk server (merging and
// backing up any existing config) and then launches Hermes' terminal CLI. No
// messaging gateway is configured.
type hermes struct{}

// Run implements Runner. It ensures Hermes is installed, writes the Kronk
// endpoint and default model into Hermes' config.yaml, and execs Hermes. When
// the caller passes through their own args they are used verbatim; otherwise
// Hermes starts its interactive terminal session against the configured model.
func (hermes) Run(defaultModel string, chatModels []Model, args []string) error {
	install, err := loadInstall("hermes")
	if err != nil {
		return fmt.Errorf("hermes: %w", err)
	}

	bin, err := ensureInstalled(install)
	if err != nil {
		return err
	}

	// Honor a model the user selected via pass-through args (e.g.
	// "-- --model X"): Hermes' CLI --model outranks the config default and the
	// env, so size the persisted context window for that effective model instead
	// of leaving the launcher default's window attached under the user's model.
	// The persisted "default" itself stays the launcher default so a one-off
	// selection does not clobber the user's saved default.
	contextModel := defaultModel
	if override := modelArgValue(args); override != "" {
		contextModel = override
	}

	if err := writeHermesConfig(defaultModel, contextModel, chatModels); err != nil {
		return fmt.Errorf("configure hermes: %w", err)
	}

	cmd := exec.Command(bin, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), hermesEnv(defaultModel)...)

	return cmd.Run()
}

// hermesEnv returns the environment overrides that keep Hermes on the local
// Kronk route the launcher just wrote to config.yaml. Hermes resolves the model
// as: CLI --model > HERMES_INFERENCE_MODEL env > config.yaml default, so an
// inherited HERMES_INFERENCE_MODEL/HERMES_MODEL would otherwise beat the
// configured local model. These are pinned to the same values written to the
// config (provider "custom", the selected model), so an inherited env cannot
// divert Hermes to a cloud model/provider. A user's own CLI --model/--provider
// still wins, as those take precedence over the environment. Go's exec keeps the
// last value for duplicate keys, so these override any inherited ones.
func hermesEnv(defaultModel string) []string {
	return []string{
		"HERMES_MODEL=" + defaultModel,
		"HERMES_INFERENCE_MODEL=" + defaultModel,
		"HERMES_TUI_PROVIDER=custom",
		"HERMES_INFERENCE_PROVIDER=custom",
	}
}

// writeHermesConfig writes the Kronk custom-provider settings and default model
// into ~/.hermes/config.yaml, merging into any existing config and backing up
// the previous file first. defaultModel is persisted as the config default;
// contextModel is the model whose resolved window is written as context_length
// (they differ only when the user selected a one-off model via pass-through
// args, so the window matches the model actually in use).
func writeHermesConfig(defaultModel, contextModel string, chatModels []Model) error {
	if defaultModel == "" || len(chatModels) == 0 {
		return fmt.Errorf("a default model and at least one model are required")
	}

	baseURL, err := client.DefaultURL("/v1")
	if err != nil {
		return fmt.Errorf("default-url: %w", err)
	}

	apiKey := hermesPlaceholderKey
	if os.Getenv("KRONK_TOKEN") != "" {
		// Hermes interpolates ${ENV} in config.yaml values at request time, so
		// the token is read from the environment (inherited by the launched
		// Hermes process) instead of being written to disk.
		apiKey = "${KRONK_TOKEN}"
	}

	path, err := hermesConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	existing := map[string]any{}
	if err := readExistingConfig(path, &existing, yaml.Unmarshal); err != nil {
		return err
	}

	merged := buildHermesConfig(existing, defaultModel, baseURL, apiKey, contextFor(contextModel, chatModels))

	data, err := yaml.Marshal(merged)
	if err != nil {
		return err
	}

	return writeFileWithBackup(path, data)
}

// buildHermesConfig merges the Kronk custom-provider settings into an existing
// Hermes config document. Only the managed keys under "model" are changed
// (provider, default, base_url, api_key, and context_length when known); any
// other model settings and all other top-level sections are preserved.
//
// Hermes' single source of truth for the active model is the top-level "model"
// block; with provider "custom" it calls base_url directly using api_key, which
// is the documented, unambiguous path for an OpenAI-compatible endpoint.
func buildHermesConfig(existing map[string]any, defaultModel, baseURL, apiKey string, contextLen int) map[string]any {
	config := existing
	if config == nil {
		config = map[string]any{}
	}

	model := hermesStringMap(config["model"])
	model["provider"] = "custom"
	model["default"] = defaultModel
	model["base_url"] = baseURL
	model["api_key"] = apiKey

	// Always clear any previously-written window first; a stale context_length
	// from an earlier launch (with a different model) must not linger when the
	// current model's window is unknown. Only re-set it when it is known.
	delete(model, "context_length")
	if contextLen > 0 {
		model["context_length"] = contextLen
	}

	config["model"] = model

	return config
}

// hermesStringMap coerces a config subsection into a string-keyed map. YAML v2
// decodes nested maps as map[any]any, so both shapes are handled; a nil or
// unexpected value yields an empty map.
func hermesStringMap(v any) map[string]any {
	switch m := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(m))
		maps.Copy(out, m)
		return out
	case map[any]any:
		out := make(map[string]any, len(m))
		for k, val := range m {
			if ks, ok := k.(string); ok {
				out[ks] = val
			}
		}
		return out
	default:
		return map[string]any{}
	}
}

// hermesConfigPath returns the path to Hermes' config.yaml, honoring the
// HERMES_HOME override the installer supports and otherwise defaulting to
// ~/.hermes/config.yaml.
func hermesConfigPath() (string, error) {
	if home := os.Getenv("HERMES_HOME"); home != "" {
		return filepath.Join(home, "config.yaml"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, ".hermes", "config.yaml"), nil
}
