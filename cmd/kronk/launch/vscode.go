package launch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
)

// vscodeProviderName is the display/group name of the Kronk provider written
// into VS Code's Copilot Chat BYOK config. The launcher fully owns the entry
// with this name.
const vscodeProviderName = "Kronk"

// vscodeVendor is the VS Code Copilot Chat BYOK provider type for an arbitrary
// OpenAI-compatible endpoint (the "Custom Endpoint" provider).
const vscodeVendor = "customendpoint"

// vscodeOutputReserveFraction is the share of a model's context window reported
// as output budget; the remainder is the input budget. Together they tell
// Copilot the model's window so it does not overflow the server.
const vscodeOutputReserveFraction = 4 // reserve 1/4 of the window for output

// minVSCodeVersion and minCopilotChatVersion are the lowest versions that carry
// the "Custom Endpoint" BYOK feature the launcher relies on. On older versions
// the model picker only offers "Auto", which is the symptom users hit when
// their editor is out of date.
const (
	minVSCodeVersion      = "1.104"
	minCopilotChatVersion = "0.41.0"
)

// vsCode implements Runner for Visual Studio Code. VS Code has no built-in
// model; "launching" it means wiring GitHub Copilot Chat's BYOK (bring your own
// key) "Custom Endpoint" provider to the local Kronk server, then opening the
// editor. Run writes the Kronk models into VS Code's BYOK config
// (chatLanguageModels.json, plus the deprecated customOAIModels setting as a
// stable-channel fallback), then opens VS Code. The final "select the model in
// the picker" step is left to the user (Copilot Chat is UI-driven), so nothing
// fragile like the editor's state database is touched.
type vsCode struct{}

// Run implements Runner. It locates VS Code, writes the Kronk BYOK config, and
// opens the editor (in the current directory by default, or with the caller's
// pass-through args).
func (vsCode) Run(defaultModel string, chatModels []Model, args []string) error {
	cli := findVSCodeCLI()
	if cli == "" {
		return fmt.Errorf("VS Code is not installed\n\ninstall it from https://code.visualstudio.com/download\n\nthen re-run: kronk launch vscode")
	}

	// Copilot Chat is what consumes the BYOK config, and the "Custom Endpoint"
	// feature needs recent VS Code + Copilot Chat. Warn (non-fatal) about the
	// common causes of an empty ("Auto" only) picker before opening the editor.
	checkVSCodeVersions(cli)

	if err := writeVSCodeConfig(chatModels); err != nil {
		return fmt.Errorf("configure vscode: %w", err)
	}

	baseURL, err := client.DefaultURL("/v1")
	if err != nil {
		return fmt.Errorf("default-url: %w", err)
	}

	printVSCodeTip(defaultModel, baseURL)

	openArgs := args
	if len(openArgs) == 0 {
		openArgs = []string{"."}
	}

	cmd := exec.Command(cli, openArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// writeVSCodeConfig writes the Kronk models into VS Code's Copilot Chat BYOK
// configuration. chatLanguageModels.json (the documented "Custom Endpoint"
// provider) is the primary and fatal on error; the deprecated
// customOAIModels setting is written as a best-effort stable-channel fallback.
func writeVSCodeConfig(chatModels []Model) error {
	if len(chatModels) == 0 {
		return fmt.Errorf("at least one model is required")
	}

	baseURL, err := client.DefaultURL("/v1")
	if err != nil {
		return fmt.Errorf("default-url: %w", err)
	}

	apiKey := "kronk"
	if os.Getenv("KRONK_TOKEN") != "" {
		// VS Code interpolates ${ENV} in the BYOK config at request time, so the
		// token is read from the environment (inherited by the launched editor)
		// instead of being written to disk.
		apiKey = "${KRONK_TOKEN}"
	}

	if err := writeVSCodeChatModels(chatModels, baseURL, apiKey); err != nil {
		return err
	}

	if err := writeVSCodeCustomOAIModels(chatModels, baseURL); err != nil {
		fmt.Fprintf(os.Stderr, "note: left VS Code settings.json unchanged (%v).\nOn stable VS Code, add the Kronk model via Copilot Chat > Manage Language Models if needed.\n\n", err)
	}

	return nil
}

// writeVSCodeChatModels writes the Kronk "Custom Endpoint" provider into
// chatLanguageModels.json, merging into any existing config and backing up the
// previous file first.
func writeVSCodeChatModels(chatModels []Model, baseURL, apiKey string) error {
	path, err := vscodeUserPath("chatLanguageModels.json")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	var existing []map[string]any
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &existing)
	}

	merged := buildVSCodeChatLanguageModels(existing, chatModels, baseURL, apiKey)

	data, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return err
	}

	return writeFileWithBackup(path, data)
}

// writeVSCodeCustomOAIModels writes the Kronk models into the deprecated
// github.copilot.chat.customOAIModels setting in settings.json. To avoid
// mangling a user's settings.json that uses comments/trailing commas (JSONC),
// it refuses to touch a file that does not parse as strict JSON.
func writeVSCodeCustomOAIModels(chatModels []Model, baseURL string) error {
	path, err := vscodeUserPath("settings.json")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	existing := map[string]any{}
	if data, err := os.ReadFile(path); err == nil && len(bytes.TrimSpace(data)) > 0 {
		if json.Unmarshal(data, &existing) != nil {
			return fmt.Errorf("settings.json uses comments/JSONC and cannot be edited safely")
		}
	}

	merged := buildVSCodeSettings(existing, chatModels, baseURL)

	data, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return err
	}

	return writeFileWithBackup(path, data)
}

// buildVSCodeChatLanguageModels merges the Kronk Custom Endpoint provider into
// an existing chatLanguageModels.json document. The launcher fully owns the
// provider named "Kronk" (rebuilt each launch so a moved server or changed
// model set is corrected); other providers the user configured are preserved.
func buildVSCodeChatLanguageModels(existing []map[string]any, chatModels []Model, baseURL, apiKey string) []map[string]any {
	out := make([]map[string]any, 0, len(existing)+1)
	for _, e := range existing {
		if name, _ := e["name"].(string); name == vscodeProviderName {
			continue
		}
		out = append(out, e)
	}

	models := make([]any, 0, len(chatModels))
	for _, m := range chatModels {
		entry := vscodeModelFields(m, baseURL)
		entry["id"] = m.ID
		models = append(models, entry)
	}

	out = append(out, map[string]any{
		"name":    vscodeProviderName,
		"vendor":  vscodeVendor,
		"apiKey":  apiKey,
		"apiType": "chat-completions",
		"models":  models,
	})

	return out
}

// buildVSCodeSettings merges the Kronk models into the customOAIModels map in a
// settings.json document. Existing custom models (keyed by id) are preserved;
// the current Kronk model ids are added or refreshed. Other settings are left
// untouched.
func buildVSCodeSettings(settings map[string]any, chatModels []Model, baseURL string) map[string]any {
	if settings == nil {
		settings = map[string]any{}
	}

	const key = "github.copilot.chat.customOAIModels"

	oai, _ := settings[key].(map[string]any)
	if oai == nil {
		oai = map[string]any{}
	}

	for _, m := range chatModels {
		oai[m.ID] = vscodeModelFields(m, baseURL)
	}

	settings[key] = oai

	return settings
}

// vscodeModelFields builds the shared VS Code BYOK model fields for a Kronk
// model. The full Chat Completions URL, tool-calling capability, vision flag,
// and token budgets (derived from the resolved context window) are included so
// Copilot treats the model correctly and does not overflow the server.
func vscodeModelFields(m Model, baseURL string) map[string]any {
	name := m.Name
	if name == "" {
		name = m.ID
	}

	fields := map[string]any{
		"name":        name,
		"url":         baseURL + "/chat/completions",
		"toolCalling": true,
	}

	if m.Vision {
		fields["vision"] = true
	}

	if m.Context > 0 {
		out := max(m.Context/vscodeOutputReserveFraction, 1)
		fields["maxOutputTokens"] = out
		fields["maxInputTokens"] = m.Context - out
	}

	return fields
}

// printVSCodeTip tells the user how to add and select the Kronk model in
// Copilot Chat. VS Code's Copilot Chat does not reliably pick up the BYOK
// config files this launcher writes (the file-based "Custom Endpoint" path is
// Insiders-only and still flaky), so the reliable step is the built-in
// "Manage Models" UI. The tip walks the user through that and prints the exact
// values to paste so nothing has to be guessed.
func printVSCodeTip(defaultModel, baseURL string) {
	fmt.Fprintf(os.Stderr, "VS Code is opening. Copilot Chat is the AI chat panel (chat icon in the\n")
	fmt.Fprintf(os.Stderr, "sidebar). To chat with your local Kronk model instead of GitHub's cloud\n")
	fmt.Fprintf(os.Stderr, "models, add it once via Copilot Chat's model picker:\n\n")
	fmt.Fprintf(os.Stderr, "  1. Open Copilot Chat, click the model picker at the bottom of the panel.\n")
	fmt.Fprintf(os.Stderr, "  2. Click \"Manage Models\" (or run \"Chat: Manage Language Models\" from the\n")
	fmt.Fprintf(os.Stderr, "     Command Palette), then \"Add Models\".\n")
	fmt.Fprintf(os.Stderr, "  3. In the provider list choose \"Custom Endpoint\" (older builds call it\n")
	fmt.Fprintf(os.Stderr, "     \"OpenAI Compatible\"). It is not one of the built-in providers shown\n")
	fmt.Fprintf(os.Stderr, "     first - scroll the list down past OpenAI/Ollama/Azure to find it.\n")
	fmt.Fprintf(os.Stderr, "     Do NOT pick plain \"OpenAI\" - that targets GitHub's hosted OpenAI and\n")
	fmt.Fprintf(os.Stderr, "     fails with \"Invalid response format\" against a local server.\n")
	fmt.Fprintf(os.Stderr, "  4. When prompted, enter:\n")
	fmt.Fprintf(os.Stderr, "       Group name   : %s\n", vscodeProviderName)
	fmt.Fprintf(os.Stderr, "       Display name : %s\n", vscodeProviderName)
	fmt.Fprintf(os.Stderr, "       API key      : %s\n", vscodeTipAPIKey())
	fmt.Fprintf(os.Stderr, "     then pick API type \"Chat Completions\".\n")
	fmt.Fprintf(os.Stderr, "  5. VS Code opens chatLanguageModels.json. Set the model's \"url\" and \"id\":\n")
	fmt.Fprintf(os.Stderr, "       url : %s/chat/completions\n", baseURL)
	fmt.Fprintf(os.Stderr, "       id  : %s\n", defaultModel)
	fmt.Fprintf(os.Stderr, "     then save. (kronk launch already pre-wrote this file, so the Kronk\n")
	fmt.Fprintf(os.Stderr, "     models may already be present - just save.)\n")
	fmt.Fprintf(os.Stderr, "  6. Pick the \"%s\" model in the model picker and start chatting.\n\n", defaultModel)
	fmt.Fprintf(os.Stderr, "Notes:\n")
	fmt.Fprintf(os.Stderr, "  - This works on the free Copilot plan; BYOK models don't need a paid plan.\n")
	fmt.Fprintf(os.Stderr, "  - The model \"url\" must be the full Chat Completions URL ending in\n")
	fmt.Fprintf(os.Stderr, "    \"/chat/completions\" (shown above) - not just the base URL.\n")
	fmt.Fprintf(os.Stderr, "  - If a previous attempt left a broken \"Invalid response format\" entry,\n")
	fmt.Fprintf(os.Stderr, "    remove it first, then re-add with \"Custom Endpoint\".\n")
	fmt.Fprintf(os.Stderr, "  - If \"Custom Endpoint\" is not in the Add Models provider list at all,\n")
	fmt.Fprintf(os.Stderr, "    your Copilot Chat is too old (or in the gap where the old \"OpenAI\n")
	fmt.Fprintf(os.Stderr, "    Compatible\" provider was removed): update the GitHub Copilot Chat\n")
	fmt.Fprintf(os.Stderr, "    extension, or switch it to its pre-release version, then reload.\n")
	fmt.Fprintf(os.Stderr, "  - If you only see \"Auto\", the model isn't added yet (repeat step 2) or\n")
	fmt.Fprintf(os.Stderr, "    VS Code / Copilot Chat is too old for Custom Endpoint - update both.\n")
	fmt.Fprintf(os.Stderr, "  - Run VS Code from a real install (e.g. /Applications), not from a\n")
	fmt.Fprintf(os.Stderr, "    mounted .dmg, so this launcher and the editor you use are the same.\n")
	fmt.Fprintf(os.Stderr, "  - Needs VS Code %s+ and Copilot Chat %s+.\n", minVSCodeVersion, minCopilotChatVersion)
	fmt.Fprintf(os.Stderr, "  - Keep the Kronk server running while you use it.\n\n")
}

// vscodeTipAPIKey returns the API key value the user should paste into the
// Manage Models prompt: their KRONK_TOKEN when set, otherwise any non-empty
// placeholder (Kronk does not require a key by default, but the field is
// mandatory in the UI).
func vscodeTipAPIKey() string {
	if t := os.Getenv("KRONK_TOKEN"); t != "" {
		return t
	}
	return "kronk (any non-empty value; Kronk needs no key unless you set one)"
}

// findVSCodeCLI returns the path to the VS Code CLI (usable both for
// --list-extensions and to open the editor), or "" when VS Code is not found.
// PATH is checked first, then platform-specific install locations (including
// the CLI inside the macOS app bundle).
func findVSCodeCLI() string {
	if p, err := exec.LookPath("code"); err == nil {
		return p
	}

	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		candidates = []string{"/Applications/Visual Studio Code.app/Contents/Resources/app/bin/code"}
	case "windows":
		if lad := os.Getenv("LOCALAPPDATA"); lad != "" {
			candidates = append(candidates, filepath.Join(lad, "Programs", "Microsoft VS Code", "bin", "code.cmd"))
		}
		if pf := os.Getenv("ProgramFiles"); pf != "" {
			candidates = append(candidates, filepath.Join(pf, "Microsoft VS Code", "bin", "code.cmd"))
		}
	default:
		candidates = []string{"/usr/bin/code", "/snap/bin/code", "/usr/share/code/bin/code"}
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}

	return ""
}

// checkVSCodeVersions prints non-fatal warnings about the two most common
// reasons the Kronk model never shows up in the picker: an out-of-date VS Code
// or a missing/old GitHub Copilot Chat extension. Both are needed for the BYOK
// "Custom Endpoint" feature; without them the picker only offers "Auto". Every
// check fails open (stays quiet when it cannot tell) so it never nags on a
// false negative.
func checkVSCodeVersions(cli string) {
	if v := vscodeVersion(cli); v != "" && compareVersions(v, minVSCodeVersion) < 0 {
		fmt.Fprintf(os.Stderr, "Warning: VS Code %s is older than %s; the Custom Endpoint model feature may be missing.\n", v, minVSCodeVersion)
		fmt.Fprintf(os.Stderr, "Update VS Code (Code > Check for Updates), then re-run.\n\n")
	}

	installed, ver := copilotChatVersion(cli)
	switch {
	case !installed:
		fmt.Fprintf(os.Stderr, "Warning: the GitHub Copilot Chat extension is not installed.\n")
		fmt.Fprintf(os.Stderr, "Install it in VS Code (Extensions > search \"GitHub Copilot Chat\"), then re-run.\n\n")
	case ver != "" && compareVersions(ver, minCopilotChatVersion) < 0:
		fmt.Fprintf(os.Stderr, "Warning: GitHub Copilot Chat %s is older than %s; custom models may not appear.\n", ver, minCopilotChatVersion)
		fmt.Fprintf(os.Stderr, "Update it in VS Code (Extensions > GitHub Copilot Chat > Update), then re-run.\n\n")
	}
}

// vscodeVersion returns the VS Code version reported by "code --version" (the
// first output line), or "" when it cannot be determined.
func vscodeVersion(cli string) string {
	out, err := exec.Command(cli, "--version").Output()
	if err != nil {
		return ""
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 {
		return ""
	}

	return strings.TrimSpace(lines[0])
}

// copilotChatVersion reports whether the GitHub Copilot Chat extension is
// installed and, if so, its version. installed is true when it cannot tell
// (e.g. the CLI query fails) so the launcher does not nag on a false negative;
// the returned version is "" in that case.
func copilotChatVersion(cli string) (installed bool, version string) {
	out, err := exec.Command(cli, "--list-extensions", "--show-versions").Output()
	if err != nil {
		return true, ""
	}

	for line := range strings.SplitSeq(string(out), "\n") {
		if !strings.HasPrefix(strings.ToLower(line), "github.copilot-chat@") {
			continue
		}
		if _, ver, ok := strings.Cut(line, "@"); ok {
			return true, strings.TrimSpace(ver)
		}
		return true, ""
	}

	return false, ""
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

// vscodeUserPath returns the path to a file under VS Code's User config
// directory for the current platform.
func vscodeUserPath(parts ...string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	var base string
	switch runtime.GOOS {
	case "darwin":
		base = filepath.Join(home, "Library", "Application Support", "Code", "User")
	case "windows":
		base = filepath.Join(os.Getenv("APPDATA"), "Code", "User")
	default:
		base = filepath.Join(home, ".config", "Code", "User")
	}

	return filepath.Join(append([]string{base}, parts...)...), nil
}
