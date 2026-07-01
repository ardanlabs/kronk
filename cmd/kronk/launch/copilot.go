package launch

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
)

// copilotInstallCmd is the npm command that installs GitHub Copilot CLI.
// Copilot CLI is distributed via npm on every platform (Node.js 22+), so the
// same command works on Windows, macOS, and Linux.
const copilotInstallCmd = "npm install -g @github/copilot"

// copilotOutputReserveFraction is the share of a model's context window held
// back for the agent's output. Copilot does not know our model, so we tell it
// the prompt/output budgets explicitly; reserving a slice for output keeps
// prompt+output within the server's context window and avoids overflow.
const copilotOutputReserveFraction = 4 // reserve 1/4 of the window for output

// copilotInstall describes how to locate and install the Copilot CLI. The
// npm global install lands on PATH; the install script variant drops the
// binary in ~/.local/bin, which may not be on PATH in the current shell yet,
// so that directory is searched as a fallback.
var copilotInstall = agentInstall{
	bin:              "copilot",
	display:          "Copilot CLI",
	fallbackDirs:     []string{".local/bin"},
	installHint:      copilotInstallHint,
	installerCommand: copilotInstallerCommand,
	checkDeps:        checkCopilotInstallDeps,
}

// copilot implements Runner for GitHub Copilot CLI. Copilot CLI has no
// provider/model config file to touch; it is configured entirely through its
// documented BYOK environment variables and talks to Kronk's
// OpenAI-compatible Chat Completions API at /v1/chat/completions.
type copilot struct{}

// Run implements Runner. It ensures Copilot CLI is installed, builds the BYOK
// environment pointing at the local Kronk server, and execs Copilot with that
// environment (args are passed straight through).
func (copilot) Run(defaultModel string, chatModels []Model, args []string) error {
	bin, err := ensureInstalled(copilotInstall)
	if err != nil {
		return err
	}

	env, err := buildCopilotEnv(defaultModel, chatModels)
	if err != nil {
		return fmt.Errorf("build copilot env: %w", err)
	}

	cmd := exec.Command(bin, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), env...)

	return cmd.Run()
}

// buildCopilotEnv returns the BYOK environment variables that point Copilot
// CLI at the local Kronk server:
//
//   - COPILOT_PROVIDER_TYPE=openai: use the OpenAI-compatible provider (Kronk
//     serves Chat Completions). Set explicitly so an inherited
//     COPILOT_PROVIDER_TYPE (azure/anthropic) from the user's environment
//     cannot divert Copilot to the wrong wire protocol.
//   - COPILOT_PROVIDER_BASE_URL: the Kronk /v1 base URL; Copilot appends
//     "/chat/completions" itself.
//   - COPILOT_PROVIDER_API_KEY: forwarded as the bearer token. Uses
//     KRONK_TOKEN when set; left empty otherwise, since a token-less Kronk
//     server needs no auth (Copilot's own docs note the key is not required
//     for local providers).
//   - COPILOT_MODEL: the default model. A user-supplied "--model" in the
//     pass-through args overrides it.
//   - COPILOT_PROVIDER_MAX_PROMPT_TOKENS / COPILOT_PROVIDER_MAX_OUTPUT_TOKENS:
//     derived from the model's resolved context window so Copilot (which would
//     otherwise assume a large window for an unrecognized model name) keeps
//     prompt+output within the server's window instead of overflowing it.
//     Omitted when the window is unknown.
//
// Unlike Ollama's integration this does not set COPILOT_PROVIDER_WIRE_API:
// that key is undocumented, and the documented "openai" provider type already
// uses Chat Completions, which Kronk serves. Copilot ignores env vars it does
// not recognize, so the budget hints above are best-effort and never fatal.
func buildCopilotEnv(defaultModel string, chatModels []Model) ([]string, error) {
	if defaultModel == "" || len(chatModels) == 0 {
		return nil, fmt.Errorf("a default model and at least one model are required")
	}

	baseURL, err := client.DefaultURL("/v1")
	if err != nil {
		return nil, fmt.Errorf("default-url: %w", err)
	}

	env := []string{
		"COPILOT_PROVIDER_TYPE=openai",
		"COPILOT_PROVIDER_BASE_URL=" + baseURL,
		"COPILOT_PROVIDER_API_KEY=" + os.Getenv("KRONK_TOKEN"),
		"COPILOT_MODEL=" + defaultModel,
	}

	if cw := contextFor(defaultModel, chatModels); cw > 0 {
		out := max(cw/copilotOutputReserveFraction, 1)
		prompt := cw - out

		env = append(env,
			"COPILOT_PROVIDER_MAX_PROMPT_TOKENS="+strconv.Itoa(prompt),
			"COPILOT_PROVIDER_MAX_OUTPUT_TOKENS="+strconv.Itoa(out),
		)
	}

	return env, nil
}

// checkCopilotInstallDeps verifies npm (Node.js) is available, since Copilot
// CLI is installed via npm on every platform.
func checkCopilotInstallDeps(goos string) error {
	switch goos {
	case "windows", "darwin", "linux":
		if _, err := exec.LookPath("npm"); err != nil {
			return fmt.Errorf("copilot is not installed and npm (Node.js 22+) is required to install it: https://nodejs.org/\n\nthen re-run: kronk launch copilot")
		}
	default:
		return fmt.Errorf("copilot is not installed and automatic install is not supported on %s\n\ninstall it manually: %s", goos, copilotInstallHint(goos))
	}

	return nil
}

// copilotInstallHint returns the human-readable install command for the given
// OS.
func copilotInstallHint(goos string) string {
	switch goos {
	case "windows", "darwin", "linux":
		return copilotInstallCmd
	default:
		return "see https://docs.github.com/copilot/how-tos/set-up/install-copilot-cli for installation instructions"
	}
}

// copilotInstallerCommand returns the command that installs Copilot CLI on the
// given OS via npm.
func copilotInstallerCommand(goos string) (string, []string, error) {
	switch goos {
	case "windows", "darwin", "linux":
		return "npm", []string{"install", "-g", "@github/copilot"}, nil
	default:
		return "", nil, fmt.Errorf("unsupported platform for copilot install: %s", goos)
	}
}
