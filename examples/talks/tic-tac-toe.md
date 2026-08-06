### Tic-Tac-Toe

Use the `writing-go` skill. Implement a two-player terminal game in `tictactoe/main.go` using only the Go standard library. Keep it short and direct; add no unrequested features.

Use a `Board` type with exactly nine cells. Cell index `0` is position `1`, index `1` is position `2`, and so on through index `8`, which is position `9`. An empty cell renders its 1-based position; an occupied cell renders `X` or `O`.

Every time the board is rendered:

1. Clear the terminal by printing `\033[2J\033[H`.
2. Print one blank line.
3. Print the score and board exactly as shown below after ANSI escape sequences are stripped.
4. Print one blank line after the last board row.

The initial screen must therefore appear as:

```text
Score: X: 0 | O: 0 | Draws: 0

 1 | 2 | 3
-----------
 4 | 5 | 6
-----------
 7 | 8 | 9

Player X's turn. Enter a number (1-9):
```

The prompt is not part of the board renderer; print it after the blank line following the board. Do not add borders, labels, indentation, or trailing spaces. Use these exact ANSI sequences, and print `\033[0m` immediately after every individually colored value or grid segment:

- Grid characters (`|` and each complete `-----------` separator): green, `\033[32m`
- Each empty-cell number: white, `\033[37m`
- Each `X`: bold red, `\033[1;31m`
- Each `O`: bold green, `\033[1;32m`

Spaces around `|` are uncolored. The score, prompts, errors, and game result are uncolored. Build each row with one leading space, one space on each side of each green `|`, and no trailing spaces, exactly as shown above.

Game loop and input requirements:

- Start every game with an empty board and Player X's turn. Alternate players only after a valid move.
- Read one complete input line at a time and trim surrounding whitespace. Use one shared buffered stdin reader so read-ahead is not lost between prompts.
- Reject an input line unless the entire trimmed line is one decimal integer from `1` through `9` and its corresponding cell is empty. Print a short error and re-prompt without clearing or re-rendering the board. Do not change the board or current player after invalid input.
- The player functions must print their own prompt, validate and re-prompt as needed, and return the chosen **zero-based cell index**. The game loop must call the appropriate function; do not inline this behavior:

```go
func playerX(b *Board) int
func playerO(b *Board) int
```

- After receiving a valid index, the game loop places that player's mark and immediately checks the eight possible winning lines: three rows, three columns, and two diagonals.
- If the player won, increment that player's session score. If all nine cells are occupied without a winner, increment the draw score. Otherwise switch players and continue.
- At game end, render the board once more so the final move and updated score are visible, then print exactly `Player X wins!`, `Player O wins!`, or `It's a draw.` as appropriate.
- Print `Play again? (y/n): ` and read one complete trimmed line. Start a new empty board with X to move only when the answer is `y`, case-insensitively. Preserve all session scores between games. For any other answer, exit normally.

Acceptance checks:

- The first board contains the numbers `1` through `9` in their corresponding cells.
- A mark replaces only the selected number; all other empty-cell numbers remain visible.
- The final move is visible before the result and replay prompt.
- A win takes precedence over a full-board draw.
- Starting another game clears the board but not the score.

Format the file with `gofmt -s -w`, run `go build` to verify it compiles, and remove any generated binary. Do not run the game.

Start coding immediately without questions or a plan.
