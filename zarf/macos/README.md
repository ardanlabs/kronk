# macOS CI runners: Metal capability on paravirtual GPUs

The macOS legs of `gpu.yml` and `macos.yml` run inside ephemeral Tart VMs managed
by [sand](https://github.com/khoi/sand) on `pachas-mac-studio`. Those VMs do not
see the real GPU, and the one they do see reports capabilities low enough that
llama.cpp takes a fallback path no kronk user ever runs. This directory holds the
two levers that change that, and the reasons to be careful with them.

## The problem

macOS guests on Apple Silicon always get a paravirtual GPU through
`ParavirtualizedGraphics.framework`. There is no passthrough, and no Tart, sand or
Virtualization.framework setting exposes a feature level — this is not a
misconfiguration, and moving off Tart/sand would not change it.

Measured in the guest:

```
ggml_metal_device_init: GPU name:   MTL0 (Apple Paravirtual device)
ggml_metal_device_init: GPU family: MTLGPUFamilyApple5 (1005)
ggml_metal_device_init: simdgroup reduction   = false
ggml_metal_device_init: simdgroup matrix mul. = false
ggml_metal_device_init: has bfloat            = false
```

llama.cpp gates on Apple 7 / Metal 3:

| flag | gate | source |
|---|---|---|
| `has_simdgroup_reduction` | `Apple7 \|\| Metal3` | `ggml-metal-device.m:731` |
| `has_simdgroup_mm` | `Apple7` | `ggml-metal-device.m:734` |
| `has_bfloat` | `Metal3 \|\| Apple6` | `ggml-metal-device.m:737` |

`has_simdgroup_reduction` alone gates `SUM`, `SUM_ROWS`, `CUMSUM`, `MEAN`,
`SOFT_MAX`, `GROUP_NORM`, `L2_NORM`, `ARGMAX`, `NORM`, `RMS_NORM`
(`:1214-1234`) and flash-attention (`:1344-1390`).

Every shipping Apple Silicon Mac is Apple 7 or better. **The Metal leg therefore
tests code paths no user hits, and skips the ones every user hits.** That is a
coverage defect, not just a slow runner.

### It may also be poisoning an upstream bug report

**Resolved — kept for the reasoning.** `sdk/kronk/tests/draft` used to skip its
acceptance assertion on Metal because acceptance was 0.00 there, and
`upstream.md` reported Metal alongside ROCm on that symptom. But `top_p`'s backend sampler graph
is `soft_max` + `cumsum` + `sum` — all three gated on `has_simdgroup_reduction`,
all three false in this VM. Metal was never instrumented at the API level; it was
inferred from the acceptance number alone.

It was re-run with capabilities raised: `draft=67 accepted=26 rate=0.39` and
`draft=126 accepted=70 rate=0.56`. The Metal row was a VM artifact and has been
withdrawn; the report covers ROCm alone, with Metal as the control. Do not
re-add a Metal skip on the 0.00 signature without instrumenting the candidate
readback first.

## The mechanism

Neither is part of Tart or sand. Nothing here requires a different VM manager.

Only the first is required — that is a measured result, not an assumption; see below.

### 1. The guest shim — the only thing that moves the family

cua's [Lume Metal capability shim](https://github.com/trycua/cua/tree/main/libs/lume/metal-capability-shim)
(MIT): a `DYLD_INSERT_LIBRARIES` dylib that raises the reported Apple GPU family
for one process. `zarf/macos/build-metal-shim.sh` builds and stages it.

Reported gains on an M1 Ultra, llama.cpp: Gemma 4 12B prompt 71.66 → 515.76 tok/s
(99.6% of bare metal), generation 3.41 → 49.67 tok/s (94.8%). MLX-LM: unchanged.

### 2. The host preference — measured irrelevant here, keep as a debugging lead

`com.apple.gpusw.ParavirtualizedGraphics ForceUnrestrictedDeviceFeatureLevel`,
which cua's writeup pairs with the shim.

**On this fleet it changes nothing about reported capabilities, in either
direction.** Both halves were tested under control on 2026-09-02, each time
confirming the preference state and that `tart run` restarted *after* the change:

| preference | shim | `Apple7` |
|---|---|---|
| set (16:54, VMs restarted 16:55:06) | off | **no** |
| absent (17:12, VMs restarted 17:13:15) | on | **yes** |

The second row is byte-identical to the run with the preference set, so the shim
is sufficient on its own and the preference is not a prerequisite.

**Do not upgrade that to "the preference does nothing."** `metal-caps.m` measures
what the device *reports*, not what it can *execute*. An unrestricted feature
level could plausibly affect whether simdgroup or bfloat kernels actually run
correctly, which no probe here touches — only real Metal work answers that. So:
leave it off for simplicity, and make it the **first thing you re-enable** if the
shimmed CI leg starts failing inside kernels rather than at capability detection.
`zarf/macos/pvg-feature-level.sh` exists for exactly that debugging round trip.

### The LaunchAgent is not involved

`~/Library/LaunchAgents/io.khoi.sand.plist` needs no edit for any of this, and
adding the key there does not work: `ForceUnrestrictedDeviceFeatureLevel` is a
CFPreferences key read by ParavirtualizedGraphics.framework, not an environment
variable, so nothing reads it from `getenv`. `EnvironmentVariables` values must
also be `<string>`, and an `<int>` there makes the whole plist fail
`plutil -lint` — which matters, because a malformed plist will not `bootstrap`
and takes both runners offline at the next reboot. Note also that
`launchctl kickstart -k` restarts the job from launchd's already-loaded
definition and never re-reads the file, so a plist edit appears to do nothing
until a `bootout`/`bootstrap` that then fails. Use `defaults write`.

Env vars set in that plist would reach `tart run` on the *host* anyway, never the
process inside the guest — so the shim's `DYLD_INSERT_LIBRARIES` does not belong
there either.

### Applying the shim

**Confirmed working on this fleet, 2026-09-02** (guest build 25G76, dylib built on
the host with CLT 26.6):

```
[LumeMetalCapabilities] Enabled for metal-caps (appleFamilyMax=1009 maxThreadgroupMemory=32768)
device   : Apple Paravirtual device
threadgroup memory : 32768 bytes
  Apple7   yes      Apple9   yes      Metal3   no
llama.cpp gates: simdgroup_reduction=true  simdgroup_mm=true  bfloat=true
```

Three things to take from that run:

- The shim logs `[LumeMetalCapabilities] Enabled for <process>` to stderr on
  activation. That is the fail-open detector — grep for it in CI, in addition to
  asserting the family, because a silently inert shim otherwise looks identical
  to a stock VM.
- `Metal3` stayed `no` as intended, so `supportsFamily:MTLGPUFamilyMetal3` remains
  a truthful "is this real hardware" gate even on a shimmed runner.
- Threadgroup memory stayed at 32768 because of the explicit override. Without it
  the guest would advertise 65536, which no shipping Apple Silicon Mac reports.

### Two traps that cost a CI run

**Inject a universal dylib, never a single arch.** `DYLD_INSERT_LIBRARIES` is
inherited by every child process, and macOS ships `/usr/bin` as **arm64e** while
Go emits plain **arm64**. Staging only the arm64 slice aborts every system binary
in the tree:

```
dyld[4185]: terminating because inserted dylib '...-arm64.dylib' could not be
loaded: (mach-o file, but is an incompatible architecture (have 'arm64',
need 'arm64e'))
```

That killed `tee` in the verify step (exit 134) and every command
`kronk diagnose` shells out to. `build-metal-shim.sh` now `lipo`s both slices
into one `LumeMetalCapabilities.dylib` and refuses to stage a file without an
arm64e slice; the CI stage step re-checks with `lipo -archs`.

**Keep `kronk diagnose` out of the shimmed environment.**
`sdk/tools/diagnose/system_darwin.go:15` parses the *combined* output of
`sysctl`, so once the arch is fixed and those binaries run, the shim's NSLog
banner lands on stderr, becomes line 0 of what the parser reads, and turns
`cpuModel` and `ramBytes` into nonsense. `gpu.yml` scopes the injection to the
`kronk devices` invocation with a shell prefix, and redirects with `>` rather
than `tee`, so no extra process joins the shimmed environment just to move
bytes. The library path is unaffected either way —
`sdk/tools/devices/system_memory_darwin.go:9` uses `unix.SysctlUint64`, a
syscall, not a shell-out.

### Host setup, once per runner host

CI now depends on this being in place; without it EVERY macOS job — in
`gpu.yml` and `macos.yml` alike — fails at setup-kronk's `Stage Metal
capability shim` step with a pointer back here.

1. Build and stage the dylib:

   ```bash
   ./build-metal-shim.sh          # → ~/ci-cache/kronk-tools/
   ```

2. Mount that directory into both runners — two lines in each runner block of
   `~/sand.yml`, beside the existing `kronk-models` mount:

   ```yaml
        - host: ~/ci-cache/kronk-tools
          name: kronk-tools
   ```

3. `sand validate --config ~/sand.yml`, then
   `launchctl kickstart -k "gui/$(id -u)/io.khoi.sand"`. The restart kills any
   in-flight job.

It surfaces in the guest at `/Volumes/My Shared Files/kronk-tools/`. This is a
mount, not a duplicated cache — the payload is one 69 KB dylib, and the model
cache stays shared. Keep the upstream checkout out of this directory; the build
script puts it in `~/.cache/kronk-metal-shim` for that reason.

### What CI does with it

**Staging and verifying live in `.github/actions/setup-kronk`, on every macOS
job.** Not in `gpu.yml`, because using Metal is a property of the platform and
not of a workflow: darwin/arm64 publishes exactly one llama.cpp artifact —
`cpu` and `metal` both resolve to `llama-<tag>-bin-macos-arm64` (yzma
`pkg/download/resolver.go:202-217`) — so the bundle always carries
`libggml-metal`, `libggml.dylib` links it at load time, and `NGpuLayers`
defaults to all-layers-on-GPU (`sdk/kronk/model/model.go:522-523`). Any macOS
job that loads a model runs Metal whatever `KRONK_PROCESSOR` says.

`macos.yml` learned that the hard way. It set `KRONK_PROCESSOR=cpu` and called
itself a CPU gate; run 33859967144 printed `MTLGPUFamilyApple5 (1005)` and
`simdgroup reduction = false` from that job, and `sdk/kronk/tests/draft`
failed there with acceptance 0.00 on every request while the shimmed Metal leg
of the same commit sat at 0.38-0.49. It is `KRONK_PROCESSOR=metal` now, and
shimmed.

- `Stage Metal capability shim` copies the dylib to `$RUNNER_TEMP` (dyld
  loading over virtiofs is a question nobody needs to answer at 3am) and
  refuses a file without an arm64e slice.
- `Verify Metal capability shim` runs **two** shimmed probes, both hard
  failures, because no single binary answers both questions:
  - `llama-bench --list-devices` (from the active bundle, per
    `kronk libs --list-installs`) answers *what ggml resolved*. It is a
    subprocess with ordinary logging, so it prints the capability banner, and
    the step requires `simdgroup reduction = true`.
  - `kronk devices` answers *whether injection reaches a Go binary* — the
    arm64e-vs-arm64 trap below — which only the shim's own NSLog banner shows.

  **Do not grep `kronk devices` for the ggml banner.** It cannot ever print
  one: `cmd/kronk/devices/cmd.go` calls `kronk.Init()` with no options, so
  `logLevel` falls to `LogSilent` and installs `llama.LogSilent()`
  (`sdk/kronk/init.go:134-141`). An earlier version of this step did exactly
  that and took its "could not confirm" warning branch on *every* run of both
  workflows — the capability assertion never once executed. An assertion that
  cannot fail is worse than none, because it reads as coverage. Found on
  2026-09-04 by reading a green log instead of trusting it.

Staging is not injecting. Each workflow decides which of its steps get the
environment, per-step and never through `$GITHUB_ENV`, so it reaches neither
checkout nor cleanup:

```yaml
        env:
          DYLD_INSERT_LIBRARIES: ${{ runner.temp }}/LumeMetalCapabilities.dylib
          LUME_METAL_APPLE_FAMILY_MAX: "1009"
          LUME_METAL_MAX_THREADGROUP_MEMORY: "32768"
```

`gpu.yml` injects on its two suite steps (matrix-conditional, since the Linux
legs must not see it) and keeps its own `Verify <backend> backend` step for the
per-matrix *device* assertion, which cannot move into the action.
`macos.yml` injects on `Run api/service tests` and `Run sdk tests`. Its
`race` job is left unshimmed on purpose: `-short` skips everything there that
loads a model.

## Order of work

1. ~~Host preference alone~~ — 2026-09-02. Applied under control; family stayed at
   Apple 5. Now removed again.
2. ~~Build and inject the shim~~ — 2026-09-02. Works, with the preference absent.
   Apple 9 reported, all three llama.cpp gates true, `Metal3` still false.
3. ~~Wire it into `gpu.yml`~~ — done. Stage step, per-step injection, and the
   two assertions in `Verify Metal backend`. Since moved into
   `setup-kronk` so `macos.yml` gets the same treatment; see above.
4. ~~Drop `metal` from `SkipOnBackends`~~ — done, in
   `sdk/kronk/tests/draft/suite_test.go`. Metal now runs the acceptance
   assertion for real.
5. ~~Confirm the capabilities reach llama.cpp in CI~~ — 2026-09-02, from the
   runner:

   ```
   ggml_metal_device_init: GPU family: MTLGPUFamilyApple9  (1009)
   ggml_metal_device_init: simdgroup reduction   = true
   ggml_metal_device_init: simdgroup matrix mul. = true
   ggml_metal_device_init: has bfloat            = true
   ggml_metal_device_init: use residency sets    = true
   ```

   The NSLog banner *does* reach stderr in a CI step, contrary to the caution
   that shaped the original asymmetric assertions — `[LumeMetalCapabilities]
   Enabled for kronk`, `for llama-cli`, `for llama-bench` all appeared. Run
   33874959831 went further and showed it firing per test binary, down to
   `Enabled for draft.test`, so it is now a hard requirement. The ggml
   capability banner, by contrast, never lands in `kronk devices` output at
   all — see the note above; that is why `llama-bench` carries that half.
6. ~~Run the Metal leg and read the result~~ — 2026-09-02, all three GPU legs
   green. `sdk/kronk/tests/draft` on Metal: `draft=67 accepted=26 rate=0.39`
   and `draft=126 accepted=70 rate=0.56`, against 0.00 on every request
   before. `DraftAcceptance` passes. The Metal rows have been withdrawn from
   the upstream report; it now covers ROCm alone, with Metal as a control. If `DraftAcceptance` passes,
   the Metal rows come out of `upstream.md` — that evidence was a paravirtual
   artifact. If it fails, instrument the candidate readback before concluding
   anything; do not widen the upstream report on a VM signature a second time.

7. ~~Ship the shim to `macos.yml`~~ — 2026-09-04, commit 322b1dd3. Staging and
   verification moved into `setup-kronk` so every macOS job gets them;
   `KRONK_PROCESSOR=metal` there now, since `cpu` never kept that gate off the
   GPU. All six runs green, and `sdk/kronk/tests/draft` on the macOS gate went
   0.00 → `rate=0.39` / `rate=0.43`, matching the GPU leg (0.48/0.49), the
   Linux CPU gate (0.38/0.47) and bare metal on an M4 Max (0.35-0.46). The
   0.00 was the paravirtual GPU and nothing else.
8. ~~Make the capability assertion real~~ — 2026-09-04. It had been grepping
   `kronk devices`, which logs silently, so it had never executed. Now two
   hard probes; see "What CI does with it".

## Guardrails

- **Assert the family in CI, and check that the assertion can fail.** The shim
  fails *open*: a missing hook or a malformed `LUME_METAL_APPLE_FAMILY_MAX`
  leaves the process unchanged, so a guest image bump silently returns you to
  the Apple 5 path with a green build. `setup-kronk`'s
  `Verify Metal capability shim` step owns this for every macOS job. When you
  touch it, confirm from a real log that the line you grep for is actually
  emitted — the first version of that step grepped a binary that runs with
  logging silenced, and passed on a warning branch for its whole life.
- **Never advertise `MTLGPUFamilyMetal3`.** Upstream is explicit that MLX-LM uses
  it to select residency sets the paravirtual device cannot create. The useful
  side effect: Metal 3 is the one probe the shim will not fool, which makes
  `supportsFamily:MTLGPUFamilyMetal3` an honest "is this real hardware" gate for
  `runner-check.yml`.
- **A reported capability is not a working one.** A red Metal leg after either
  lever is ambiguous between a paravirtual driver gap and a real kronk bug. Keep
  bare metal as the control for anything going upstream.
- **Version sensitivity.** Upstream relies on private guest Metal internals and
  says so. Re-verify after any host or `macos-runner` image bump.

## Verified environment (2026-09-02)

| | value |
|---|---|
| host | macOS 26.6.2 (25G83), M4 Max, 128 GiB |
| host toolchain | CLT 26.6.0, Apple clang 21.0.0 (clang-2100.1.1.101) |
| guest | macOS 26.6.1 (25G76), arm64, Xcode 26.6, same clang build |
| guest SIP | **disabled** — `DYLD_INSERT_LIBRARIES` is not stripped |
| guest shares | `/Volumes/My Shared Files/{kronk-models,sand-cache}` |
| VM image | `ghcr.io/cirruslabs/macos-runner:tahoe` |
| tart / sand | 2.32.1 / 1.4.0 |
| `ForceUnrestrictedDeviceFeatureLevel` | unset |

Host and guest carry the same compiler, so building on the host is what the guest
would have produced. cua's evidence-matched release used CLT 26.4, so their
`Release/SHA256SUMS` will not match a local build — expected; the build script
writes its own `PROVENANCE.txt` instead.

`system_profiler SPDisplaysDataType` returns nothing in the guest — the VMs run
`--no-graphics`, so the `Runner specs (macOS)` step in `gpu.yml` prints an empty
display section. Use `kronk devices` for GPU facts there.

## Result

The Metal leg now runs the full suite set, MTP included, on a GPU reporting
Apple 9. Measured 2026-09-02 against the Vulkan leg of the same run:

| suite | Metal | Vulkan |
|---|---|---|
| qwen3 | 33.4s | 41.8s |
| vision | 17.6s | 33.4s |
| vision_imc | 16.9s | 54.0s |
| hybrid | 41.4s | 40.6s |
| mtp | 12.6s | 23.6s |
| gemma4mtp | 10.3s | 30.5s |

Reading models over virtiofs is not a bottleneck at any size in these lists,
which is what released the 26B/35B MTP targets onto this leg.
