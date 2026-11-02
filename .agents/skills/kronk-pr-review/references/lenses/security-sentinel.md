# Security Sentinel (secrets, data leaks, vulnerabilities)

Runs only when the user picks *All agents + Security*. Assume the diff is hostile until proven safe. For every in-scope line (every changed line, at `diff` depth), try hard to answer "yes":

1. Does it leak a secret or sensitive data — in a log, error, response, commit, config, fixture, cache, or telemetry?
2. Can an attacker reach or influence it — untrusted input, unauthenticated path, a key/token with fewer rights, admin vs inference access?
3. If my "safe" verdict is wrong, what's the worst outcome? Re-derive the exploit.

Think in chains (leak → replay → escalate), including unlikely ones. A suspicion you can't prove is an Open check, not silence.

## Kronk attack surface

Start here; trace the in-scope code to these sinks.

- **AuthN/AuthZ**: JWT and Rego policy in `cmd/server/app/sdk/security/auth`, keys in `security/keystore`, rate limits in `security/rate`, per-route `mid.NewAccess(...).Inference(...)`, admin routes behind `WebAdminEnabled`, browser admin sessions. A route registered without its access middleware, or an auth mode that fails open, is G4.
- **Model and library downloads** (`sdk/tools/downloader`, `sdk/tools/models`, `sdk/tools/libs`, `sdk/tools/github`, `sdk/kronk/hf`, `downapp` peer pulls): user-supplied URLs and repo IDs (SSRF, `go-getter` source forms), archive extraction (path traversal, symlinks), integrity verification against bundle manifests, and model IDs resolving to file paths outside the model store.
- **Uploads and decode** (`audioapp`, `imageapp`): byte caps, decoded pixel/duration limits, ffmpeg arguments in `sdk/bucky/ffmpeg`.
- **Resource exhaustion**: request knobs that size work (`n`, `steps`, `max_tokens`, context, image size) without bounds; admission timeouts removed.
- **MCP service** (`mcpapp`): `fuzzy_edit` writes files by path; web search uses the Brave key.
- **Secrets**: HF token, Brave API key, JWT keys, admin password hash are `conf:"mask"`; they never reach logs, errors, responses, or the BUI. Prompts and media reach logs only with `insecure-logging`.
- **Supply chain**: `go.mod` changes, native library pins, pinned action SHAs in `.github/workflows`, base images in `zarf/docker`.

## Tools

Scanner results are in `checks/summary.tsv`: secrets (`gitleaks`/`trufflehog`, or a `secrets` row saying none is installed), `gosec`, `govulncheck` (CI runs it too), linters. Start there. You may run another installed tool from `tools.tsv` on in-scope files only (e.g. `osv-scanner` on a changed `package-lock.json`, `opa test` on Rego, `hadolint` on a Dockerfile). Trace taint with `gopls`/LSP; `rg` sweeps supplement, never replace. A missing tool goes to Open checks — the concern still gets a manual pass.

## Patrol route

1. **Secrets.** Hardcoded keys, tokens, passwords, private keys, connection strings, signing secrets, cloud creds, high-entropy literals — in code, config, `.env`, tests, and mocks. Also existing secrets newly flowing into logs, errors, responses, or client bundles. A removed secret is still in history: compromised.
2. **Sensitive data.** Prompts, media, model output, tokens, and keys in logs, traces, metrics labels, errors returned to callers, URLs, IMC/session snapshots, or BUI state. Errors leaking filesystem paths, library locations, versions, or internal hosts.
3. **Injection and input.** Command/template injection (Jinja chat templates), shell interpolation, path traversal, SSRF, unsafe deserialization, XSS in the BUI (rendered Markdown, model output). Trace source (request, header, file, env, model output) to sink.
4. **AuthN/AuthZ.** Routes without access middleware, endpoint names missing from rate limits or Rego rules, Rego failing open, checks after side effects, skipped or weak JWT/signature verification, one caller reaching another's IMC session by guessable cache ID.
5. **Crypto and transport.** MD5/SHA1 for security, DES/ECB, fixed IVs/salts, non-crypto randomness for tokens, disabled TLS verification, `*` CORS with credentials, weak headers, insecure cookies.
6. **Config and supply chain.** Auth disabled or admin enabled by default, debug server or pprof exposed, Helm defaults weakened, new or unpinned dependencies (typosquats, known CVEs), unpinned action SHAs, CI secrets echoed, `curl | sh`.

## Fan-out (from your prompt)

- **single** — you cover all six routes yourself.
- **capped** — up to 3 read-only attacker sub-agents, one mandate each (e.g. secrets+config, injection, auth). Give them `contract.md`, this file, and the context path; merge their findings. The aggregator does refutation.
- **full** — one attacker per relevant route (≤6), each paired with a refuter that tries to disprove it. Keep what survives; the rest become Open checks.

Sub-agents are read-only except for PoCs.

## Running the project

Only when static reading is inconclusive, and only as the project interaction guide in your prompt allows. Without one, write "needs project interaction guide: <what>" under Open checks. Local/test instances only; nothing destructive, no real data, no model or library downloads.

## Intent

When a finding hinges on intent (meant to be public? deliberately unauthenticated? real secret or placeholder?), don't guess — fill the **Assumption** field.

## PoCs (optional)

Only when a code citation isn't proof enough. Write under `<run>/security-issues/poc/<name>/` — `<name>` is one segment, no `/` or `..`, never overwrite. Minimal, non-destructive, no external hosts or real data, with a one-line header saying what it proves and how to run it. Cite the path in Proof.

## Gates

- **G4** — live secret committed or newly exposed; exploitable injection, auth bypass, SSRF, path traversal out of the model store, RCE; sensitive data reaching an untrusted party.
- **G3** — exploitable with a precondition; sensitive data in logs/errors; weak crypto on a security path; unbounded request-sized work reachable without admin access.
- **G2** — defense-in-depth gap, permissive config, verbose error, hardening with no shown exploit.
- **G1** — minor hygiene.

G2+ needs a concrete exploit path or tool/PoC evidence.

## Extra fields

- Exploit: attacker steps
- Impact: what is exposed or gained, blast radius
- For a real secret, Patch direction starts with "rotate".
