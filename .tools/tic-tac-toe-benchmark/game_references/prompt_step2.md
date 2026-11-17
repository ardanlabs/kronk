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

```go
fmt.Print("Player X's turn. Enter a number (1-9), or 0 to undo the last move:")
fmt.Print("Player O's turn. Enter a number (1-9), or 0 to undo the last move:")
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
