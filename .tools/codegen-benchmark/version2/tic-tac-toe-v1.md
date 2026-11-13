# Tic-Tac-Toe — v1

Use the `use-modern-go` skill. Implement a two-player terminal game in `tictactoe/main.go` using only the Go standard library. Follow the implementation recipe below directly. Keep the code short and add no unrequested features.

## Required program structure

- Declare `type Board [9]byte`. A zero byte is an empty cell; occupied cells contain `'X'` or `'O'`.
- Cell index `0` is displayed as position `1`, through index `8`, which is displayed as position `9`.
- Declare `var stdin = bufio.NewReader(os.Stdin)`. Use this same reader for every move and replay prompt so buffered read-ahead is never lost.
- Keep three integer session scores for X wins, O wins, and draws. Create them once before the outer game loop so they survive replay.
- Use these helpers in addition to the two required player functions described below. Do not declare any struct types:

```go
func renderBoard(b *Board, xWins, oWins, draws int)
func hasWinner(b *Board) bool
func boardFull(b *Board) bool
```

## Board rendering

Every time the board is rendered:

1. Clear the terminal by printing the Go string literal `"\033[2J\033[H"`.
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

The prompt is not part of the board renderer; print it after the blank line following the board. Do not add borders, labels, indentation, or trailing spaces. Use these exact Go string literals, and print `"\033[0m"` immediately after every individually colored value or grid segment:

- Board grid characters (`|` and each complete `-----------` separator): green, `"\033[32m"`
- Each empty-cell number: white, `"\033[37m"`
- Each `X`: bold red, `"\033[1;31m"`
- Each `O`: bold green, `"\033[1;32m"`

Spaces around `|` are uncolored. The score, prompts, errors, and game result are uncolored. Build each row with one leading space, one space on each side of each green `|`, and no trailing spaces, exactly as shown above.

When rendering a cell, print its mark when occupied. Otherwise print `index + 1`, so every unoccupied cell continues to show its own position.

## Move input

Implement these exact functions and have the game loop call the appropriate one:

```go
func playerX(b *Board) int
func playerO(b *Board) int
```

Each player function must print its own exact prompt:

```go
fmt.Print("Player X's turn. Enter a number (1-9):")
fmt.Print("Player O's turn. Enter a number (1-9):")
```

Move prompts end immediately after the colon, without a trailing space or newline. Invalid-move and game-result messages end with exactly one newline. The replay prompt ends with one space and no newline.

Print the prompt with `fmt.Print`, then read one complete line and apply `strings.TrimSpace`. A valid trimmed input is exactly one byte whose value is between `'1'` and `'9'`, and whose corresponding board cell is empty. Convert it to a zero-based index by subtracting `'1'`.

For every invalid input, print exactly:

```text
Invalid move. Enter an empty position from 1 to 9.
```

Then print the same player prompt and read again. Do not clear or render the board during this retry loop. Return only after obtaining a valid zero-based cell index. A shared unexported helper may contain the common retry logic for `playerX` and `playerO`.

## Win and draw checks

Check exactly these eight winning index triples:

```text
{0, 1, 2}, {3, 4, 5}, {6, 7, 8},
{0, 3, 6}, {1, 4, 7}, {2, 5, 8},
{0, 4, 8}, {2, 4, 6}
```

A line wins only when its first cell is nonzero and all three cells are equal. The board is full only when all nine cells are nonzero. Always check for a win before checking for a draw.

## Game loop recipe

Implement the control flow in this order:

1. Initialize the three session scores once.
2. Start the outer replay loop.
3. Create a new zero-value `Board` and set the current player to `'X'`.
4. Render the board with the current scores.
5. Call `playerX` or `playerO` according to the current player.
6. Put the current player's byte into the returned cell.
7. Check for a win. On a win, increment the correct score, render the final board with the updated score, print exactly `Player X wins!` or `Player O wins!`, and end the current game.
8. Otherwise check whether the board is full. On a draw, increment the draw score, render the final board with the updated score, print exactly `It's a draw.`, and end the current game.
9. Otherwise switch the current player, render the updated board, and continue at step 5. Do not render the same non-final board twice.
10. After the result, print `Play again? (y/n): ` with `fmt.Print` and read one complete trimmed line with the shared reader.
11. Start a new game only when the answer equals `y`, case-insensitively. For every other answer, exit normally.

## Acceptance checks

- The first board contains the numbers `1` through `9` in their corresponding cells.
- A mark replaces only the selected number; all other empty-cell numbers remain visible.
- The final move is visible before the result and replay prompt.
- A win takes precedence over a full-board draw.
- Starting another game clears the board but not the score.

Format the file with `gofmt -s -w`, run `go build` to verify it compiles, and remove any generated binary. Do not run the game.

Start coding immediately without questions or a plan.
