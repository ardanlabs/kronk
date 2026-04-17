package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

const modelURL = "https://huggingface.co/unsloth/gemma-4-26B-A4B-it-GGUF/resolve/main/gemma-4-26B-A4B-it-UD-Q8_K_XL.gguf"

func main() {
	if err := run(); err != nil {
		fmt.Printf("\nERROR: %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	mp, err := installSystem()
	if err != nil {
		return fmt.Errorf("unable to installation system: %w", err)
	}

	krn, err := newKronk(mp)
	if err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}

	defer func() {
		fmt.Println("\nUnloading Kronk")
		if err := krn.Unload(context.Background()); err != nil {
			fmt.Printf("failed to unload model: %v", err)
		}
	}()

	return runGame(krn)
}

func installSystem() (models.Path, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	libs, err := libs.New(
		libs.WithVersion(defaults.LibVersion("")),
	)
	if err != nil {
		return models.Path{}, err
	}

	if _, err := libs.Download(ctx, kronk.FmtLogger); err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	// -------------------------------------------------------------------------

	mdls, err := models.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	mp, err := mdls.Download(ctx, kronk.FmtLogger, modelURL, "")
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install model: %w", err)
	}

	return mp, nil
}

func newKronk(mp models.Path) (*kronk.Kronk, error) {
	fmt.Println("loading model...")

	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	cfg := model.Config{
		ContextWindow: 131072,
		ModelFiles:    mp.ModelFiles,
	}

	krn, err := kronk.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("unable to create inference model: %w", err)
	}

	fmt.Print("- system info:\n\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	fmt.Println("- contextWindow  :", krn.ModelConfig().ContextWindow)
	fmt.Printf("- k/v            : %s/%s\n", krn.ModelConfig().CacheTypeK, krn.ModelConfig().CacheTypeV)
	fmt.Println("- flashAttention :", krn.ModelConfig().FlashAttention)
	fmt.Println("- nBatch         :", krn.ModelConfig().NBatch)
	fmt.Println("- nuBatch        :", krn.ModelConfig().NUBatch)
	fmt.Println("- modelType      :", krn.ModelInfo().Type)
	fmt.Println("- isGPT          :", krn.ModelInfo().IsGPTModel)
	fmt.Println("- template       :", krn.ModelInfo().Template.FileName)
	fmt.Println("- grammar        :", krn.ModelConfig().DefaultParams.Grammar != "")
	fmt.Println("- nSeqMax        :", krn.ModelConfig().NSeqMax)
	fmt.Println("- vramTotal      :", krn.ModelInfo().VRAMTotal/(1024*1024), "MiB")
	fmt.Println("- slotMemory     :", krn.ModelInfo().SlotMemory/(1024*1024), "MiB")
	fmt.Println("- modelSize      :", krn.ModelInfo().Size/(1000*1000), "MB")
	fmt.Println("- spc            :", krn.ModelConfig().SystemPromptCache)
	fmt.Println("- imc            :", krn.ModelConfig().IncrementalCache)
	if n := krn.ModelConfig().NGpuLayers; n != nil {
		fmt.Println("- nGPULayers     :", *n)
	} else {
		fmt.Println("- nGPULayers     : all")
	}

	return krn, nil
}

func runGame(krn *kronk.Kronk) error {
	reader := bufio.NewReader(os.Stdin)

	for {
		var b board
		for i := 0; i < 9; i++ {
			b[i] = strconv.Itoa(i + 1)
		}

		for {
			b.render()

			var idx int
			var err error

			xCount, oCount := 0, 0
			for _, cell := range b {
				switch cell {
				case "X":
					xCount++
				case "O":
					oCount++
				}
			}

			if xCount <= oCount {
				idx, err = playerX(&b, reader)
			} else {
				idx, err = playerO(&b, krn)
			}

			if err != nil {
				return fmt.Errorf("error reading input: %w", err)
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

	return nil
}

// =============================================================================

type board [9]string

func (b *board) render() {
	fmt.Println()
	fmt.Printf("%s | %s | %s\n", b[0], b[1], b[2])
	fmt.Println("----------")
	fmt.Printf("%s | %s | %s\n", b[3], b[4], b[5])
	fmt.Println("----------")
	fmt.Printf("%s | %s | %s\n", b[6], b[7], b[8])
}

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

// =============================================================================

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

func playerO(b *board, krn *kronk.Kronk) (int, error) {
	for {
		fmt.Print("\nPlayer O's turn. Enter a number (1-9): ")
		input, err := PickSpace(b, krn)
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

func PickSpace(b *board, krn *kronk.Kronk) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*5*time.Second)
	defer cancel()

	var xSpaces []int
	var oSpaces []int
	var aSpaces []int

	for i := range b {
		switch b[i] {
		case "X":
			xSpaces = append(xSpaces, i+1)
		case "O":
			oSpaces = append(oSpaces, i+1)
		default:
			aSpaces = append(aSpaces, i+1)
		}
	}

	finalPrompt := fmt.Sprintf(prompt, xSpaces, oSpaces, aSpaces)

	// -------------------------------------------------------------------------

	final, err := modelNonStreaming(ctx, krn, finalPrompt)
	if err != nil {
		return "", err
	}

	// -------------------------------------------------------------------------

	var resp struct {
		Space int `json:"space"`
	}

	if err := json.Unmarshal([]byte(final), &resp); err != nil {
		return "", fmt.Errorf("unmarshal: %s: %w", final, err)
	}

	return fmt.Sprintf("%d", resp.Space), nil
}

func modelNonStreaming(ctx context.Context, krn *kronk.Kronk, finalPrompt string) (string, error) {
	schema := model.D{
		"type": "object",
		"properties": model.D{
			"space": model.D{
				"type": "integer",
			},
		},
		"required": []string{"space"},
	}

	d := model.D{
		"messages": model.DocumentArray(
			model.TextMessage(model.RoleSystem, systemPrompt),
			model.TextMessage(model.RoleUser, finalPrompt),
		),
		"enable_thinking": false,
		"json_schema":     schema,
		"temperature":     1.0,
		"top_p":           0.95,
		"top_k":           64,
	}

	fmt.Println("\n\nModel thinking...")

	mdlResp, err := krn.Chat(ctx, d)
	if err != nil {
		return "", fmt.Errorf("chat streaming: %w", err)
	}

	fmt.Printf("Model response:\n%s\n", mdlResp.Choices[0].Message.Content)

	return mdlResp.Choices[0].Message.Content, nil
}

const systemPrompt = `
Direct answer only. Include only the absolute minimum reasoning necessary to
justify your response. Avoid all preamble, postamble, and non-essential explanation.

This is the JSON document you will be returning:

{"space","CHOSEN_SPACE"}

Only return a JSON document as your answer. Do not send anything else but the
JSON document.
`

const prompt = `
You are playing a game of Tic-Tac-Toe. You need to pick a space by selecting
a number from 1 through 9. This is what the game board looks like.

1 | 2 | 3
----------
4 | 5 | 6
----------
7 | 8 | 9

You can see how each number coresponds to a different space.

You are player2 which uses the "O" marker.

Player1 is current occupying spaces %v and You are currently occupying
spaces %v. The available spaces are %v.

Please choose a space from the available spaces list that you think gives you
the best chance to win.

You will return the space number you select using this JSON document format
provided in the system prompt.
`
