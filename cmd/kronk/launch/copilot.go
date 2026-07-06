package launch

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
)

// copilotOutputReserveFraction is the share of a model's context window held
// back for the agent's output. Copilot does not know our model, so we tell it
// the prompt/output budgets explicitly; reserving a slice for output keeps
// prompt+output within the server's context window and avoids overflow.
const copilotOutputReserveFraction = 4 // reserve 1/4 of the window for output

// copilot implements Runner for GitHub Copilot CLI. Copilot CLI has no
// provider/model config file to touch; it is configured entirely through its
// documented BYOK environment variables and talks to Kronk's
// OpenAI-compatible Chat Completions API at /v1/chat/completions.
type copilot struct{}

// Run implements Runner. It ensures Copilot CLI is installed, builds the BYOK
// environment pointing at the local Kronk server, and execs Copilot with that
// environment (args are passed straight through).
func (copilot) Run(defaultModel string, chatModels []Model, args []string) error {
	install, err := loadInstall("copilot")
	if err != nil {
		return fmt.Errorf("copilot: %w", err)
	}

	bin, err := ensureInstalled(install)
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
