# macOS CI: Metal capability on paravirtual GPUs

`gpu.yml` and `macos.yml` run in ephemeral Tart VMs (sand) on `pachas-mac-studio`.
A macOS guest on Apple Silicon always gets a paravirtual GPU — no passthrough, no
Tart/sand/Virtualization.framework setting exposes a feature level.

## Stock guest

```
ggml_metal_device_init: GPU name:   MTL0 (Apple Paravirtual device)
ggml_metal_device_init: GPU family: MTLGPUFamilyApple5 (1005)
ggml_metal_device_init: simdgroup reduction   = false
ggml_metal_device_init: simdgroup matrix mul. = false
ggml_metal_device_init: has bfloat            = false
```

| flag | gate | source |
|---|---|---|
| `has_simdgroup_reduction` | `Apple7 \|\| Metal3` | `ggml-metal-device.m:731` |
| `has_simdgroup_mm` | `Apple7` | `ggml-metal-device.m:734` |
| `has_bfloat` | `Metal3 \|\| Apple6` | `ggml-metal-device.m:737` |

`has_simdgroup_reduction` gates `SUM SUM_ROWS CUMSUM MEAN SOFT_MAX GROUP_NORM
L2_NORM ARGMAX NORM RMS_NORM` (`:1214-1234`) and flash attention (`:1344-1390`).
Every shipping Apple Silicon Mac is Apple7+, so an unshimmed leg tests paths no
user hits and skips the ones every user hits.

## Shimmed guest

cua's [Lume Metal capability shim](https://github.com/trycua/cua/tree/main/libs/lume/metal-capability-shim)
(MIT) — a `DYLD_INSERT_LIBRARIES` dylib. `build-metal-shim.sh` builds and stages it.

```
[LumeMetalCapabilities] Enabled for kronk (appleFamilyMax=1009 maxThreadgroupMemory=32768)
ggml_metal_device_init: GPU family: MTLGPUFamilyApple9  (1009)
ggml_metal_device_init: simdgroup reduction   = true
ggml_metal_device_init: simdgroup matrix mul. = true
ggml_metal_device_init: has bfloat            = true
```

- `Metal3` stays `no`. Keep it: MLX-LM uses it to select residency sets the
  paravirtual device cannot create, and it is the one probe the shim will not
  fool — an honest "is this real hardware" gate.
- `LUME_METAL_MAX_THREADGROUP_MEMORY=32768` pins what real silicon reports. The
  shim defaults to 65536, which no Mac has.

## Host setup, once per host

```bash
./build-metal-shim.sh                 # → ~/ci-cache/kronk-tools/ (69 KB dylib)
```

```yaml
# ~/sand.yml, each runner block, beside the kronk-models mount
     - host: ~/ci-cache/kronk-tools
       name: kronk-tools
```

```bash
sand validate --config ~/sand.yml
launchctl kickstart -k "gui/$(id -u)/io.khoi.sand"   # kills any in-flight job
```

Surfaces at `/Volumes/My Shared Files/kronk-tools/`. Without it **every** macOS
job fails at `setup-kronk`'s `Stage Metal capability shim`. Keep the upstream
checkout out of that dir; the build script uses `~/.cache/kronk-metal-shim`.

## What CI does

Staging and verification live in `.github/actions/setup-kronk`, `runner.os == 'macOS'`,
so every macOS job in every workflow gets them. Metal is a property of the
platform, not of a workflow:

```
darwin/arm64 publishes ONE llama.cpp artifact
  cpu   → llama-<tag>-bin-macos-arm64.tar.gz   (yzma pkg/download/resolver.go:202-217)
  metal → llama-<tag>-bin-macos-arm64.tar.gz
→ the bundle always carries libggml-metal; libggml.dylib links it at load time
→ NGpuLayers unset ⇒ all layers on GPU       (sdk/kronk/model/model.go:522-523)
⇒ any macOS job that loads a model runs Metal, whatever KRONK_PROCESSOR says
```

`Stage Metal capability shim` — copies to `$RUNNER_TEMP` (dyld over virtiofs is a
question nobody needs at 3am), rejects a dylib with no arm64e slice.

`Verify Metal capability shim` — two probes, both hard failures, because no one
binary answers both questions:

| probe | answers | assertion |
|---|---|---|
| `llama-bench --list-devices` | what ggml resolved | `simdgroup reduction = true` present |
| `kronk devices` | does injection reach a Go binary | `[LumeMetalCapabilities] Enabled` present |

Bundle path comes from `kronk libs --local --list-installs`'s `(active)` row.

Staging is not injecting. Each workflow picks its own steps, per-step, never
`$GITHUB_ENV`, so it reaches neither checkout nor cleanup:

```yaml
        env:
          DYLD_INSERT_LIBRARIES: ${{ runner.temp }}/LumeMetalCapabilities.dylib
          LUME_METAL_APPLE_FAMILY_MAX: "1009"
          LUME_METAL_MAX_THREADGROUP_MEMORY: "32768"
```

- `gpu.yml` — both suite steps, matrix-conditional (Linux legs must not see it);
  keeps `Verify <backend> backend` for the per-matrix *device* assertion.
- `macos.yml` — `Run api/service tests`, `Run sdk tests`. `KRONK_PROCESSOR: metal`.
  The `race` job is unshimmed on purpose: `-short` loads no model.

## Traps

**Inject a universal dylib.** `/usr/bin` is arm64e, Go emits arm64. A single-slice
arm64 dylib aborts every system binary in the tree:

```
dyld[4185]: terminating because inserted dylib '...-arm64.dylib' could not be
loaded: (mach-o file, but is an incompatible architecture (have 'arm64',
need 'arm64e'))
```

`build-metal-shim.sh` `lipo`s both slices; the stage step re-checks `lipo -archs`.

**Keep `kronk diagnose` unshimmed.** `sdk/tools/diagnose/system_darwin.go:15`
parses the *combined* output of its shell-outs, so the shim's NSLog banner becomes
the line it reads as `cpuModel`. Scope injection with a shell prefix and redirect
with `>`, not `tee`, so no extra process joins the environment to move bytes.
Library paths are unaffected — `sdk/tools/devices/system_memory_darwin.go:9` uses
`unix.SysctlUint64`, a syscall.

**Never grep `kronk devices` for the ggml banner.** It cannot print one:

```
cmd/kronk/devices/cmd.go:34   kronk.Init()          // no options
sdk/kronk/init.go:134-141     logLevel → LogSilent  // llama.LogSet(llama.LogSilent())
```

The first version of the verify step did exactly that, and took its
"could not confirm" warning branch on **every** run of both workflows from
`03613354` to `ce3c421d` — the capability assertion never once executed. An
assertion that cannot fail is worse than none; it reads as coverage.

## Evidence

`sdk/kronk/tests/draft` acceptance, classic separate-GGUF drafter, same commit:

| where | rate |
|---|---|
| macOS gate, unshimmed Apple5 (run 33859967144) | **0.00 / 0.00**, red |
| macOS gate, shimmed (322b1dd3) | 0.39 / 0.43 |
| gpu.yml Metal leg, shimmed | 0.48 / 0.49 |
| gpu.yml Vulkan leg | 0.36 - 0.39 |
| linux.yml CPU gate | 0.38 / 0.47 |
| bare metal, M4 Max, no VM | 0.35 - 0.46 |

Shimming also made the gate cheaper — real kernels beat the Apple5 fallback:
`macos.yml` `sdk tests` 601s → 338s.

`upstream.md` originally reported Metal beside ROCm on the 0.00 signature.
**Withdrawn**: it was `has_simdgroup_reduction=false` refusing `SOFT_MAX`,
`CUMSUM` and `SUM` — every op in `top_p`'s backend sampler graph — not
`llama_set_sampler`. Metal is the report's control now. Do not re-add a Metal
skip on a 0.00 without instrumenting the candidate readback first.

## `ForceUnrestrictedDeviceFeatureLevel` — measured irrelevant, keep as a lead

`com.apple.gpusw.ParavirtualizedGraphics`, which cua's writeup pairs with the shim.
Tested under control 2026-09-02, confirming preference state and that `tart run`
restarted after each change:

| preference | shim | `Apple7` |
|---|---|---|
| set | off | no |
| unset | on | **yes** |

Shim-only output is byte-identical to shim+preference, so the preference is not a
prerequisite. It is **not** "does nothing" — `metal-caps.m` measures what the
device *reports*, not what it *executes*. Re-enable it first
(`pvg-feature-level.sh`) if a shimmed leg starts failing inside kernels rather
than at capability detection.

Not a launchd concern either: it is a CFPreferences key read by
ParavirtualizedGraphics.framework, not `getenv`. `EnvironmentVariables` needs
`<string>` values, an `<int>` fails `plutil -lint`, a malformed plist will not
`bootstrap` and takes both runners offline at the next reboot. `launchctl
kickstart -k` never re-reads the file. Use `defaults write`.

## Guardrails

- **Check that the assertion can fail.** The shim fails *open*: a moved hook or a
  malformed `LUME_METAL_APPLE_FAMILY_MAX` leaves the process untouched, so a guest
  image bump returns you to Apple5 with a green build. When you touch the verify
  step, confirm from a real log that the line you grep for is emitted.
- **A reported capability is not a working one.** A red Metal leg is ambiguous
  between a paravirtual driver gap and a kronk bug. Keep bare metal as the control
  for anything going upstream.
- **Version sensitivity.** The shim relies on private guest Metal internals.
  Re-verify after any host or `macos-runner` image bump.
- The attach-banner check is hard on the strength of NSLog reaching stderr in a CI
  step (observed on every run to date, per test binary down to `draft.test`). If a
  future image routes it to the unified log, that check fails on a logging detail.

## Verified environment

| | value |
|---|---|
| host | macOS 26.6.2 (25G83), M4 Max, 128 GiB |
| host toolchain | CLT 26.6.0, Apple clang 21.0.0 (clang-2100.1.1.101) |
| guest | macOS 26.6.1 (25G76), arm64, Xcode 26.6, same clang build |
| guest SIP | **disabled** — `DYLD_INSERT_LIBRARIES` is not stripped |
| guest shares | `/Volumes/My Shared Files/{kronk-models,kronk-tools,sand-cache}` |
| VM image | `ghcr.io/cirruslabs/macos-runner:tahoe` |
| tart / sand | 2.32.1 / 1.4.0 |
| `ForceUnrestrictedDeviceFeatureLevel` | unset |

Host and guest carry the same compiler, so a host build is what the guest would
have produced. cua's release used CLT 26.4, so `Release/SHA256SUMS` will not match
a local build — expected; the script writes its own `PROVENANCE.txt`.

`system_profiler SPDisplaysDataType` returns nothing in these VMs (`--no-graphics`).
Use `kronk devices` for GPU facts.

## Suite timings, Metal vs Vulkan (2026-09-02, same run)

| suite | Metal | Vulkan |
|---|---|---|
| qwen3 | 33.4s | 41.8s |
| vision | 17.6s | 33.4s |
| vision_imc | 16.9s | 54.0s |
| hybrid | 41.4s | 40.6s |
| mtp | 12.6s | 23.6s |
| gemma4mtp | 10.3s | 30.5s |
| rerank | 16.2s | **3.8s** |

virtiofs is not a bottleneck at any size here, which is what released the 26B/35B
MTP targets onto this leg. `rerank` is the one suite Metal loses, and by enough to
be worth a look if it ever matters.

## History

| date | commit | change |
|---|---|---|
| 2026-09-02 | — | preference alone: family stayed Apple5. Removed. |
| 2026-09-02 | — | shim alone: Apple9, all three gates true, `Metal3` false. |
| 2026-09-02 | `03613354` | wired into `gpu.yml` |
| 2026-09-03 | `8f9b22fd` | dropped `metal` from `SkipOnBackends` in `tests/draft` |
| 2026-09-04 | `322b1dd3` | staging moved to `setup-kronk`; `macos.yml` shimmed, `KRONK_PROCESSOR=metal` |
| 2026-09-04 | `ce3c421d` | capability assertion made real (`llama-bench`), attach check hard |
