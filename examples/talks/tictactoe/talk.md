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

Write a 2-player terminal tic-tac-toe game in a single file
`tictactoe/main.go` using only the Go standard library. Keep the code
short and direct — do not add features I haven't asked for.

**Board layout** (green lines, white numbers, red bold X, green bold O):

```
Score: X: 0 | O: 0 | Draws: 0

1 | 2 | 3
----------
4 | 5 | 6
----------
7 | 8 | 9

Player X's turn. Enter a number (1-9):
```

Empty cells show their number (1–9). Taken cells show `X` or `O`.

**Rules**

- Player X goes first, then alternate.
- A move is valid if the chosen cell still shows a number.
- After each move, check for a winner (3 in a row: horizontal,
  vertical, or diagonal) or a draw (board full).
- On invalid input (non-numeric, out of range, or taken), print an
  error and re-prompt the same player.
- When a game ends, print the result, update the score, and ask
  `Play again? (y/n)`. Scores persist across games in the session.
- Clear the screen before every board render (ANSI `\033[2J\033[H`).
- Print a blank line before and after each board render.
- Use ANSI escape codes for color. Reset after each colored segment.

**Required functions** — you MUST call these from the game loop, not
re-inline the logic:

```go
// playerX prompts player X and returns the chosen 1-9 cell.
func playerX(b *Board) int

// playerO prompts player O and returns the chosen 1-9 cell.
func playerO(b *Board) int
```

**Finish**

- Run `go build` to confirm it compiles, then delete the binary.
- Run `gofmt -s -w` on the file.
- Do not run the game.

Start coding now. No questions, no plan — just write the code.
