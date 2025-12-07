// This example shows you how to create a simple chat application against an
// inference model using kronk. Thanks to Kronk and yzma, reasoning and tool
// calling is enabled.
//
// The first time you run this program the system will download and install
// the model and libraries.
//
// Run the example like this from the root of the project:
// $ make example-chat

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"

	"github.com/ardanlabs/kronk"
	"github.com/ardanlabs/kronk/defaults"
	"github.com/ardanlabs/kronk/model"
	"github.com/ardanlabs/kronk/tools"
	"github.com/hybridgroup/yzma/pkg/download"
)

const (
	modelURL = "https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf"
	// modelURL  = "https://huggingface.co/unsloth/gpt-oss-20b-GGUF/resolve/main/gpt-oss-20b-Q8_0.gguf"
	modelInstances = 1
)

var (
	libPath   = defaults.LibsDir("")
	modelPath = defaults.ModelsDir("")
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("\nERROR: %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	info, err := installSystem()
	if err != nil {
		return fmt.Errorf("unable to installation system: %w", err)
	}

	krn, err := newKronk(libPath, info)
	if err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}
	defer func() {
		fmt.Println("\nUnloading Kronk")
		if err := krn.Unload(context.Background()); err != nil {
			fmt.Printf("failed to unload model: %v", err)
		}
	}()

	// -------------------------------------------------------------------------

	messages := model.DocumentArray()

	for {
		messages, err = userInput(messages)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("user input: %w", err)
		}

		messages, err = func() ([]model.D, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()

			d := model.D{
				"messages":    messages,
				"tools":       toolDocuments(krn.ModelInfo().IsGPT),
				"max_tokens":  2048,
				"temperature": 0.7,
				"top_p":       0.9,
				"top_k":       40,
			}

			ch, err := performChat(ctx, krn, d)
			if err != nil {
				return nil, fmt.Errorf("unable to perform chat: %w", err)
			}

			messages, err = modelResponse(krn, messages, ch)
			if err != nil {
				return nil, fmt.Errorf("model response: %w", err)
			}

			return messages, nil
		}()

		if err != nil {
			return fmt.Errorf("unable to perform chat: %w", err)
		}
	}
}

func installSystem() (tools.ModelPath, error) {
	libCfg, err := tools.NewLibConfig(
		libPath,
		runtime.GOARCH,
		runtime.GOOS,
		download.CPU.String(),
		true,
	)
	if err != nil {
		return tools.ModelPath{}, err
	}

	_, err = tools.DownloadLibraries(context.Background(), tools.FmtLogger, libCfg)
	if err != nil {
		return tools.ModelPath{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	info, err := tools.DownloadModel(context.Background(), tools.FmtLogger, modelURL, "", modelPath)
	if err != nil {
		return tools.ModelPath{}, fmt.Errorf("unable to install model: %w", err)
	}

	return info, nil
}

func newKronk(libPath string, mp tools.ModelPath) (*kronk.Kronk, error) {
	if err := kronk.Init(libPath, kronk.LogSilent); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	krn, err := kronk.New(modelInstances, model.Config{
		ModelFile: mp.ModelFile,
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
	fmt.Println("- embeddings   :", krn.ModelConfig().Embeddings)
	fmt.Println("- isGPT        :", krn.ModelInfo().IsGPT)

	return krn, nil
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
		model.TextMessage("user", userInput),
	)

	return messages, nil
}

func toolDocuments(isGPT bool) []model.D {
	if isGPT {
		return model.DocumentArray(
			model.D{
				"type": "function",
				"function": model.D{
					"name":        "get_weather",
					"description": "Get the current weather for a location",
					"parameters": model.D{
						"type": "object",
						"properties": model.D{
							"location": model.D{
								"type":        "string",
								"description": "The location to get the weather for, e.g. San Francisco, CA",
							},
						},
						"required": []any{"location"},
					},
				},
			},
			model.D{
				"type": "function",
				"function": model.D{
					"name":        "invoke_cli_command",
					"description": "Use this anytime you need to run a CLI command of any kind",
					"parameters": model.D{
						"type": "object",
						"properties": model.D{
							"call": model.D{
								"type":        "string",
								"description": "The full set of parameters to pass to the CLI command",
							},
						},
						"required": []any{"call"},
					},
				},
			},
		)
	}

	return model.DocumentArray(
		model.D{
			"type": "function",
			"function": model.D{
				"name":        "get_weather",
				"description": "Get the current weather for a location",
				"arguments": model.D{
					"location": model.D{
						"type":        "string",
						"description": "The location to get the weather for, e.g. San Francisco, CA",
					},
				},
			},
		},
		model.D{
			"type": "function",
			"function": model.D{
				"name":        "invoke_cli_command",
				"description": "Use this anytime you need to run a CLI command of any kind",
				"arguments": model.D{
					"call": model.D{
						"type":        "string",
						"description": "The full set of parameters to pass to the CLI command",
					},
				},
				"required": []any{"call"},
			},
		},
	)
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
			if krn.ModelInfo().IsGPT {
				fmt.Println()
			}

			fmt.Printf("\u001b[92mModel Asking For Tool Call:\nToolID[%s]: %s(%s)\u001b[0m",
				resp.Choice[0].Delta.ToolCalls[0].ID,
				resp.Choice[0].Delta.ToolCalls[0].Name,
				resp.Choice[0].Delta.ToolCalls[0].Arguments,
			)

			messages = append(messages,
				model.TextMessage("tool", fmt.Sprintf("Tool call %s: %s(%v)",
					resp.Choice[0].Delta.ToolCalls[0].ID,
					resp.Choice[0].Delta.ToolCalls[0].Name,
					resp.Choice[0].Delta.ToolCalls[0].Arguments),
				),
			)
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
				if krn.ModelInfo().IsGPT {
					fmt.Println()
				}
			}

			fmt.Printf("%s", resp.Choice[0].Delta.Content)
		}
	}

	// -------------------------------------------------------------------------

	contextTokens := lr.Usage.InputTokens + lr.Usage.CompletionTokens
	contextWindow := krn.ModelConfig().ContextWindow
	percentage := (float64(contextTokens) / float64(contextWindow)) * 100
	of := float32(contextWindow) / float32(1024)

	fmt.Printf("\n\n\u001b[90mInput: %d  Reasoning: %d  Completion: %d  Output: %d  Window: %d (%.0f%% of %.0fK) TPS: %.2f\u001b[0m\n",
		lr.Usage.InputTokens, lr.Usage.ReasoningTokens, lr.Usage.CompletionTokens, lr.Usage.OutputTokens, contextTokens, percentage, of, lr.Usage.TokensPerSecond)

	return messages, nil
}
