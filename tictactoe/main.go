package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	reset     = "\033[0m"
	boldRed   = "\033[1;31m"
	boldGreen = "\033[1;32m"
	green     = "\033[32m"
	white     = "\033[37m"
)

type Board [9]rune

type Score struct {
	x     int
	o     int
	draws int
}

var reader = bufio.NewReader(os.Stdin)

func main() {
	score := &Score{}
	for {
		board := Board{'1', '2', '3', '4', '5', '6', '7', '8', '9'}
		winner, isDraw := playGame(&board, score)

		if winner == 1 {
			fmt.Println("X wins!")
		} else if winner == 2 {
			fmt.Println("O wins!")
		} else if isDraw {
			fmt.Println("It's a draw!")
		}

		fmt.Printf("\nScore: X: %d | O: %d | Draws: %d\n", score.x, score.o, score.draws)
		fmt.Print("Play again? (y/n): ")
		input, _ := reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(input)) != "y" {
			break
		}
	}
}

func playGame(b *Board, s *Score) (int, bool) {
	turn := 1 // 1 for X, 2 for O
	for {
		renderBoard(b, s, turn)
		var move int
		if turn == 1 {
			move = playerX(b)
		} else {
			move = playerO(b)
		}

		idx := move - 1
		if turn == 1 {
			b[idx] = 'X'
		} else {
			b[idx] = 'O'
		}

		if checkWin(b, turn) {
			renderBoard(b, s, 0)
			if turn == 1 {
				s.x++
			} else {
				s.o++
			}
			return turn, false
		}

		if isFull(b) {
			renderBoard(b, s, 0)
			s.draws++
			return 0, true
		}

		if turn == 1 {
			turn = 2
		} else {
			turn = 1
		}
	}
}

func renderBoard(b *Board, s *Score, turn int) {
	fmt.Print("\033[2J\033[H\n")
	fmt.Printf("Score: X: %d | O: %d | Draws: %d\n\n", s.x, s.o, s.draws)

	cells := [9]string{}
	for i, v := range b {
		if v == 'X' {
			cells[i] = boldRed + "X" + reset
		} else if v == 'O' {
			cells[i] = boldGreen + "O" + reset
		} else {
			cells[i] = white + string(v) + reset
		}
	}

	fmt.Printf("%s %s %s | %s %s %s | %s %s %s\n", green, reset, cells[0], green, reset, cells[1], green, reset, cells[2])
	fmt.Printf("%s-----------%s\n", green, reset)
	fmt.Printf("%s %s %s | %s %s %s | %s %s %s\n", green, reset, cells[3], green, reset, cells[4], green, reset, cells[5])
	fmt.Printf("%s-----------%s\n", green, reset)
	fmt.Printf("%s %s %s | %s %s %s | %s %s %s\n", green, reset, cells[6], green, reset, cells[7], green, reset, cells[8])
	fmt.Print("\n")

	if turn != 0 {
		label := "X"
		if turn == 2 {
			label = "O"
		}
		fmt.Printf("Player %s's turn. Enter a number (1-9): ", label)
	}
}

func playerX(b *Board) int {
	return getMove(b, "X")
}

func playerO(b *Board) int {
	return getMove(b, "O")
}

func getMove(b *Board, _ string) int {
	for {
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		val, err := strconv.Atoi(input)

		if err != nil || val < 1 || val > 9 || b[val-1] == 'X' || b[val-1] == 'O' {
			fmt.Print("Invalid move. Enter a number (1-9): ")
			continue
		}
		return val
	}
}

func checkWin(b *Board, player int) bool {
	p := 'X'
	if player == 2 {
		p = 'O'
	}

	wins := [8][3]int{
		{0, 1, 2}, {3, 4, 5}, {6, 7, 8},
		{0, 3, 6}, {1, 4, 7}, {2, 5, 8},
		{0, 4, 8}, {2, 4, 6},
	}

	for _, w := range wins {
		if b[w[0]] == p && b[w[1]] == p && b[w[2]] == p {
			return true
		}
	}
	return false
}

func isFull(b *Board) bool {
	for _, v := range b {
		if v == '1' || v == '2' || v == '3' || v == '4' || v == '5' || v == '6' || v == '7' || v == '8' || v == '9' {
			return false
		}
	}
	return true
}
