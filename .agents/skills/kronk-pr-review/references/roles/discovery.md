# Discovery

Find what the change is built with and how the repo itself checks it, so the run adapts: which idiom files reviewers load, what the built-in checks miss, and which of the repo's own quality gates to offer the user. **Read configs, not code** — no findings, no opinions. Write nothing; your reply is the output.

**Inputs** (your prompt): skill directory, repo root, review depth, and intake's preview — the `languages=` / `ecosystems=` / `idioms=` lines, `# stack` rows (`eco unit files marker`), and `# unmapped` rows (`lang path`). Read only build, CI, lint, and package configuration: `Makefile`/`justfile`/`Taskfile*`, CI files (`.github/workflows/*`, `.gitlab-ci.yml`, `Jenkinsfile`, …), `.pre-commit-config.yaml`, lint and scanner configs (`.golangci*`, `clippy.toml`, `rustfmt.toml`, `deny.toml`, `.tflint.hcl`, `.ansible-lint`, `ruff.toml`, tool sections of `pyproject.toml`, `.hadolint.yaml`, `.yamllint*`, `.kube-linter*`, `.checkov*`, `trivy.yaml`, `.semgrep*`), manifests (`Cargo.toml`, `package.json` scripts, `pyproject.toml`, `Chart.yaml`, `kustomization.yaml`, `ansible.cfg`, `go.mod`), and at most the first 20 lines of an `# unmapped` file. Never source code, generated or vendored files, or symlinks. One batch of reads.

## Steps

1. **Stack.** Confirm the `# stack` rows. For `# unmapped` paths and marker files the scripts don't know (e.g. Pulumi, CDK, CloudFormation/SAM, Serverless, Packer, Nomad, Bazel, Nix, Earthly, Tilt, Crossplane, Argo CD/Flux, Jenkins shared libraries, Java/Kotlin/C#/Ruby/PHP/C/C++ builds), name the ecosystem and its units.
2. **Idioms.** Each ecosystem in scope maps to a file in `<skill>/references/langs/` (list the directory) or, when none fits, gets ≤ 8 terse review bullets under *Inline idioms*, one per concern: correctness, failure paths, contracts, tests, security.
3. **Repo gates.** They execute the reviewed repo's own commands, so they are only ever offered for a maintainer PR; list them either way. From the Makefile, CI, and pre-commit files: the commands the repo runs to lint, type-check, test, or scan the in-scope ecosystems. Keep those the built-in checks (below) don't cover, or that wrap the repo's own configuration (`make lint`, `npm run lint`, `just check`, `nox -s lint`, `./gradlew check -x test`, `mvn -q verify -DskipTests`).
4. **Extra checks** — propose a gate only if it is safe to run unattended:
   - read-only on the working tree: no `--fix`, `--write`, or formatter without `--check`/`--diff`;
   - no deploy, apply, push, publish, or release: no `terraform plan|apply`, `helm install|upgrade`, `kubectl apply`, cloud CLIs, `docker push`;
   - no credentials beyond the test setup; bounded runtime; scoped to the changed units where the tool allows;
   - one command, valid in both sh and PowerShell: no pipes, `&&`, globs, or inline env assignments — use the repo's wrapper target instead;
   - tests only for an ecosystem whose tests the built-in checks don't run.
   - Kronk (`AGENTS.md`): never a full repo test run or anything that needs models, native libraries, or downloads — no `make test*` target, `go test ./...`, `sdk/kronk/tests`, `sdk/bucky/tests`, `kronk model pull`, or `make install-libraries`.
5. **Missing analyzers.** For an in-scope ecosystem with no built-in check, name the standard static analyzer and its install command. Never install anything.

## Built-in checks (don't propose these again)

| Ecosystem          | Checks                                                                                              |
|--------------------|-----------------------------------------------------------------------------------------------------|
| Go                 | build, vet, `test -race`, staticcheck, golangci-lint (repo config), checklocks, errcheck, nilaway, deadcode, apidiff, `fix -diff`, `mod tidy -diff`, gofmt, gosec, govulncheck, dupl |
| Rust               | `cargo fmt --check`, clippy `-D warnings` (or `cargo check`), `cargo test`, cargo-audit, cargo-deny (`deny.toml`) |
| Terraform/OpenTofu | `fmt -check`, `init -backend=false` + `validate`, tflint (repo config)                              |
| Helm / Kustomize / K8s | `helm lint`, `helm template` / `kustomize build` → kubeconform + kube-linter                    |
| IaC (all of these, Dockerfiles) | trivy config, else checkov                                                             |
| Python             | ruff check + format, mypy (repo config), pytest, bandit, pip-audit (`requirements*.txt`)           |
| Ansible / YAML     | ansible-lint, yamllint (repo config)                                                                 |
| Dockerfile         | hadolint                                                                                             |
| GitHub Actions     | actionlint, zizmor                                                                                   |
| Shell / JS / proto | shellcheck, jscpd, buf lint + breaking (`buf.yaml`)                                                  |
| All                | gitleaks or trufflehog, semgrep (repo config)                                                        |
| Kronk              | `bui-tsc` (BUI `tsc --noEmit`), `helm-test` (`zarf/helm/kronk/test.sh`); Go tests run as `go test -race -short` with the model-backed trees excluded |

## Reply

The summary line, then both blocks, nothing after. `stack.md` ≤ 60 lines. `extra-checks.tsv` has one tab-separated row per proposed check — `name` (`[A-Za-z0-9._-]`), `dir` (repo-relative, `.` for the root), `lens` (`service-steward`, `error-tripwire`, `boundary-keeper`, `harness-map`, `security-sentinel`, `clone-hunter`, or `straight-line-pass`), `command`, `source` (`file:line`) — and is empty when you propose none.

````
discovery: ecosystems=<eco,…> idioms=<name,…> inline=<n> extra=<n> missing=<n>
===== stack.md =====
# Stack
## Ecosystems
- <eco> — units: `<dir>`, …; idioms: `langs/<name>.md` | inline; <tool or version pin worth knowing>
## Inline idioms
### <eco>
- <bullet>
## Repo gates
- `<command>` (`<file:line>`) — <what it checks>; built-in `<check>` | proposed `<name>` | not proposed: <why>
## Missing analyzers
- <eco>: `<tool>` — <install command>
===== extra-checks.tsv =====
<name>	<dir>	<lens>	<command>	<file:line>
===== end =====
````

Omit empty sections.
