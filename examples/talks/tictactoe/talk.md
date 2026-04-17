## Kronk: Hardware accelerated local inference

### About this Session

In this talk Bill will introduce Kronk, a new SDK that allows you to write AI based apps without the need of a model server. If you have Apple Metal (Mac), CUDA (NVIDIA), or Vulkan, Kronk can tap into that GPU power instead of grinding through the work on the CPU alone.

To dog food the SDK, Bill wrote a model server (KMS) that is optimized to run your local AI workloads with performance in mind. During the talk, Bill will show how you can use Agents like Kilo Code to run local agentic workloads to perform basic work.

### Outline

- Introduction
  - Who am I and why I built Kronk
  - What is Kronk
  - How local inference is the future
- Build Tic-Tac-Toe App Using Kronk
  - Go TUI app that can perform basic game play
  - Integrate Kronk SDK
  - Use Kronk to be Player2
  - Show JSON Schema capabilities

### Basic Game Play Prompt

- I want to write a basic tic-tac-toe game only using the Go standard library.
- The game play can be in the terminal.
- Allow 2 players to play against each other.
- Add the code to a file name `examples/talks/tictactoe/main.go`

- Paint the board like this:

```
1 | 2 | 3
----------
4 | 5 | 6
----------
7 | 8 | 9

Player X's turn. Enter a number (1-9):
```

- Make sure there is a line space before rendering a new board.
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
- There is a winner when there are 3 `X`'s or 3 `O`'s in a straight horizontal,
  vertical, or diagonal line.
- When a game is over announce the winner and give the user an option to play
  again for quit the game.
- I need functions called playerX and playerO that is used when it's that
  player's turn to choose a space. I need to encapsulate that functionality.
- Make sure the code complies before you report the code is complete.
