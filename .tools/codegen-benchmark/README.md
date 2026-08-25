# OpenCode code-generation benchmark

This harness measures how the `/AGENT` models configured in
`zarf/kms/model_config.yaml` complete the tic-tac-toe task through OpenCode's
headless agent loop. OpenCode can write the program, run the requested Go
toolchain, observe compiler failures, and repair its work before the harness
grades the final `tictactoe/main.go`.

The harness creates a fresh nested Git repository and isolated OpenCode home for
every attempt. The repository boundary prevents OpenCode from treating the
surrounding Kronk checkout as its workspace. It installs only the repository's
`writing-go` skill, disables project configuration, plugins, MCP, web access, and
subagents, and denies filesystem access outside the attempt directory through
OpenCode's file tools. The structural tests and scripted game inputs remain hidden
from OpenCode.

Before a model's attempts begin, the harness warms it with a one-token response
to `hello model`. After all attempts for that model, it unloads the model before
moving to the next one. Warm-up work is not included in the reported OpenCode
token or timing measurements.

## Requirements

Install OpenCode and start the repository's Kronk server from another terminal:

```shell
opencode --version
make kronk-server
```

The server must use `zarf/kms/model_config.yaml`. A selected model must:

- be a key in that file;
- end in `/AGENT`; and
- be returned by the server's `GET /v1/models` endpoint.

If authentication is enabled, `KRONK_TOKEN` must authorize chat requests and
the administrative model-unload endpoint.

## Run

Run every eligible configured model once, which is the default:

```shell
make benchmark-codegen
```

Run one model or increase the number of independent attempts:

```shell
make benchmark-codegen \
    CODEGEN_MODEL=ornith-ai/Ornith-1.5-35B-Q8_0/AGENT \
    CODEGEN_ATTEMPTS=3
```

List eligible models without contacting the server:

```shell
make benchmark-codegen-list
```

The primary knobs are:

| Make variable | Default | Purpose |
| --- | --- | --- |
| `CODEGEN_HOST` | `http://localhost:11435` | Running Kronk server |
| `CODEGEN_MODEL` | `all` | One ID, comma-separated IDs, or every `/AGENT` model |
| `CODEGEN_ATTEMPTS` | `1` | Fresh attempts per model |
| `CODEGEN_STEPS` | `40` | Maximum OpenCode agent steps per attempt |
| `CODEGEN_TIMEOUT` | `30m` | Timeout per attempt |
| `CODEGEN_OUT` | timestamped directory | Output directory |

Outputs are written beneath `.tools/codegen-benchmark/output/` by default.
Each generated `tictactoe/main.go` is stored directly beneath its `attempt-XX`
directory. The attempt also preserves OpenCode configuration and state, the JSON
event stream, stderr, grader result, and aggregate Markdown report.

In the report, `Checks passed` is the aggregate score across the 20 grader checks.
`Perfect attempts` counts only attempts that passed every check; 0% there does not
mean the program failed to build or run. The separate `Buildable attempts` and
`Agent completed` columns report those outcomes.

Regenerate a report with:

```shell
make benchmark-codegen-report CODEGEN_OUT=.tools/codegen-benchmark/output/<run>
```
