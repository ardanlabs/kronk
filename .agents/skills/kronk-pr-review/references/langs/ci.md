# CI pipeline idioms

Covers GitHub Actions, GitLab CI, Jenkins, CircleCI, Azure Pipelines, Bitbucket, Buildkite, Tekton, Drone, Travis, Cloud Build, Woodpecker — `stack.tsv` names the system in the marker column. Apply the general rules below in that system's syntax.

**Tools:** `rg`. **Checks:** `actionlint` and `zizmor` for GitHub Actions; other systems have no offline linter — review by reading (repo-defined validators arrive as extra checks when approved, for a maintainer PR only). Never trigger a pipeline.

## Correctness
Triggers run where intended (PRs, forks, tags, branches, path filters, `rules`/`only`/`except`, `when`); job ordering (`needs`, stages, `dependsOn`); matrix coverage; cache keys that miss inputs or poison builds; artifact names and paths between jobs; concurrency groups and cancel-in-progress; `if:` expressions (string vs boolean, `success()` vs `always()`); shell steps quoting and `set -euo pipefail`; reusable workflow / template inputs.

## Failure paths
`continue-on-error`, `allow_failure`, `|| true`, `catchError` hiding failures; retries masking flakes; missing timeouts; post/cleanup steps that can't fail the build when they should.

## Contracts
Job and workflow names used as required status checks (renaming breaks branch protection); reusable workflow / template / shared-library inputs, outputs, and secrets; artifact and image names consumed downstream.

## Tests
No automated path: say so and name the manual check (a run on a test branch, `act`, `gitlab-ci-local`, Jenkins replay).

## Security
- GitHub Actions: `pull_request_target` or `workflow_run` checking out PR code (pwn request); `${{ github.event.* }}` interpolated into `run:` (script injection); third-party actions not pinned to a full SHA; `permissions:` missing or broader than needed (defaults to write); secrets reachable from fork PRs; OIDC trust conditions too broad; self-hosted runners on public repos; cache poisoning across trust levels.
- GitLab/Jenkins/others: unprotected variables exposed to MR/fork pipelines; credentials echoed or interpolated into `sh`; Groovy `sh "${params.X}"` injection; script approvals and sandbox escapes; agents/runners shared across trust levels.

## Clarity
Step names describe what they do; comments on non-obvious conditions.
