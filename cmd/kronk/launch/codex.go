package launch

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"

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

	codexArgs, err := buildCodexArgs(defaultModel, chatModels)
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
//   - -m <model>: the default model to use.
//
// Codex prints a harmless "model metadata not found" warning for models it
// does not recognize and then runs with fallback metadata. We deliberately do
// not supply a model catalog to silence it: Codex's catalog schema changes
// between releases, and a catalog that does not match the installed Codex
// version is discarded wholesale (bringing the warning back) or rejected
// outright. The context-window override above is a stable config key and
// prevents the one failure that actually matters (prompt overflow).
//
// Codex parses -c values as TOML, so string values are quoted with %q.
func buildCodexArgs(defaultModel string, chatModels []Model) ([]string, error) {
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

	args := make([]string, 0, len(overrides)*2+2)
	for _, o := range overrides {
		args = append(args, "-c", o)
	}
	args = append(args, "-m", defaultModel)

	return args, nil
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
