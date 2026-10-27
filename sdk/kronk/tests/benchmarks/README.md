# Native inference regression benchmarks

These are SDK Go benchmarks, not server/reliability tools. They load installed
models directly, serially, without downloading models or starting a server.
Execution is deliberately opt-in (`KRONK_BENCHMARK=true`). Missing selected fixtures,
wrong taxonomy/modes, invalid responses, absent replay caching or absent aggregate
MTP acceptance fail the benchmark instead of producing a misleading speed result.

## Selected matrix

| Type | Attention | Fixture | IMC | MTP |
| --- | --- | --- | --- | --- |
| Dense | Attention-only | Qwen3-1.7B Q4_K_M | off/on | off |
| Dense | Hybrid | Qwopus3.5-4B Q4_K_M | off/on | off |
| MoE | Attention-only | Gemma4-26B-A4B Q4_K_M | off/on | off/on, shared-KV companion |
| MoE | Hybrid | Qwen3.6-35B-A3B Q2_K_XL | off/on | off/on, embedded own-KV |

This gives **12 mode configurations and 24 benchmark results per repetition**.
Purely Recurrent fixtures and Dense MTP fixtures are not qualified in this
selection. Architecture is representative, not another exhaustive dimension.
Use `make install-test-models` separately if fixtures are missing; the benchmarks
do not call it. Gemma MTP also requires its installed companion artifact.

Each configuration uses one slot, 8K context, F16 K/V, 512 logical/physical batch,
cache minimum 1 and, for MTP, NDraft 3. No projection is loaded: this is text-only.
The seed is 42, temperature zero, thinking disabled, with exactly 256 generated
tokens required. Early stopping fails, rather than appearing faster. Input token
counts differ by tokenizer, so compare a case with itself, not different models.

- **Miss:** change the complete system prefix each request; require zero cached
  tokens, including when IMC is enabled. Measures full-prefix processing plus decode.
- **Replay:** prime an exact request outside measurement and repeat it; IMC-on
  must reuse tokens, IMC-off must not. Measures cache restoration plus decode.

Native-kernel warmup, model load/unload, request construction and metric aggregation
are outside timing. `ns/op` measures a complete streaming SDK request, including
consumer work and response validation. It is not model-load latency.
`ttft-ms/op` and `decode-tok/s` use runtime usage measurements; decode throughput
is weighted by decode time, not an arithmetic average of rates. Token counts and
aggregate draft acceptance provide workload/mode diagnostics. There is no allocation
budget claim: benchmark memory statistics do not measure peak native/GPU memory.

## Establish and compare a baseline

Run deliberately on an otherwise idle machine with sufficient memory, with other
model servers/workloads stopped by their operator. Do not use race instrumentation
for performance numbers. There is no native execution in the normal unit test run.

Run from the repository root:

```shell
make benchmark-inference
```

Each invocation streams output to the terminal and a unique timestamped
`results/<UTC>.<unique>.txt` file in this benchmark directory, including stderr
and failed runs. Raw files are locally ignored by Git and are never overwritten.
After a successful run with the expected samples, `CURRENT.md` is atomically
updated with per-case median request latency, TTFT, decode throughput, token
counts and draft acceptance. It also records UTC time, revision/dirty state,
Go/host information, selection/settings and the source raw file. A default run
requires all 24 cases; a filtered run is clearly labeled as partial coverage.
Skipped, failed, malformed or incomplete runs leave `CURRENT.md` unchanged and
return nonzero. Review and commit `CURRENT.md` to maintain the team's current
numbers. The raw link is local; share/archive the referenced file separately when
others need the underlying samples.

The current snapshot is **not automatically a regression baseline**. Preserve the
raw output from an approved known-good revision, then run the candidate with the
same settings. Compare their two printed raw output paths:

```shell
# Install golang.org/x/perf/cmd/benchstat separately if not already available.
benchstat sdk/kronk/tests/benchmarks/results/<baseline>.txt \
  sdk/kronk/tests/benchmarks/results/<candidate>.txt
```

`make benchmark-inference` sets the workspace and native opt-in environment for
that command. Defaults are `INFERENCE_BENCH_TIME=5x`, `INFERENCE_BENCH_COUNT=10`
and `INFERENCE_BENCH_TIMEOUT=4h`. `INFERENCE_BENCH` selects benchmark names.
It has no installation prerequisites and does not run the ordinary test suites.

`-cpu=1` fixes Go's GOMAXPROCS, not native llama.cpp thread count or CPU/GPU
placement. Keep those settings unchanged too. Ten samples allow benchstat's
statistical comparison; five measured requests per sample reduce request noise.
The complete matrix can take hours. For an initial qualification or a targeted
comparison, select one stable case, for example:

```shell
make benchmark-inference \
  INFERENCE_BENCH='BenchmarkInferenceMatrix/Dense/Attention-only/Qwen3/IMC=true/MTP=false/Replay' \
  INFERENCE_BENCH_TIMEOUT=1h
```

Keep the benchmark source identical on baseline and candidate revisions (apply
the benchmark-only files to the older revision if necessary). Preserve each run's
Git revision/dirty diff, Go version, model/assistant hashes, native-library version
and hashes, device/backend, driver/OS, thread/offload settings, and power/thermal
conditions alongside its raw output. `-v` records resolved fixture/configuration;
Go reports its OS/architecture/CPU. Those alone are not a complete environment
fingerprint. A changed native library may be the intended comparison, but record
that fact rather than attributing its effect solely to Go code.

## Regression policy

Compare **every selected case**, not just a matrix-wide average. Require all 24
results in both full runs and a successful command exit; never compare a truncated,
failed or skipped run as though it were a baseline. Inspect token/cache/acceptance
diagnostics too: a mode failure is a failed qualification, not an optimization.

Suggested initial review threshold: a **statistically significant slowdown of at
least 5%** in request latency or TTFT, or a decrease of at least 5% in decode
throughput, flags that individual case for investigation. Benchstat reports the
differences and significance; it does not enforce a failing CI gate. Small but
consistent changes below 5% still deserve attention on sensitive paths. First run
the known-good revision twice (A/A) to establish the machine's noise floor; adjust
the threshold or sample count if its variation is too large. Alternate baseline
and candidate runs for confirmation rather than accepting a thermal/order effect.

These benchmarks do not yet cover append/history growth, media, oversubscribed
multi-slot throughput, long context or load time. They are a repeatable single-slot
text baseline, not evidence that every kind of performance regression is covered.
Do not claim regression protection until native qualification, A/A noise checking
and a known-good baseline have actually been collected.
