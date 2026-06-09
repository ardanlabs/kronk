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

# ==============================================================================
# Agents — Rote bundle
#
# All targets related to the rote execution layer (https://www.modiqo.ai/).
# Full documentation: .agents/rote/NOTES.md.
#
# Rote is OPT-IN — none of these targets are pulled in by install-tooling
# or any default-bundle target. Standard order for opting into rote:
#
#   make agents-rote-install        # installs the rote CLI
#   make agents-rote-login          # one-time interactive registry login
#                                   # (browser flow). Persisted on disk under
#                                   # ~/.rote/, survives reboots; only needs
#                                   # re-running after a wipe or token expiry.
#   make agents-rote-seed           # seeds ~/.rote/ with the project's
#                                   # adapters, rebuilds the search index,
#                                   # and ensures the `playground`
#                                   # workspace exists.
#   make agents-rote-<host>         # ships the rote-aware bundle for
#                                   # the agent host you actually use.
#
# Per-host targets ship .agents/rote/<host>/* + the rote-aware AGENTS.md
# + the rote skill to the host's config directory.

# Install the rote CLI from the upstream installer. The script is idempotent
# (re-run upgrades the binary without touching ~/.rote/), so we only skip it
# when `rote` is already on PATH. The VS Code extension is NOT required —
# this gives you the same ~/.rote/ state the extension would.
agents-rote-install:
	@command -v rote >/dev/null 2>&1 \
		&& echo "rote already installed at $$(command -v rote)" \
		|| curl -fsSL https://getrote.dev/install | bash

# Run the rote registry login flow. Required after `agents-rote-install` on
# a fresh box (or after `rm -rf ~/.rote`) before `agents-rote-seed` will
# work — `rote init` (used by agents-rote-playground) refuses to run without
# a registry session. Login state persists on disk under ~/.rote/secrets/
# and ~/.rote/registry/, so this is one-time per machine until you wipe.
# Modiqo's registry is invite-only — see .agents/rote/NOTES.md §3.
agents-rote-login:
	@rote whoami 2>&1 | grep -q "Not logged in" \
		&& rote login \
		|| echo "rote already logged in"

# Internal: fail fast with a clear pointer when seed/playground are run
# without a registry session, instead of letting `rote init` emit its
# generic "rote requires login" error and a non-obvious make stack trace.
agents-rote-login-check:
	@rote whoami 2>&1 | grep -q "Not logged in" && { \
		echo "rote is not logged in — run \`make agents-rote-login\` first."; \
		echo "(invite-only registry; see .agents/rote/NOTES.md §3)"; \
		exit 1; \
	} || true

# Create the long-lived `playground` workspace used for ad-hoc exploration
# with the adapter. (Modiqo's docs sometimes call this a "canvas" — same
# thing as a workspace, see .agents/rote/NOTES.md §1.) `rote init` is NOT
# idempotent — running it twice on the same name exits 1 with a verbose
# error — so we guard with a directory existence check. See
# .agents/rote/NOTES.md §8 step 3 for why workspace creation is a make
# target rather than something agents do.
agents-rote-playground: agents-rote-login-check
	@if [ -d "$$HOME/.rote/rote/workspaces/playground" ]; then \
		echo "playground workspace already exists at $$HOME/.rote/rote/workspaces/playground"; \
	else \
		rote init playground --seq && echo "playground workspace created"; \
	fi

# Seed the user's ~/.rote/ tree with the project's rote artifacts (the kronk
# adapter so far). See .agents/rote/NOTES.md §6 for what lives in this mirror
# and why.
#
# What we mirror: manifest.json, tools.json, agent.md, config/, toolsets/.
# What we exclude: runtime/ (per-execution scratch), index/ (Tantivy search
# index — segment UUIDs change on every reindex, so committing them creates
# binary noise on every diff). The index is rebuilt locally with
# `rote adapter reindex` immediately after the rsync, producing a fully
# usable adapter from a single make invocation.
agents-rote-seed: agents-rote-playground
	mkdir -p $$HOME/.rote/adapters
	rsync -a \
		--exclude 'runtime/' \
		--exclude 'index/' \
		--exclude '.tantivy-*.lock' \
		.agents/rote/adapters/kronk/ $$HOME/.rote/adapters/kronk/
	rote adapter reindex kronk

agents-rote-opencode:
	mkdir -p $$HOME/.config/opencode
	cp .agents/rote/opencode/opencode.jsonc $$HOME/.config/opencode/opencode.jsonc
	cp .agents/rote/opencode/tui.jsonc $$HOME/.config/opencode/tui.jsonc
	cp .agents/rote/opencode/auth.json $$HOME/.config/opencode/auth.json
	cp .agents/rote/AGENTS.md $$HOME/.config/opencode/AGENTS.md
	rm -rf $$HOME/.config/opencode/skills
	cp -r .agents/rote/skills $$HOME/.config/opencode/skills


# ==============================================================================
# Agents — Wipe
#
# Nuke every trace of every agent bundle this makefile knows how to install,
# so the next `agents-default-<host>` or `agents-rote-<host>` runs against a
# clean box. Use this when you want to verify a bundle in isolation —
# without it you'd be testing one bundle layered over leftovers from the
# other (or from a previous install of the same one), which has bitten us
# before.
#
# What this removes (regardless of which bundle put it there):
#   1. ~/.rote/                                 — workspaces, adapters,
#                                                 secrets, registry session,
#                                                 runtime caches. Per
#                                                 .agents/rote/NOTES.md
#                                                 §Update/uninstall.
#   2. The `rote` binary on PATH                — installed by
#                                                 agents-rote-install.
#   3. Every host config dir we ever write to   — opencode
#                                                 We blow away the whole
#                                                 directory (configs,
#                                                 sessions, threads, the lot)
#                                                 rather than cherry-picking
#                                                 files, so anything either
#                                                 bundle (default or rote)
#                                                 ever dropped is gone.
#
# Idempotent: every step uses `rm -f`/`rm -rf`, so re-running on an
# already-clean machine is a no-op.
agents-wipe:
	@echo "==> removing ~/.rote/"
	rm -rf $$HOME/.rote
	@echo "==> removing rote binary (if present)"
	@if command -v rote >/dev/null 2>&1; then \
		rm -f "$$(command -v rote)" && echo "removed $$(command -v rote 2>/dev/null || echo rote)"; \
	else \
		echo "rote binary not on PATH — skipping"; \
	fi
	@echo "==> removing opencode agent config"
	rm -rf $$HOME/.config/opencode
	@echo "==> done. machine is in a pre-install state; run agents-default-<host> or agents-rote-<host> to reinstall."
