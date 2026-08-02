package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[1;31m"
	colorGreen  = "\033[1;32m"
	colorWhite  = "\033[37m"
	colorGrid   = "\033[32m"
	clearScreen = "\033[2J\033[H"
)

type Board [9]rune

type Score struct {
	X     int
	O     int
	Draws int
}

var scanner = bufio.NewScanner(os.Stdin)

func main() {
	score := &Score{}
	for {
		board := Board{'1', '2', '3', '4', '5', '6', '7', '8', '9'}
		winner, isDraw := playGame(&board, score)

		fmt.Printf("\n%s\n", winner)
		if isDraw {
			fmt.Println("It's a draw!")
		}

		fmt.Print("\nPlay again? (y/n): ")
		if !scanner.Scan() {
			break
		}
		if strings.ToLower(strings.TrimSpace(scanner.Text())) != "y" {
			break
		}
	}
}

func playGame(b *Board, s *Score) (string, bool) {
	for {
		printBoard(b, s)

		var turn string
		var playerFunc func(*Board) int
		xCount, oCount := 0, 0
		for _, r := range b {
			if r == 'X' {
				xCount++
			} else if r == 'O' {
				oCount++
			}
		}

		if xCount == oCount {
			turn = "X"
			playerFunc = playerX
		} else {
			turn = "O"
			playerFunc = playerO
		}

		move := playerFunc(b)
		b[move-1] = rune(turn[0])

		if winner, isDraw := checkWinner(b); winner != "" || isDraw {
			if winner != "" {
				if turn == "X" {
					s.X++
				} else {
					s.O++
				}
				return "Player " + turn + " wins!", false
			}
			s.Draws++
			return "", true
		}
	}
}

func printBoard(b *Board, s *Score) {
	fmt.Print(clearScreen)
	fmt.Printf("\nScore: X: %d | O: %d | Draws: %d\n\n", s.X, s.O, s.Draws)

	for i := 0; i < 9; i++ {
		char := b[i]
		color := colorWhite
		if char == 'X' {
			color = colorRed
		} else if char == 'O' {
			color = colorGreen
		}

		fmt.Printf(" %s%c%s", color, char, colorReset)
		if i%3 == 2 {
			fmt.Println()
		} else {
			fmt.Printf("%s | %s", colorGrid, colorReset)
		}

		if i == 2 || i == 5 {
			fmt.Printf("%s-----------\n%s", colorGrid, colorReset)
		}
	}
}

func playerX(b *Board) int {
	return getMove(b, "X")
}

func playerO(b *Board) int {
	return getMove(b, "O")
}

func getMove(b *Board, player string) int {
	for {
		fmt.Printf("Player %s's turn. Enter a number (1-9): ", player)
		if !scanner.Scan() {
			return -1
		}
		input := strings.TrimSpace(scanner.Text())
		num, err := strconv.Atoi(input)

		if err != nil {
			fmt.Println("Invalid input. Please enter a number (1-9).")
			continue
		}
		if num < 1 || num > 9 {
			fmt.Println("Out of range. Please enter a number (1-9).")
			continue
		}
		if b[num-1] == 'X' || b[num-1] == 'O' {
			fmt.Println("Cell is already occupied.")
			continue
		}
		return num
	}
}

func checkWinner(b *Board) (string, bool) {
	wins := [][]int{
		{0, 1, 2}, {3, 4, 5}, {6, 7, 8},
		{0, 3, 6}, {1, 4, 7}, {2, 5, 8},
		{0, 4, 8}, {2, 4, 6},
	}

	for _, w := range wins {
		if b[w[0]] == b[w[1]] && b[w[1]] == b[w[2]] {
			if b[w[0]] == 'X' || b[w[0]] == 'O' {
				return string(b[w[0]]), false
			}
		}
	}

	for _, r := range b {
		if r >= '1' && r <= '9' {
			return "", false
		}
	}

	return "", true
}
