package launch

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
)

// codexProvider is the id used for the Kronk provider Codex is pointed at.
// Codex reserves several built-in provider ids (e.g. "openai"), so a distinct
// id is required.
const codexProvider = "kronk"

// codexMinCatalogVersion is the lowest Codex CLI version whose model-catalog
// schema is known to accept the catalog the launcher writes. On older versions
// the catalog is skipped (Codex falls back to its "metadata not found" warning,
// which is cosmetic) rather than risk Codex rejecting an unrecognized schema.
const codexMinCatalogVersion = "0.134.0"

// codexCmdTimeout bounds the "codex --version" probe so a wedged Codex CLI
// cannot hang the launch. It is generous (a cold Node.js start is still far
// under it) so it never fires on a legitimately slow-but-working CLI.
const codexCmdTimeout = 30 * time.Second

// codex implements Runner for the Codex CLI. Codex is configured entirely
// through one-off "-c key=value" overrides passed at launch time so we never
// touch the user's ~/.codex/config.toml. It talks to Kronk's OpenAI-compatible
// Responses API at /v1/responses (Codex only supports wire_api "responses").
type codex struct{}

// Run implements Runner. It ensures Codex is installed, builds the Kronk
// provider overrides from the installed models, and execs Codex with them
// (plus any pass-through args).
func (codex) Run(defaultModel string, chatModels []Model, args []string) error {
	install, err := loadInstall("codex")
	if err != nil {
		return fmt.Errorf("codex: %w", err)
	}

	bin, err := ensureInstalled(install)
	if err != nil {
		return err
	}

	// Honor a model the user selected via pass-through args (e.g. "-- -m X" or
	// "-- exec --model X") instead of pinning the launcher's default over it.
	// The effective model drives the catalog and context-window sizing, and when
	// the user supplied one we must not also emit our own "-m" (Codex rejects a
	// top-level --model given twice).
	override := modelArgValue(args)
	effectiveModel := defaultModel
	if override != "" {
		effectiveModel = override
	}

	// Best-effort: silence Codex's "model metadata not found" warning by
	// supplying a model catalog, but only when the model's real context window
	// is known and only on Codex versions whose catalog schema we have verified
	// (older versions may reject an unrecognized schema). When the window is
	// unknown the catalog is skipped rather than advertising a fabricated one:
	// a wrong (too large) window causes real context-overflow failures, whereas
	// the missing catalog only costs Codex's cosmetic warning. Any failure here
	// is non-fatal.
	var catalogPath string
	if cw := contextFor(effectiveModel, chatModels); cw > 0 && codexCatalogSupported(bin) {
		if p, err := writeCodexCatalog(buildCodexCatalog(effectiveModel, cw, chatModels)); err == nil {
			catalogPath = p
			defer os.Remove(catalogPath)
		}
	}

	codexArgs, err := buildCodexArgs(effectiveModel, chatModels, catalogPath, override == "")
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
//   - -m <model>: the model to use — emitted only when emitModelFlag is true.
//     It is suppressed when the user selected their own model via pass-through
//     args (Codex rejects a top-level --model given twice); their selection
//     then supplies the model instead.
//
// model is the effective model (the launcher default, or the user's pass-through
// selection) and drives both the -m value and the context-window override.
//
// When no catalog is supplied (older/unverified Codex), Codex prints a harmless
// "model metadata not found" warning and runs with fallback metadata. The
// context-window override above is a stable config key and prevents the one
// failure that actually matters (prompt overflow) regardless of the catalog.
//
// Codex parses -c values as TOML, so string values are quoted with %q.
func buildCodexArgs(model string, chatModels []Model, catalogPath string, emitModelFlag bool) ([]string, error) {
	if model == "" || len(chatModels) == 0 {
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

	if cw := contextFor(model, chatModels); cw > 0 {
		overrides = append(overrides, "model_context_window="+strconv.Itoa(cw))
	}

	if catalogPath != "" {
		overrides = append(overrides, fmt.Sprintf("model_catalog_json=%q", catalogPath))
	}

	args := make([]string, 0, len(overrides)*2+2)
	for _, o := range overrides {
		args = append(args, "-c", o)
	}
	if emitModelFlag {
		args = append(args, "-m", model)
	}

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
	ctx, cancel := context.WithTimeout(context.Background(), codexCmdTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, bin, "--version").Output()
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
// the default model is described (that is the model Codex launches with);
// contextWindow is its resolved context window (the caller only builds a catalog
// when it is known) and the vision capability comes from the discovered Model.
func buildCodexCatalog(defaultModel string, contextWindow int, chatModels []Model) map[string]any {
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

// writeCodexCatalog writes the catalog document to a Kronk-owned temp file
// (never the user's ~/.codex config) and returns its path; the caller removes it
// once Codex exits. It uses os.CreateTemp for a unique, owner-only (0600) file:
// a predictable name in a shared temp dir could be pre-created as a symlink or
// clobbered by a concurrent launch, so a fresh unique file is used each time.
func writeCodexCatalog(catalog map[string]any) (string, error) {
	data, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return "", err
	}

	f, err := os.CreateTemp(os.TempDir(), "kronk-codex-catalog-*.json")
	if err != nil {
		return "", err
	}

	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}

	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", err
	}

	return f.Name(), nil
}

// compareVersions compares two dot-separated numeric version strings and
// returns -1 when a < b, 0 when equal, and 1 when a > b. Non-numeric or missing
// segments are treated as 0, which is enough for the coarse "is it new enough"
// checks here.
func compareVersions(a, b string) int {
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")

	n := max(len(aParts), len(bParts))
	for i := range n {
		var an, bn int
		if i < len(aParts) {
			an, _ = strconv.Atoi(aParts[i])
		}
		if i < len(bParts) {
			bn, _ = strconv.Atoi(bParts[i])
		}
		if an != bn {
			if an < bn {
				return -1
			}
			return 1
		}
	}

	return 0
}
