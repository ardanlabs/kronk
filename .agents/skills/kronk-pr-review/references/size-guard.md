# Size guard

Intake writes `context/size.tsv` (`flag path metric est_tokens suggestion`; tokens ≈ bytes ÷ 4, a rough guide) and prints it after the scope. Read this when `size_warnings > 0` and tell the user before any agent runs — every reviewer pays for each flagged file.

| Flag        | Trigger (override)                                    | Why it matters                                           |
|-------------|-------------------------------------------------------|----------------------------------------------------------|
| `big-pr`    | > 30 code files or > 2500 changed lines               | Heavy agents run once per shard; the diff is read many times. |
| `big-diff`  | one file > 800 changed lines (`--max-file-diff`)      | Its hunks dominate every agent's reading.                |
| `huge-file` | a changed file > 2000 lines (`--max-file-lines`)      | Agents open the whole file for context, repeatedly.      |
| `noise`     | generated, vendored, or lockfile                      | Costs nothing — already skipped. Expected in Kronk: regenerated docs, BUI bundle, `go.sum`, and `gomod2nix.toml` are committed with their sources. |

Show a compact table (flag, path, metric, ~tokens) with `diff_tokens_est` from `scope.txt`, then ask:

- **Proceed** — review everything.
- **Exclude the flagged files** — re-run the preview with `--exclude <path>` for each `big-diff`/`huge-file` path and confirm. The report lists them under Open checks as "not reviewed (excluded for size)".
- **Review a file on its own** — a separate run with `--files <path>` and the same `--id`.
- **Split the PR** — suggest splits from `shards.tsv` (package groups) and stop.

When only `noise` is flagged, don't ask — say "N generated/vendored files skipped" and move on. Secret-scanning them is a separate opt-in (SKILL.md step 3).
