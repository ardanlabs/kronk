package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type board [3][3]string

func (b *board) print() {
	fmt.Println("\n  0 1 2")
	for i, row := range b {
		fmt.Printf("%d %s %s %s\n", i, row[0], row[1], row[2])
		if i < 2 {
			fmt.Println("  -----")
		}
	}
	fmt.Println()
}

func (b *board) checkWinner() string {
	// Rows and columns
	for i := 0; i < 3; i++ {
		if b[i][0] != "" && b[i][0] == b[i][1] && b[i][1] == b[i][2] {
			return b[i][0]
		}
		if b[0][i] != "" && b[0][i] == b[1][i] && b[1][i] == b[2][i] {
			return b[0][i]
		}
	}
	// Diagonals
	if b[0][0] != "" && b[0][0] == b[1][1] && b[1][1] == b[2][2] {
		return b[0][0]
	}
	if b[0][2] != "" && b[0][2] == b[1][1] && b[1][1] == b[2][0] {
		return b[0][2]
	}
	return ""
}

func (b *board) isFull() bool {
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			if b[i][j] == "" {
				return false
			}
		}
	}
	return true
}

func main() {
	reader := bufio.NewReader(os.Stdin)

	for {
		var b board
		currentPlayer := "X"
		gameWon := false

		fmt.Println("--- Tic-Tac-Toe ---")

		for !gameWon {
			b.print()
			fmt.Printf("Player %s, enter your move (row and column, e.g., '0 1'): ", currentPlayer)
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)
			parts := strings.Fields(input)

			if len(parts) != 2 {
				fmt.Println("Invalid input. Please enter row and column separated by a space.")
				continue
			}

			var r, c int
			_, err1 := fmt.Sscanf(parts[0], "%d", &r)
			_, err2 := fmt.Sscanf(parts[1], "%d", &c)

			if err1 != nil || err2 != nil || r < 0 || r > 2 || c < 0 || c > 2 {
				fmt.Println("Invalid coordinates. Use 0, 1, or 2.")
				continue
			}

			if b[r][c] != "" {
				fmt.Println("Cell already taken. Try again.")
				continue
			}

			b[r][c] = currentPlayer
			winner := b.checkWinner()

			if winner != "" {
				b.print()
				fmt.Printf("Player %s wins!\n", winner)
				gameWon = true
			} else if b.isFull() {
				b.print()
				fmt.Println("It's a draw!")
				gameWon = true
			} else {
				if currentPlayer == "X" {
					currentPlayer = "O"
				} else {
					currentPlayer = "X"
				}
			}
		}

		fmt.Print("\nPlay again? (y/n): ")
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(strings.ToLower(choice))
		if choice != "y" {
			fmt.Println("Thanks for playing!")
			break
		}
	}
}
