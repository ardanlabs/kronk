# Kronk on Kubernetes

This chart runs the published Kronk image as **one server with retained storage**.
Use independent releases for separate servers; replicated administrative state,
model placement, and shared quotas are not provided.

## Before you install

This installs Kronk into an **existing Kubernetes cluster**. It does not create
the cluster, install a storage provider, or configure GPU nodes. If you do not
have a working cluster, arrange one before continuing.

Requirements: Kubernetes 1.27+, Helm 3, kubectl, a default StorageClass (or an
existing PVC), and enough RAM/disk for your models. Initial startup needs access
to the image registry, model provider, and any native libraries not bundled in
the selected release. GPU drivers, device plugins, and runtimes are cluster
prerequisites, not installed by the chart.

### Helm terms used here

| Term | Meaning in this guide |
| --- | --- |
| Chart | Our Kubernetes deployment templates and defaults in `zarf/helm/kronk` |
| Values file | Your configuration overrides, such as `my-kronk.yaml` |
| Release | One named installation of the chart; we use `kronk` |
| Namespace | The cluster compartment containing the installation; we also use `kronk` |
| Context | The cluster and credentials selected by kubectl and Helm |
| PVC | A PersistentVolumeClaim: the storage request that keeps Kronk's data across pod replacements |

Check your tools and target cluster before installing:

```shell
helm version --short
kubectl version --client
kubectl config current-context
kubectl get nodes
kubectl get storageclass
```

**These commands target the selected cluster, which may not be your local
machine.** Confirm the context is the one you intend to change. Nodes should be
Ready. This walkthrough assumes a default StorageClass; the chart requests a
`50Gi` data claim. If no default class is available, ask your administrator for
a class to set in `persistence.storageClass` or a PVC to use through
`persistence.existingClaim`. If you cannot inspect nodes or storage because of
RBAC permissions, ask your administrator to confirm these prerequisites and
your installation permissions.

## Install and send your first request

You do not need to edit the chart or its `values.yaml`. We provide the chart
(the Kubernetes deployment templates and defaults) and example settings. You
provide a separate **Helm values file** for your deployment. Helm combines your
settings with the chart defaults; unspecified settings keep their defaults.

From a repository checkout, copy the CPU example to your own file:

```shell
cp zarf/helm/kronk/examples/cpu.yaml my-kronk.yaml
```

Edit `my-kronk.yaml` if needed, then install using that file:

```shell
kubectl config current-context
helm upgrade --install kronk zarf/helm/kronk \
  -n kronk --create-namespace \
  -f my-kronk.yaml --wait --timeout 20m
helm test kronk -n kronk
```

Here, `kronk` is the release name, `zarf/helm/kronk` is the local chart directory,
`-n kronk` selects the namespace, and `--create-namespace` creates it if needed.
`-f` supplies your settings. `upgrade --install` installs a missing release or
updates an existing one; `--wait --timeout 20m` waits for startup. The CPU example
downloads a small starter model and requests 2 CPUs and 4Gi RAM, with an 8Gi RAM
limit. Downloads can take several minutes; see [Startup and troubleshooting](#startup-and-troubleshooting)
if the command times out. A timeout does not necessarily stop the workload.
`helm test` checks HTTP connectivity, not model inference; the chat request below
checks inference.

Forward the API to your machine and leave this terminal running:

```shell
kubectl -n kronk port-forward svc/kronk 11435:11435
```

Port-forwarding is temporary local access, not a public endpoint. Ctrl-C stops
the forwarding, not Kronk. The chart enables authentication and disables the BUI
by default. In another terminal, retrieve the admin token and send a request:

```shell
set +x
KRONK_TOKEN=$(kubectl -n kronk exec kronk-0 -c kronk -- cat /kronk/keys/master.jwt)
curl -fsS http://localhost:11435/v1/chat/completions \
  -H "Authorization: Bearer $KRONK_TOKEN" -H "Content-Type: application/json" \
  -d '{"model":"unsloth/Qwen3-0.6B-Q8_0","messages":[{"role":"user","content":"Hello!"}]}'
unset KRONK_TOKEN
```

A successful response contains generated text in `choices[0].message.content`.
Keep the admin token private: do not echo it, enable shell tracing or verbose
curl output, or put it into Helm values or logs.

## Startup and troubleshooting

Check installation status, pods, storage, and recent events:

```shell
helm status kronk -n kronk
kubectl -n kronk get pods,pvc
kubectl -n kronk describe pod kronk-0
kubectl -n kronk get events --sort-by=.metadata.creationTimestamp
```

| Symptom | What to check |
| --- | --- |
| Pod `Pending` | Pod events for insufficient CPU/RAM, unavailable requested GPUs, or an unbound PVC; inspect the PVC with `kubectl -n kronk describe pvc` |
| `ImagePullBackOff` | Pod events for a missing image/tag, registry access, or network problems |
| `Init:…` / init container still running | The seed or model-pull logs; a first model download can take time and requires provider access |
| `CrashLoopBackOff` | Logs for the failing container, including its previous attempt, for configuration, library, memory, or permission errors |
| Local curl cannot connect | Keep port-forward running and check that the pod is Ready; if local port 11435 is occupied, forward `11436:11435` and use port 11436 in curl |
| HTTP 401 | Use the bearer token from this installation and keep authentication enabled |

For the CPU example, inspect the relevant container logs:

```shell
kubectl -n kronk logs kronk-0 -c seed
kubectl -n kronk logs kronk-0 -c pull-models -f
kubectl -n kronk logs kronk-0 -c kronk --tail=100
# If the server has restarted:
kubectl -n kronk logs kronk-0 -c kronk --previous --tail=100
```

The server cannot start until init containers finish; server logs may be
unavailable during a model download. Ctrl-C exits log following without stopping
the download. Do not uninstall or delete storage just to retry a slow download.

## Choose CPU or a GPU backend

Choose for the hardware on your **Kubernetes nodes**, not your laptop. If you
are unsure, start with CPU. A GPU install has two separate requirements: your
cluster administrator makes the GPU available to workloads, and your values
file selects a compatible image and requests the GPU.

| Node hardware | `image.variant` | What must already be configured in the cluster |
| --- | --- | --- |
| No GPU / initial setup | `cpu` (default) | Enough CPU and RAM; no GPU setup required |
| NVIDIA GPU | `cuda` | Compatible NVIDIA drivers, container runtime/toolkit, and NVIDIA device plugin |
| AMD GPU supported by ROCm | `rocm` | Compatible AMD drivers/runtime and device plugin; this image is amd64-only |
| GPU supported by Vulkan, such as AMD or Intel | `vulkan` | Compatible Vulkan drivers and workload access to GPU devices; setup depends on the cluster |
| Multiple backends bundled | `all` | The same driver and device-access prerequisites for the backend used; this does not automatically provision GPUs |

For an NVIDIA deployment, start with the CUDA example **instead of** the CPU
example when creating your file:

```shell
cp zarf/helm/kronk/examples/cuda.yaml my-kronk.yaml
```

It selects `image.variant: cuda`, requests one GPU with
`resources.limits.nvidia.com/gpu: 1`, and selects amd64 nodes. Edit the copy to
match your cluster; add `runtimeClassName: nvidia` only if your administrator
provides that RuntimeClass. Then use the same install command above with
`-f my-kronk.yaml`. Selecting an image alone does not install drivers or allocate
a GPU. If the cluster prerequisites are missing, ask your administrator to set
them up or use CPU. Validate GPU inference on the actual nodes before rollout;
CPU smoke tests do not validate CUDA, ROCm, or Vulkan.

Keep your values file outside the chart directory and reuse it on every upgrade.
Editing it does not update a running installation: rerun `helm upgrade --install`
with `-f my-kronk.yaml` to apply changes. This deployment file is **not** Kronk's
`model_config.yaml`; model settings can be supplied separately through
`modelConfig` as described below.

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

### Apply changes and upgrade

Back up important data and keys before maintenance. Keep your values file and
use it every time you apply changes:

```shell
helm upgrade --install kronk zarf/helm/kronk \
  -n kronk --create-namespace \
  -f my-kronk.yaml --wait --timeout 20m
helm test kronk -n kronk
helm history kronk -n kronk
```

Helm does not automatically update Kronk when a new image is published. The
chart's `version` identifies the deployment templates; its `appVersion` supplies
the default Kronk image version. Use a reviewed chart checkout for updates. To
select another already-published image explicitly, set `image.tag` in your values
file to its full tag, including the backend suffix, and match `image.variant`.
The tag override persists until you change or remove it. Do not assume rerunning
an unchanged command selects the newest image.

Updates that recreate this single-server pod interrupt service; this is not a
zero-downtime deployment. `helm rollback kronk -n kronk` restores the previous
release revision. If a bad revision's pod prevents
StatefulSet recovery, correct/roll back the values then delete that failed pod.
Neither Helm rollback nor uninstall rolls back data or restores backups.

Claim-template size, class, access mode, and switching between generated/existing
claims are **creation-time** choices. For expansion, use a StorageClass that
supports it, expand the actual PVC, and leave `persistence.size` unchanged for
that release. Class/access-mode changes require a deliberate storage migration.

### Uninstall without deleting data

```shell
helm uninstall kronk -n kronk
kubectl -n kronk get pvc
```

Uninstall retains the PVC. **Deleting the `kronk` namespace also deletes its PVCs;
it is not a data-preserving cleanup step.** Back up first, and leave the namespace
and claim in place if you want to reuse this installation's data.

Delete a claim explicitly only after backup and review:
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
