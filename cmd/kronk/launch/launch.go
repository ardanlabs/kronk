package launch

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// run resolves the requested agent, discovers the installed chat models on
// the running Kronk server, chooses a default model, and launches the
// agent.
func run(cmd *cobra.Command, args []string) error {
	name, passArgs, err := parseArgs(args, cmd.ArgsLenAtDash())
	if err != nil {
		return err
	}

	if name == "" {
		return fmt.Errorf("an agent name is required (supported: %s)\n\nexample: kronk launch opencode", strings.Join(supported(), ", "))
	}

	runner, err := lookup(name)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	chatModels, err := fetchChatModels(ctx)
	if err != nil {
		return fmt.Errorf("unable to reach the Kronk server: %w\n\nis it running? start it with: kronk server start", err)
	}

	if len(chatModels) == 0 {
		return fmt.Errorf("no installed chat models found\n\ninstall one first, for example: kronk model pull unsloth/Qwen3-8B-Q8_0")
	}

	requested, _ := cmd.Flags().GetString("model")
	defaultModel, err := resolveDefaultModel(requested, chatModels)
	if err != nil {
		return err
	}

	// The context window is discovered best-effort; when it is unknown the
	// agent falls back to its own default, which for a large local model can
	// overflow the server's window ("input tokens exceed context window").
	// Warn so the user can pass a model whose window resolves, or size it in
	// their model_config.yaml.
	if contextFor(defaultModel, chatModels) == 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not determine the context window for %q; the agent may assume too large a window and overflow the server.\n\n", defaultModel)
	}

	return runner.Run(defaultModel, chatModels, passArgs)
}

// parseArgs splits "<agent> [-- extra args]" into the agent name and the
// pass-through args for the agent. dash is the number of args before a "--"
// separator (cobra's ArgsLenAtDash), or -1 when there is no "--".
func parseArgs(args []string, dash int) (name string, passArgs []string, err error) {
	if dash == -1 {
		if len(args) > 1 {
			return "", nil, fmt.Errorf("unexpected arguments: %v\nuse '--' to pass extra args to the agent", args[1:])
		}
		if len(args) == 1 {
			name = args[0]
		}
		return name, nil, nil
	}

	if dash > 1 {
		return "", nil, fmt.Errorf("expected at most one agent name before '--', got %d", dash)
	}
	if dash == 1 {
		name = args[0]
	}

	return name, args[dash:], nil
}

// resolveDefaultModel returns the model to use as the agent default. When
// requested is empty it prefers a profile variant (e.g. "<base>/AGENT"),
// which carries the large context window an agent needs, and otherwise
// falls back to the first (sorted) chat model. When requested is set it
// validates that it is an installed chat model.
func resolveDefaultModel(requested string, chatModels []Model) (string, error) {
	if requested == "" {
		for _, m := range chatModels {
			if m.Variant {
				return m.ID, nil
			}
		}
		return chatModels[0].ID, nil
	}

	for _, m := range chatModels {
		if m.ID == requested {
			return requested, nil
		}
	}

	ids := make([]string, 0, len(chatModels))
	for _, m := range chatModels {
		ids = append(ids, m.ID)
	}

	return "", fmt.Errorf("model %q is not an installed chat model (available: %s)", requested, strings.Join(ids, ", "))
}
