package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	reset = "\033[0m"
	green = "\033[32m"
	red   = "\033[31m"
	blue  = "\033[34m"
	clear = "\033[2J\033[H"
)

type Board struct {
	spaces [9]int
}

func NewBoard() *Board {
	b := &Board{}
	for i := range b.spaces {
		b.spaces[i] = i + 1
	}
	return b
}

func (b *Board) Display() {
	fmt.Println()
	fmt.Println(clear)

	for i := 0; i < 9; i += 3 {
		row := ""
		for j := 0; j < 3; j++ {
			idx := i + j
			val := b.spaces[idx]
			if val == 'X' {
				row += red + "X" + reset
			} else if val == 'O' {
				row += blue + "O" + reset
			} else {
				row += strconv.Itoa(val)
			}
			if j < 2 {
				row += " | "
			}
		}
		fmt.Println(row)
		if i < 6 {
			fmt.Println(green + "----------" + reset)
		}
	}
}

func (b *Board) IsAvailable(n int) bool {
	return b.spaces[n-1] >= 1 && b.spaces[n-1] <= 9
}

func (b *Board) Mark(n int, mark byte) {
	b.spaces[n-1] = int(mark)
}

func (b *Board) IsFull() bool {
	for _, s := range b.spaces {
		if s >= 1 && s <= 9 {
			return false
		}
	}
	return true
}

func (b *Board) CheckWinner() byte {
	wins := [][3]int{
		{0, 1, 2}, {3, 4, 5}, {6, 7, 8},
		{0, 3, 6}, {1, 4, 7}, {2, 5, 8},
		{0, 4, 8}, {2, 4, 6},
	}
	for _, w := range wins {
		if b.spaces[w[0]] == b.spaces[w[1]] && b.spaces[w[1]] == b.spaces[w[2]] {
			m := byte(b.spaces[w[0]])
			if m == 'X' || m == 'O' {
				return m
			}
		}
	}
	return 0
}

func playerX(b *Board) int {
	fmt.Printf("\n%sPlayer X's turn. Enter a number (1-9): ", red)
	return getMove(b)
}

func playerO(b *Board) int {
	fmt.Printf("\n%sPlayer O's turn. Enter a number (1-9): ", blue)
	return getMove(b)
}

func getMove(b *Board) int {
	scanner := bufio.NewScanner(os.Stdin)
	for {
		scanner.Scan()
		input := strings.TrimSpace(scanner.Text())
		n, err := strconv.Atoi(input)
		if err != nil || n < 1 || n > 9 {
			fmt.Print("Invalid input. Enter a number (1-9): ")
			continue
		}
		if !b.IsAvailable(n) {
			fmt.Print("Space taken. Enter a number (1-9): ")
			continue
		}
		return n
	}
}

func playAgain() bool {
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("\nPlay again? (y/n): ")
		scanner.Scan()
		resp := strings.ToLower(strings.TrimSpace(scanner.Text()))
		if resp == "y" || resp == "yes" {
			return true
		}
		if resp == "n" || resp == "no" {
			fmt.Println("\nThanks for playing!")
			return false
		}
		fmt.Print("Please enter y or n: ")
	}
}

func main() {
	for {
		b := NewBoard()
		current := byte('X')

		for {
			b.Display()

			var move int
			if current == 'X' {
				move = playerX(b)
			} else {
				move = playerO(b)
			}

			b.Mark(move, current)

			winner := b.CheckWinner()
			if winner != 0 {
				b.Display()
				if winner == 'X' {
					fmt.Println(red + "Player X wins!" + reset)
				} else {
					fmt.Println(blue + "Player O wins!" + reset)
				}
				break
			}

			if b.IsFull() {
				b.Display()
				fmt.Println(green + "It's a draw!" + reset)
				break
			}

			if current == 'X' {
				current = 'O'
			} else {
				current = 'X'
			}
		}

		if !playAgain() {
			break
		}
	}
}
