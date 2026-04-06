// This example shows you how to create a simple agent application against an
// inference model using kronk. Thanks to Kronk and yzma, reasoning and tool
// calling is enabled.
//
// The first time you run this program the system will download and install
// the model and libraries.
//
// Run the example like this from the root of the project:
// $ make example-agent

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

type modelSpec struct {
	SourceURL string
	ModelID   string
}

// Configure this to switch between URL and catalog downloads.
// Set either SourceURL or ModelID, not both.
var modelSpecConfig = modelSpec{
	//SourceURL: "https://huggingface.co/unsloth/gpt-oss-20b-GGUF/resolve/main/gpt-oss-20b-Q8_0.gguf",
	ModelID: "gpt-oss-20b-Q8_0",
}

// =============================================================================

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	mp, err := installSystem()
	if err != nil {
		return fmt.Errorf("run: unable to installation system: %w", err)
	}

	scanner := bufio.NewScanner(os.Stdin)
	getUserMessage := func() (string, bool) {
		if !scanner.Scan() {
			return "", false
		}
		return scanner.Text(), true
	}

	agent, err := NewAgent(getUserMessage, mp)
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	return agent.Run(context.TODO())
}

// =============================================================================

// Tool describes the features which all tools must implement.
type Tool interface {
	Call(ctx context.Context, toolCall model.ResponseToolCall) model.D
}

// =============================================================================

// Agent represents the chat agent that can use tools to perform tasks.
type Agent struct {
	krn            *kronk.Kronk
	getUserMessage func() (string, bool)
	tools          map[string]Tool
	toolDocuments  []model.D
}

// NewAgent creates a new instance of Agent.
func NewAgent(getUserMessage func() (string, bool), mp models.Path) (*Agent, error) {
	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	krn, err := newKronk(mp)
	if err != nil {
		return nil, fmt.Errorf("unable to create kronk instance: %w", err)
	}

	// Build tool documents by registering each tool with its own tools map.
	toolsMap := make(map[string]Tool)
	toolDocuments := []model.D{
		RegisterReadFile(toolsMap),
		RegisterSearchFiles(toolsMap),
		RegisterCreateFile(toolsMap),
		RegisterGoCodeEditor(toolsMap),
	}

	agent := Agent{
		krn:            krn,
		getUserMessage: getUserMessage,
		tools:          toolsMap,
		toolDocuments:  toolDocuments,
	}

	return &agent, nil
}

// systemPrompt defines how the agent should behave when assisting with coding tasks.
const systemPrompt = `You are a helpful coding assistant that has tools to assist you in coding.

After you request a tool call, you will receive a JSON document with two fields,
"status" and "data". Always check the "status" field to know if the call "SUCCEED"
or "FAILED". The information you need to respond will be provided under the "data"
field. If the called "FAILED", just inform the user and don't try using the tool
again for the current response.

When reading Go source code always start counting lines of code from the top of
the source code file.

If you get back results from a tool call, do not verify the results.

Reasoning: high
`

// Run starts the agent and runs the chat loop.
func (a *Agent) Run(ctx context.Context) error {
	conversation := []model.D{
		{"role": "system", "content": systemPrompt},
	}

	fmt.Printf("\nChat with %s (use 'ctrl-c' to quit)\n", a.krn.ModelInfo().ID)

	needUserInput := true

	for {
		// ---------------------------------------------------------------------
		// If we need user input, prompt the user for their next question or
		// request. Otherwise, we are continuing a tool call.

		if needUserInput {
			if ok := a.promptUser(&conversation); !ok {
				return nil
			}
		}

		// ---------------------------------------------------------------------
		// Make a streaming call to the model. This returns the assistant's
		// text content and any tool calls requested by the model.

		content, toolCalls, usage, err := a.streamModelTurn(ctx, conversation)
		if err != nil {
			return err
		}

		a.printUsage(usage)

		// ---------------------------------------------------------------------
		// If the model requested tool calls, execute them and continue the
		// loop without asking the user for input.

		if len(toolCalls) > 0 {
			a.appendToolCalls(&conversation, toolCalls)

			results := a.callTools(ctx, toolCalls)
			if len(results) > 0 {
				conversation = append(conversation, results...)
			}

			needUserInput = false
			continue
		}

		// ---------------------------------------------------------------------
		// The model produced a text response. Add it to the conversation
		// and go back to asking the user for input.

		a.appendAssistant(&conversation, content)

		needUserInput = true
	}
}

// promptUser asks the user for input and appends it to the conversation.
func (a *Agent) promptUser(conversation *[]model.D) bool {
	fmt.Print("\u001b[94m\nYou\u001b[0m: ")

	userInput, ok := a.getUserMessage()
	if !ok {
		return false
	}

	*conversation = append(*conversation, model.D{
		"role":    "user",
		"content": userInput,
	})

	return true
}

// streamModelTurn sends the conversation to the model and streams back the
// response. It returns the assembled text content, any tool calls, and usage.
func (a *Agent) streamModelTurn(ctx context.Context, conversation []model.D) (string, []model.ResponseToolCall, *model.Usage, error) {
	d := model.D{
		"messages":       conversation,
		"temperature":    0.0,
		"top_p":          0.1,
		"top_k":          1,
		"stream":         true,
		"tools":          a.toolDocuments,
		"tool_selection": "auto",
	}

	fmt.Printf("\u001b[93m\n%s\u001b[0m: 0.000", a.krn.ModelInfo().ID)

	callCtx, cancelCall := context.WithTimeout(ctx, 5*time.Minute)
	defer cancelCall()

	ch, err := a.krn.ChatStreaming(callCtx, d)
	if err != nil {
		return "", nil, nil, fmt.Errorf("error chat streaming: %w", err)
	}

	// Start the latency printer and ensure it stops.
	stopPrinter := a.startLatencyPrinter(ctx)
	defer stopPrinter()

	var chunks []string
	var lastResp model.ChatResponse
	firstChunk := true
	reasonThinking := false

	for resp := range ch {
		lastResp = resp

		if len(resp.Choices) == 0 {
			continue
		}

		// On the first real chunk, stop the latency printer and add spacing.
		if firstChunk {
			firstChunk = false
			stopPrinter()
			fmt.Println()
		}

		switch resp.Choices[0].FinishReason() {
		case model.FinishReasonError:
			return "", nil, lastResp.Usage, fmt.Errorf("error from model: %s", resp.Choices[0].Delta.Content)

		case model.FinishReasonStop:
			text := strings.TrimLeft(strings.Join(chunks, " "), "\n")
			return text, nil, lastResp.Usage, nil

		case model.FinishReasonTool:
			return "", resp.Choices[0].Delta.ToolCalls, lastResp.Usage, nil

		default:
			delta := resp.Choices[0].Delta

			switch {
			case delta.Reasoning != "":
				reasonThinking = true

				fmt.Printf("\u001b[91m%s\u001b[0m", delta.Reasoning)

			case delta.Content != "":
				if reasonThinking {
					reasonThinking = false
					fmt.Print("\n\n")
				}

				fmt.Print(delta.Content)
				chunks = append(chunks, delta.Content)
			}
		}
	}

	// Stream ended without an explicit finish reason.
	text := strings.TrimLeft(strings.Join(chunks, " "), "\n")
	return text, nil, lastResp.Usage, nil
}

// startLatencyPrinter starts a goroutine that displays elapsed time while
// waiting for the model's first response chunk. The returned function stops
// the printer; it is safe to call multiple times.
func (a *Agent) startLatencyPrinter(ctx context.Context) (stop func()) {
	modelID := a.krn.ModelInfo().ID
	start := time.Now()

	ticker := time.NewTicker(100 * time.Millisecond)
	done := make(chan struct{})
	exited := make(chan struct{})

	var once sync.Once
	stop = func() {
		once.Do(func() {
			close(done)
			<-exited
		})
	}

	go func() {
		defer ticker.Stop()
		defer close(exited)

		for {
			select {
			case <-ticker.C:
				m := time.Since(start).Milliseconds()
				fmt.Printf("\r\u001b[93m%s %d.%03d\u001b[0m: ", modelID, m/1000, m%1000)

			case <-done:
				fmt.Print("\n")
				return

			case <-ctx.Done():
				fmt.Print("\n")
				return
			}
		}
	}()

	return stop
}

// appendToolCalls adds the assistant's tool call request to the conversation.
func (a *Agent) appendToolCalls(conversation *[]model.D, toolCalls []model.ResponseToolCall) {
	fmt.Print("\n\n")

	var toolCallDocs []model.D
	for _, tc := range toolCalls {
		argsJSON, _ := json.Marshal(tc.Function.Arguments)
		toolCallDocs = append(toolCallDocs, model.D{
			"id":   tc.ID,
			"type": "function",
			"function": model.D{
				"name":      tc.Function.Name,
				"arguments": string(argsJSON),
			},
		})
	}

	*conversation = append(*conversation, model.D{
		"role":       "assistant",
		"tool_calls": toolCallDocs,
	})
}

// appendAssistant adds the assistant's text response to the conversation.
func (a *Agent) appendAssistant(conversation *[]model.D, content string) {
	if content == "" {
		return
	}

	fmt.Print("\n")
	*conversation = append(*conversation, model.D{"role": "assistant", "content": content})
}

// printUsage displays token usage information after each model call.
func (a *Agent) printUsage(usage *model.Usage) {
	if usage == nil {
		return
	}

	contextTokens := usage.PromptTokens + usage.CompletionTokens
	contextWindow := a.krn.ModelConfig().ContextWindow
	percentage := (float64(contextTokens) / float64(contextWindow)) * 100
	of := float32(contextWindow) / float32(1024)

	fmt.Printf("\n\n\u001b[90mInput: %d  Reasoning: %d  Completion: %d  Output: %d  Window: %d (%.0f%% of %.0fK) TPS: %.2f\u001b[0m",
		usage.PromptTokens, usage.ReasoningTokens, usage.CompletionTokens, usage.OutputTokens, contextTokens, percentage, of, usage.TokensPerSecond)
}

// callTools looks up requested tools by name and executes them.
func (a *Agent) callTools(ctx context.Context, toolCalls []model.ResponseToolCall) []model.D {
	resps := make([]model.D, 0, len(toolCalls))

	for _, toolCall := range toolCalls {
		tool, exists := a.tools[toolCall.Function.Name]
		if !exists {
			fmt.Printf("\u001b[91mUnknown tool: %s\u001b[0m\n", toolCall.Function.Name)
			continue
		}

		fmt.Printf("\u001b[92m%s(%v)\u001b[0m: ", toolCall.Function.Name, toolCall.Function.Arguments)

		resp := tool.Call(ctx, toolCall)

		content, _ := resp["content"].(string)
		if strings.Contains(content, `"FAILED"`) {
			fmt.Printf("\u001b[91m%s\u001b[0m\n", content)
		} else {
			fmt.Printf("\u001b[90mok\u001b[0m\n")
		}

		resps = append(resps, resp)
	}

	return resps
}

// =============================================================================

func installSystem() (models.Path, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	// Install llama.cpp libraries.
	libs, err := libs.New(libs.WithVersion(defaults.LibVersion("")))
	if err != nil {
		return models.Path{}, err
	}
	if _, err := libs.Download(ctx, kronk.FmtLogger); err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	// Download catalog system.
	ctlg, err := catalog.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to create catalog system: %w", err)
	}
	if err := ctlg.Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download catalog: %w", err)
	}

	// Download model.
	mdls, err := models.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to create models manager: %w", err)
	}

	var mp models.Path
	switch {
	case modelSpecConfig.SourceURL != "":
		fmt.Println("Downloading model from URL:", modelSpecConfig.SourceURL)
		mp, err = mdls.Download(ctx, kronk.FmtLogger, modelSpecConfig.SourceURL, "")

	case modelSpecConfig.ModelID != "":
		fmt.Println("Downloading model from catalog:", modelSpecConfig.ModelID)
		mp, err = ctlg.DownloadModel(ctx, kronk.FmtLogger, modelSpecConfig.ModelID)

	default:
		return models.Path{}, fmt.Errorf("modelSpecConfig requires either SourceURL or ModelID to be set")
	}
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install model: %w", err)
	}

	return mp, nil
}

func newKronk(mp models.Path) (*kronk.Kronk, error) {
	fmt.Println("loading model...")

	cfg := model.Config{ModelFiles: mp.ModelFiles}
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
