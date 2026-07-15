package launch

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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

	// Validate an explicit --model up front, before installing the agent, so a
	// typo or uninstalled model is surfaced immediately instead of after a
	// (possibly network) agent install. maybePullCuratedModel below is a no-op
	// when --model is set, so chatModels (and thus this resolution) is identical
	// to the final resolveDefaultModel call — the same source of truth, checked
	// early.
	if requested != "" {
		if _, err := resolveDefaultModel(requested, chatModels); err != nil {
			return err
		}
	}

	// Install the agent now — after confirming the server is reachable but
	// before any (possibly multi-GB) curated model pull below — so a missing or
	// uninstallable agent is surfaced up front instead of after a long download.
	// ensureInstalled is idempotent, so the runner's own install call is then a
	// fast no-op.
	if err := ensureAgentInstalled(name); err != nil {
		return err
	}

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

// ensureAgentInstalled ensures the named agent's binary is installed, prompting
// to install it when interactive. name is normalized to match the agent
// metadata keys (the same normalization lookup uses). It returns nil when the
// binary is already present, so calling it before a model pull only adds the
// install prompt to the front of the flow without changing behavior when the
// agent is already installed.
func ensureAgentInstalled(name string) error {
	install, err := loadInstall(strings.ToLower(strings.TrimSpace(name)))
	if err != nil {
		return err
	}

	_, err = ensureInstalled(install)
	return err
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
	if err == nil {
		return chatModels, nil
	}

	switch {
	// The server answered but refused the request: a missing, invalid, or
	// expired token — not a down server. Kronk returns 403 (permission denied,
	// which the client maps to ErrUnauthorized) or 401 (unauthenticated, which
	// the client surfaces in the error text as "status: 401"). Point the user
	// at the token rather than telling them to start an already-running server.
	case errors.Is(err, client.ErrUnauthorized) || strings.Contains(err.Error(), "status: 401"):
		return nil, errors.New(
			"the Kronk server refused the request — your KRONK_TOKEN is missing, invalid, or expired.\n\n" +
				"  Set a valid token, then try again:\n\n" +
				"      export KRONK_TOKEN=<token>")

	// The request did not complete within the timeout. This is distinct from an
	// outright refused connection: the server may be reachable but overloaded,
	// or the configured address may be wrong. "start the server" would be the
	// wrong advice, so give timeout-specific guidance.
	case errors.Is(err, context.DeadlineExceeded):
		return nil, fmt.Errorf(
			"the Kronk server at %s did not respond in time.\n\n"+
				"  It may be overloaded, or the address may be wrong.\n"+
				"  Check that it is running and reachable, then try again",
			serverAddr())

	// The HTTP round-trip never completed (connection refused, no route, DNS,
	// TLS). The raw dial error is noise, so hide it behind a clear, actionable
	// next step. When a custom host is configured, "start the local server" is
	// the wrong advice, so point at the configured address instead.
	case isTransportError(err):
		if os.Getenv("KRONK_WEB_API_HOST") != "" {
			return nil, fmt.Errorf(
				"cannot reach the Kronk server at %s (from KRONK_WEB_API_HOST).\n\n"+
					"  Check the address is correct and the server is running there, then try again",
				serverAddr())
		}
		return nil, errors.New(
			"cannot reach the Kronk server — it does not appear to be running.\n\n" +
				"  Start it with this command, then try again:\n\n" +
				"      kronk server start")

	// Anything else (5xx, a malformed response) is a real error the user needs
	// to see rather than a misleading "not running" message.
	default:
		return nil, fmt.Errorf("could not query the Kronk server for its models: %w", err)
	}
}

// serverAddr returns the Kronk server base address for use in error messages:
// the KRONK_WEB_API_HOST override when set, otherwise the default local address.
func serverAddr() string {
	if base, err := client.DefaultURL(""); err == nil {
		return strings.TrimRight(base, "/")
	}

	return "http://localhost:11435"
}

// isTransportError reports whether err is a failure to complete the HTTP
// round-trip (connection refused, DNS, TLS) rather than a response the server
// actually sent. net/http surfaces all such failures as *url.Error, which lets
// the launcher tell "server unreachable" apart from "server answered with an
// error status". A timeout is also a *url.Error, so callers check
// context.DeadlineExceeded before this to give timeout-specific guidance.
func isTransportError(err error) bool {
	var urlErr *url.Error
	return errors.As(err, &urlErr)
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
	// verify by outcome: the model the user actually chose must now be
	// installed. Checking the specific selection (not just "any curated model")
	// avoids a false success if a different curated model was installed
	// concurrently while this pull failed.
	if _, ok := installedCuratedModel(entry.Key, refreshed); !ok {
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

	line, ok := readPromptLine()
	if !ok || line == "" {
		// EOF or an empty line (the user pressed Enter): skip quietly.
		return launchModel{}, false
	}

	n, err := strconv.Atoi(line)
	if err != nil || n < 1 || n > len(curated) {
		fmt.Fprintln(w, "invalid selection; skipping.")
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

	// The SSE client signals a mid-stream timeout/cancel only by closing the
	// channel (there is no error channel), so a deadline hit here would
	// otherwise look like a successful pull. Check the context so the user gets
	// the real cause instead of the generic "did not complete" downstream.
	if ctx.Err() != nil {
		return fmt.Errorf("pull of %s did not finish in time (the connection may be slow); try again manually: kronk model pull --local %s", m.PullID, m.PullID)
	}

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
