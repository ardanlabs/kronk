package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

const (
	reset  = "\033[0m"
	green  = "\033[32m"
	white  = "\033[37m"
	red    = "\033[31m"
	blue   = "\033[34m"
	bold   = "\033[1m"
	clear  = "\033[2J"
	cursor = "\033[H"
)

var (
	board      [10]string
	currentPlayer string
	history    []move
)

type move struct {
	player string
	space  int
}

func clearScreen() {
	fmt.Print(clear)
	fmt.Print(cursor)
}

func renderBoard() {
	fmt.Println()

	row1 := formatCell(1) + " | " + formatCell(2) + " | " + formatCell(3)
	row2 := formatCell(4) + " | " + formatCell(5) + " | " + formatCell(6)
	row3 := formatCell(7) + " | " + formatCell(8) + " | " + formatCell(9)

	sep := green + "----------" + reset

	fmt.Println(row1)
	fmt.Println(sep)
	fmt.Println(row2)
	fmt.Println(sep)
	fmt.Println(row3)

	fmt.Println()
	fmt.Printf("%sPlayer %s's turn. Enter a number (1-9): %s ", bold, currentPlayer, reset)
}

func formatCell(n int) string {
	val := board[n]
	if val == "X" {
		return red + bold + val + reset
	}
	if val == "O" {
		return blue + bold + val + reset
	}
	return white + val + reset
}

func checkWinner() string {
	wins := [][3]int{
		{1, 2, 3}, {4, 5, 6}, {7, 8, 9},
		{1, 4, 7}, {2, 5, 8}, {3, 6, 9},
		{1, 5, 9}, {3, 5, 7},
	}

	for _, w := range wins {
		if board[w[0]] != " " && board[w[0]] == board[w[1]] && board[w[1]] == board[w[2]] {
			return board[w[0]]
		}
	}
	return ""
}

func isDraw() bool {
	for i := 1; i <= 9; i++ {
		if board[i] == " " {
			return false
		}
	}
	return true
}

func playerX(scanner *bufio.Scanner) int {
	for {
		line := readLine(scanner)
		line = strings.TrimSpace(line)
		num, err := strconv.Atoi(line)
		if err != nil || num < 1 || num > 9 {
			fmt.Print("Invalid input. Enter a number (1-9): ")
			continue
		}
		if board[num] != " " {
			fmt.Print("Space taken. Enter a number (1-9): ")
			continue
		}
		return num
	}
}

func playerO(scanner *bufio.Scanner) int {
	for {
		line := readLine(scanner)
		line = strings.TrimSpace(line)
		num, err := strconv.Atoi(line)
		if err != nil || num < 1 || num > 9 {
			fmt.Print("Invalid input. Enter a number (1-9): ")
			continue
		}
		if board[num] != " " {
			fmt.Print("Space taken. Enter a number (1-9): ")
			continue
		}
		return num
	}
}

func readLine(scanner *bufio.Scanner) string {
	scanner.Scan()
	return scanner.Text()
}

func initBoard() {
	for i := 1; i <= 9; i++ {
		board[i] = strconv.Itoa(i)
	}
}

func announceWinner(winner string) {
	fmt.Printf("\n%s*** Player %s wins! ***%s\n\n", bold+green, winner, reset)
}

func announceDraw() {
	fmt.Printf("\n%s*** It's a draw! ***%s\n\n", bold+yellow, reset)
}

func playGame() {
	initBoard()
	history = nil

	if len(history) == 0 {
		currentPlayer = "X"
	}

	for {
		clearScreen()
		renderBoard()

		scanner := bufio.NewScanner(os.Stdin)
		var space int
		if currentPlayer == "X" {
			space = playerX(scanner)
		} else {
			space = playerO(scanner)
		}

		board[space] = currentPlayer
		history = append(history, move{player: currentPlayer, space: space})

		winner := checkWinner()
		if winner != "" {
			clearScreen()
			renderBoard()
			announceWinner(winner)
			return
		}

		if isDraw() {
			clearScreen()
			renderBoard()
			announceDraw()
			return
		}

		if currentPlayer == "X" {
			currentPlayer = "O"
		} else {
			currentPlayer = "X"
		}
	}
}

func main() {
	clearScreen()

	fmt.Println(bold + green)
	fmt.Println("  ================================")
	fmt.Println("  |     Welcome to Tic-Tac-Toe!    |")
	fmt.Println("  ================================")
	fmt.Println(reset)
	fmt.Println("Player 1 will use X.")
	fmt.Println("Player 2 will use O.")
	fmt.Println()
	fmt.Print("Press Enter to start... ")
	bufio.NewScanner(os.Stdin).Scan()

	for {
		playGame()

		clearScreen()
		fmt.Print("Play again? (y/n): ")
		answer := bufio.NewScanner(os.Stdin).Scan()
		_ = answer
		fmt.Println()

		if strings.ToLower(bufio.NewScanner(os.Stdin).Text()) != "y" {
			fmt.Println("Thanks for playing!")
			break
		}

		currentPlayer = "X"
	}
}