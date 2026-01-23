import { useEffect, useRef } from 'react';
import Prism from 'prismjs';
import 'prismjs/components/prism-go';
import 'prismjs/themes/prism-tomorrow.css';

function GoCode({ children }: { children: string }) {
  const codeRef = useRef<HTMLElement>(null);

  useEffect(() => {
    if (codeRef.current) {
      Prism.highlightElement(codeRef.current);
    }
  }, [children]);

  return (
    <pre className="code-go">
      <code ref={codeRef} className="language-go">
        {children}
      </code>
    </pre>
  );
}

const questionExample = `// This example shows you a basic program of using Kronk to ask a model a question.
//
// The first time you run this program the system will download and install
// the model and libraries.
//
// Run the example like this from the root of the project:
// $ make example-question

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

const modelURL = "https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf"

func main() {
	if err := run(); err != nil {
		fmt.Printf("\\nERROR: %s\\n", err)
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
		fmt.Println("\\nUnloading Kronk")
		if err := krn.Unload(context.Background()); err != nil {
			fmt.Printf("failed to unload model: %v", err)
		}
	}()

	if err := question(krn); err != nil {
		fmt.Println(err)
	}

	return nil
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

	// -------------------------------------------------------------------------
	// You could also download this model using the catalog system.
	// templates.Catalog().DownloadModel("Qwen3-8B-Q8_0")

	return mp, nil
}

func newKronk(mp models.Path) (*kronk.Kronk, error) {
	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	krn, err := kronk.New(model.Config{
		ModelFiles: mp.ModelFiles,
		CacheTypeK: model.GGMLTypeQ8_0,
		CacheTypeV: model.GGMLTypeQ8_0,
		NSeqMax:    2,
	})

	if err != nil {
		return nil, fmt.Errorf("unable to create inference model: %w", err)
	}

	fmt.Print("- system info:\\n\\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	fmt.Println("  - contextWindow:", krn.ModelConfig().ContextWindow)
	fmt.Println("  - embeddings   :", krn.ModelInfo().IsEmbedModel)
	fmt.Println("  - isGPT        :", krn.ModelInfo().IsGPTModel)

	return krn, nil
}

func question(krn *kronk.Kronk) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	question := "Hello model"

	fmt.Println()
	fmt.Println("QUESTION:", question)
	fmt.Println()

	d := model.D{
		"messages": model.DocumentArray(
			model.TextMessage(model.RoleUser, question),
		),
		"temperature": 0.7,
		"top_p":       0.9,
		"top_k":       40,
		"max_tokens":  2048,
	}

	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		return fmt.Errorf("chat streaming: %w", err)
	}

	// -------------------------------------------------------------------------

	var reasoning bool

	for resp := range ch {
		switch resp.Choice[0].FinishReason() {
		case model.FinishReasonError:
			return fmt.Errorf("error from model: %s", resp.Choice[0].Delta.Content)

		case model.FinishReasonStop:
			return nil

		default:
			if resp.Choice[0].Delta.Reasoning != "" {
				reasoning = true
				fmt.Printf("\\u001b[91m%s\\u001b[0m", resp.Choice[0].Delta.Reasoning)
				continue
			}

			if reasoning {
				reasoning = false
				fmt.Println()
				continue
			}

			fmt.Printf("%s", resp.Choice[0].Delta.Content)
		}
	}

	return nil
}
`;

const chatExample = `// This example shows you how to create a simple chat application against an
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
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/ardanlabs/kronk/sdk/tools/templates"
)

//const modelURL = "https://huggingface.co/unsloth/Qwen3-Coder-30B-A3B-Instruct-GGUF/resolve/main/Qwen3-Coder-30B-A3B-Instruct-Q8_0.gguf"
//const modelURL = "https://huggingface.co/bartowski/cerebras_Qwen3-Coder-REAP-25B-A3B-GGUF/resolve/main/cerebras_Qwen3-Coder-REAP-25B-A3B-Q8_0.gguf"
//const modelURL = "https://huggingface.co/unsloth/gpt-oss-20b-GGUF/resolve/main/gpt-oss-20b-Q8_0.gguf"
//const modelURL = "https://huggingface.co/unsloth/GLM-4.7-Flash-GGUF/resolve/main/GLM-4.7-Flash-Q8_0.gguf"
//const modelURL = "https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf"

const modelURL = "https://huggingface.co/unsloth/gpt-oss-120b-GGUF/resolve/main/gpt-oss-120b-F16.gguf"

func main() {
	if err := run(); err != nil {
		fmt.Printf("\\nERROR: %s\\n", err)
		os.Exit(1)
	}
}

func run() error {
	mp, err := installSystem()
	if err != nil {
		return fmt.Errorf("run: unable to installation system: %w", err)
	}

	krn, err := newKronk(mp)
	if err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}

	defer func() {
		fmt.Println("\\nUnloading Kronk")
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
	// This is not mandatory if you won't be using models from the catalog. That
	// being said, if you are using a model that is part of the catalog with
	// a corrected jinja file, having the catalog system up to date will allow
	// the system to pull that jinja file.

	templates, err := templates.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to create template system: %w", err)
	}

	if err := templates.Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download templates: %w", err)
	}

	if err := templates.Catalog().Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download catalog: %w", err)
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

	// -------------------------------------------------------------------------
	// You could also download this model using the catalog system.
	// templates.Catalog().DownloadModel("Qwen3-8B-Q8_0")

	return mp, nil
}

func newKronk(mp models.Path) (*kronk.Kronk, error) {
	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	cfg := model.Config{
		ModelFiles:        mp.ModelFiles,
		CacheTypeK:        model.GGMLTypeF16,
		CacheTypeV:        model.GGMLTypeF16,
		NSeqMax:           2,
		SystemPromptCache: true,
		JinjaFile:         "/Users/bill/code/go/src/github.com/ardanlabs/kronk_catalogs/templates/gpt-oss.jinja",
	}

	krn, err := kronk.New(cfg)

	if err != nil {
		return nil, fmt.Errorf("unable to create inference model: %w", err)
	}

	fmt.Print("- system info:\\n\\t")
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

func chat(krn *kronk.Kronk) error {
	messages := model.DocumentArray()

	var systemPrompt = \`
		You are a helpful AI assistant. You are designed to help users answer
		questions, create content, and provide information in a helpful and
		accurate manner. Always follow the user's instructions carefully and
		respond with clear, concise, and well-structured answers. You are a
		helpful AI assistant. You are designed to help users answer questions,
		create content, and provide information in a helpful and accurate manner.
		Always follow the user's instructions carefully and respond with clear,
		concise, and well-structured answers. You are a helpful AI assistant.
		You are designed to help users answer questions, create content, and
		provide information in a helpful and accurate manner. Always follow the
		user's instructions carefully and respond with clear, concise, and
		well-structured answers.\`

	messages = append(messages,
		model.TextMessage(model.RoleSystem, systemPrompt),
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
				"messages":       messages,
				"tools":          toolDocuments(),
				"max_tokens":     2048,
				"temperature":    0.7,
				"top_p":          0.8,
				"top_k":          20,
				"repeat_penalty": 1.05,
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
	fmt.Print("\\nUSER> ")

	reader := bufio.NewReader(os.Stdin)

	userInput, err := reader.ReadString('\\n')
	if err != nil {
		return messages, fmt.Errorf("unable to read user input: %w", err)
	}

	if userInput == "quit\\n" {
		return nil, io.EOF
	}

	messages = append(messages,
		model.TextMessage(model.RoleUser, userInput),
	)

	return messages, nil
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

func performChat(ctx context.Context, krn *kronk.Kronk, d model.D) (<-chan model.ChatResponse, error) {
	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		return nil, fmt.Errorf("chat streaming: %w", err)
	}

	return ch, nil
}

func modelResponse(krn *kronk.Kronk, messages []model.D, ch <-chan model.ChatResponse) ([]model.D, error) {
	fmt.Print("\\nMODEL> ")

	var reasoning bool
	var lr model.ChatResponse

loop:
	for resp := range ch {
		lr = resp

		switch resp.Choice[0].FinishReason() {
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

			fmt.Printf("\\u001b[92mModel Asking For Tool Calls:\\n\\u001b[0m")

			for _, tool := range resp.Choice[0].Delta.ToolCalls {
				fmt.Printf("\\u001b[92mToolID[%s]: %s(%s)\\n\\u001b[0m",
					tool.ID,
					tool.Function.Name,
					tool.Function.Arguments,
				)

				messages = append(messages,
					model.TextMessage("tool", fmt.Sprintf("Tool call %s: %s(%v)\\n",
						tool.ID,
						tool.Function.Name,
						tool.Function.Arguments),
					),
				)
			}

			break loop

		default:
			if resp.Choice[0].Delta.Reasoning != "" {
				fmt.Printf("\\u001b[91m%s\\u001b[0m", resp.Choice[0].Delta.Reasoning)
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

	// -------------------------------------------------------------------------

	contextTokens := lr.Usage.PromptTokens + lr.Usage.CompletionTokens
	contextWindow := krn.ModelConfig().ContextWindow
	percentage := (float64(contextTokens) / float64(contextWindow)) * 100
	of := float32(contextWindow) / float32(1024)

	fmt.Printf("\\n\\n\\u001b[90mInput: %d  Reasoning: %d  Completion: %d  Output: %d  Window: %d (%.0f%% of %.0fK) TPS: %.2f\\u001b[0m\\n",
		lr.Usage.PromptTokens, lr.Usage.ReasoningTokens, lr.Usage.CompletionTokens, lr.Usage.OutputTokens, contextTokens, percentage, of, lr.Usage.TokensPerSecond)

	return messages, nil
}
`;

const responseExample = `// This example shows you how to create a simple chat application against an
// inference model using the kronk Response api. Thanks to Kronk and yzma,
// reasoning and tool calling is enabled.
//
// The first time you run this program the system will download and install
// the model and libraries.
//
// Run the example like this from the root of the project:
// $ make example-response

package main

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
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/ardanlabs/kronk/sdk/tools/templates"
)

const modelURL = "https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf"

func main() {
	if err := run(); err != nil {
		fmt.Printf("\\nERROR: %s\\n", err)
		os.Exit(1)
	}
}

func run() error {
	mp, err := installSystem()
	if err != nil {
		return fmt.Errorf("run: unable to installation system: %w", err)
	}

	krn, err := newKronk(mp)
	if err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}

	defer func() {
		fmt.Println("\\nUnloading Kronk")
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
	// This is not mandatory if you won't be using models from the catalog. That
	// being said, if you are using a model that is part of the catalog with
	// a corrected jinja file, having the catalog system up to date will allow
	// the system to pull that jinja file.

	templates, err := templates.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to create template system: %w", err)
	}

	if err := templates.Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download templates: %w", err)
	}

	if err := templates.Catalog().Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download catalog: %w", err)
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

	// -------------------------------------------------------------------------
	// You could also download this model using the catalog system.
	// templates.Catalog().DownloadModel("Qwen3-8B-Q8_0")

	return mp, nil
}

func newKronk(mp models.Path) (*kronk.Kronk, error) {
	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	krn, err := kronk.New(model.Config{
		ModelFiles: mp.ModelFiles,
		CacheTypeK: model.GGMLTypeQ8_0,
		CacheTypeV: model.GGMLTypeQ8_0,
		NSeqMax:    2,
	})

	if err != nil {
		return nil, fmt.Errorf("unable to create inference model: %w", err)
	}

	fmt.Print("- system info:\\n\\t")
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

func chat(krn *kronk.Kronk) error {
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
				"input":       messages,
				"tools":       toolDocuments(),
				"max_tokens":  2048,
				"temperature": 0.7,
				"top_p":       0.9,
				"top_k":       40,
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
	fmt.Print("\\nUSER> ")

	reader := bufio.NewReader(os.Stdin)

	userInput, err := reader.ReadString('\\n')
	if err != nil {
		return messages, fmt.Errorf("unable to read user input: %w", err)
	}

	if userInput == "quit\\n" {
		return nil, io.EOF
	}

	messages = append(messages,
		model.TextMessage(model.RoleUser, userInput),
	)

	return messages, nil
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

func performChat(ctx context.Context, krn *kronk.Kronk, d model.D) (<-chan kronk.ResponseStreamEvent, error) {
	ch, err := krn.ResponseStreaming(ctx, d)
	if err != nil {
		return nil, fmt.Errorf("response streaming: %w", err)
	}

	return ch, nil
}

func modelResponse(krn *kronk.Kronk, messages []model.D, ch <-chan kronk.ResponseStreamEvent) ([]model.D, error) {
	fmt.Print("\\nMODEL> ")

	var fullText string
	var finalResp *kronk.ResponseResponse
	var reasoning bool

	for event := range ch {
		switch event.Type {
		case "response.reasoning_summary_text.delta":
			fmt.Printf("\\u001b[91m%s\\u001b[0m", event.Delta)
			reasoning = true

		case "response.output_text.delta":
			if reasoning {
				reasoning = false
				fmt.Println()
				if krn.ModelInfo().IsGPTModel {
					fmt.Println()
				}
			}
			fmt.Printf("%s", event.Delta)

		case "response.output_text.done":
			fullText = event.Text

		case "response.function_call_arguments.done":
			fmt.Println()
			if krn.ModelInfo().IsGPTModel {
				fmt.Println()
			}

			fmt.Printf("\\u001b[92mModel Asking For Tool Calls:\\n\\u001b[0m")
			fmt.Printf("\\u001b[92mToolID[%s]: %s(%s)\\n\\u001b[0m",
				event.ItemID,
				event.Name,
				event.Arguments,
			)

			messages = append(messages,
				model.TextMessage("tool", fmt.Sprintf("Tool call %s: %s(%s)\\n",
					event.ItemID,
					event.Name,
					event.Arguments),
				),
			)

		case "response.completed":
			finalResp = event.Response
		}
	}

	if fullText != "" {
		messages = append(messages,
			model.TextMessage("assistant", fullText),
		)
	}

	// -------------------------------------------------------------------------

	if finalResp != nil {
		contextTokens := finalResp.Usage.InputTokens + finalResp.Usage.OutputTokens
		contextWindow := krn.ModelConfig().ContextWindow
		percentage := (float64(contextTokens) / float64(contextWindow)) * 100
		of := float32(contextWindow) / float32(1024)

		fmt.Printf("\\n\\n\\u001b[90mInput: %d  Reasoning: %d  Completion: %d  Output: %d  Window: %d (%.0f%% of %.0fK)\\u001b[0m\\n",
			finalResp.Usage.InputTokens,
			finalResp.Usage.OutputTokenDetail.ReasoningTokens,
			finalResp.Usage.OutputTokens,
			finalResp.Usage.OutputTokens,
			contextTokens,
			percentage,
			of,
		)
	}

	return messages, nil
}
`;

const embeddingExample = `// This example shows you how to use an embedding model.
//
// The first time you run this program the system will download and install
// the model and libraries.
//
// Run the example like this from the root of the project:
// $ make example-embedding

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/ardanlabs/kronk/sdk/tools/templates"
)

const modelURL = "https://huggingface.co/ggml-org/embeddinggemma-300m-qat-q8_0-GGUF/resolve/main/embeddinggemma-300m-qat-Q8_0.gguf"

func main() {
	if err := run(); err != nil {
		fmt.Printf("\\nERROR: %s\\n", err)
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
		fmt.Println("\\nUnloading Kronk")
		if err := krn.Unload(context.Background()); err != nil {
			fmt.Printf("failed to unload model: %v", err)
		}
	}()

	if err := embedding(krn); err != nil {
		return err
	}

	return nil
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

	templates, err := templates.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to create template system: %w", err)
	}

	if err := templates.Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download templates: %w", err)
	}

	if err := templates.Catalog().Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download catalog: %w", err)
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

	// -------------------------------------------------------------------------
	// You could also download this model using the catalog system.
	// templates.Catalog().DownloadModel("embeddinggemma-300m-qat-Q8_0")

	return mp, nil
}

func newKronk(mp models.Path) (*kronk.Kronk, error) {
	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	krn, err := kronk.New(model.Config{
		ModelFiles:     mp.ModelFiles,
		ContextWindow:  2048,
		NBatch:         2048,
		NUBatch:        512,
		CacheTypeK:     model.GGMLTypeQ8_0,
		CacheTypeV:     model.GGMLTypeQ8_0,
		FlashAttention: model.FlashAttentionEnabled,
	})

	if err != nil {
		return nil, fmt.Errorf("unable to create inference model: %w", err)
	}

	fmt.Print("- system info:\\n\\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	fmt.Println("  - contextWindow:", krn.ModelConfig().ContextWindow)
	fmt.Println("  - embeddings   :", krn.ModelInfo().IsEmbedModel)
	fmt.Println("  - isGPT        :", krn.ModelInfo().IsGPTModel)

	return krn, nil
}

func embedding(krn *kronk.Kronk) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	d := model.D{
		"input":              "Why is the sky blue?",
		"truncate":           true,
		"truncate_direction": "right",
	}

	resp, err := krn.Embeddings(ctx, d)
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("Model  :", resp.Model)
	fmt.Println("Object :", resp.Object)
	fmt.Println("Created:", time.UnixMilli(resp.Created))
	fmt.Println("  Index    :", resp.Data[0].Index)
	fmt.Println("  Object   :", resp.Data[0].Object)
	fmt.Printf("  Embedding: [%v...%v]\\n", resp.Data[0].Embedding[:3], resp.Data[0].Embedding[len(resp.Data[0].Embedding)-3:])

	return nil
}
`;

const rerankExample = `// This example shows you how to use a reranker model.
//
// The first time you run this program the system will download and install
// the model and libraries.
//
// Run the example like this from the root of the project:
// $ make example-rerank

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/ardanlabs/kronk/sdk/tools/templates"
)

const modelURL = "https://huggingface.co/gpustack/bge-reranker-v2-m3-GGUF/resolve/main/bge-reranker-v2-m3-Q8_0.gguf"

func main() {
	if err := run(); err != nil {
		fmt.Printf("\\nERROR: %s\\n", err)
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
		fmt.Println("\\nUnloading Kronk")
		if err := krn.Unload(context.Background()); err != nil {
			fmt.Printf("failed to unload model: %v", err)
		}
	}()

	if err := rerank(krn); err != nil {
		return err
	}

	return nil
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

	templates, err := templates.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to create template system: %w", err)
	}

	if err := templates.Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download templates: %w", err)
	}

	if err := templates.Catalog().Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download catalog: %w", err)
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
	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	krn, err := kronk.New(model.Config{
		ModelFiles:    mp.ModelFiles,
		ContextWindow: 2048,
		NBatch:        2048,
		NUBatch:       512,
		CacheTypeK:    model.GGMLTypeQ8_0,
		CacheTypeV:    model.GGMLTypeQ8_0,
	})

	if err != nil {
		return nil, fmt.Errorf("unable to create reranker model: %w", err)
	}

	fmt.Print("- system info:\\n\\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	fmt.Println("  - contextWindow:", krn.ModelConfig().ContextWindow)
	fmt.Println("  - embeddings   :", krn.ModelInfo().IsEmbedModel)
	fmt.Println("  - reranking    :", krn.ModelInfo().IsRerankModel)
	fmt.Println("  - isGPT        :", krn.ModelInfo().IsGPTModel)

	return krn, nil
}

func rerank(krn *kronk.Kronk) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	d := model.D{
		"query": "What is the capital of France?",
		"documents": []string{
			"Paris is the capital and largest city of France.",
			"Berlin is the capital of Germany.",
			"The Eiffel Tower is located in Paris.",
			"London is the capital of England.",
			"France is a country in Western Europe.",
		},
		"top_n":            3,
		"return_documents": true,
	}

	resp, err := krn.Rerank(ctx, d)
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("Model  :", resp.Model)
	fmt.Println("Object :", resp.Object)
	fmt.Println("Created:", time.UnixMilli(resp.Created))
	fmt.Println()
	fmt.Println("Results (sorted by relevance):")
	for i, result := range resp.Data {
		fmt.Printf("  %d. Score: %.4f, Index: %d, Doc: %s\\n",
			i+1, result.RelevanceScore, result.Index, result.Document)
	}
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  Prompt Tokens:", resp.Usage.PromptTokens)
	fmt.Println("  Total Tokens :", resp.Usage.TotalTokens)

	return nil
}
`;

const visionExample = `// This example shows you how to execute a simple prompt against a vision model.
//
// The first time you run this program the system will download and install
// the model and libraries.
//
// Run the example like this from the root of the project:
// $ make example-vision

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/ardanlabs/kronk/sdk/tools/templates"
)

const (
	modelURL  = "https://huggingface.co/ggml-org/Qwen2.5-VL-3B-Instruct-GGUF/resolve/main/Qwen2.5-VL-3B-Instruct-Q8_0.gguf"
	projURL   = "https://huggingface.co/ggml-org/Qwen2.5-VL-3B-Instruct-GGUF/resolve/main/mmproj-Qwen2.5-VL-3B-Instruct-Q8_0.gguf"
	imageFile = "examples/samples/giraffe.jpg"
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("\\nERROR: %s\\n", err)
		os.Exit(1)
	}
}

func run() error {
	info, err := installSystem()
	if err != nil {
		return fmt.Errorf("unable to install system: %w", err)
	}

	krn, err := newKronk(info)
	if err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}

	defer func() {
		fmt.Println("\\nUnloading Kronk")
		if err := krn.Unload(context.Background()); err != nil {
			fmt.Printf("failed to unload model: %v", err)
		}
	}()

	if err := vision(krn); err != nil {
		return err
	}

	return nil
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

	templates, err := templates.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to create template system: %w", err)
	}

	if err := templates.Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download templates: %w", err)
	}

	if err := templates.Catalog().Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download catalog: %w", err)
	}

	// -------------------------------------------------------------------------

	mdls, err := models.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	mp, err := mdls.Download(ctx, kronk.FmtLogger, modelURL, projURL)
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install model: %w", err)
	}

	// -------------------------------------------------------------------------
	// You could also download this model using the catalog system.
	// templates.Catalog().DownloadModel("Qwen2.5-VL-3B-Instruct-Q8_0")

	return mp, nil
}

func newKronk(mp models.Path) (*kronk.Kronk, error) {
	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	krn, err := kronk.New(model.Config{
		ModelFiles:    mp.ModelFiles,
		ProjFile:      mp.ProjFile,
		ContextWindow: 8192,
		NBatch:        2048,
		NUBatch:       2048,
		CacheTypeK:    model.GGMLTypeQ8_0,
		CacheTypeV:    model.GGMLTypeQ8_0,
	})

	if err != nil {
		return nil, fmt.Errorf("unable to create inference model: %w", err)
	}

	fmt.Print("- system info:\\n\\t")
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

func vision(krn *kronk.Kronk) error {
	question := "What is in this picture?"

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	ch, err := performChat(ctx, krn, question, imageFile)
	if err != nil {
		return fmt.Errorf("perform chat: %w", err)
	}

	if err := modelResponse(krn, ch); err != nil {
		return fmt.Errorf("model response: %w", err)
	}

	return nil
}

func performChat(ctx context.Context, krn *kronk.Kronk, question string, imageFile string) (<-chan model.ChatResponse, error) {
	image, err := readImage(imageFile)
	if err != nil {
		return nil, fmt.Errorf("read image: %w", err)
	}

	fmt.Printf("\\nQuestion: %s\\n", question)

	d := model.D{
		"messages":    model.RawMediaMessage(question, image),
		"temperature": 0.7,
		"top_p":       0.9,
		"top_k":       40,
		"max_tokens":  2048,
	}

	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		return nil, fmt.Errorf("vision streaming: %w", err)
	}

	return ch, nil
}

func modelResponse(krn *kronk.Kronk, ch <-chan model.ChatResponse) error {
	fmt.Print("\\nMODEL> ")

	var reasoning bool
	var lr model.ChatResponse

loop:
	for resp := range ch {
		lr = resp

		switch resp.Choice[0].FinishReason() {
		case model.FinishReasonStop:
			break loop

		case model.FinishReasonError:
			return fmt.Errorf("error from model: %s", resp.Choice[0].Delta.Content)
		}

		if resp.Choice[0].Delta.Reasoning != "" {
			fmt.Printf("\\u001b[91m%s\\u001b[0m", resp.Choice[0].Delta.Reasoning)
			reasoning = true
			continue
		}

		if reasoning {
			reasoning = false
			fmt.Print("\\n\\n")
		}

		fmt.Printf("%s", resp.Choice[0].Delta.Content)
	}

	// -------------------------------------------------------------------------

	contextTokens := lr.Usage.PromptTokens + lr.Usage.CompletionTokens
	contextWindow := krn.ModelConfig().ContextWindow
	percentage := (float64(contextTokens) / float64(contextWindow)) * 100
	of := float32(contextWindow) / float32(1024)

	fmt.Printf("\\n\\n\\u001b[90mInput: %d  Reasoning: %d  Completion: %d  Output: %d  Window: %d (%.0f%% of %.0fK) TPS: %.2f\\u001b[0m\\n",
		lr.Usage.PromptTokens, lr.Usage.ReasoningTokens, lr.Usage.CompletionTokens, lr.Usage.OutputTokens, contextTokens, percentage, of, lr.Usage.TokensPerSecond)

	return nil
}

func readImage(imageFile string) ([]byte, error) {
	if _, err := os.Stat(imageFile); err != nil {
		return nil, fmt.Errorf("error accessing file %q: %w", imageFile, err)
	}

	image, err := os.ReadFile(imageFile)
	if err != nil {
		return nil, fmt.Errorf("error reading file %q: %w", imageFile, err)
	}

	return image, nil
}
`;

const audioExample = `// This example shows you how to execute a simple prompt against a vision model.
//
// The first time you run this program the system will download and install
// the model and libraries.
//
// Run the example like this from the root of the project:
// $ make example-vision

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/ardanlabs/kronk/sdk/tools/templates"
)

const (
	modelURL  = "https://huggingface.co/mradermacher/Qwen2-Audio-7B-GGUF/resolve/main/Qwen2-Audio-7B.Q8_0.gguf"
	projURL   = "https://huggingface.co/mradermacher/Qwen2-Audio-7B-GGUF/resolve/main/Qwen2-Audio-7B.mmproj-Q8_0.gguf"
	audioFile = "examples/samples/jfk.wav"
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("\\nERROR: %s\\n", err)
		os.Exit(1)
	}
}

func run() error {
	mp, err := installSystem()
	if err != nil {
		return fmt.Errorf("unable to install system: %w", err)
	}

	krn, err := newKronk(mp)
	if err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}

	defer func() {
		fmt.Println("\\nUnloading Kronk")
		if err := krn.Unload(context.Background()); err != nil {
			fmt.Printf("failed to unload model: %v", err)
		}
	}()

	if err := audio(krn); err != nil {
		return err
	}

	return nil
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

	templates, err := templates.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to create template system: %w", err)
	}

	if err := templates.Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download templates: %w", err)
	}

	if err := templates.Catalog().Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download catalog: %w", err)
	}

	// -------------------------------------------------------------------------

	mdls, err := models.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	mp, err := mdls.Download(ctx, kronk.FmtLogger, modelURL, projURL)
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install model: %w", err)
	}

	// -------------------------------------------------------------------------
	// You could also download this model using the catalog system.
	// templates.Catalog().DownloadModel("Qwen2-Audio-7B.Q8_0")

	return mp, nil
}

func newKronk(mp models.Path) (*kronk.Kronk, error) {
	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	krn, err := kronk.New(model.Config{
		ModelFiles:    mp.ModelFiles,
		ProjFile:      mp.ProjFile,
		ContextWindow: 8192,
		NBatch:        2048,
		NUBatch:       2048,
		CacheTypeK:    model.GGMLTypeQ8_0,
		CacheTypeV:    model.GGMLTypeQ8_0,
	})

	if err != nil {
		return nil, fmt.Errorf("unable to create inference model: %w", err)
	}

	fmt.Print("- system info:\\n\\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	fmt.Println("- contextWindow:", krn.ModelConfig().ContextWindow)
	fmt.Println("- embeddings   :", krn.ModelInfo().IsEmbedModel)
	fmt.Println("- isGPT        :", krn.ModelInfo().IsGPTModel)

	return krn, nil
}

func audio(krn *kronk.Kronk) error {
	question := "Please describe what you hear in the following audio clip."

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	ch, err := performChat(ctx, krn, question, audioFile)
	if err != nil {
		return fmt.Errorf("perform chat: %w", err)
	}

	if err := modelResponse(krn, ch); err != nil {
		return fmt.Errorf("model response: %w", err)
	}

	return nil
}

func performChat(ctx context.Context, krn *kronk.Kronk, question string, imageFile string) (<-chan model.ChatResponse, error) {
	image, err := readImage(imageFile)
	if err != nil {
		return nil, fmt.Errorf("read image: %w", err)
	}

	fmt.Printf("\\nQuestion: %s\\n", question)

	d := model.D{
		"messages":    model.RawMediaMessage(question, image),
		"max_tokens":  2048,
		"temperature": 0.7,
		"top_p":       0.9,
		"top_k":       40,
	}

	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		return nil, fmt.Errorf("chat streaming: %w", err)
	}

	return ch, nil
}

func modelResponse(krn *kronk.Kronk, ch <-chan model.ChatResponse) error {
	fmt.Print("\\nMODEL> ")

	var reasoning bool
	var lr model.ChatResponse

loop:
	for resp := range ch {
		lr = resp

		switch resp.Choice[0].FinishReason() {
		case model.FinishReasonStop:
			break loop

		case model.FinishReasonError:
			return fmt.Errorf("error from model: %s", resp.Choice[0].Delta.Content)
		}

		if resp.Choice[0].Delta.Reasoning != "" {
			fmt.Printf("\\u001b[91m%s\\u001b[0m", resp.Choice[0].Delta.Reasoning)
			reasoning = true
			continue
		}

		if reasoning {
			reasoning = false
			fmt.Print("\\n\\n")
		}

		fmt.Printf("%s", resp.Choice[0].Delta.Content)
	}

	// -------------------------------------------------------------------------

	contextTokens := lr.Usage.PromptTokens + lr.Usage.CompletionTokens
	contextWindow := krn.ModelConfig().ContextWindow
	percentage := (float64(contextTokens) / float64(contextWindow)) * 100
	of := float32(contextWindow) / float32(1024)

	fmt.Printf("\\n\\n\\u001b[90mInput: %d  Reasoning: %d  Completion: %d  Output: %d  Window: %d (%.0f%% of %.0fK) TPS: %.2f\\u001b[0m\\n",
		lr.Usage.PromptTokens, lr.Usage.ReasoningTokens, lr.Usage.CompletionTokens, lr.Usage.OutputTokens, contextTokens, percentage, of, lr.Usage.TokensPerSecond)

	return nil
}

func readImage(imageFile string) ([]byte, error) {
	if _, err := os.Stat(imageFile); err != nil {
		return nil, fmt.Errorf("error accessing file %q: %w", imageFile, err)
	}

	image, err := os.ReadFile(imageFile)
	if err != nil {
		return nil, fmt.Errorf("error reading file %q: %w", imageFile, err)
	}

	return image, nil
}
`;

export default function DocsSDKExamples() {
  return (
    <div>
      <div className="page-header">
        <h2>SDK Examples</h2>
        <p>Complete working examples demonstrating how to use the Kronk SDK</p>
      </div>

      <div className="doc-layout">
        <div className="doc-content">

          <div className="card" id="example-question">
            <h3>Question</h3>
            <p className="doc-description">Ask a single question to a model</p>
            <GoCode>{questionExample}</GoCode>
          </div>

          <div className="card" id="example-chat">
            <h3>Chat</h3>
            <p className="doc-description">Interactive chat with conversation history</p>
            <GoCode>{chatExample}</GoCode>
          </div>

          <div className="card" id="example-response">
            <h3>Response</h3>
            <p className="doc-description">Interactive chat using the Response API with tool calling</p>
            <GoCode>{responseExample}</GoCode>
          </div>

          <div className="card" id="example-embedding">
            <h3>Embedding</h3>
            <p className="doc-description">Generate embeddings for semantic search</p>
            <GoCode>{embeddingExample}</GoCode>
          </div>

          <div className="card" id="example-rerank">
            <h3>Rerank</h3>
            <p className="doc-description">Rerank documents by relevance to a query</p>
            <GoCode>{rerankExample}</GoCode>
          </div>

          <div className="card" id="example-vision">
            <h3>Vision</h3>
            <p className="doc-description">Analyze images using vision models</p>
            <GoCode>{visionExample}</GoCode>
          </div>

          <div className="card" id="example-audio">
            <h3>Audio</h3>
            <p className="doc-description">Process audio files using audio models</p>
            <GoCode>{audioExample}</GoCode>
          </div>
        </div>

        <nav className="doc-sidebar">
          <div className="doc-sidebar-content">
            <div className="doc-index-section">
              <span className="doc-index-header">Examples</span>
              <ul>
                <li><a href="#example-question">Question</a></li>
                <li><a href="#example-chat">Chat</a></li>
                <li><a href="#example-response">Response</a></li>
                <li><a href="#example-embedding">Embedding</a></li>
                <li><a href="#example-rerank">Rerank</a></li>
                <li><a href="#example-vision">Vision</a></li>
                <li><a href="#example-audio">Audio</a></li>
              </ul>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
