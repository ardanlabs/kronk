package launch

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
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

	chatModels, err := discoverChatModels()
	if err != nil {
		return err
	}

	requested, _ := cmd.Flags().GetString("model")

	// When no model is requested and none of the curated launch models are
	// installed, list the curated models and let the user pick one to download
	// so launch runs on a curated model. Skipping falls back to whatever is
	// installed.
	chatModels, err = maybePullCuratedModel(cmd, requested, chatModels)
	if err != nil {
		return err
	}

	if len(chatModels) == 0 {
		return noChatModelsError()
	}

	// Let the user know when they are running with only some of the curated
	// launch models installed, so a partial install (e.g. only one of three)
	// is visible rather than silent. This never blocks the launch.
	noteMissingCuratedModels(cmd, chatModels)

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
		// Prefer a curated launch model in metadata order; these are the
		// coding models launch is meant to run on.
		if _, m, ok := firstInstalledCurated(chatModels); ok {
			return m.ID, nil
		}

		// Otherwise prefer any profile variant (which carries the large
		// context window an agent needs) and finally the first chat model.
		for _, m := range chatModels {
			if m.Variant {
				return m.ID, nil
			}
		}
		return chatModels[0].ID, nil
	}

	// An exact installed id wins.
	for _, m := range chatModels {
		if m.ID == requested {
			return requested, nil
		}
	}

	// A curated alias/key (e.g. "qwen", "gemma") resolves to the installed
	// model for that curated entry, preferring its AGENT profile. This is the
	// uniform, agent-agnostic way to swap between the curated models.
	if entry, ok := lookupCurated(requested); ok {
		if m, ok := installedCuratedModel(entry.Key, chatModels); ok {
			return m.ID, nil
		}
		return "", fmt.Errorf("model %q (%s) is not installed\n\npull it first (--local uses a longer timeout, better for these large models): kronk model pull --local %s", requested, entry.Display, entry.PullID)
	}

	ids := make([]string, 0, len(chatModels))
	for _, m := range chatModels {
		ids = append(ids, m.ID)
	}

	return "", fmt.Errorf("model %q is not an installed chat model (available: %s)", requested, strings.Join(ids, ", "))
}

// discoverChatModels queries the running Kronk server for its installed
// chat-capable models under a short timeout.
func discoverChatModels() ([]Model, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	chatModels, err := fetchChatModels(ctx)
	if err != nil {
		// The underlying dial/HTTP error (e.g. "connection refused") is noise
		// for the common case: the server simply isn't running. Hide it and
		// surface a clear, actionable next step instead.
		return nil, errors.New(
			"cannot reach the Kronk server — it does not appear to be running.\n\n" +
				"  Start it with this command, then try again:\n\n" +
				"      kronk server start")
	}

	return chatModels, nil
}

// maybePullCuratedModel lists the curated launch models and lets the user pick
// one to download when no --model was requested and none of the curated models
// are installed. It only prompts on an interactive terminal, and skipping (or a
// non-interactive terminal) falls back to whatever is installed. On a
// successful pull it re-discovers the installed models so the new one is picked
// up.
func maybePullCuratedModel(cmd *cobra.Command, requested string, chatModels []Model) ([]Model, error) {
	// An explicit --model is honored as-is (resolveDefaultModel validates it);
	// only the curated default is auto-pulled.
	if requested != "" {
		return chatModels, nil
	}

	// Already have a curated model installed — nothing to pull.
	if _, _, ok := firstInstalledCurated(chatModels); ok {
		return chatModels, nil
	}

	curated := orderedCurated()
	if len(curated) == 0 {
		// No curated metadata available; leave discovery as-is.
		return chatModels, nil
	}

	// Never run a network pull on a non-interactive terminal; fall back to
	// whatever is installed (the caller surfaces guidance if nothing is).
	if !isInteractive() {
		return chatModels, nil
	}

	entry, ok := chooseCuratedModel(curated)
	if !ok {
		// User skipped the selection.
		return chatModels, nil
	}

	if err := pullCuratedModel(cmd, entry); err != nil {
		return nil, err
	}

	// Re-discover so the freshly pulled model (and its AGENT profile variant)
	// is picked up.
	refreshed, err := discoverChatModels()
	if err != nil {
		return nil, err
	}

	// The pull stream can end without surfacing a server-side failure, so
	// verify by outcome: the curated model must now be installed. (Nothing
	// curated was installed before the pull, so a match here is the model we
	// just pulled.)
	if _, _, ok := firstInstalledCurated(refreshed); !ok {
		return nil, fmt.Errorf("pull of %s did not complete; it is still not installed\n\ntry again manually (--local uses a longer timeout, better for these large models): kronk model pull --local %s", entry.Display, entry.PullID)
	}

	return refreshed, nil
}

// chooseCuratedModel lists the curated launch models and lets the user pick one
// to download, or skip. It avoids fabricating a size; the optional size_note
// from metadata is shown when present, and the pull itself streams real
// progress. It reads a single line: a valid number selects that model; anything
// else (including empty) skips. ok is false when the user skips.
func chooseCuratedModel(curated []launchModel) (launchModel, bool) {
	w := os.Stderr

	fmt.Fprintln(w, "No curated launch model is installed. Pick one to download:")
	fmt.Fprintln(w)
	for i, m := range curated {
		size := ""
		if m.SizeNote != "" {
			size = " (" + m.SizeNote + ")"
		}
		fmt.Fprintf(w, "  %d) %s [%s]%s\n", i+1, m.Display, m.Quant, size)
	}
	fmt.Fprintln(w)
	fmt.Fprint(w, "Enter a number to download, or press Enter to skip: ")

	var response string
	fmt.Scanln(&response)

	n, err := strconv.Atoi(strings.TrimSpace(response))
	if err != nil || n < 1 || n > len(curated) {
		return launchModel{}, false
	}

	return curated[n-1], true
}

// pullStatus is the subset of the server's pull SSE response the launcher
// needs. It is defined locally so this package does not depend on the server's
// internal types.
type pullStatus struct {
	Status string `json:"status"`
}

// pullCuratedModel streams a model pull from the running Kronk server, using the
// same POST /v1/kronk/models/pull endpoint as "kronk model pull". A pull can
// take a long time, so it runs under a generous timeout.
func pullCuratedModel(cmd *cobra.Command, m launchModel) error {
	url, err := client.DefaultURL("/v1/kronk/models/pull")
	if err != nil {
		return fmt.Errorf("default-url: %w", err)
	}

	cln := client.NewSSE[pullStatus](
		client.NoopLogger,
		client.WithBearer(os.Getenv("KRONK_TOKEN")),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	fmt.Fprintf(cmd.ErrOrStderr(), "Pulling %s...\n", m.PullID)

	ch := make(chan pullStatus)
	if err := cln.Do(ctx, http.MethodPost, url, client.D{"model_url": m.PullID}, ch); err != nil {
		return fmt.Errorf("pull %s: %w", m.PullID, err)
	}

	for st := range ch {
		if st.Status != "" {
			fmt.Fprint(cmd.ErrOrStderr(), st.Status)
		}
	}
	fmt.Fprintln(cmd.ErrOrStderr())

	return nil
}

// noteMissingCuratedModels prints an informational notice when only some of
// the curated launch models are installed, listing the missing ones and the
// exact command to pull each. It is advisory only and never blocks the launch;
// it prints nothing when all curated models are installed (or none are, since
// that case is handled by the auto-pull prompt / no-models error).
func noteMissingCuratedModels(cmd *cobra.Command, chatModels []Model) {
	total, missing := curatedInstallStatus(chatModels)

	// Nothing to say when metadata is unavailable, all are installed, or none
	// are installed (the latter is handled elsewhere).
	if total == 0 || len(missing) == 0 || len(missing) == total {
		return
	}

	w := cmd.ErrOrStderr()
	fmt.Fprintf(w, "Note: %d of %d curated launch models are installed; %d missing:\n", total-len(missing), total, len(missing))
	for _, m := range missing {
		fmt.Fprintf(w, "  - %s [%s]  →  kronk model pull --local %s\n", m.Display, m.Quant, m.PullID)
	}
	fmt.Fprintln(w, "(--local uses a longer timeout than the default web pull, better for these large models.)")
	fmt.Fprintln(w)
}

// noChatModelsError builds the "no models installed" error. When curated
// metadata is available it lists the exact pull commands for the curated
// models; otherwise it falls back to a generic example.
func noChatModelsError() error {
	lm, err := loadLaunchModels()
	if err != nil || len(lm.Order) == 0 {
		return errors.New("no installed chat models found\n\ninstall one first, for example: kronk model pull --local Qwen/Qwen3-8B-Q8_0")
	}

	var b strings.Builder
	b.WriteString("no installed chat models found\n\ninstall one of the curated coding models, for example:\n")
	for _, key := range lm.Order {
		fmt.Fprintf(&b, "  kronk model pull --local %s\n", lm.Models[key].PullID)
	}
	b.WriteString("(--local uses a longer timeout than the default web pull, better for these large models.)")

	return errors.New(strings.TrimRight(b.String(), "\n"))
}
