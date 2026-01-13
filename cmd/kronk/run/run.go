package run

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/ardanlabs/kronk/sdk/tools/templates"
)

type Config struct {
	ModelName   string
	Instances   int
	MaxTokens   int
	Temperature float64
	TopP        float64
	TopK        int
	BasePath    string
}

func runChat(cfg Config) error {
	mp, err := installSystem(cfg)
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
			fmt.Printf("run: failed to unload model: %v\n", err)
		}
	}()

	return chat(krn, cfg)
}

func installSystem(cfg Config) (models.Path, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	lbs, err := libs.New()
	if err != nil {
		return models.Path{}, err
	}

	if _, err := lbs.Download(ctx, kronk.FmtLogger); err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	tmpls, err := templates.New(templates.WithBasePath(cfg.BasePath))
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to create template system: %w", err)
	}

	if err := tmpls.Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download templates: %w", err)
	}

	if err := tmpls.Catalog().Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download catalog: %w", err)
	}

	mdls, err := models.NewWithPaths(cfg.BasePath)
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to create models system: %w", err)
	}

	mp, err := mdls.RetrievePath(cfg.ModelName)
	if err != nil {
		return models.Path{}, fmt.Errorf("model %q not found - use 'kronk model pull' first: %w", cfg.ModelName, err)
	}

	return mp, nil
}

func newKronk(mp models.Path) (*kronk.Kronk, error) {
	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	krn, err := kronk.New(model.Config{
		ModelFiles: mp.ModelFiles,
	})

	if err != nil {
		return nil, fmt.Errorf("unable to create inference model: %w", err)
	}

	fmt.Print("- system info:\n\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	fmt.Println("- contextWindow:", krn.ModelConfig().ContextWindow)
	fmt.Println("- embeddings   :", krn.ModelInfo().IsEmbedModel)
	fmt.Println("- isGPT        :", krn.ModelInfo().IsGPTModel)
	fmt.Println("- template     :", krn.ModelInfo().Template.FileName)

	return krn, nil
}

func chat(krn *kronk.Kronk, cfg Config) error {
	messages := model.DocumentArray()

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
				"messages":    messages,
				"max_tokens":  cfg.MaxTokens,
				"temperature": cfg.Temperature,
				"top_p":       cfg.TopP,
				"top_k":       cfg.TopK,
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

	if userInput == "quit\n" {
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

	var reasoning bool
	var lr model.ChatResponse

loop:
	for resp := range ch {
		lr = resp

		switch resp.Choice[0].FinishReason {
		case model.FinishReasonError:
			return messages, fmt.Errorf("error from model: %s", resp.Choice[0].Delta.Content)

		case model.FinishReasonStop:
			messages = append(messages,
				model.TextMessage("assistant", resp.Choice[0].Delta.Content),
			)
			break loop

		case model.FinishReasonTool:
			fmt.Println()
			if krn.ModelInfo().IsGPTModel {
				fmt.Println()
			}

			fmt.Printf("\u001b[92mModel Asking For Tool Calls:\n\u001b[0m")

			for _, tool := range resp.Choice[0].Delta.ToolCalls {
				fmt.Printf("\u001b[92mToolID[%s]: %s(%s)\n\u001b[0m",
					tool.ID,
					tool.Function.Name,
					tool.Function.Arguments,
				)

				messages = append(messages,
					model.TextMessage("tool", fmt.Sprintf("Tool call %s: %s(%v)\n",
						tool.ID,
						tool.Function.Name,
						tool.Function.Arguments),
					),
				)
			}

			break loop

		default:
			if resp.Choice[0].Delta.Reasoning != "" {
				fmt.Printf("\u001b[91m%s\u001b[0m", resp.Choice[0].Delta.Reasoning)
				reasoning = true
				continue
			}

			if reasoning {
				reasoning = false

				fmt.Println()
				if krn.ModelInfo().IsGPTModel {
					fmt.Println()
				}
			}

			fmt.Printf("%s", resp.Choice[0].Delta.Content)
		}
	}

	contextTokens := lr.Usage.PromptTokens + lr.Usage.CompletionTokens
	contextWindow := krn.ModelConfig().ContextWindow
	percentage := (float64(contextTokens) / float64(contextWindow)) * 100
	of := float32(contextWindow) / float32(1024)

	fmt.Printf("\n\n\u001b[90mInput: %d  Reasoning: %d  Completion: %d  Output: %d  Window: %d (%.0f%% of %.0fK) TPS: %.2f\u001b[0m\n",
		lr.Usage.PromptTokens, lr.Usage.ReasoningTokens, lr.Usage.CompletionTokens, lr.Usage.OutputTokens, contextTokens, percentage, of, lr.Usage.TokensPerSecond)

	return messages, nil
}
