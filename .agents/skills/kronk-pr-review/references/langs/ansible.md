# Ansible idioms

**Tools:** `rg`; `ansible-doc <module>` if installed. **Checks:** `yamllint` (with the repo's config); `ansible-lint` (offline) for a maintainer PR only — it loads the repo's plugins and custom rules. Never run playbooks against hosts; `--syntax-check` or `--check` only for a maintainer PR and as the project interaction guide allows.

## Correctness
Idempotency: `command`/`shell` without `creates`/`removes`/`changed_when`; FQCN module names; `become` scope; handler names that `notify` no longer matches; variable precedence surprises (role defaults vs vars vs group_vars vs extra vars); `when` on bare variables or string booleans; `loop` vs `with_*`; Jinja2 templating and escaping in `.j2`; check-mode support; `serial` for rolling changes.

## Failure paths
`ignore_errors` hiding failures; wrong `failed_when`/`changed_when`; `block`/`rescue`/`always`; `retries`/`until` without a sensible bound; `any_errors_fatal` and `max_fail_percentage` on rolling plays.

## Contracts
Role `defaults/`, `vars/`, and `meta/argument_specs.yml` are the role's API: renamed variables break playbooks; inventory group names and tags used by other plays or CI.

## Tests
Molecule scenarios, `--check --diff` runs, `ansible-test` for collections; otherwise name the manual check (a check-mode run against a staging inventory).

## Security
`no_log: true` on tasks handling secrets; secrets in plain vars instead of Vault; file and directory modes/owners; `validate:` on files such as sudoers; variables interpolated into `shell`/`command`; `get_url` without `checksum`; `validate_certs: false`; broad `become_user`.

## Clarity
Task names describe the outcome; README and `argument_specs` match the defaults.
