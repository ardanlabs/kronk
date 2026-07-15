package launch

import (
	"strings"
	"testing"
)

// TestAgentsMetadataParses verifies the embedded metadata parses and reports a
// schema version this build understands.
func TestAgentsMetadataParses(t *testing.T) {
	af, err := loadAgents()
	if err != nil {
		t.Fatalf("loadAgents: %v", err)
	}

	if af.Schema != agentsSchemaVersion {
		t.Fatalf("schema: got %d, want %d", af.Schema, agentsSchemaVersion)
	}

	if len(af.Agents) == 0 {
		t.Fatal("expected at least one agent in metadata")
	}
}

// TestEveryRegisteredAgentHasMetadata ensures every agent in the registry
// resolves via loadInstall, so a launchable agent can never be missing its
// install metadata.
func TestEveryRegisteredAgentHasMetadata(t *testing.T) {
	for name := range registry {
		install, err := loadInstall(name)
		if err != nil {
			t.Errorf("loadInstall(%q): %v", name, err)
			continue
		}

		if install.bin == "" {
			t.Errorf("%s: empty bin", name)
		}
		if install.display == "" {
			t.Errorf("%s: empty display", name)
		}

		// Every agent must have a working install recipe on the three
		// first-class platforms so the launcher can always offer a real fix.
		for _, goos := range []string{"darwin", "linux", "windows"} {
			bin, args, err := install.installerCommand(goos)
			if err != nil {
				t.Errorf("%s/%s: installerCommand error: %v", name, goos, err)
				continue
			}
			if bin == "" || len(args) == 0 {
				t.Errorf("%s/%s: empty installer command", name, goos)
			}
			if install.installHint(goos) == "" {
				t.Errorf("%s/%s: empty install hint", name, goos)
			}
		}
	}
}

// TestLoadInstallUnknownAgent verifies an unknown agent degrades to an error
// (Option B) rather than panicking or returning a zero value.
func TestLoadInstallUnknownAgent(t *testing.T) {
	if _, err := loadInstall("does-not-exist"); err == nil {
		t.Fatal("expected error for unknown agent, got nil")
	}
}

// TestCopilotDepsNotePreservesNuance is a guardrail: the Copilot missing-deps
// message must still surface the "Node.js 22+" version nuance that the old
// hand-written message carried, so the shared template does not silently drop
// important specifics.
func TestCopilotDepsNotePreservesNuance(t *testing.T) {
	af, err := loadAgents()
	if err != nil {
		t.Fatalf("loadAgents: %v", err)
	}

	note := af.Agents["copilot"].Install["darwin"].DepsNote
	if !strings.Contains(note, "22+") {
		t.Errorf("copilot deps_note: got %q, want it to mention Node.js 22+", note)
	}
}

// TestCheckDepsMessageIsNeutralAndActionable is a guardrail on the shared
// missing-deps template: it must stay neutral about why a dep is needed ("for
// setup", not a false "to install it" claim) and always show the real install
// command as the fix.
func TestCheckDepsMessageIsNeutralAndActionable(t *testing.T) {
	install, err := loadInstall("codex")
	if err != nil {
		t.Fatalf("loadInstall: %v", err)
	}

	// Point at a dep that cannot exist so checkDeps always reports it missing,
	// independent of the machine running the test.
	install.perOS = map[string]osInstall{
		"linux": {
			Deps: []string{"definitely-not-a-real-binary-xyz"},
			Hint: "npm install -g @openai/codex",
		},
	}

	err = install.checkDeps("linux")
	if err == nil {
		t.Fatal("expected a missing-deps error, got nil")
	}

	msg := err.Error()
	if !strings.Contains(msg, "for setup") {
		t.Errorf("message should use neutral wording %q; got: %q", "for setup", msg)
	}
	if !strings.Contains(msg, "npm install -g @openai/codex") {
		t.Errorf("message should include the real install command as the fix; got: %q", msg)
	}
}

// TestDepsErrorEscapeHatch verifies a fully-bespoke deps_error, when supplied,
// wins verbatim over the shared template.
func TestDepsErrorEscapeHatch(t *testing.T) {
	install := agentInstall{
		bin:     "widget",
		display: "Widget",
		perOS: map[string]osInstall{
			"linux": {
				Deps:      []string{"definitely-not-a-real-binary-xyz"},
				DepsError: "bespoke: install the Widget toolchain from https://example.com",
				Hint:      "widget install",
			},
		},
	}

	err := install.checkDeps("linux")
	if err == nil {
		t.Fatal("expected a missing-deps error, got nil")
	}

	if err.Error() != "bespoke: install the Widget toolchain from https://example.com" {
		t.Errorf("deps_error should win verbatim; got: %q", err.Error())
	}
}
