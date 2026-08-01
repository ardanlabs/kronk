## Self-Hosted Inference in Go: No Python, No CGO, No Network Hop

### About this Session

Self-hosted inference — running models on hardware you control — means no per-token costs, no data leaving your environment, no vendor lock-in, and access to the long tail of open-source models that go well beyond the LLMs everyone is talking about. And contrary to popular belief, you don't need a GPU rack: small models like `Qwen3.5-0.8B-Q8_0` run comfortably on the same laptop you're using right now. The hard part has been doing it from Go without CGO, Python, or a network hop to something like Ollama.

In this talk, Bill will show why self-hosted inference belongs in your Go applications and how to actually do it — natively, with GPU acceleration when you have it and CPU-friendly performance when you don't. To make it concrete, Bill will live-code a tic-tac-toe game and refactor it so a local model becomes Player2, using JSON Schema to constrain its moves. Kronk, the open-source Go SDK Bill built to make this possible, will naturally show up as the tool doing the heavy lifting.

### Talking Points

- Why Self-Hosted Inference
  - Cost, privacy, control, and vendor lock-in
  - The world beyond LLMs: vision, audio, embeddings, rerankers
  - When self-hosted is the right choice (and when it isn't)
- "But I Don't Have the Hardware" — Yes You Do
  - Small, capable models that run on a laptop (e.g. `Qwen3.5-0.8B-Q8_0`)
  - Quantization in plain English: trading a little quality for a lot of speed and memory
  - Picking the right model size for your machine
- Where Kronk fits in as a FOSS Go SDK
  - The usual paths: CGO, Python, network hop to Ollama — and why they hurt
- How Kronk Works
  - What "native Go inference" actually requires (GPU/CPU, batching, caching)

### Live Demo

- Tic-Tac-Toe With a Local Model as Player2
  - Build a Go TUI tic-tac-toe game
  - Drop in local inference as Player2
  - Use JSON Schema to constrain model output to legal moves

---

### Tic-Tac-Toe

Use the `writing-go` skill. Implement a two-player terminal game in `tictactoe/main.go` using only the Go standard library. Keep it short and direct; add no unrequested features.

Render this board, clearing the screen first with `\033[2J\033[H` and printing a blank line before and after it:

```text
Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):
```

Use ANSI colors, resetting after each colored segment:

- Grid lines: green
- Empty-cell numbers: white
- `X`: bold red
- `O`: bold green

Requirements:

- Empty cells display their positions (`1`–`9`).
- X moves first; players then alternate.
- Reject non-numeric, out-of-range, and occupied-cell input with an error, then re-prompt the same player.
- After each valid move, detect horizontal, vertical, or diagonal wins and full-board draws.
- At game end, print the result, update the session score, and ask `Play again? (y/n)`.
- Call these functions from the game loop; do not inline their logic:

```go
func playerX(b *Board) int
func playerO(b *Board) int
```

Format the file with `gofmt -s -w`, run `go build` to verify it compiles, and remove any generated binary. Do not run the game.

Start coding immediately without questions or a plan.
