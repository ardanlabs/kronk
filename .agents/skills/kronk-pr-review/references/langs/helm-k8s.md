# Helm / Kubernetes / Kustomize idioms

**Tools:** `rg`; `helm template <chart> -f <values-file>` to render one environment's values when a finding depends on it. **Checks:** `helm-lint`, `helm-render` (default values → `kubeconform`, `kube-linter`), `kustomize` (build → same), `k8s-manifests`, `iac-scan`. Only default values are rendered: read the other values files for environment-specific paths. A render failure from missing chart dependencies (`charts/` not built) is an Open check. Kronk's chart is `zarf/helm/kronk`; for a maintainer PR `helm-test` also runs its `test.sh` (a failure on a changed chart is G3).

## Correctness
Immutable fields changed (Deployment/StatefulSet `spec.selector`, Service `clusterIP`, PVC size/class) — the upgrade fails; selectors vs pod labels; Service `targetPort` vs container port names; probes (liveness identical to readiness, no startup probe for slow boots); requests/limits; rollout strategy, `maxUnavailable` vs replicas and PDBs; HPA fighting fixed `replicas`; ConfigMap/Secret changes not rolling pods (checksum annotation); image tag `latest` or unpinned; deprecated `apiVersion`s; Kustomize patches targeting names that no longer exist. Templates: `nindent`/whitespace, `quote` for YAML-ambiguous strings (`on`, `yes`, numbers), `toYaml` of nil, `default` masking a required value.

## Failure paths
`required`/`fail` for mandatory values; graceful shutdown (`preStop`, `terminationGracePeriodSeconds`); Jobs: `backoffLimit`, `activeDeadlineSeconds`, `restartPolicy`; init containers that can hang; hooks with no delete policy.

## Contracts
Kronk chart contract: single replica with a retained PVC (model store), authenticated defaults, `version` bumped for any chart change, `zarf/helm/kronk/README.md` matching the values. `values.yaml` is the chart's API: renamed or removed keys break every install (update `values.schema.json`, document the migration); `Chart.yaml` `version` bumped by SemVer, `appVersion`, pinned dependencies; Service names and ports, CRD schemas, and labels that other workloads, NetworkPolicies, or monitors select on.

## Tests
helm-unittest suites, `templates/tests` hooks, chart-testing (`ct`); each Kustomize overlay rendered. Without them, name the manual check: render each environment's values and diff against the current release.

## Security
`securityContext` (`runAsNonRoot`, `readOnlyRootFilesystem`, `allowPrivilegeEscalation: false`, drop `ALL` capabilities, seccomp); `privileged`, `hostNetwork`, `hostPID`, `hostPath`; RBAC least privilege (`*` verbs or resources, `cluster-admin` bindings, secret `get/list`); `automountServiceAccountToken`; secrets in values files or ConfigMaps; missing NetworkPolicies; untrusted image registries.

## Clarity
`NOTES.txt`, chart README, and values comments match the templates.
