package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type Board [9]byte

var stdin = bufio.NewReader(os.Stdin)

var winLines = [][3]int{
	{0, 1, 2}, {3, 4, 5}, {6, 7, 8},
	{0, 3, 6}, {1, 4, 7}, {2, 5, 8},
	{0, 4, 8}, {2, 4, 6},
}

func renderBoard(b *Board, xWins, oWins, draws int) {
	fmt.Print("\033[2J\033[H\n")
	fmt.Printf("Score: X: %d | O: %d | Draws: %d\n", xWins, oWins, draws)
	for r := 0; r < 3; r++ {
		i := r * 3
		fmt.Print(" ", cell(b, i), " ", "\033[32m|\033[0m", " ", cell(b, i+1), " ", "\033[32m|\033[0m", " ", cell(b, i+2), "\n")
	}
	fmt.Print("\033[32m-----------\033[0m\n")
	fmt.Print("\033[32m-----------\033[0m\n")
	fmt.Print("\n")
}

func cell(b *Board, i int) string {
	if b[i] == 0 {
		return fmt.Sprintf("\033[37m%d\033[0m", i+1)
	}
	switch b[i] {
	case 'X':
		return "\033[1;31mX\033[0m"
	case 'O':
		return "\033[1;32mO\033[0m"
	}
	return string(b[i])
}

func hasWinner(b *Board) bool {
	for _, line := range winLines {
		a, c, d := line[0], line[1], line[2]
		if a != 0 && a == c && c == d {
			return true
		}
	}
	return false
}

func boardFull(b *Board) bool {
	return b[0] != 0 && b[1] != 0 && b[2] != 0 && b[3] != 0 && b[4] != 0 && b[5] != 0 && b[6] != 0 && b[7] != 0 && b[8] != 0
}

func playerX(b *Board) int { return moveFor(b, 'X') }
func playerO(b *Board) int { return moveFor(b, 'O') }

func moveFor(b *Board, player byte) int {
	for {
		fmt.Print(turnPrompt(player))
		line, _ := stdin.ReadString('\n')
		if idx := validMove(b, strings.TrimSpace(line)); idx >= 0 {
			return idx
		}
		fmt.Print("Invalid move. Enter an empty position from 1 to 9.\n")
	}
}

func validMove(b *Board, s string) int {
	if len(s) != 1 {
		return -1
	}
	c := s[0]
	if c < '1' || c > '9' {
		return -1
	}
	idx := int(c) - '1'
	if b[idx] != 0 {
		return -1
	}
	return idx
}

func turnPrompt(player byte) string {
	if player == 'X' {
		return "Player X's turn. Enter a number (1-9):"
	}
	return "Player O's turn. Enter a number (1-9):"
}

func flip(player byte) byte {
	if player == 'X' {
		return 'O'
	}
	return 'X'
}

func continueGame(answer string) bool {
	return strings.EqualFold(strings.TrimSpace(answer), "y")
}

func main() {
	var xWins, oWins, draws int

replay:
	for {
		var b Board
		var player byte = 'X'

		renderBoard(&b, xWins, oWins, draws)

		idx := moveFor(&b, player)
		b[idx] = player

		if hasWinner(&b) {
			if player == 'X' {
				xWins++
			} else {
				oWins++
			}
			renderBoard(&b, xWins, oWins, draws)
			fmt.Printf("Player %c wins!\n", player)
			break
		}

		if boardFull(&b) {
			draws++
			renderBoard(&b, xWins, oWins, draws)
			fmt.Println("It's a draw.")
			break
		}

		player = flip(player)
		renderBoard(&b, xWins, oWins, draws)
	}

	fmt.Print("Play again? (y/n): ")
	answer, _ := stdin.ReadString('\n')
	if !continueGame(answer) {
		return
	}
	goto replay
}
