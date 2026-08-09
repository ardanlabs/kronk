// Package agentdefaults embeds the default coding-agent configuration.
package agentdefaults

import "embed"

// Files contains the default OpenCode configuration, instructions, and skills.
//
//go:embed AGENTS.md opencode skills
var Files embed.FS
