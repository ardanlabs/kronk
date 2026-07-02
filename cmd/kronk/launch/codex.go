package launch

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
)

// codexInstallCmd is the npm command that installs the Codex CLI. Codex is
// distributed via npm on every platform, so the same command works on
// Windows, macOS, and Linux.
const codexInstallCmd = "npm install -g @openai/codex"

// codexProvider is the id used for the Kronk provider Codex is pointed at.
// Codex reserves the built-in ids "openai", "ollama", and "lmstudio", so a
// distinct id is required.
const codexProvider = "kronk"

// codexMinCatalogVersion is the lowest Codex CLI version whose model-catalog
// schema is known to accept the catalog the launcher writes. On older versions
// the catalog is skipped (Codex falls back to its "metadata not found" warning,
// which is cosmetic) rather than risk Codex rejecting an unrecognized schema.
const codexMinCatalogVersion = "0.134.0"

// codexFallbackContextWindow is the context window written into the catalog when
// a model's real window could not be resolved, so the catalog entry is still
// well-formed.
const codexFallbackContextWindow = 128000

// codexInstall describes how to locate and install the Codex CLI.
var codexInstall = agentInstall{
	bin:              "codex",
	display:          "Codex CLI",
	installHint:      codexInstallHint,
	installerCommand: codexInstallerCommand,
	checkDeps:        checkCodexInstallDeps,
}

// codex implements Runner for the Codex CLI. Codex is configured entirely
// through one-off "-c key=value" overrides passed at launch time so we never
// touch the user's ~/.codex/config.toml. It talks to Kronk's OpenAI-compatible
// Responses API at /v1/responses (Codex only supports wire_api "responses").
type codex struct{}

// Run implements Runner. It ensures Codex is installed, builds the Kronk
// provider overrides from the installed models, and execs Codex with them
// (plus any pass-through args).
func (codex) Run(defaultModel string, chatModels []Model, args []string) error {
	bin, err := ensureInstalled(codexInstall)
	if err != nil {
		return err
	}

	// Best-effort: silence Codex's "model metadata not found" warning by
	// supplying a model catalog, but only on Codex versions whose catalog
	// schema we have verified (older versions may reject an unrecognized
	// schema). Any failure here is non-fatal: launch proceeds without the
	// catalog and Codex just shows the cosmetic warning.
	var catalogPath string
	if codexCatalogSupported(bin) {
		if p, err := writeCodexCatalog(buildCodexCatalog(defaultModel, chatModels)); err == nil {
			catalogPath = p
		}
	}

	codexArgs, err := buildCodexArgs(defaultModel, chatModels, catalogPath)
	if err != nil {
		return fmt.Errorf("build codex args: %w", err)
	}
	codexArgs = append(codexArgs, args...)

	cmd := exec.Command(bin, codexArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	return cmd.Run()
}

// buildCodexArgs returns the Codex CLI arguments that point it at the local
// Kronk server without writing any config file:
//
//   - -c model_provider="kronk": select the injected provider.
//   - -c model_providers.kronk.*: define the provider (display name, the
//     Kronk /v1 base URL, and wire_api "responses", which is the only
//     protocol Codex still supports).
//   - -c model_providers.kronk.env_key="KRONK_TOKEN": only when KRONK_TOKEN is
//     set, so Codex sends it as the bearer token; omitted for a token-less
//     server so no auth header is sent.
//   - -c model_context_window=N: the default model's resolved context window,
//     so Codex compacts prompts to fit instead of assuming a larger window for
//     an unrecognized model name. Omitted when the window is unknown.
//   - -c model_catalog_json="<path>": only when catalogPath is non-empty (a
//     supported Codex version), pointing Codex at a catalog file that describes
//     the model so Codex does not print its "metadata not found" warning.
//   - -m <model>: the default model to use.
//
// When no catalog is supplied (older/unverified Codex), Codex prints a harmless
// "model metadata not found" warning and runs with fallback metadata. The
// context-window override above is a stable config key and prevents the one
// failure that actually matters (prompt overflow) regardless of the catalog.
//
// Codex parses -c values as TOML, so string values are quoted with %q.
func buildCodexArgs(defaultModel string, chatModels []Model, catalogPath string) ([]string, error) {
	if defaultModel == "" || len(chatModels) == 0 {
		return nil, fmt.Errorf("a default model and at least one model are required")
	}

	baseURL, err := client.DefaultURL("/v1")
	if err != nil {
		return nil, fmt.Errorf("default-url: %w", err)
	}

	overrides := []string{
		fmt.Sprintf("model_provider=%q", codexProvider),
		fmt.Sprintf("model_providers.%s.name=%q", codexProvider, "Kronk (local)"),
		fmt.Sprintf("model_providers.%s.base_url=%q", codexProvider, baseURL),
		fmt.Sprintf("model_providers.%s.wire_api=%q", codexProvider, "responses"),
		// A token-less Kronk server needs no OpenAI auth; pin this off so Codex
		// does not demand OPENAI_API_KEY for the custom provider (and so we
		// never have to clobber the user's real OPENAI_API_KEY).
		fmt.Sprintf("model_providers.%s.requires_openai_auth=false", codexProvider),
	}

	if os.Getenv("KRONK_TOKEN") != "" {
		overrides = append(overrides, fmt.Sprintf("model_providers.%s.env_key=%q", codexProvider, "KRONK_TOKEN"))
	}

	if cw := contextFor(defaultModel, chatModels); cw > 0 {
		overrides = append(overrides, "model_context_window="+strconv.Itoa(cw))
	}

	if catalogPath != "" {
		overrides = append(overrides, fmt.Sprintf("model_catalog_json=%q", catalogPath))
	}

	args := make([]string, 0, len(overrides)*2+2)
	for _, o := range overrides {
		args = append(args, "-c", o)
	}
	args = append(args, "-m", defaultModel)

	return args, nil
}

// codexCatalogSupported reports whether the installed Codex CLI is new enough
// for the model-catalog schema the launcher writes. It fails closed (returns
// false when the version cannot be determined) so an unknown Codex version
// never gets a catalog it might reject.
func codexCatalogSupported(bin string) bool {
	v := codexVersion(bin)
	if v == "" {
		return false
	}

	return compareVersions(v, codexMinCatalogVersion) >= 0
}

// codexVersion returns the Codex CLI version parsed from "codex --version"
// (output like "codex-cli 0.134.0"), or "" when it cannot be determined.
func codexVersion(bin string) string {
	out, err := exec.Command(bin, "--version").Output()
	if err != nil {
		return ""
	}

	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) == 0 {
		return ""
	}

	return fields[len(fields)-1]
}

// buildCodexCatalog builds the Codex model-catalog document for the default
// model, describing it so Codex does not fall back to guessed metadata. Only
// the default model is described (that is the model Codex launches with); its
// resolved context window and vision capability come from the discovered Model.
func buildCodexCatalog(defaultModel string, chatModels []Model) map[string]any {
	contextWindow := codexFallbackContextWindow
	if cw := contextFor(defaultModel, chatModels); cw > 0 {
		contextWindow = cw
	}

	modalities := []any{"text"}
	for _, m := range chatModels {
		if m.ID == defaultModel && m.Vision {
			modalities = append(modalities, "image")
			break
		}
	}

	entry := map[string]any{
		"slug":                         defaultModel,
		"display_name":                 defaultModel,
		"context_window":               contextWindow,
		"shell_type":                   "default",
		"visibility":                   "list",
		"supported_in_api":             true,
		"priority":                     0,
		"truncation_policy":            map[string]any{"mode": "bytes", "limit": 10000},
		"input_modalities":             modalities,
		"base_instructions":            "",
		"support_verbosity":            true,
		"default_verbosity":            "low",
		"supports_parallel_tool_calls": false,
		"supports_reasoning_summaries": false,
		"supported_reasoning_levels":   []any{},
		"experimental_supported_tools": []any{},
	}

	return map[string]any{
		"models": []any{entry},
	}
}

// writeCodexCatalog writes the catalog document to a Kronk-owned file (in the
// system temp dir, never the user's ~/.codex config) and returns its path. The
// file is overwritten on each launch so it always reflects the current model.
func writeCodexCatalog(catalog map[string]any) (string, error) {
	data, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return "", err
	}

	path := filepath.Join(os.TempDir(), "kronk-codex-catalog.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}

	return path, nil
}

// checkCodexInstallDeps verifies npm (Node.js) is available, since Codex is
// installed via npm on every platform.
func checkCodexInstallDeps(goos string) error {
	switch goos {
	case "windows", "darwin", "linux":
		if _, err := exec.LookPath("npm"); err != nil {
			return fmt.Errorf("codex is not installed and npm (Node.js) is required to install it: https://nodejs.org/\n\nthen re-run: kronk launch codex")
		}
	default:
		return fmt.Errorf("codex is not installed and automatic install is not supported on %s\n\ninstall it manually: %s", goos, codexInstallHint(goos))
	}

	return nil
}

// codexInstallHint returns the human-readable install command for the given
// OS.
func codexInstallHint(goos string) string {
	switch goos {
	case "windows", "darwin", "linux":
		return codexInstallCmd
	default:
		return "see https://developers.openai.com/codex for installation instructions"
	}
}

// codexInstallerCommand returns the command that installs Codex on the given
// OS via npm.
func codexInstallerCommand(goos string) (string, []string, error) {
	switch goos {
	case "windows", "darwin", "linux":
		return "npm", []string{"install", "-g", "@openai/codex"}, nil
	default:
		return "", nil, fmt.Errorf("unsupported platform for codex install: %s", goos)
	}
}
