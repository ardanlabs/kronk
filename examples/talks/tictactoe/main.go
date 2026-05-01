package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const (
	red    = "\033[31m"
	green  = "\033[32m"
	blue   = "\033[34m"
	reset  = "\033[0m"
	clear  = "\033[2J\033[H"
)

type Board [9]rune

func NewBoard() Board {
	var b Board
	for i := range b {
		b[i] = rune('1' + i)
	}
	return b
}

func (b *Board) Display(playerX, playerO string) {
	fmt.Println()
	fmt.Printf("%s%s\n", clear, green)

	fmt.Printf("%s1%s | %s2%s | %s3%s\n", green, reset, green, reset, green, reset)
	fmt.Println("----------")
	fmt.Printf("%s4%s | %s5%s | %s6%s\n", green, reset, green, reset, green, reset)
	fmt.Println("----------")
	fmt.Printf("%s7%s | %s8%s | %s9%s\n", green, reset, green, reset, green, reset)

	fmt.Println()
	fmt.Printf("Board:\n")
	fmt.Printf("%s | %s | %s\n",
		displayCell(b[0], playerX, playerO),
		displayCell(b[1], playerX, playerO),
		displayCell(b[2], playerX, playerO))
	fmt.Println("----------")
	fmt.Printf("%s | %s | %s\n",
		displayCell(b[3], playerX, playerO),
		displayCell(b[4], playerX, playerO),
		displayCell(b[5], playerX, playerO))
	fmt.Println("----------")
	fmt.Printf("%s | %s | %s\n",
		displayCell(b[6], playerX, playerO),
		displayCell(b[7], playerX, playerO),
		displayCell(b[8], playerX, playerO))
	fmt.Print(reset)
}

func displayCell(cell rune, playerX, playerO string) string {
	if cell == 'X' {
		return fmt.Sprintf("%s%s%s", red, playerX, reset)
	}
	if cell == 'O' {
		return fmt.Sprintf("%s%s%s", blue, playerO, reset)
	}
	return fmt.Sprintf("%s%d%s", green, cell, reset)
}

func (b *Board) IsFull() bool {
	for _, c := range b {
		if c >= '1' && c <= '9' {
			return false
		}
	}
	return true
}

func (b *Board) CheckWinner() rune {
	wins := [][3]int{
		{0, 1, 2}, {3, 4, 5}, {6, 7, 8},
		{0, 3, 6}, {1, 4, 7}, {2, 5, 8},
		{0, 4, 8}, {2, 4, 6},
	}
	for _, w := range wins {
		if b[w[0]] != ' ' && b[w[0]] == b[w[1]] && b[w[1]] == b[w[2]] {
			return b[w[0]]
		}
	}
	return ' '
}

func (b *Board) Move(pos int, symbol rune) bool {
	if pos < 1 || pos > 9 {
		return false
	}
	if b[pos-1] >= '1' && b[pos-1] <= '9' {
		b[pos-1] = symbol
		return true
	}
	return false
}

func getPlayerInput(prompt string) (int, error) {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return 0, err
	}
	input = strings.TrimSpace(input)
	return strconv.Atoi(input)
}

func playerX(b *Board, name string) (int, error) {
	for {
		fmt.Printf("\n%s%s's turn. Enter a number (1-9): %s", red, name, reset)
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return 0, err
		}
		input = strings.TrimSpace(input)
		pos, err := strconv.Atoi(input)
		if err != nil || pos < 1 || pos > 9 {
			fmt.Println("Invalid input. Please enter a number between 1 and 9.")
			continue
		}
		if b[pos-1] >= '1' && b[pos-1] <= '9' {
			return pos, nil
		}
		fmt.Println("Space already taken. Choose another number.")
	}
}

func playerO(b *Board, name string) (int, error) {
	for {
		fmt.Printf("\n%s%s's turn. Enter a number (1-9): %s", blue, name, reset)
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return 0, err
		}
		input = strings.TrimSpace(input)
		pos, err := strconv.Atoi(input)
		if err != nil || pos < 1 || pos > 9 {
			fmt.Println("Invalid input. Please enter a number between 1 and 9.")
			continue
		}
		if b[pos-1] >= '1' && b[pos-1] <= '9' {
			return pos, nil
		}
		fmt.Println("Space already taken. Choose another number.")
	}
}

func clearScreen() {
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()
}

func playAgain() bool {
	fmt.Print("\nPlay again? (y/n): ")
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))
	return input == "y" || input == "yes"
}

func main() {
	// Set terminal to raw mode
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error setting terminal: %v\n", err)
		os.Exit(1)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	var player1Name, player2Name string

	fmt.Print("Enter Player X name: ")
	reader := bufio.NewReader(os.Stdin)
	player1Name, _ = reader.ReadString('\n')
	player1Name = strings.TrimSpace(player1Name)
	if player1Name == "" {
		player1Name = "Player X"
	}

	fmt.Print("Enter Player O name: ")
	player2Name, _ = reader.ReadString('\n')
	player2Name = strings.TrimSpace(player2Name)
	if player2Name == "" {
		player2Name = "Player O"
	}

	for {
		clearScreen()
		fmt.Printf("\n%sWelcome to Tic-Tac-Toe!%s\n", green, reset)
		fmt.Printf("%s vs %s\n\n", player1Name, player2Name)

		board := NewBoard()
		var winner rune

		for {
			board.Display(player1Name, player2Name)

			var pos int

			// Player X's turn
			pos, err = playerX(&board, player1Name)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
				return
			}
			board[pos-1] = 'X'

			winner = board.CheckWinner()
			if winner != ' ' || board.IsFull() {
				break
			}

			// Player O's turn
			pos, err = playerO(&board, player2Name)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
				return
			}
			board[pos-1] = 'O'

			winner = board.CheckWinner()
			if winner != ' ' || board.IsFull() {
				break
			}
		}

		clearScreen()
		board.Display(player1Name, player2Name)

		if winner == 'X' {
			fmt.Printf("\n%s%s wins!%s\n", red, player1Name, reset)
		} else if winner == 'O' {
			fmt.Printf("\n%s%s wins!%s\n", blue, player2Name, reset)
		} else {
			fmt.Printf("\n%sIt's a draw!%s\n", green, reset)
		}

		if !playAgain() {
			fmt.Println("\nThanks for playing!")
			break
		}
	}
}