package launch

import (
	"embed"
	"fmt"
	"sync"

	"go.yaml.in/yaml/v2"
)

//go:embed yaml/agents.yaml
var agentsFS embed.FS

// embeddedAgentsPath mirrors the //go:embed directive above.
const embeddedAgentsPath = "yaml/agents.yaml"

// agentsSchemaVersion is the schema version this build understands. The loader
// rejects any other value so the format can evolve safely.
const agentsSchemaVersion = 1

// agentsFile is the top-level shape of yaml/agents.yaml.
type agentsFile struct {
	Schema int                  `yaml:"schema"`
	Agents map[string]agentMeta `yaml:"agents"`
}

// agentMeta is the install metadata for a single agent.
type agentMeta struct {
	Display      string               `yaml:"display"`
	Bin          string               `yaml:"bin"`
	FallbackDirs []string             `yaml:"fallback_dirs"`
	DocsURL      string               `yaml:"docs_url"`
	Install      map[string]osInstall `yaml:"install"`
}

// osInstall is a per-OS install recipe (os keys: darwin | linux | windows).
type osInstall struct {
	Deps      []string  `yaml:"deps"`
	DepsNote  string    `yaml:"deps_note"`
	DepsError string    `yaml:"deps_error"`
	Hint      string    `yaml:"hint"`
	Command   osCommand `yaml:"command"`
}

// osCommand is the command actually executed to install an agent.
type osCommand struct {
	Bin  string   `yaml:"bin"`
	Args []string `yaml:"args"`
}

var (
	agentsOnce sync.Once
	agentsData agentsFile
	agentsErr  error
)

// loadAgents parses the embedded agents metadata once, caching the result (and
// any error). It validates the schema version so an incompatible file fails
// loudly rather than silently misbehaving.
func loadAgents() (agentsFile, error) {
	agentsOnce.Do(func() {
		data, err := agentsFS.ReadFile(embeddedAgentsPath)
		if err != nil {
			agentsErr = fmt.Errorf("read embedded agents metadata: %w", err)
			return
		}

		if err := yaml.Unmarshal(data, &agentsData); err != nil {
			agentsErr = fmt.Errorf("parse agents metadata: %w", err)
			return
		}

		if agentsData.Schema != agentsSchemaVersion {
			agentsErr = fmt.Errorf("agents metadata: unsupported schema %d (want %d)", agentsData.Schema, agentsSchemaVersion)
		}
	})

	return agentsData, agentsErr
}

// loadInstall returns the install metadata for the named agent. It returns an
// error (never panics) when the metadata cannot be loaded or the agent is
// absent, so a missing/broken entry degrades to a single failing agent instead
// of taking down the whole CLI.
func loadInstall(name string) (agentInstall, error) {
	af, err := loadAgents()
	if err != nil {
		return agentInstall{}, err
	}

	meta, ok := af.Agents[name]
	if !ok {
		return agentInstall{}, fmt.Errorf("launch metadata: agent %q not found", name)
	}

	return agentInstall{
		bin:          meta.Bin,
		display:      meta.Display,
		fallbackDirs: meta.FallbackDirs,
		docsURL:      meta.DocsURL,
		perOS:        meta.Install,
	}, nil
}
