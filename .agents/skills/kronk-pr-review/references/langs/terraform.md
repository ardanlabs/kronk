# Terraform / OpenTofu idioms

**Tools:** the binary named in `stack.tsv` (`terraform` or `tofu`); `rg` for references (`refs.tsv` names are addresses: `aws_s3_bucket.this`, `module.vpc`, `var.name`). **Checks:** `tf-fmt`, `tflint`, `iac-scan`; for a maintainer PR also `tf-validate` (`init -backend=false`: providers and modules, no state, no plan) and `tflint` with the repo's config and plugins — otherwise tflint runs its bundled rules only. Never run `plan`, `apply`, `destroy`, `import`, `state`, or `test` against real infrastructure — no reviewer has the state or the credentials. An init failure (private registry, credentials) is an Open check, not a finding.

## Correctness
- **Replacement risk:** a renamed or moved resource/module without a `moved` block, a `count` ↔ `for_each` switch, changed `for_each` keys, or a changed force-new argument destroys and recreates it. On stateful resources (databases, buckets, volumes, KMS keys, DNS zones) that is G4; name the address.
- `for_each`/`count` on values unknown at plan time; implicit vs explicit `depends_on`; data sources read at plan time; hardcoded account IDs, regions, AMIs; provider and `required_version` constraints vs `.terraform.lock.hcl`; per-environment `tfvars` drifting from the variable set.
- `lifecycle`: `prevent_destroy` missing on stateful resources, `ignore_changes` masking drift, `create_before_destroy` for zero-downtime replacement.

## Failure paths
Variable `validation`, `precondition`/`postcondition`, `check` blocks; `try()`/`can()`/`coalesce()` hiding a misconfiguration; `nullable` and defaults that silently apply.

## Contracts
A module's interface is its variables (type, default, `nullable`, `sensitive`, `validation`) and outputs: removed or renamed ones break callers (find them in `refs.tsv` and parent modules), and a changed type breaks `tfvars`. Backend and state configuration, workspace names, `terraform_remote_state` consumers, and provider configuration passed into modules are contracts too.

## Tests
`terraform test` (`.tftest.hcl`) with mock providers, Terratest, policy tests (OPA/conftest, Sentinel). Without them, say so and name the manual check: a `plan` reviewed by someone with access, looking for the replacements above.

## Security
IAM wildcards (`"*"` actions or resources), trust policies open to any principal, public exposure (`0.0.0.0/0` ingress, public buckets/ACLs/IPs), encryption at rest or in transit off, logging/versioning disabled, secrets in variable defaults, `tfvars`, or outputs not marked `sensitive` (state stores them in plain text), `local-exec` / `external` running commands, unpinned module sources (`?ref=` a branch).

## Clarity
README variable/output tables (terraform-docs) match the code; descriptions on every variable and output.
