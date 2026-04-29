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
- Doing It The Go Way
  - The usual paths: CGO, Python, network hop to Ollama — and why they hurt
  - What "native Go inference" actually requires (GPU/CPU, batching, caching)
  - Where Kronk fits in as a FOSS Go SDK

### Live Demo

- Tic-Tac-Toe With a Local Model as Player2
  - Build a Go TUI tic-tac-toe game
  - Drop in local inference as Player2
  - Use JSON Schema to constrain model output to legal moves

### Basic Game Play Prompt

- I want to write a simple tic-tac-toe game using only the Go standard library.
- Do not overthink writing this game and be concise when writing or refactoring the code.
- The game play can be in the terminal.
- Allow 2 players to play against each other.
- Add the code to a file name `examples/talks/tictactoe/main.go`

- Paint the board exactly like this:

```

1 | 2 | 3
----------
4 | 5 | 6
----------
7 | 8 | 9

Player X's turn. Enter a number (1-9):

```

- Make sure there is a line space before and after rendering a new board.
- Use the color green for the lines, red for X, and blue for O.
- Clear the screen when rendering a new board.
- When the game is over, clear the screen, render the final board, and show the outcome of the game.
- Ask Player1 to go first.
- Each player will choose a number that corresponds to a place on the board.
- The first player uses `X` and the second player uses `O`.

- The following board shows the first player selecting space number 5.

```
1 | 2 | 3
----------
4 | X | 6
----------
7 | 8 | 9

Player O's turn. Enter a number (1-9):
```

- The following board shows the second player selecting space number 1.

```
O | 2 | 3
----------
4 | X | 6
----------
7 | 8 | 9

Player X's turn. Enter a number (1-9):
```

- If the board is showing a number for that space, then that number is a valid move.
- Always check for a winner or a draw after every move.
- A draw would be all spaces having an `X` or an `O`.
- There is a winner when there are 3 `X`'s or 3 `O`'s in a straight horizontal, vertical, or diagonal line.
- When a game is over announce the winner and give the user an option to play again for quit the game.
- I need functions called playerX and playerO that is used when it's that player's turn to choose a space. I need to encapsulate that functionality.
- Do not attempt to run the game yourself.
- Compile the program to validate it compiles. Fix any errors that you find. Then remove the binary you created to validate the code.
- Run go fmt to make sure the code is properly formatted.
