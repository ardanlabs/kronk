package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	colorX = "\033[34m" // Blue
	colorO = "\033[31m" // Red
	reset  = "\033[0m"
)

func renderBoard(board [9]string) {
	fmt.Println() // Line space before rendering new board
	display := make([]string, 9)
	for i, cell := range board {
		switch cell {
		case "X":
			display[i] = colorX + "X" + reset
		case "O":
			display[i] = colorO + "O" + reset
		default:
			display[i] = cell
		}
	}
	fmt.Printf("%s | %s | %s\n", display[0], display[1], display[2])
	fmt.Println("----------")
	fmt.Printf("%s | %s | %s\n", display[3], display[4], display[5])
	fmt.Println("----------")
	fmt.Printf("%s | %s | %s\n", display[6], display[7], display[8])
	fmt.Println()
}

func checkWinner(board [9]string) string {
	wins := [][]int{
		{0, 1, 2}, {3, 4, 5}, {6, 7, 8}, // Rows
		{0, 3, 6}, {1, 4, 7}, {2, 5, 8}, // Cols
		{0, 4, 8}, {2, 4, 6}, // Diagonals
	}

	for _, w := range wins {
		if board[w[0]] == "X" && board[w[1]] == "X" && board[w[2]] == "X" {
			return "X"
		}
		if board[w[0]] == "O" && board[w[1]] == "O" && board[w[2]] == "O" {
			return "O"
		}
	}
	return ""
}

func isDraw(board [9]string) bool {
	for _, cell := range board {
		if cell != "X" && cell != "O" {
			return false
		}
	}
	return true
}

var scanner = bufio.NewScanner(os.Stdin)

func playerX(board [9]string) int {
	for {
		fmt.Print("Player X's turn. Enter a number (1-9): ")
		if !scanner.Scan() {
			return -1
		}
		input := strings.TrimSpace(scanner.Text())
		num, err := strconv.Atoi(input)
		if err != nil || num < 1 || num > 9 {
			continue
		}
		idx := num - 1
		if board[idx] == "X" || board[idx] == "O" {
			continue
		}
		return idx
	}
}

func playerO(board [9]string) int {
	for {
		fmt.Print("Player O's turn. Enter a number (1-9): ")
		if !scanner.Scan() {
			return -1
		}
		input := strings.TrimSpace(scanner.Text())
		num, err := strconv.Atoi(input)
		if err != nil || num < 1 || num > 9 {
			continue
		}
		idx := num - 1
		if board[idx] == "X" || board[idx] == "O" {
			continue
		}
		return idx
	}
}

func main() {
	for {
		board := [9]string{"1", "2", "3", "4", "5", "6", "7", "8", "9"}
		winner := ""

		for {
			renderBoard(board)

			var moveIdx int
			currentTurn := ""

			xCount, oCount := 0, 0
			for _, cell := range board {
				if cell == "X" {
					xCount++
				}
				if cell == "O" {
					oCount++
				}
			}

			if xCount <= oCount {
				currentTurn = "X"
				moveIdx = playerX(board)
			} else {
				currentTurn = "O"
				moveIdx = playerO(board)
			}

			if moveIdx == -1 {
				return
			}

			board[moveIdx] = currentTurn

			winner = checkWinner(board)
			if winner != "" {
				renderBoard(board)
				if winner == "X" {
					fmt.Printf("Player %s wins!\n", colorX+"X"+reset)
				} else {
					fmt.Printf("Player %s wins!\n", colorO+"O"+reset)
				}
				break
			}

			if isDraw(board) {
				renderBoard(board)
				fmt.Println("It's a draw!")
				break
			}
		}

		fmt.Print("Play again? (y/n): ")
		if !scanner.Scan() {
			break
		}
		choice := strings.ToLower(strings.TrimSpace(scanner.Text()))
		if choice != "y" {
			break
		}
	}
}
