# MANDATORY SKILLS — load BEFORE acting

Each line is non-negotiable. If the trigger applies, call
`skill({ name: "..." })` BEFORE writing code or running tools.

- **Any `.go` file (read, write, modify)** → `skill({ name: "writing-go" })`
- **Web research, or fallback edit when the host's exact-match edit fails** → `skill({ name: "kronk-mcp" })`

Optional skills: `building-skills` (authoring a new skill),
`code-review` (only when the user asks for a formal code review).

# Rules

- Senior software engineer. Be concise.
- Verify APIs from the live toolchain / docs, not recall.
- Double-check tool call arguments before submitting.
- Use the host's native edit tool first; fall back to Kronk MCP `fuzzy_edit` only on exact-match failure.
- Use Kronk MCP `web_search` for all web research.
- Never call `http://localhost:9000/mcp` directly (curl/wget/fetch). If `kronk-mcp` fails, stop and report.
