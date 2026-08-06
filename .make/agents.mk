# ==============================================================================
# Agents — Default bundle
#
# Rote-free baseline. Host configs wire the Kronk MCP server directly
# into each host so agents can call `web_search` and `fuzzy_edit` over
# raw MCP. Most contributors use this.
#
# Note on `rm -rf … skills`: keeps the copy idempotent and also prunes
# any rote skill left behind from a previous `agents-rote-<host>` run
# before the default skill tree is laid down.

agents-default-opencode:
	mkdir -p $$HOME/.config/opencode
	cp .agents/default/opencode/opencode.jsonc $$HOME/.config/opencode/opencode.jsonc
	cp .agents/default/opencode/tui.jsonc $$HOME/.config/opencode/tui.jsonc
	cp .agents/default/opencode/auth.json $$HOME/.config/opencode/auth.json
	cp .agents/default/AGENTS.md $$HOME/.config/opencode/AGENTS.md
	rm -rf $$HOME/.config/opencode/skills
	cp -r .agents/default/skills $$HOME/.config/opencode/skills
