//

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// modelSource is the model to download. It may be a HuggingFace URL,
// a canonical "provider/modelID", or a bare model id.
var modelSource = "unsloth/Qwen3.6-35B-A3B-UD-Q8_K_XL"

func main() {
	if err := run(); err != nil {
		fmt.Printf("\nERROR: %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	mp, err := installSystem()

	if err != nil {
		return fmt.Errorf("run: unable to install system: %w", err)
	}

	krn, err := newKronk(mp)

	if err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}

	defer func() {
		fmt.Println("\nUnloading Kronk")
		if err := krn.Unload(context.Background()); err != nil {
			fmt.Printf("run: failed to unload model: %v", err)
		}
	}()

	if err := chat(krn); err != nil {
		return err
	}

	return nil
}

func installSystem() (models.Path, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	libMgr, err := libs.New(
		libs.WithVersion(defaults.LibVersion("")),
	)
	if err != nil {
		return models.Path{}, err
	}

	if _, err := libMgr.Download(ctx, kronk.FmtLogger); err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	mdls, err := models.New()

	if err != nil {
		return models.Path{}, fmt.Errorf("unable to init models: %w", err)
	}

	fmt.Println("Downloading model:", modelSource)

	mp, err := mdls.Download(ctx, kronk.FmtLogger, modelSource)

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

	krn, err := kronk.New(
		model.WithModelFiles(mp.ModelFiles),
		model.WithContextWindow(131072),
	)

	if err != nil {
		return nil, fmt.Errorf("unable to create inference model: %w", err)
	}

	fmt.Print("- system info:\n\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}

	fmt.Println()
	fmt.Println("- contextWindow  :", krn.ModelConfig().ContextWindow())
	fmt.Printf("- k/v            : %s/%s\n", krn.ModelConfig().CacheTypeK, krn.ModelConfig().CacheTypeV)
	fmt.Println("- flashAttention :", krn.ModelConfig().FlashAttention)
	fmt.Println("- nBatch         :", krn.ModelConfig().NBatch())
	fmt.Println("- nuBatch        :", krn.ModelConfig().NUBatch())
	fmt.Println("- modelType      :", krn.ModelInfo().Type)
	fmt.Println("- isGPT          :", krn.ModelInfo().IsGPTModel)
	fmt.Println("- template       :", krn.ModelInfo().Template.FileName)
	fmt.Println("- grammar        :", krn.ModelConfig().DefaultParams.Grammar != "")
	fmt.Println("- nSeqMax        :", krn.ModelConfig().NSeqMax())
	fmt.Println("- vramTotal      :", krn.ModelInfo().VRAMTotal/(1024*1024), "MiB")
	fmt.Println("- slotMemory     :", krn.ModelInfo().SlotMemory/(1024*1024), "MiB")
	fmt.Println("- modelSize      :", krn.ModelInfo().Size/(1000*1000), "MB")
	fmt.Println("- imc            :", krn.ModelConfig().IncrementalCache())

	if n := krn.ModelConfig().PtrNGpuLayers; n != nil {
		fmt.Println("- nGPULayers     :", *n)
	} else {
		fmt.Println("- nGPULayers     : all")
	}

	return krn, nil
}

func chat(krn *kronk.Kronk) error {
	messages := model.DocumentArray()

	var systemPrompt = `You will be given a large computer program and asked questions
	about the code in that program. We want to see how well you can return
	exact code blocks that you see.`

	// WRITE SOME CODE / FUNCTION READS THE FILE
	code, err := os.ReadFile("kaleah/code.chunk")
	if err != nil {
		return fmt.Errorf("chat: read code.chunk: %w", err)
	}

	messages = append(messages,
		model.TextMessage(model.RoleSystem, systemPrompt),
		// USER MESSAGE WITH THE CONTEXT OF THE FILE
		model.TextMessage("user", "Here is the code to analyze:\n\n"+string(code)),
	)

	for {
		var err error
		messages, err = userInput(messages)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("run:user input: %w", err)
		}

		messages, err = func() ([]model.D, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()

			d := model.D{
				"messages":        messages,
				"max_tokens":      4096,
				"enable_thinking": false,
			}

			ch, err := performChat(ctx, krn, d)

			if err != nil {
				return nil, fmt.Errorf("run: unable to perform chat: %w", err)
			}

			messages, err = modelResponse(krn, messages, ch)

			if err != nil {
				return nil, fmt.Errorf("run: model response: %w", err)
			}

			return messages, nil
		}()

		if err != nil {
			return fmt.Errorf("run: unable to perform chat: %w", err)
		}
	}
}

func userInput(messages []model.D) ([]model.D, error) {
	fmt.Print("\nUSER> ")

	reader := bufio.NewReader(os.Stdin)

	userInput, err := reader.ReadString('\n')

	if err != nil {
		return messages, fmt.Errorf("unable to read user input: %w", err)
	}

	if strings.TrimSpace(userInput) == "quit" || userInput == "quit\n" {
		return nil, io.EOF
	}

	messages = append(messages,
		model.TextMessage(model.RoleUser, userInput),
	)

	return messages, nil
}

func performChat(ctx context.Context, krn *kronk.Kronk, d model.D) (<-chan model.ChatResponse, error) {
	ch, err := krn.ChatStreaming(ctx, d)

	if err != nil {
		return nil, fmt.Errorf("chat streaming: %w", err)
	}

	return ch, nil
}

func modelResponse(krn *kronk.Kronk, messages []model.D, ch <-chan model.ChatResponse) ([]model.D, error) {
	fmt.Print("\nMODEL> ")

	var lr model.ChatResponse
	var content strings.Builder
	var reasoning strings.Builder

loop:
	for resp := range ch {
		lr = resp

		if len(resp.Choices) == 0 {
			continue
		}

		switch resp.Choices[0].FinishReason() {
		case model.FinishReasonError:
			return messages, fmt.Errorf("error from model: %s", resp.Choices[0].Delta.Content)
		case model.FinishReasonStop:
			break loop
		default:
			if resp.Choices[0].Delta.Reasoning != "" {
				reasoning.WriteString(resp.Choices[0].Delta.Reasoning)
				fmt.Printf("\033[91m%s\033[0m", resp.Choices[0].Delta.Reasoning)
				continue
			}

			content.WriteString(resp.Choices[0].Delta.Content)

			fmt.Printf("%s", resp.Choices[0].Delta.Content)
		}
	}

	if content.Len() > 0 {
		messages = append(messages, model.TextMessage(model.RoleAssistant, content.String()))
	}

	fmt.Printf("\n\033[90mTokens: %d input, %d output | TPS: %.2f\033[0m\n",
		lr.Usage.PromptTokens, lr.Usage.CompletionTokens, lr.Usage.TokensPerSecond)
	return messages, nil
}
