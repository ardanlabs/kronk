# Code-generation benchmarks

These benchmarks compare how Kronk's configured agent models perform on the
tic-tac-toe implementation task in `examples/talks/tic-tac-toe.md`.

Each operation records the effective model and sampling configuration, token
usage, reasoning usage, throughput, time to first token, speculative-decoding
metrics, model-memory estimates, raw responses, extracted source, and an
automated correctness grade. The grader builds and vets each response, checks
the required Go structure, and runs scripted win, draw, replay, invalid-input,
and score-persistence scenarios.

The Go heap delta and `B/op` metrics cover SDK-managed Go memory only. Model
size, slot memory, and total model-memory estimates come separately from GGUF
metadata because llama.cpp's native allocations are outside the Go heap.

The benchmark uses `zarf/kms/model_config.yaml` by default. Set
`KRONK_POOL_MODEL_CONFIG_FILE` to use another model config. `nseq-max` is
overridden to 1 because each benchmark performs one coding session at a time;
all other resolved settings come from the model config, auto-tuning, and GGUF
metadata.

Run one model at a time. Model-backed tests under `sdk/kronk/tests` are
deliberate human runs and can require substantial unified memory:

```shell
make benchmark-codegen-ornith
```

Available make targets:

- `benchmark-codegen-ornith`
- `benchmark-codegen-mtp-qwen36`
- `benchmark-codegen-qwen38`
- `benchmark-codegen-gemma4`
- `benchmark-codegen` runs all four sequentially.

Each model attempts the task three times by default so the final report
captures quality variance rather than a single lucky result. Override
`BENCH_CODEGEN_TIME` for a deliberate smoke run, or use
`BENCH_CODEGEN_TIMEOUT=60m` to change the per-model timeout:

```shell
make benchmark-codegen-ornith BENCH_CODEGEN_TIME=1x
```

By default, artifacts are written beneath `runs/<timestamp>/<model>/`. Set
`CODEGEN_RESULTS_DIR` to choose another location. Each iteration writes a
separate artifact folder. The combined `benchmark-codegen` target also writes
`report.md` at the root of the run directory with the aggregate scores,
performance metrics, configurations, finish reasons, and grader failures.

Generate or refresh the report for an existing run with:

```shell
make benchmark-codegen-report \
    CODEGEN_RESULTS_DIR=sdk/kronk/tests/benchmarks/runs/20260824-105806
```

The benchmark protocol tells the model that no filesystem or tools are
available and requires one complete `main.go` response. This keeps the task
executable while preserving the implementation prompt verbatim. A fully passing
attempt must build and vet cleanly, pass generated unit tests, render the exact
initial board after ANSI codes are stripped, and pass all five scripted gameplay
scenarios.
