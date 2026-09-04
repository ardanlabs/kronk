# macOS CI runner: setup and Metal capability

Self-hosted macOS runner for `macos.yml`, `gpu.yml`'s Metal leg and
`runner-check.yml`. Ephemeral Tart VMs orchestrated by
[sand](https://github.com/khoi/sand), one Apple Silicon host.

Files here:

| file | purpose |
|---|---|
| `build-metal-shim.sh` | build + stage the Metal capability shim |
| `metal-caps.m` | standalone probe: what the guest GPU reports |
| `pvg-feature-level.sh` | toggle `ForceUnrestrictedDeviceFeatureLevel` |

## 1. Host prerequisites

```bash
# Apple Silicon Mac. Current fleet: Mac Studio Mac16,9, M4 Max, 128 GiB.
sw_vers                                  # macOS 26.6+
xcode-select -p                          # CLT or Xcode required (cgo)
brew install cirruslabs/cli/tart         # 2.32.1
brew install khoi/sand/sand              # 1.4.0
```

Budget per runner: `ramGb: 48`, `cpuCores: 8`, ~520 GB sparse disk (inherited
from the image, not settable in sand 1.4.0). Two runners on a 128 GiB / 16-core
host is 96 GiB RAM and 1:1 vCPU — no host headroom at 2 runners, so do not add a
third.

Disk: each VM's sparse disk plus the shared model cache land on the same host
volume. Budget ~700 GiB before the model set is warm.

## 2. GitHub App

sand registers runners with a GitHub App private key, org-scoped.

```bash
chmod 600 ~/<app-id>-kronk-runners.<date>.private-key.pem
```

sand 1.4.0 exposes no `runnerGroup` key, so runners land in the org's `default`
group. Move them in GitHub org settings if you need them scoped.

## 3. `~/sand.yml`

Not versioned in this repo — the live copy is on the host. These are the fields
CI depends on, with the fleet's current values (derived from the running config,
2026-09-02):

```yaml
runners:
  - name: kronk-macos-metal-1
    source:
      oci: ghcr.io/cirruslabs/macos-runner:tahoe
    ramGb: 48
    cpuCores: 8
    ephemeral: true                 # config.sh --ephemeral --unattended --replace
    extraLabels: [gpu, metal]
    provisioner:
      type: github
      appId: "<app-id>"
      org: ardanlabs
      privateKey: ~/<app-id>-kronk-runners.<date>.private-key.pem
    mounts:
      - host: ~/ci-cache/kronk-models
        name: kronk-models          # shared between runners, rw
      - host: ~/ci-cache/kronk-tools
        name: kronk-tools           # the Metal shim
      - host: ~/.cache/sand/actions-runner
        name: sand-cache
    healthCheck:
      command: pgrep -fl /Users/admin/actions-runner/run.sh
      interval: 30
      startDelay: 60
  # - name: kronk-macos-metal-2   (identical, same two ci-cache mounts)
```

**Labels.** sand supplies `self-hosted, macOS, ARM64` plus `sand`; `extraLabels`
adds `gpu, metal`. That satisfies every workflow that targets this host:

```
macos.yml, runner-check.yml   [self-hosted, macOS, ARM64, sand]
gpu.yml  (Metal leg)          [self-hosted, macOS, ARM64, sand, metal]
```

An unmatched `runs-on` does not fail — it queues silently until the 24h run
limit. Retiring a runner means dropping the workflow leg in the same change.

**Mounts** surface in the guest at `/Volumes/My Shared Files/<name>` (the path is
not selectable):

| share | consumed by | missing ⇒ |
|---|---|---|
| `kronk-tools` | `setup-kronk` → `Stage Metal capability shim` | **every macOS job fails** |
| `kronk-models` | `setup-kronk` → `Persist models on ephemeral macOS VMs` | warning; re-downloads the model set per job |

**Ephemeral is the point.** The guest is cold every job: `~/.kronk`, `~/go` and
the Go build cache do not survive. That is why the model cache is a host mount
and not a runner volume.

**Cold-cache race.** Both runners share one models directory and kronk has no
cross-process download lock; `sdk/tools/models/download.go` finishes a download
with a rename. Two concurrent cold fetches of the same *missing* model can lose
that race:

```
ERROR[download-model: unable to rename proj file: rename
.../mmproj-F16.gguf .../mmproj-Qwen3.5-0.8B-Q8_0.gguf: no such file or directory]
```

Dormant while the cache is complete; re-arms whenever `.github/test-models.txt`
grows. Pre-seed `~/ci-cache/kronk-models` serially after any model-list change.

## 4. The Metal capability shim

**Required.** A macOS guest on Apple Silicon always gets a paravirtual GPU —
no passthrough, and no tart/sand/Virtualization.framework setting exposes a
feature level. Stock guest:

```
ggml_metal_device_init: GPU name:   MTL0 (Apple Paravirtual device)
ggml_metal_device_init: GPU family: MTLGPUFamilyApple5 (1005)
ggml_metal_device_init: simdgroup reduction   = false
ggml_metal_device_init: simdgroup matrix mul. = false
ggml_metal_device_init: has bfloat            = false
```

llama.cpp gates on Apple7 / Metal3:

| flag | gate | source |
|---|---|---|
| `has_simdgroup_reduction` | `Apple7 \|\| Metal3` | `ggml-metal-device.m:731` |
| `has_simdgroup_mm` | `Apple7` | `ggml-metal-device.m:734` |
| `has_bfloat` | `Metal3 \|\| Apple6` | `ggml-metal-device.m:737` |

`has_simdgroup_reduction` alone gates `SUM SUM_ROWS CUMSUM MEAN SOFT_MAX
GROUP_NORM L2_NORM ARGMAX NORM RMS_NORM` (`:1214-1234`) and flash attention
(`:1344-1390`). Every shipping Apple Silicon Mac is Apple7+, so an unshimmed
runner tests paths no user hits and skips the ones every user hits.

Fix: cua's
[Lume Metal capability shim](https://github.com/trycua/cua/tree/main/libs/lume/metal-capability-shim)
(MIT), a `DYLD_INSERT_LIBRARIES` dylib that raises the reported family per
process.

```bash
cd zarf/macos
./build-metal-shim.sh                            # clone, build, verify, stage
CUA_REF=<sha> ./build-metal-shim.sh              # pin a known-good upstream rev
STAGE_DIR=~/ci-cache/kronk-tools ./build-metal-shim.sh
```

Defaults: stages to `~/ci-cache/kronk-tools` (one 69 KB universal dylib plus a
`PROVENANCE.txt` it generates), clones into `~/.cache/kronk-metal-shim`.

Then add the `kronk-tools` mount from §3 and restart sand (§5). Keep the
upstream checkout out of `STAGE_DIR` — that whole directory is mounted into both
guests.

Shimmed guest:

```
[LumeMetalCapabilities] Enabled for kronk (appleFamilyMax=1009 maxThreadgroupMemory=32768)
ggml_metal_device_init: GPU family: MTLGPUFamilyApple9  (1009)
ggml_metal_device_init: simdgroup reduction   = true
ggml_metal_device_init: simdgroup matrix mul. = true
ggml_metal_device_init: has bfloat            = true
```

- `Metal3` stays `no`, deliberately. MLX-LM uses it to select residency sets the
  paravirtual device cannot create, and it is the one probe the shim will not
  fool — an honest "is this real hardware" gate.
- `LUME_METAL_MAX_THREADGROUP_MEMORY=32768` pins what real silicon reports. The
  shim defaults to 65536, which no Mac has.
- Guest SIP must be **disabled** or `DYLD_INSERT_LIBRARIES` is stripped. The
  `macos-runner:tahoe` image ships it disabled.

## 5. Start, restart, verify

sand reads `~/sand.yml` once at process start, so **any** edit needs a restart,
which destroys and recreates both VMs:

```bash
sand validate --config ~/sand.yml
gh run list --status in_progress                      # check first: this kills jobs
launchctl kickstart -k "gui/$(id -u)/io.khoi.sand"
```

The launchd agent (`~/Library/LaunchAgents/io.khoi.sand.plist`, KeepAlive +
RunAtLoad) supervises sand. `launchctl kickstart -k` restarts from launchd's
already-loaded definition and never re-reads the plist — a plist edit needs
`bootout`/`bootstrap`. A malformed plist will not bootstrap and takes the host
offline at the next reboot, so `plutil -lint` it first.

Verify from the host:

```bash
tart list                                             # both VMs running
sand status
```

Verify inside the guest — the shim and the GPU:

```bash
/Volumes/My\ Shared\ Files/kronk-tools/LumeMetalCapabilities.dylib   # present
lipo -archs ~/…/LumeMetalCapabilities.dylib           # must include arm64e

# what the device reports, with and without the shim
clang -fobjc-arc -framework Metal -framework Foundation zarf/macos/metal-caps.m -o /tmp/metal-caps
/tmp/metal-caps
DYLD_INSERT_LIBRARIES=/Volumes/My\ Shared\ Files/kronk-tools/LumeMetalCapabilities.dylib \
LUME_METAL_APPLE_FAMILY_MAX=1009 LUME_METAL_MAX_THREADGROUP_MEMORY=32768 \
  /tmp/metal-caps
```

Expected shimmed:

```
device   : Apple Paravirtual device
threadgroup memory : 32768 bytes
  Apple7   yes      Apple9   yes      Metal3   no
llama.cpp gates: simdgroup_reduction=true  simdgroup_mm=true  bfloat=true
```

`system_profiler SPDisplaysDataType` returns nothing in these VMs
(`--no-graphics`) — it exits 0 with no output, so even an `|| echo` fallback
never fires. Use `kronk devices` or `llama-bench --list-devices` for GPU facts.

## 6. What CI does with the shim

Staging and verification live in `.github/actions/setup-kronk`, gated on
`runner.os == 'macOS'`, so every macOS job in every workflow gets them. Metal is
a property of the platform, not of a workflow:

```
darwin/arm64 publishes ONE llama.cpp artifact
  cpu   → llama-<tag>-bin-macos-arm64.tar.gz   (yzma pkg/download/resolver.go:202-217)
  metal → llama-<tag>-bin-macos-arm64.tar.gz
→ the bundle always carries libggml-metal; libggml.dylib links it at load time
→ NGpuLayers unset ⇒ all layers on GPU       (sdk/kronk/model/model.go:522-523)
⇒ any macOS job that loads a model runs Metal, whatever KRONK_PROCESSOR says
```

`Stage Metal capability shim` copies the dylib to `$RUNNER_TEMP` (dyld loading
over virtiofs is a question nobody needs to answer at 3am) and rejects one with
no arm64e slice.

`Verify Metal capability shim` runs two probes, both hard failures, because no
single binary answers both questions:

| probe | answers | assertion |
|---|---|---|
| `llama-bench --list-devices` | what ggml resolved | `simdgroup reduction = true` present |
| `kronk devices` | does injection reach a Go binary | `[LumeMetalCapabilities] Enabled` present |

Bundle path comes from `kronk libs --local --list-installs`'s `(active)` row.

Staging is not injecting. Each workflow picks which of its own steps get the
environment — per-step, never `$GITHUB_ENV`, so it reaches neither checkout nor
cleanup:

```yaml
        env:
          DYLD_INSERT_LIBRARIES: ${{ runner.temp }}/LumeMetalCapabilities.dylib
          LUME_METAL_APPLE_FAMILY_MAX: "1009"
          LUME_METAL_MAX_THREADGROUP_MEMORY: "32768"
```

`macos.yml` injects on `Run api/service tests` and `Run sdk tests`, and sets
`KRONK_PROCESSOR: metal`. Its `race` job is left unshimmed on purpose: `-short`
loads no model there.

## 7. Traps

**Inject a universal dylib, never a single arch.** `/usr/bin` is arm64e, Go
emits plain arm64, and `DYLD_INSERT_LIBRARIES` is inherited by every child
process. A single-slice arm64 dylib aborts the whole tree:

```
dyld[4185]: terminating because inserted dylib '...-arm64.dylib' could not be
loaded: (mach-o file, but is an incompatible architecture (have 'arm64',
need 'arm64e'))
```

`build-metal-shim.sh` `lipo`s both slices and refuses to stage a file without
arm64e; the CI stage step re-checks with `lipo -archs`.

**Keep `kronk diagnose` out of the shimmed environment.**
`sdk/tools/diagnose/system_darwin.go:15` parses the *combined* output of the
commands it shells out to, so the shim's NSLog banner becomes the line it reads
as `cpuModel`. Scope injection with a shell prefix and redirect with `>`, not
`tee`, so no extra process joins the environment just to move bytes. Library
paths are unaffected — `sdk/tools/devices/system_memory_darwin.go:9` uses
`unix.SysctlUint64`, a syscall.

**Never grep `kronk devices` for the ggml banner.** It cannot print one:

```
cmd/kronk/devices/cmd.go:34   kronk.Init()          // no options
sdk/kronk/init.go:134-141     logLevel → LogSilent  // llama.LogSet(llama.LogSilent())
```

The first version of the verify step did exactly that and took its "could not
confirm" warning branch on every run for its whole life — the capability
assertion never once executed. An assertion that cannot fail is worse than none;
it reads as coverage.

## 8. Is the shim working? Read the numbers

`sdk/kronk/tests/draft` acceptance rate is the sharpest signal, because
`top_p`'s backend sampler graph is `soft_max` + `cumsum` + `sum` — all three
gated on `has_simdgroup_reduction`:

| runner state | rate |
|---|---|
| unshimmed, Apple5 | **0.00**, `DraftAcceptance` fails |
| shimmed, Apple9 | 0.39 - 0.49 |
| bare metal M4 Max, no VM (control) | 0.35 - 0.46 |

A 0.00 here was once filed upstream as a Metal defect. It was **withdrawn** — the
cause was the Apple5 gate refusing those three ops, not llama.cpp. Do not re-add
a Metal skip on a 0.00 without instrumenting the candidate readback first.

Shimming is also faster, because real kernels beat the Apple5 fallback:
`macos.yml`'s `sdk tests` job went 601s → 338s.

Suite wall-clock on a healthy shimmed leg, for comparison when one looks slow:

| suite | time |
|---|---|
| gemma4mtp | 10.3s |
| mtp | 12.6s |
| rerank | 16.2s |
| vision_imc | 16.9s |
| vision | 17.6s |
| qwen3 | 33.4s |
| hybrid | 41.4s |

Reading models over virtiofs is not a bottleneck at any size in that list, which
is what released the 26B/35B MTP targets onto this runner.

## 9. `ForceUnrestrictedDeviceFeatureLevel` — not needed, keep as a lead

`com.apple.gpusw.ParavirtualizedGraphics`, which cua's writeup pairs with the
shim. Tested under control 2026-09-02, confirming the preference state and that
`tart run` restarted after each change:

| preference | shim | `Apple7` |
|---|---|---|
| set | off | no |
| unset | on | **yes** |

Shim-only output is byte-identical to shim+preference, so the preference is not a
prerequisite. It is **not** "does nothing": `metal-caps.m` measures what the
device *reports*, not what it *executes*. Re-enable it first if a shimmed run starts failing
inside kernels rather than at capability detection:

```bash
./pvg-feature-level.sh status | enable | disable   # restarts the VMs; no sudo
```

It is a CFPreferences key read by ParavirtualizedGraphics.framework, not
`getenv`, so it does not belong in the launchd plist — and env vars set there
would reach `tart run` on the host, never the process inside the guest. Use
`defaults write`.

## 10. Guardrails

- **Check that the assertion can fail.** The shim fails *open*: a moved hook or a
  malformed `LUME_METAL_APPLE_FAMILY_MAX` leaves the process untouched, so a
  guest image bump returns you to Apple5 with a green build. When you touch the
  verify step, confirm from a real log that the line you grep for is emitted.
- **A reported capability is not a working one.** `metal-caps.m` reads what the
  device advertises. Only real Metal work proves the kernels run. Keep bare metal
  as the control for anything going upstream.
- **Version sensitivity.** The shim relies on private guest Metal internals.
  Re-verify after any host or `macos-runner` image bump.
- The attach-banner check is hard on the strength of NSLog reaching stderr in a
  CI step — observed on every run to date, per test binary down to `draft.test`.
  If a future image routes it to the unified log, that check fails on a logging
  detail rather than a capability regression.

## 11. Verified environment

| | value |
|---|---|
| host | macOS 26.6.2 (25G83), Mac Studio Mac16,9, M4 Max, 128 GiB |
| host toolchain | CLT 26.6.0, Apple clang 21.0.0 (clang-2100.1.1.101) |
| guest | macOS 26.6.1 (25G76), arm64, Xcode 26.6, same clang build |
| guest SIP | **disabled** — `DYLD_INSERT_LIBRARIES` is not stripped |
| guest shares | `/Volumes/My Shared Files/{kronk-models,kronk-tools,sand-cache}` |
| VM image | `ghcr.io/cirruslabs/macos-runner:tahoe` |
| tart / sand | 2.32.1 / 1.4.0 |
| runners | 2 × `ramGb: 48`, `cpuCores: 8`, ephemeral |
| `ForceUnrestrictedDeviceFeatureLevel` | unset |

Host and guest carry the same compiler, so a host build is what the guest would
have produced. cua's release used CLT 26.4, so their `Release/SHA256SUMS` will
not match a local build — expected; `build-metal-shim.sh` writes its own
`PROVENANCE.txt`.

Guest gaps worth knowing: passwordless sudo is on, there is no `go` (setup-go
installs it per job), and there is no `setsid`.

## 12. History

| date | commit | change |
|---|---|---|
| 2026-09-02 | — | preference alone: family stayed Apple5. Removed. |
| 2026-09-02 | — | shim alone: Apple9, all three gates true, `Metal3` false. |
| 2026-09-02 | `03613354` | shim wired into `gpu.yml`'s Metal leg |
| 2026-09-03 | `8f9b22fd` | `tests/draft` runs its acceptance assertion on Metal |
| 2026-09-04 | `322b1dd3` | staging moved to `setup-kronk`; `macos.yml` shimmed, `KRONK_PROCESSOR=metal` |
| 2026-09-04 | `ce3c421d` | capability assertion made real (`llama-bench`), attach check hard |
