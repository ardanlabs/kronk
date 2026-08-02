package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	colorReset     = "\033[0m"
	colorGreen     = "\033[32m"
	colorWhite     = "\033[37m"
	colorBoldRed   = "\033[1;31m"
	colorBoldGreen = "\033[1;32m"
	clearScreen    = "\033[2J\033[H"
)

type Board struct {
	cells [9]string
}

type Score struct {
	X     int
	O     int
	Draws int
}

var (
	currentScore Score
	reader       = bufio.NewReader(os.Stdin)
)

func renderBoard(b *Board, turn string) {
	fmt.Print(clearScreen)
	fmt.Printf("\nScore: X: %d | O: %d | Draws: %d\n\n", currentScore.X, currentScore.O, currentScore.Draws)

	formatCell := func(val string) string {
		switch val {
		case "X":
			return colorBoldRed + "X" + colorReset
		case "O":
			return colorBoldGreen + "O" + colorReset
		default:
			return colorWhite + val + colorReset
		}
	}

	sep := colorGreen + "|" + colorReset
	line := colorGreen + "-----------" + colorReset

	fmt.Printf(" %s %s %s %s %s\n", formatCell(b.cells[0]), sep, formatCell(b.cells[1]), sep, formatCell(b.cells[2]))
	fmt.Println(line)
	fmt.Printf(" %s %s %s %s %s\n", formatCell(b.cells[3]), sep, formatCell(b.cells[4]), sep, formatCell(b.cells[5]))
	fmt.Println(line)
	fmt.Printf(" %s %s %s %s %s\n", formatCell(b.cells[6]), sep, formatCell(b.cells[7]), sep, formatCell(b.cells[8]))
	fmt.Println()

	if turn != "" {
		fmt.Printf("Player %s's turn. Enter a number (1-9): ", turn)
	}
}

func playerX(b *Board) int {
	return getMove(b, "X")
}

func playerO(b *Board) int {
	return getMove(b, "O")
}

func getMove(b *Board, symbol string) int {
	for {
		input, err := reader.ReadString('\n')
		if err != nil {
			continue
		}
		input = strings.TrimSpace(input)

		idx, err := strconv.Atoi(input)
		if err != nil || idx < 1 || idx > 9 {
			fmt.Println("Invalid input. Please enter a number (1-9):")
			renderBoard(b, symbol)
			continue
		}

		cellIdx := idx - 1
		if b.cells[cellIdx] == "X" || b.cells[cellIdx] == "O" {
			fmt.Println("Cell already occupied. Please enter a different number:")
			renderBoard(b, symbol)
			continue
		}

		return cellIdx
	}
}

func checkWin(b *Board) string {
	wins := [8][3]int{
		{0, 1, 2}, {3, 4, 5}, {6, 7, 8}, // rows
		{0, 3, 6}, {1, 4, 7}, {2, 5, 8}, // cols
		{0, 4, 8}, {2, 4, 6}, // diags
	}

	for _, w := range wins {
		if (b.cells[w[0]] == "X" || b.cells[w[0]] == "O") &&
			b.cells[w[0]] == b.cells[w[1]] && b.cells[w[0]] == b.cells[w[2]] {
			return b.cells[w[0]]
		}
	}

	full := true
	for _, c := range b.cells {
		if c != "X" && c != "O" {
			full = false
			break
		}
	}
	if full {
		return "Draw"
	}

	return ""
}

func main() {
	for {
		board := Board{}
		for i := 0; i < 9; i++ {
			board.cells[i] = strconv.Itoa(i + 1)
		}

		turn := 0 // 0 for X, 1 for O
		for {
			var currentSymbol string
			if turn == 0 {
				currentSymbol = "X"
			} else {
				currentSymbol = "O"
			}

			renderBoard(&board, currentSymbol)

			var move int
			if turn == 0 {
				move = playerX(&board)
				board.cells[move] = "X"
			} else {
				move = playerO(&board)
				board.cells[move] = "O"
			}

			result := checkWin(&board)
			if result != "" {
				renderBoard(&board, "")
				if result == "X" {
					fmt.Printf("%sX wins!%s\n", colorBoldRed, colorReset)
					currentScore.X++
				} else if result == "O" {
					fmt.Printf("%sO wins!%s\n", colorBoldGreen, colorReset)
					currentScore.O++
				} else {
					fmt.Println("Draw!")
					currentScore.Draws++
				}
				break
			}
			turn = 1 - turn
		}

		fmt.Print("Play again? (y/n): ")
		choice, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(choice)) != "y" {
			break
		}
	}
}
