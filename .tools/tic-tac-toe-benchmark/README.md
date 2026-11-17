# Tic-tac-toe benchmark

Evaluates Kronk-backed models through OpenCode in two steps: build a basic game
(**37 checks**), then add undo (**48 checks**). Step 2 runs only after Step 1 passes.
Each step allows up to three submissions, with grader feedback for repairs in the
same session. Acceptance includes exact terminal formatting, not just gameplay.

## Requirements

- Go 1.27 or newer, OpenCode on `PATH`, and a Unix environment with Git, `sh`, and `stty`.
- A Kronk server using `zarf/kms/tools_model_config.yaml`. Selected models must have
  `/BENCHMARK` profiles in that file and be available from the server.
- If authentication is enabled, set `KRONK_TOKEN` with chat and model-unload access.

**Generated code runs on your machine without an OS sandbox. Use a disposable or
trusted environment.** Run the commands below from the Kronk project root.

## Run

Start the dedicated tools server, then list or evaluate the configured models:

```shell
make server-for-tools
make benchmark-tic-tac-toe-list
make benchmark-tic-tac-toe
```

Restart the server after changing its model configuration. Listing models does
not contact the server.

Select one model and override the timeout or submission budget:

```shell
make benchmark-tic-tac-toe \
    TICTACTOE_MODEL=ornith-ai/Ornith-1.5-35B-Q8_0/BENCHMARK \
    TICTACTOE_TIMEOUT=30m \
    TICTACTOE_SUBMISSIONS=2
```

To disable grader-assisted repairs:

```shell
make benchmark-tic-tac-toe TICTACTOE_FEEDBACK=false
```

| Make variable | Default | Meaning |
| --- | --- | --- |
| `TICTACTOE_HOST` | `http://localhost:11435` | Kronk server URL |
| `TICTACTOE_MODEL` | `all` | One model ID, comma-separated IDs, or all eligible models |
| `TICTACTOE_SUBMISSIONS` | `3` | Maximum submissions per step, from 1 to 3 |
| `TICTACTOE_FEEDBACK` | `true` | Allow repairs; false permits one submission per step |
| `TICTACTOE_STEPS` | `40` | Maximum OpenCode agent steps per submission |
| `TICTACTOE_TIMEOUT` | `15m` | Per-submission timeout; also applied separately to warm-up and unload |

Models are evaluated sequentially and unloaded before the next model starts.
Agent errors, context overflow, and submission timeouts fail only that model.
The tool waits for busy models to unload; unload timeouts and infrastructure
failures stop the run.

## Results

Each run is saved under `.tools/tic-tac-toe-benchmark/runs/YYYY-MM-DD/run-…/`:

- `report.txt`: one row per model, with separate outcomes for both steps.
- `<model>/attempt-XX/result.json`: grades, diagnostics, timing, and token usage.
- `<model>/attempt-XX/program/`: the submitted `main.go` and a runnable `go.mod`.
- `<model>/attempt-XX/events.jsonl` and `opencode.stderr`: agent logs.

Attempt numbers continue across both steps. Report time and `generated-toks` are
cumulative within each step; generated tokens include output and reasoning, not
input. Usage may be incomplete when OpenCode does not report it. `Not reached`
means the step was not attempted; `Agent error` submissions are not graded.

`history.txt` is maintained manually—copy the desired report into it. Runs are
Git-ignored, so back up any results you want to keep. Abrupt termination can leave
an incomplete attempt and temporary workspace.

Regenerate the latest report, or select a saved run, without invoking a model:

```shell
make benchmark-tic-tac-toe-report
make benchmark-tic-tac-toe-report \
    TICTACTOE_RUN=.tools/tic-tac-toe-benchmark/runs/YYYY-MM-DD/run-…
```

## Play a generated game

In an interactive terminal, run its saved source file (replace the placeholders):

```shell
go run .tools/tic-tac-toe-benchmark/runs/YYYY-MM-DD/run-…/<model>/attempt-XX/program/main.go
```

A failed submission may not compile. To play the reference games instead:

```shell
go run ./.tools/tic-tac-toe-benchmark/game_references/example_step1
go run ./.tools/tic-tac-toe-benchmark/game_references/example_step2
```
