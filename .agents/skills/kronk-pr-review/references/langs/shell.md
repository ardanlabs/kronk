# Shell / PowerShell / Make idioms

**Tools:** `rg`. **Checks:** `shellcheck` (sh/bash).

## Correctness
Quoting and word splitting; globbing on untrusted names; `sh` vs bash-isms for the shebang in use; `cd` without a check; temp files via `mktemp` and cleaned with `trap`; portable flags (GNU vs BSD `sed -i`, `date`, `readlink`). PowerShell: `Set-StrictMode`, `-LiteralPath`, scalar-vs-array pipeline results, `$LASTEXITCODE` after native commands. Make: `.PHONY`, recipe lines run in separate shells, `-` prefix ignoring errors.

## Failure paths
`set -euo pipefail` (or explicit checks), `$ErrorActionPreference = 'Stop'`; exit codes captured right after the command; pipeline failures; `|| true` hiding real errors.

## Contracts
Script arguments, environment variables, exit codes, and output formats other scripts or CI parse.

## Tests
bats/Pester tests if the repo has them; otherwise name the manual run.

## Security
`eval`, unquoted variables in commands, `curl | sh`, secrets in arguments (visible in `ps`) or echoed to logs, world-writable temp paths.

## Clarity
Usage header matches the real flags; comments explain non-obvious commands.
