# Kronk on Kubernetes

This chart runs the published Kronk image as **one server with retained storage**.
Use independent releases for separate servers; replicated administrative state,
model placement, and shared quotas are not provided.

## Install

Requirements: Kubernetes 1.27+, Helm 3, kubectl, a default StorageClass (or an
existing PVC), and enough RAM/disk for your models. Initial startup needs access
to the image registry, model provider, and any native libraries not bundled in
the selected release. GPU drivers, device plugins, and runtimes are cluster
prerequisites, not installed by the chart.

From a repository checkout:

```shell
helm upgrade --install kronk zarf/helm/kronk \
  -n kronk --create-namespace \
  -f zarf/helm/kronk/examples/cpu.yaml --wait --timeout 20m
helm test kronk -n kronk
kubectl -n kronk port-forward svc/kronk 11435:11435
```

In another terminal:

```shell
KRONK_TOKEN=$(kubectl -n kronk exec kronk-0 -c kronk -- cat /kronk/keys/master.jwt)
curl -fsS http://localhost:11435/v1/chat/completions \
  -H "Authorization: Bearer $KRONK_TOKEN" -H "Content-Type: application/json" \
  -d '{"model":"unsloth/Qwen3-0.6B-Q8_0","messages":[{"role":"user","content":"Hello!"}]}'
unset KRONK_TOKEN
```

Use `examples/cuda.yaml` instead for an NVIDIA device-plugin deployment. The
example requests one GPU; add `runtimeClassName: nvidia` only when your cluster
defines that RuntimeClass. Other accelerators require the appropriate device
plugin and device access configuration. A different image tag alone does not
allocate hardware. Validate inference on the actual GPU nodes before rollout.

## Configuration and security

[`values.yaml`](values.yaml) documents the supported controls. Keep overrides in
a values file, reuse it for upgrades, and size resource requests/limits for the
weights, KV cache, context, and concurrency. `image.tag` must match `image.variant`;
the default tag comes from the chart's published `appVersion`, not repository HEAD.

- `auth.mode` defaults to `authenticated`. `open` exposes management as well as
  inference and is only suitable for a trusted isolated environment.
- The BUI is disabled. To enable it, set `env.KRONK_WEB_ADMIN_ENABLED: true` and
  configure `KRONK_WEB_ADMIN_PASSWORD_SHA_256` using `extraEnv.valueFrom` before
  exposure. See the manual's security chapter for token and BUI password policies.
- Do not put credentials in Helm values. Use `extraEnv` with `secretKeyRef` for
  `KRONK_HF_TOKEN`, for example. These entries reach the model-pull init container
  and the server. Chart-owned paths, ports, processor, and auth mode are reserved.
- Configure TLS and access restrictions before enabling public ingress or a
  LoadBalancer. ClusterIP is the default; it is not a network-isolation policy.
- `metrics.enabled` creates a separate **ClusterIP** Service even if the API is
  public. It exposes unauthenticated pprof as well as metrics. Restrict it using
  cluster network policy. ServiceMonitor requires the Prometheus Operator CRDs.

### Existing keys and model configuration

Without `auth.existingSecret`, Kronk generates keys on first startup and keeps
them on the PVC. Back them up. To use an existing keyset, create a Secret from a
trusted installation's key files, in the release namespace:

```shell
kubectl -n kronk create secret generic kronk-keys --from-file=/secure/backup/keys/
```

Set `auth.existingSecret: kronk-keys`. It must contain `master.pem` and
`master.jwt` and may contain additional PEM signing keys. **This Secret owns the
entire signing keyset.** Each pod initialization replaces the PVC's PEM/JWT files
with it, including deletions. Back up/export existing keys before enabling this
setting. API-created keys not added to the Secret are discarded on restart.
Revoking a key means removing it from the Secret and restarting the pod; do not
leave a revoked key in the Secret, where it would be restored.

`modelConfig.enabled` mounts either `modelConfig.content` or an existing ConfigMap
with a `model_config.yaml` key. The file is read-only. Chart-managed content
changes trigger a rollout. Existing Secret/ConfigMap changes require:

```shell
kubectl -n kronk rollout restart statefulset/kronk
kubectl -n kronk rollout status statefulset/kronk --timeout=20m
```

## Storage, upgrade, and uninstall

The PVC at `/kronk` holds models, catalogs, keys, and Badger state. An init container
copies the running image's native libraries on every pod creation, including
architecture changes and rollback; it does not overwrite an existing model config.
`models.pull` rechecks requested models at startup using Kronk's existing download
validation rather than trusting independent marker files.

`persistence.storageClass: ""` uses the default class; `"-"` requests no class.
Use `persistence.existingClaim` for pre-provisioned storage. The storage driver
must support UID/GID/fsGroup 10001. Network filesystems with incompatible locking
or permission semantics are not a workaround for sharing one Badger database.

Repeat the install command with the same values for upgrades; `helm rollback
kronk -n kronk` restores the previous revision. If a bad revision's pod prevents
StatefulSet recovery, correct/roll back the values then delete that failed pod.
Neither Helm rollback nor uninstall rolls back data or restores backups.

Claim-template size, class, access mode, and switching between generated/existing
claims are **creation-time** choices. For expansion, use a StorageClass that
supports it, expand the actual PVC, and leave `persistence.size` unchanged for
that release. Class/access-mode changes require a deliberate storage migration.

```shell
helm uninstall kronk -n kronk
kubectl -n kronk get pvc
```

Uninstall retains the PVC. Delete a claim explicitly only after backup and review:
`kubectl -n kronk delete pvc <claim-name>` destroys its models and keys according
to the storage provider's reclaim policy.

## Validation

```shell
# Helm plus Python 3/PyYAML; no cluster required.
zarf/helm/kronk/test.sh

# Installs into a temporary namespace and deletes its test data on exit.
# Requires an explicitly named disposable kind cluster, Helm, kubectl, and jq.
zarf/helm/kronk/smoke-test.sh kind-kronk-test
```

The smoke test checks restricted Pod Security admission, Helm connectivity,
authenticated chat, unauthenticated rejection, PVC reuse, upgrade, rollback, and
Secret key revocation. It never targets the ambient context. Run these checks
manually when changing the chart; no GitHub Actions workflow is required. The CPU
smoke test uses a disposable kind cluster; GPU execution requires a separate
real-node validation.
