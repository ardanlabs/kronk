package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type board [9]string

func (b *board) render() {
	fmt.Println()
	fmt.Printf("%s | %s | %s\n", b[0], b[1], b[2])
	fmt.Println("----------")
	fmt.Printf("%s | %s | %s\n", b[3], b[4], b[5])
	fmt.Println("----------")
	fmt.Printf("%s | %s | %s\n", b[6], b[7], b[8])
}

func (b *board) isFull() bool {
	for _, cell := range b {
		if cell == "" || cell == "1" || cell == "2" || cell == "3" || cell == "4" || cell == "5" || cell == "6" || cell == "7" || cell == "8" || cell == "9" {
			// The prompt says if the board is showing a number, it's a valid move.
			// However, a draw is when all spaces have an X or an O.
			// Let's check for X or O specifically for the draw condition.
			continue
		}
		if cell != "X" && cell != "O" {
			// This part is a bit tricky because of how we initialize the board.
			// If it's a number, it's not X or O.
		}
	}
	// Let's redefine the board to use numbers initially for rendering.
	return false
}

// Re-evaluating board initialization:
// The prompt says: "If the board is showing a number for that space, then that number is a valid move."
// And "A draw would be all spaces having an X or an O."

func (b *board) hasWinner(player string) bool {
	wins := [][3]int{
		{0, 1, 2}, {3, 4, 5}, {6, 7, 8}, // rows
		{0, 3, 6}, {1, 4, 7}, {2, 5, 8}, // cols
		{0, 4, 8}, {2, 4, 6}, // diags
	}
	for _, win := range wins {
		if b[win[0]] == player && b[win[1]] == player && b[win[2]] == player {
			return true
		}
	}
	return false
}

func (b *board) isDraw() bool {
	for _, cell := range b {
		if cell != "X" && cell != "O" {
			return false
		}
	}
	return true
}

func playerX(b *board, reader *bufio.Reader) (int, error) {
	for {
		fmt.Print("\nPlayer X's turn. Enter a number (1-9): ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return 0, err
		}
		input = strings.TrimSpace(input)
		num, err := strconv.Atoi(input)
		if err != nil || num < 1 || num > 9 {
			fmt.Println("Invalid input. Please enter a number between 1 and 9.")
			continue
		}
		idx := num - 1
		if b[idx] == "X" || b[idx] == "O" {
			fmt.Println("That space is already taken. Try again.")
			continue
		}
		return idx, nil
	}
}

func playerO(b *board, reader *bufio.Reader) (int, error) {
	for {
		fmt.Print("\nPlayer O's turn. Enter a number (1-9): ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return 0, err
		}
		input = strings.TrimSpace(input)
		num, err := strconv.Atoi(input)
		if err != nil || num < 1 || num > 9 {
			fmt.Println("Invalid input. Please enter a number between 1 and 9.")
			continue
		}
		idx := num - 1
		if b[idx] == "X" || b[idx] == "O" {
			fmt.Println("That space is already taken. Try again.")
			continue
		}
		return idx, nil
	}
}

func main() {
	reader := bufio.NewReader(os.Stdin)

	for {
		var b board
		for i := 0; i < 9; i++ {
			b[i] = strconv.Itoa(i + 1)
		}

		for {
			b.render()

			// Note: the render function needs to be slightly different to match requirements
			// "Make sure there is a line space before rendering a new board."
			// The prompt also shows the board rendering then the prompt.

			var idx int
			var err error
			// We need to know whose turn it is.
			// Let's count X and O to decide.

			xCount, oCount := 0, 0
			for _, cell := range b {
				if cell == "X" {
					xCount++
				} else if cell == "O" {
					oCount++
				}
			}

			if xCount <= oCount {
				idx, err = playerX(&b, reader)
			} else {
				idx, err = playerO(&b, reader)
			}

			if err != nil {
				fmt.Println("Error reading input:", err)
				return
			}

			if xCount <= oCount {
				b[idx] = "X"
			} else {
				b[idx] = "O"
			}

			if b.hasWinner("X") {
				b.render()
				fmt.Println("\nPlayer X wins!")
				break
			}
			if b.hasWinner("O") {
				b.render()
				fmt.Println("\nPlayer O wins!")
				break
			}
			if b.isDraw() {
				b.render()
				fmt.Println("\nIt's a draw!")
				break
			}
		}

		fmt.Print("\nPlay again? (y/n): ")
		choice, _ := reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(choice)) != "y" {
			break
		}
	}
}
