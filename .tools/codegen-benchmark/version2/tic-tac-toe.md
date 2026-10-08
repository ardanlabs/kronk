# Tic-Tac-Toe

Use the `writing-go` skill. Implement a two-player terminal game in `tictactoe/main.go` using only the Go standard library. Follow the implementation recipe below directly. Keep the code short and add no unrequested features.

## Required program structure

- Declare `type Board [9]byte`. A zero byte is an empty cell; occupied cells contain `'X'` or `'O'`.
- Cell index `0` is displayed as position `1`, through index `8`, which is displayed as position `9`.
- Declare `var stdin = bufio.NewReader(os.Stdin)`. Use this same reader for every move and replay prompt so buffered read-ahead is never lost.
- Keep three integer session scores for X wins, O wins, and draws. Create them once before the outer game loop so they survive replay.
- Use these helpers in addition to the two required player functions described below. Do not introduce structs other than `Board`:

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

```text
Player X's turn. Enter a number (1-9):
Player O's turn. Enter a number (1-9):
```

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

================================================================================

# Add One-Move Undo

Modify the tic-tac-toe game you just implemented in `tictactoe/main.go`. Add only the undo feature described below. Preserve the existing board rendering, colors, win and draw rules, replay behavior, and session scores.

## Required behavior

Allow the most recent player to enter `0` at the next turn prompt to undo the move they just made. Undo removes exactly that one mark and gives the turn back to the player whose mark was removed so they can make a replacement move.

For example:

1. X places a mark in position `5`.
2. The board is rendered and O's prompt is displayed.
3. If `0` is entered, remove X from position `5`.
4. Render the board again with position `5` empty and showing the number `5`.
5. Prompt X, not O, to make the replacement move.
6. X may choose position `5` again or any other empty position.

The terminal cannot identify which person typed a line. Therefore, whenever `0` is entered at the current prompt, treat it as an undo request from the player who made the immediately preceding move.

## Input changes

Keep these function signatures unchanged:

```go
func playerX(b *Board) int
func playerO(b *Board) int
```

Change their prompts to exactly:

```text
Player X's turn. Enter a number (1-9), or 0 to undo the last move:
Player O's turn. Enter a number (1-9), or 0 to undo the last move:
```

Keep using `fmt.Print`, the shared `stdin` reader, one complete input line, and `strings.TrimSpace`.

- For a valid empty position from `1` through `9`, return its zero-based board index exactly as before.
- For a trimmed input of exactly `0`, return `-1`. This is the undo sentinel.
- Continue rejecting every other input exactly as before.
- The player functions must not change the board when they receive `0`; the game loop handles the undo.

## State to add

Inside each new game, declare:

```go
lastMove := -1
```

`lastMove == -1` means there is currently no move available to undo. Any value from `0` through `8` is the board index of the move that can be undone.

- After placing a normal move, assign its zero-based index to `lastMove`.
- After undoing a move, set `lastMove` back to `-1` before asking that player for a replacement move.
- After the replacement move is placed, assign its index to `lastMove`, just like any other normal move.
- Do not add a move-history slice or stack. Track only this one board index.

## Exact game-loop handling

After calling `playerX` or `playerO`, handle the returned integer before placing a mark:

1. If the returned value is `-1` and `lastMove` is also `-1`, there is nothing to undo. Print exactly:

   ```text
   Invalid move. Enter an empty position from 1 to 9.
   ```

   Do not render the board, change the board, or switch players. Call the same player's input function again.

2. If the returned value is `-1` and `lastMove` is between `0` and `8`:
   - Save the mark being removed with `undonePlayer := b[lastMove]` before clearing the cell.
   - Clear the move with `b[lastMove] = 0`.
   - Set the current player to `undonePlayer`.
   - Set `lastMove = -1`.
   - Render the updated board once.
   - Continue the move loop so the restored player is prompted again.
   - Do not run win or draw checks and do not change any score for an undo.

3. Otherwise, the returned value is a normal zero-based cell index:
   - Place the current player's mark in that cell.
   - Set `lastMove` to that cell index.
   - Continue with the existing win check, draw check, player switch, and rendering logic.

Because `lastMove` is `-1` immediately after an undo, entering `0` again instead of making a replacement move is invalid. The player must first place a new mark from `1` through `9`.

A winning move or draw-ending move cannot be undone because the game ends before another move prompt. Starting a replayed game must reset `lastMove` to `-1`. Undo must never alter the X-win, O-win, or draw scores.

## Acceptance checks

- Entering `0` after X moves removes X's most recent mark and gives X another turn.
- Entering `0` after O moves removes O's most recent mark and gives O another turn.
- The cleared cell displays its position number again.
- The replacement move may use the same cell or a different empty cell.
- Entering `0` on the first turn prints the existing invalid-move message and keeps X's turn.
- Entering `0` immediately after an undo prints the invalid-move message and keeps the restored player's turn.
- Undo never changes session scores.
- Replay starts with an empty board, X's turn, and no move available to undo.

Format the changed Go file with `gofmt -s -w`. Run `go vet`, `staticcheck`, `go fix`, and `go build` for the changed package. Remove any generated binary. Do not run the game.

Start coding immediately without questions or a plan.
