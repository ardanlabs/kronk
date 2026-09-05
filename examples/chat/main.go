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
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// modelSource is the model to download. It may be a HuggingFace URL,
// a canonical "provider/modelID", or a bare model id.
var modelSource = "unsloth/Qwen3-0.6B-Q8_0"

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

	if err := chat(context.Background(), krn); err != nil {
		return err
	}

	return nil
}

func installSystem() (models.Path, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	libs, err := libs.New(
		libs.WithDetect(ctx, kronk.FmtLogger),
	)
	if err != nil {
		return models.Path{}, err
	}

	if _, err := libs.Download(ctx, kronk.FmtLogger); err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	if err := kronk.Init(kronk.WithLibPath(libs.LibsPath())); err != nil {
		return models.Path{}, fmt.Errorf("unable to init kronk: %w", err)
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

	krn, err := kronk.New(
		model.WithModelFiles(mp.ModelFiles),
		model.WithAutoTune(true),
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
	fmt.Println("- flashAttention :", krn.ModelConfig().FlashAttention())
	fmt.Println("- prefill batch  :", krn.ModelConfig().PrefillBatchSize())
	fmt.Println("- modelType      :", krn.ModelInfo().Type)
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
	if sm := krn.ModelConfig().PtrSplitMode; sm != nil {
		fmt.Println("- splitMode      :", sm)
	} else {
		fmt.Println("- splitMode      : auto")
	}

	return krn, nil
}

const systemPrompt = `You are designed to help users answer questions, create
content, and provide information in a helpful and accurate manner. Always follow
the user's instructions carefully and respond with clear, concise, and
well-structured answers.`

func chat(ctx context.Context, krn *kronk.Kronk) error {
	conversation := []model.D{
		{"role": "system", "content": systemPrompt},
	}

	scanner := bufio.NewScanner(os.Stdin)

	for {
		nextConversation, ok := promptUser(scanner, conversation)
		if !ok {
			return scanner.Err()
		}
		conversation = nextConversation

		// Keep running model turns until the assistant responds without asking
		// for another tool. Only then prompt the user again.
		for {
			content, toolCalls, usage, err := streamModelTurn(ctx, krn, conversation)
			if err != nil {
				return err
			}

			printUsage(krn, usage)

			if len(toolCalls) == 0 {
				conversation = appendAssistant(conversation, content)
				break
			}

			conversation = appendToolCalls(conversation, toolCalls)
			conversation = append(conversation, callTools(toolCalls)...)
		}
	}
}

func promptUser(scanner *bufio.Scanner, conversation []model.D) ([]model.D, bool) {
	fmt.Print("\nUSER> ")

	if !scanner.Scan() {
		return conversation, false
	}

	userInput := scanner.Text()
	if userInput == "quit" {
		return conversation, false
	}

	conversation = append(conversation, model.D{
		"role":    "user",
		"content": userInput,
	})

	return conversation, true
}

func toolDocuments() []model.D {
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
	)
}

func streamModelTurn(ctx context.Context, krn *kronk.Kronk, conversation []model.D) (string, []model.ResponseToolCall, *model.Usage, error) {
	d := model.D{
		"messages":   conversation,
		"tools":      toolDocuments(),
		"max_tokens": 2048,
		"stream_options": model.D{
			"include_usage": true,
		},
	}

	fmt.Print("\nMODEL> ")

	callCtx, cancelCall := context.WithTimeout(ctx, 2*time.Minute)
	defer cancelCall()

	ch, err := krn.ChatStreaming(callCtx, d)
	if err != nil {
		return "", nil, nil, fmt.Errorf("chat streaming: %w", err)
	}

	var content strings.Builder
	var usage *model.Usage
	var toolCalls []model.ResponseToolCall
	reasoning := false

	for resp := range ch {
		if len(resp.Choices) == 0 {
			usage = resp.Usage
			continue
		}

		switch resp.Choices[0].FinishReason() {
		case model.FinishReasonError:
			return "", nil, usage, fmt.Errorf("error from model: %s", resp.Choices[0].Delta.Content)

		case model.FinishReasonStop, model.FinishReasonLength:
			continue

		case model.FinishReasonTool:
			toolCalls = resp.Choices[0].Message.ToolCalls
			continue

		default:
			delta := resp.Choices[0].Delta
			for _, tool := range delta.ToolCallDeltas {
				if tool.Function.Name != "" {
					fmt.Printf("\n\n\u001b[92mExecuting %s...\u001b[0m", tool.Function.Name)
				}
			}

			switch {
			case delta.Reasoning != "":
				reasoning = true
				fmt.Printf("\u001b[91m%s\u001b[0m", delta.Reasoning)

			case delta.Content != "":
				if reasoning {
					reasoning = false
					fmt.Print("\n\n")
				}

				fmt.Print(delta.Content)
				content.WriteString(delta.Content)
			}
		}
	}

	if len(toolCalls) > 0 {
		return "", toolCalls, usage, nil
	}

	return strings.TrimLeft(content.String(), "\n"), nil, usage, nil
}

func appendToolCalls(conversation []model.D, toolCalls []model.ResponseToolCall) []model.D {
	fmt.Print("\n\n")

	var toolCallDocs []model.D
	for _, toolCall := range toolCalls {
		argsJSON, _ := json.Marshal(toolCall.Function.Arguments)
		toolCallDocs = append(toolCallDocs, model.D{
			"id":   toolCall.ID,
			"type": "function",
			"function": model.D{
				"name":      toolCall.Function.Name,
				"arguments": string(argsJSON),
			},
		})
	}

	return append(conversation, model.D{
		"role":       "assistant",
		"tool_calls": toolCallDocs,
	})
}

func appendAssistant(conversation []model.D, content string) []model.D {
	if content == "" {
		return conversation
	}

	fmt.Print("\n")
	return append(conversation, model.D{"role": "assistant", "content": content})
}

func printUsage(krn *kronk.Kronk, usage *model.Usage) {
	if usage == nil {
		return
	}

	contextTokens := usage.PromptTokens + usage.CompletionTokens
	contextWindow := krn.ModelConfig().ContextWindow()
	percentage := (float64(contextTokens) / float64(contextWindow)) * 100
	of := float32(contextWindow) / float32(1024)

	fmt.Printf("\n\n\u001b[90mInput: %d  Reasoning: %d  Completion: %d  Total: %d  Window: %d (%.0f%% of %.0fK) TPS: %.2f\u001b[0m",
		usage.PromptTokens, usage.CompletionTokensDetails.ReasoningTokens, usage.CompletionTokens, usage.TotalTokens, contextTokens, percentage, of, usage.TokensPerSecond)
}

func callTools(toolCalls []model.ResponseToolCall) []model.D {
	results := make([]model.D, 0, len(toolCalls))

	for _, toolCall := range toolCalls {
		fmt.Printf("\u001b[92m%s(%v)\u001b[0m: \u001b[90mok\u001b[0m\n",
			toolCall.Function.Name, toolCall.Function.Arguments)

		// This example hard-codes the tool execution and its result. The agent
		// example replaces this with registered Tool implementations.
		results = append(results, model.D{
			"role":         "tool",
			"name":         toolCall.Function.Name,
			"tool_call_id": toolCall.ID,
			"content":      `{"temperature": "72°F", "condition": "sunny"}`,
		})
	}

	return results
}
