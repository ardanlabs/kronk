package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type Board [9]byte

var stdin = bufio.NewReader(os.Stdin)

func renderBoard(b *Board, xWins, oWins, draws int) {
	fmt.Print("\033[2J\033[H\n")
	fmt.Printf("Score: X: %d | O: %d | Draws: %d\n\n", xWins, oWins, draws)
	for row := range 3 {
		fmt.Print(" ")
		for col := range 3 {
			i := row*3 + col
			if col > 0 {
				fmt.Print(" \033[32m|\033[0m ")
			}
			switch b[i] {
			case 'X':
				fmt.Print("\033[1;31mX\033[0m")
			case 'O':
				fmt.Print("\033[1;32mO\033[0m")
			default:
				fmt.Printf("\033[37m%d\033[0m", i+1)
			}
		}
		fmt.Println()
		if row < 2 {
			fmt.Println("\033[32m-----------\033[0m")
		}
	}
	fmt.Println()
}

func hasWinner(b *Board) bool {
	for _, line := range [][3]int{{0, 1, 2}, {3, 4, 5}, {6, 7, 8}, {0, 3, 6}, {1, 4, 7}, {2, 5, 8}, {0, 4, 8}, {2, 4, 6}} {
		if b[line[0]] != 0 && b[line[0]] == b[line[1]] && b[line[1]] == b[line[2]] {
			return true
		}
	}
	return false
}

func boardFull(b *Board) bool {
	for _, c := range b {
		if c == 0 {
			return false
		}
	}
	return true
}

func move(b *Board, player byte) int {
	for {
		fmt.Print(fmt.Sprintf("Player %c's turn. Enter a number (1-9):", player))
		line, err := stdin.ReadString('\n')
		if err != nil {
			os.Exit(0)
		}
		line = strings.TrimSpace(line)
		if len(line) == 1 && line[0] >= '1' && line[0] <= '9' && b[line[0]-'1'] == 0 {
			return int(line[0] - '1')
		}
		fmt.Println("Invalid move. Enter an empty position from 1 to 9.")
	}
}

func playerX(b *Board) int { return move(b, 'X') }
func playerO(b *Board) int { return move(b, 'O') }

func main() {
	xWins, oWins, draws := 0, 0, 0
	for {
		var b Board
		player := byte('X')
		renderBoard(&b, xWins, oWins, draws)
		for {
			var index int
			if player == 'X' {
				index = playerX(&b)
			} else {
				index = playerO(&b)
			}
			b[index] = player
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
			if player == 'X' {
				player = 'O'
			} else {
				player = 'X'
			}
			renderBoard(&b, xWins, oWins, draws)
		}
		fmt.Print("Play again? (y/n): ")
		answer, _ := stdin.ReadString('\n')
		if !strings.EqualFold(strings.TrimSpace(answer), "y") {
			return
		}
	}
}
