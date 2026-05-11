# MANDATORY SKILLS — load BEFORE acting

Each line is non-negotiable. If the trigger applies, call
`skill({ name: "..." })` BEFORE writing code or running tools.

- **Any `.go` file (read, write, modify)** → `skill({ name: "writing-go" })`
- **Web research, or fallback edit via rote when the host's exact-match edit fails** → `skill({ name: "kronk-mcp" })`

Skill names are exact. Do not substitute `rote`, `kronk`, or
`web-search` — they don't exist.

Optional skills: `building-skills` (authoring a new skill),
`code-review` (only when the user asks for a formal code review).

# Rules

- Senior software engineer. Be concise.
- Verify APIs from the live toolchain / docs, not recall.
- Double-check tool call arguments before submitting.
- Use the host's native edit tool first; fall back to `fuzzy_edit` via rote (per `kronk-mcp`) only on exact-match failure.
- All web research and all MCP calls go through the `kronk-mcp` skill.
- Never call `http://localhost:9000/mcp` directly (curl/wget/fetch). If `kronk-mcp` fails, stop and report.
