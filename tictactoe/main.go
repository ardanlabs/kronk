package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type move struct {
	cell  int
	value string
}

type Board struct {
	cells   [9]string
	history []move
}

func NewBoard() *Board {
	b := &Board{}
	for i := range b.cells {
		b.cells[i] = strconv.Itoa(i + 1)
	}
	return b
}

func (b *Board) Place(cell int, value string) {
	b.history = append(b.history, move{cell: cell, value: b.cells[cell]})
	b.cells[cell] = value
}

func (b *Board) Undo() bool {
	if len(b.history) == 0 {
		return false
	}
	last := b.history[len(b.history)-1]
	b.cells[last.cell] = last.value
	b.history = b.history[:len(b.history)-1]
	return true
}

func (b *Board) Render() {
	fmt.Print("\033[2J\033[H")
	fmt.Println()
	fmt.Printf("Score: X: %d | O: %d | Draws: %d\n", scores[0], scores[1], scores[2])
	fmt.Println()

	sep := "\033[32m-----------\033[0m"

	for row := range 3 {
		fmt.Print(" ")
		for col := range 3 {
			idx := row*3 + col
			val := b.cells[idx]
			switch val {
			case "X":
				fmt.Print("\033[1;31mX\033[0m")
			case "O":
				fmt.Print("\033[1;32mO\033[0m")
			default:
				fmt.Print("\033[37m" + val + "\033[0m")
			}
			if col < 2 {
				fmt.Print(" \033[32m|\033[0m ")
			}
		}
		fmt.Println()
		if row < 2 {
			fmt.Println(sep)
		}
	}
	fmt.Println()
}

var scores [3]int // X wins, O wins, draws

func playerX(b *Board) int {
	for {
		fmt.Print("Player X's turn. Enter a number (1-9), 0 to undo: ")
		line := readLine()
		idx, ok := parseMove(line)
		if !ok {
			fmt.Println("Invalid move. Try again.")
			continue
		}
		if idx == -1 {
			if b.Undo() {
				b.Render()
				fmt.Print("Player X's turn. Enter a number (1-9), 0 to undo: ")
			}
			continue
		}
		if idx < 0 || idx > 8 || b.cells[idx] != strconv.Itoa(idx+1) {
			fmt.Println("Invalid move. Try again.")
			continue
		}
		return idx
	}
}

func playerO(b *Board) int {
	for {
		fmt.Print("Player O's turn. Enter a number (1-9), 0 to undo: ")
		line := readLine()
		idx, ok := parseMove(line)
		if !ok {
			fmt.Println("Invalid move. Try again.")
			continue
		}
		if idx == -1 {
			if b.Undo() {
				b.Render()
				fmt.Print("Player O's turn. Enter a number (1-9), 0 to undo: ")
			}
			continue
		}
		if idx < 0 || idx > 8 || b.cells[idx] != strconv.Itoa(idx+1) {
			fmt.Println("Invalid move. Try again.")
			continue
		}
		return idx
	}
}

var reader = bufio.NewReader(os.Stdin)

func readLine() string {
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

func parseMove(s string) (int, bool) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n - 1, true
}

var winLines = [][3]int{
	{0, 1, 2}, {3, 4, 5}, {6, 7, 8},
	{0, 3, 6}, {1, 4, 7}, {2, 5, 8},
	{0, 4, 8}, {2, 4, 6},
}

func checkWin(cells [9]string) string {
	for _, line := range winLines {
		a, b, c := cells[line[0]], cells[line[1]], cells[line[2]]
		if a != " " && a == b && b == c {
			return a
		}
	}
	return ""
}

func isFull(cells [9]string) bool {
	for _, c := range cells {
		n, err := strconv.Atoi(c)
		if err != nil || n < 1 || n > 9 {
			return false
		}
	}
	return true
}

func main() {
	b := NewBoard()
	current := "X"
	for {
		for {
			b.Render()

			var idx int
			switch current {
			case "X":
				idx = playerX(b)
			default:
				idx = playerO(b)
			}

			b.Place(idx, current)

			winner := checkWin(b.cells)
			if winner != "" {
				if winner == "X" {
					scores[0]++
				} else {
					scores[1]++
				}
				b.Render()
				fmt.Printf("Player %s wins!\n", winner)
				break
			}

			if isFull(b.cells) {
				scores[2]++
				b.Render()
				fmt.Println("It's a draw.")
				break
			}

			b.Render()
			fmt.Print("Undo? (y/n): ")
			line := readLine()
			if strings.ToLower(line) == "y" {
				b.Undo()
				if current == "X" {
					current = "O"
				} else {
					current = "X"
				}
				continue
			}

			if current == "X" {
				current = "O"
			} else {
				current = "X"
			}
		}

		fmt.Print("Play again? (y/n): ")
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line != "y" {
			break
		}
	}
}
