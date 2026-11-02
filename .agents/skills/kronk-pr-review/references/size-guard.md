# Size guard

Intake writes `context/size.tsv` (`flag path metric est_tokens suggestion`; tokens ≈ bytes ÷ 4, a rough guide) and prints it after the scope. Read this when `size_warnings > 0` and tell the user before any agent runs — every reviewer pays for each flagged file.

| Flag             | Trigger (override)                                              | Why it matters                                                           |
|------------------|-----------------------------------------------------------------|--------------------------------------------------------------------------|
| `big-pr`         | > 30 code files or > 2500 changed lines                         | Heavy agents run once per shard; the diff is read many times.            |
| `big-review-set` | full depth: > 30 code files or > 6000 lines in `review-set.tsv` | Agents read whole files, not hunks; heavy agents run once per shard.     |
| `big-diff`       | one file > 800 changed lines (`--max-file-diff`)                | Its hunks dominate every agent's reading.                                |
| `huge-file`      | a changed file > 2000 lines (`--max-file-lines`)                | Agents open the whole file for context, repeatedly.                      |
| `noise`          | generated, vendored, lockfile, or symlink                       | Costs nothing — already skipped. Expected in Kronk: regenerated docs, BUI bundle, `go.sum`, and `gomod2nix.toml` are committed with their sources. |

Show a compact table (flag, path, metric, ~tokens) with `diff_tokens_est` (and `review_tokens_est` at full depth) from `scope.txt`, then ask:

- **Proceed** — review everything.
- **Exclude the flagged files** — re-run the preview with `--exclude <path>` for each `big-diff`/`huge-file` path and confirm. The report lists them under Open checks as "not reviewed (excluded for size)".
- **Review a file on its own** — a separate run with `--files <path>` and the same `--id`.
- **Split the PR** — suggest splits from `shards.tsv` (package groups) and stop. At full depth `shards.tsv` splits the review set, so base the split on `files.tsv` areas instead.
- **Narrow the depth** — only for `big-review-set`: re-run the preview with a smaller `--depth` (project → packages → files) and confirm. A whole-project Kronk review is ~3M tokens before agents read anything.

When only `noise` is flagged, don't ask — say "N generated/vendored files and symlinks skipped" and move on. Secret-scanning them is a separate opt-in (SKILL.md step 3).
