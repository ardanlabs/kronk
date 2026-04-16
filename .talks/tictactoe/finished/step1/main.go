package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func main() {
	for {
		playGame()
		fmt.Print("Play again? (y/n): ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			input := strings.ToLower(strings.TrimSpace(scanner.Text()))
			if input != "y" {
				break
			}
		} else {
			break
		}
	}
}

func playGame() {
	board := [9]string{"1", "2", "3", "4", "5", "6", "7", "8", "9"}
	currentPlayer := "X"

	for {
		printBoard(board)

		var move int
		var err error

		if currentPlayer == "X" {
			move, err = playerX()
		} else {
			move, err = playerO()
		}

		if err != nil {
			fmt.Printf("Invalid move: %v. Try again.\n", err)
			continue
		}

		// Validate move
		index := move - 1
		if index < 0 || index > 8 || board[index] == "X" || board[index] == "O" {
			fmt.Printf("Invalid move: %d is not a valid empty space. Try again.\n", move)
			continue
		}

		board[index] = currentPlayer

		if checkWinner(board, currentPlayer) {
			printBoard(board)
			fmt.Printf("Player %s wins!\n", currentPlayer)
			return
		}

		if isDraw(board) {
			printBoard(board)
			fmt.Println("It's a draw!")
			return
		}

		if currentPlayer == "X" {
			currentPlayer = "O"
		} else {
			currentPlayer = "X"
		}
	}
}

func printBoard(board [9]string) {
	fmt.Printf("%s | %s | %s\n", board[0], board[1], board[2])
	fmt.Println("----------")
	fmt.Printf("%s | %s | %s\n", board[3], board[4], board[5])
	fmt.Println("----------")
	fmt.Printf("%s | %s | %s\n", board[6], board[7], board[8])
}

func playerX() (int, error) {
	return getMove("Player X's turn. Enter a number (1-9): ")
}

func playerO() (int, error) {
	return getMove("Player O's turn. Enter a number (1-9): ")
}

func getMove(prompt string) (int, error) {
	fmt.Print(prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		move, err := strconv.Atoi(input)
		if err != nil {
			return 0, fmt.Errorf("please enter a number")
		}
		return move, nil
	}
	return 0, fmt.Errorf("failed to read input")
}

func checkWinner(board [9]string, player string) bool {
	wins := [8][3]int{
		{0, 1, 2}, {3, 4, 5}, {6, 7, 8}, // rows
		{0, 3, 6}, {1, 4, 7}, {2, 5, 8}, // cols
		{0, 4, 8}, {2, 4, 6}, // diagonals
	}

	for _, win := range wins {
		if board[win[0]] == player && board[win[1]] == player && board[win[2]] == player {
			return true
		}
	}
	return false
}

func isDraw(board [9]string) bool {
	for _, cell := range board {
		if cell != "X" && cell != "O" {
			return false
		}
	}
	return true
}
