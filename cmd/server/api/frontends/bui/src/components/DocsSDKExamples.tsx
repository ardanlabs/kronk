import { useEffect } from 'react';
import { useLocation } from 'react-router-dom';
import CodeBlock from './CodeBlock';

const agentExample = `// This example shows you how to create a simple agent application against an
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
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// modelSource is the model to download. It may be a HuggingFace URL,
// a canonical "provider/modelID", or a bare model id.
var modelSource = "ornith-ai/Ornith-1.5-9B-Q4_K_M"

// =============================================================================

func main() {
	if err := run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run() error {
	mp, err := installSystem()
	if err != nil {
		return fmt.Errorf("run: unable to install system: %w", err)
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
const systemPrompt = \`You are a helpful coding assistant that has tools to assist you in coding.

After you request a tool call, you will receive a JSON document with two fields,
"status" and "data". Always check the "status" field to know if the call "SUCCEED"
or "FAILED". The information you need to respond will be provided under the "data"
field. If the called "FAILED", just inform the user and don't try using the tool
again for the current response.

When reading Go source code always start counting lines of code from the top of
the source code file.

If you get back results from a tool call, do not verify the results.

Reasoning: high
\`

// Run starts the agent and runs the chat loop.
func (a *Agent) Run(ctx context.Context) error {
	conversation := []model.D{
		{"role": "system", "content": systemPrompt},
	}

	fmt.Printf("\\nChat with %s (use 'ctrl-c' to quit)\\n", a.krn.ModelInfo().ID)

	for {
		nextConversation, ok := a.promptUser(conversation)
		if !ok {
			return nil
		}
		conversation = nextConversation

		// Keep running model turns until the assistant responds without asking
		// for another tool. Only then prompt the user again.
		for {
			content, toolCalls, usage, err := a.streamModelTurn(ctx, conversation)
			if err != nil {
				return err
			}

			a.printUsage(usage)

			if len(toolCalls) == 0 {
				conversation = a.appendAssistant(conversation, content)
				break
			}

			conversation = a.appendToolCalls(conversation, toolCalls)
			conversation = append(conversation, a.callTools(ctx, toolCalls)...)
		}
	}
}

// promptUser asks the user for input and appends it to the conversation.
func (a *Agent) promptUser(conversation []model.D) ([]model.D, bool) {
	fmt.Print("\\u001b[94m\\nYou\\u001b[0m: ")

	userInput, ok := a.getUserMessage()
	if !ok {
		return conversation, false
	}

	conversation = append(conversation, model.D{
		"role":    "user",
		"content": userInput,
	})

	return conversation, true
}

// streamModelTurn sends the conversation to the model and streams back the
// response. It returns the assembled text content, any tool calls, and usage.
func (a *Agent) streamModelTurn(ctx context.Context, conversation []model.D) (string, []model.ResponseToolCall, *model.Usage, error) {
	d := model.D{
		"messages":       conversation,
		"temperature":    0.0,
		"top_p":          0.1,
		"top_k":          1,
		"tools":          a.toolDocuments,
		"tool_selection": "auto",
		"stream_options": model.D{
			"include_usage": true,
		},
	}

	fmt.Printf("\\u001b[93m\\n%s\\u001b[0m: 0.000", a.krn.ModelInfo().ID)

	callCtx, cancelCall := context.WithTimeout(ctx, 5*time.Minute)
	defer cancelCall()

	ch, err := a.krn.ChatStreaming(callCtx, d)
	if err != nil {
		return "", nil, nil, fmt.Errorf("error chat streaming: %w", err)
	}

	// Start the latency printer and ensure it stops.
	stopPrinter := a.startLatencyPrinter(ctx)
	defer stopPrinter()

	var content strings.Builder
	var usage *model.Usage
	var toolCalls []model.ResponseToolCall
	firstChunk := true
	reasoning := false

	for resp := range ch {
		if len(resp.Choices) == 0 {
			usage = resp.Usage
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
					fmt.Printf("\\n\\n\\u001b[92mExecuting %s...\\u001b[0m", tool.Function.Name)
				}
			}

			switch {
			case delta.Reasoning != "":
				reasoning = true

				fmt.Printf("\\u001b[91m%s\\u001b[0m", delta.Reasoning)

			case delta.Content != "":
				if reasoning {
					reasoning = false
					fmt.Print("\\n\\n")
				}

				fmt.Print(delta.Content)
				content.WriteString(delta.Content)
			}
		}
	}

	if len(toolCalls) > 0 {
		return "", toolCalls, usage, nil
	}

	return strings.TrimLeft(content.String(), "\\n"), nil, usage, nil
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
				fmt.Printf("\\r\\u001b[93m%s %d.%03d\\u001b[0m: ", modelID, m/1000, m%1000)

			case <-done:
				fmt.Print("\\n")
				return

			case <-ctx.Done():
				fmt.Print("\\n")
				return
			}
		}
	}()

	return stop
}

// appendToolCalls adds the assistant's tool call request to the conversation.
func (a *Agent) appendToolCalls(conversation []model.D, toolCalls []model.ResponseToolCall) []model.D {
	fmt.Print("\\n\\n")

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

	return append(conversation, model.D{
		"role":       "assistant",
		"tool_calls": toolCallDocs,
	})
}

// appendAssistant adds the assistant's text response to the conversation.
func (a *Agent) appendAssistant(conversation []model.D, content string) []model.D {
	if content == "" {
		return conversation
	}

	fmt.Print("\\n")
	return append(conversation, model.D{"role": "assistant", "content": content})
}

// printUsage displays token usage information after each model call.
func (a *Agent) printUsage(usage *model.Usage) {
	if usage == nil {
		return
	}

	contextTokens := usage.PromptTokens + usage.CompletionTokens
	contextWindow := a.krn.ModelConfig().ContextWindow()
	percentage := (float64(contextTokens) / float64(contextWindow)) * 100
	of := float32(contextWindow) / float32(1024)

	fmt.Printf("\\n\\n\\u001b[90mInput: %d  Reasoning: %d  Completion: %d  Total: %d  Window: %d (%.0f%% of %.0fK) TPS: %.2f\\u001b[0m",
		usage.PromptTokens, usage.CompletionTokensDetails.ReasoningTokens, usage.CompletionTokens, usage.TotalTokens, contextTokens, percentage, of, usage.TokensPerSecond)
}

// callTools looks up requested tools by name and executes them.
func (a *Agent) callTools(ctx context.Context, toolCalls []model.ResponseToolCall) []model.D {
	resps := make([]model.D, 0, len(toolCalls))

	for _, toolCall := range toolCalls {
		tool, exists := a.tools[toolCall.Function.Name]
		if !exists {
			fmt.Printf("\\u001b[91mUnknown tool: %s\\u001b[0m\\n", toolCall.Function.Name)
			resps = append(resps, toolErrorResponse(toolCall.ID, toolCall.Function.Name,
				fmt.Errorf("unknown tool %q", toolCall.Function.Name)))
			continue
		}

		fmt.Printf("\\u001b[92m%s(%v)\\u001b[0m: ", toolCall.Function.Name, toolCall.Function.Arguments)

		resp := tool.Call(ctx, toolCall)

		content, _ := resp["content"].(string)
		if strings.Contains(content, \`"FAILED"\`) {
			fmt.Printf("\\u001b[91m%s\\u001b[0m\\n", content)
		} else {
			fmt.Printf("\\u001b[90mok\\u001b[0m\\n")
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

	// Download model.
	mdls, err := models.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to create models manager: %w", err)
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
		model.WithContextWindow(65536),
		model.WithAutoTune(true),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to create inference model: %w", err)
	}

	fmt.Print("- system info:\\n\\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	fmt.Println("- contextWindow  :", krn.ModelConfig().ContextWindow())
	fmt.Printf("- k/v            : %s/%s\\n", krn.ModelConfig().CacheTypeK, krn.ModelConfig().CacheTypeV)
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
`;

const audioExample = `// This example shows you how to execute a simple prompt against an audio model.
//
// The first time you run this program the system will download and install
// the model and libraries.
//
// Run the example like this from the root of the project:
// $ make example-audio

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// modelSource is the model to download. It may be a HuggingFace URL,
// a canonical "provider/modelID", or a bare model id.
var modelSource = "ggml-org/Qwen2.5-Omni-3B-Q8_0"

const audioFile = "samples/jfk.wav"

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

	if err := audio(krn); err != nil {
		return err
	}

	return nil
}

func installSystem() (models.Path, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
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
		model.WithProjFile(mp.ProjFile),
		model.WithAutoTune(true),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to create inference model: %w", err)
	}

	fmt.Print("- system info:\\n\\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	fmt.Println("- contextWindow  :", krn.ModelConfig().ContextWindow())
	fmt.Printf("- k/v            : %s/%s\\n", krn.ModelConfig().CacheTypeK, krn.ModelConfig().CacheTypeV)
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

func audio(krn *kronk.Kronk) error {
	question := "Transcribe the following audio and then summarize who said it and when."

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

func performChat(ctx context.Context, krn *kronk.Kronk, question string, audioFile string) (<-chan model.ChatResponse, error) {
	audio, err := readImage(audioFile)
	if err != nil {
		return nil, fmt.Errorf("read image: %w", err)
	}

	fmt.Printf("\\nQuestion: %s\\n", question)

	d := model.D{
		"messages":    model.AudioMessage(question, audio, "wav"),
		"max_tokens":  2048,
		"temperature": 0.7,
		"top_p":       0.9,
		"top_k":       40,
		"stream_options": model.D{
			"include_usage": true,
		},
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

	for resp := range ch {
		lr = resp
		if len(resp.Choices) == 0 {
			continue
		}

		switch resp.Choices[0].FinishReason() {
		case model.FinishReasonStop, model.FinishReasonLength:
			continue

		case model.FinishReasonError:
			return fmt.Errorf("error from model: %s", resp.Choices[0].Delta.Content)
		}

		if resp.Choices[0].Delta.Reasoning != "" {
			fmt.Printf("\\u001b[91m%s\\u001b[0m", resp.Choices[0].Delta.Reasoning)
			reasoning = true
			continue
		}

		if reasoning {
			reasoning = false
			fmt.Print("\\n\\n")
		}

		fmt.Printf("%s", resp.Choices[0].Delta.Content)
	}

	// -------------------------------------------------------------------------
	if lr.Usage == nil {
		return fmt.Errorf("stream ended without usage")
	}

	contextTokens := lr.Usage.PromptTokens + lr.Usage.CompletionTokens
	contextWindow := krn.ModelConfig().ContextWindow()
	percentage := (float64(contextTokens) / float64(contextWindow)) * 100
	of := float32(contextWindow) / float32(1024)

	fmt.Printf("\\n\\n\\u001b[90mInput: %d  Reasoning: %d  Completion: %d  Total: %d  Window: %d (%.0f%% of %.0fK) TPS: %.2f\\u001b[0m\\n",
		lr.Usage.PromptTokens, lr.Usage.CompletionTokensDetails.ReasoningTokens, lr.Usage.CompletionTokens, lr.Usage.TotalTokens, contextTokens, percentage, of, lr.Usage.TokensPerSecond)

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

const buckyExample = `// This example shows you how to transcribe an audio file with the
// bucky SDK (whisper.cpp under the hood).
//
// The first time you run this program the system will download and
// install the whisper.cpp libraries and a small whisper model.
//
// Run the example like this from the root of the project:
// $ make example-bucky

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/bucky"
	"github.com/ardanlabs/kronk/sdk/bucky/model"
	buckylibs "github.com/ardanlabs/kronk/sdk/tools/bucky/libs"
	buckymodels "github.com/ardanlabs/kronk/sdk/tools/bucky/models"
)

// modelSource names the bucky whisper model to download. Valid short
// names are listed by models.SupportedModels().
const modelSource = "ggml-tiny.bin"

// audioFile is a 16 kHz mono WAV sample of JFK's "ask not" speech.
const audioFile = "samples/jfk.wav"

func main() {
	if err := run(); err != nil {
		fmt.Printf("\\nERROR: %s\\n", err)
		os.Exit(1)
	}
}

func run() error {
	mp, err := installSystem()
	if err != nil {
		return fmt.Errorf("install system: %w", err)
	}

	w, err := newBucky(mp)
	if err != nil {
		return fmt.Errorf("new whisper: %w", err)
	}
	defer func() {
		fmt.Println("\\nUnloading whisper")
		if err := w.Unload(context.Background()); err != nil {
			fmt.Printf("unload: %v\\n", err)
		}
	}()

	samples, err := loadSamples(audioFile)
	if err != nil {
		return fmt.Errorf("load samples: %w", err)
	}

	if err := transcribe(w, samples); err != nil {
		return fmt.Errorf("transcribe: %w", err)
	}

	if err := detectLanguage(w, samples); err != nil {
		return fmt.Errorf("detect language: %w", err)
	}

	return nil
}

// =============================================================================

func installSystem() (buckymodels.Path, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	lib, err := buckylibs.New(
		buckylibs.WithDetect(ctx, bucky.FmtLogger),
	)
	if err != nil {
		return buckymodels.Path{}, fmt.Errorf("libs new: %w", err)
	}

	if _, err := lib.Download(ctx, bucky.FmtLogger); err != nil {
		return buckymodels.Path{}, fmt.Errorf("download whisper.cpp libs: %w", err)
	}

	if err := bucky.Init(bucky.WithLibPath(lib.LibsPath())); err != nil {
		return buckymodels.Path{}, fmt.Errorf("bucky init: %w", err)
	}

	mdls, err := buckymodels.New()
	if err != nil {
		return buckymodels.Path{}, fmt.Errorf("models new: %w", err)
	}

	fmt.Println("Downloading whisper model:", modelSource)

	mp, err := mdls.Download(ctx, bucky.FmtLogger, modelSource)
	if err != nil {
		return buckymodels.Path{}, fmt.Errorf("download model: %w", err)
	}

	return mp, nil
}

func newBucky(mp buckymodels.Path) (*bucky.Bucky, error) {
	fmt.Println("Initializing bucky / whisper.cpp")

	if len(mp.ModelFiles) == 0 {
		return nil, fmt.Errorf("no model files on disk")
	}

	b, err := bucky.New(
		model.WithModelPath(mp.ModelFiles[0]),
		model.WithUseGPU(true),
		model.WithLog(bucky.FmtLogger),
	)
	if err != nil {
		return nil, fmt.Errorf("create whisper handle: %w", err)
	}

	mi := b.ModelInfo()
	fmt.Println("- model           :", mi.ID)
	fmt.Println("- model type      :", mi.Type)
	fmt.Println("- multilingual    :", mi.IsMultilingual)
	fmt.Println("- text-ctx        :", mi.NTextCtx)
	fmt.Println("- audio-ctx       :", mi.NAudioCtx)
	fmt.Println("- mels            :", mi.NMels)
	fmt.Println("- vocab           :", mi.NVocab)
	fmt.Println("- active-streams  :", b.ActiveStreams())

	return b, nil
}

func loadSamples(path string) ([]float32, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", path, err)
	}
	defer f.Close()

	samples, err := model.Decode(context.Background(), f)
	if err != nil {
		return nil, fmt.Errorf("decode %q: %w", path, err)
	}

	return samples, nil
}

// =============================================================================

func transcribe(b *bucky.Bucky, samples []float32) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	fmt.Println("\\nTranscribing...")
	start := time.Now()

	tr, err := b.Transcribe(ctx, samples,
		model.WithLanguage("en"),
		model.WithOnSegment(func(seg model.Segment) {
			fmt.Printf("  segment %2d [%6dms → %6dms] %s\\n",
				seg.Index, seg.StartMs, seg.EndMs, seg.Text)
		}),
	)
	if err != nil {
		return err
	}

	fmt.Println("\\nFinal Transcription")
	fmt.Println("- language   :", tr.Language)
	fmt.Println("- segments   :", len(tr.Segments))
	fmt.Println("- text       :", tr.Text)
	fmt.Println("- elapsed    :", time.Since(start).Round(time.Millisecond))

	return nil
}

func detectLanguage(w *bucky.Bucky, samples []float32) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Println("\\nDetecting language...")

	lang, probs, err := w.DetectLanguage(ctx, samples, true)
	if err != nil {
		return err
	}

	fmt.Println("- detected   :", lang)

	// Print top 5 candidates by probability.
	type cand struct {
		code string
		prob float32
	}
	tops := make([]cand, 0, 5)
	for id, p := range probs {
		c := cand{code: bucky.LangStr(int32(id)), prob: p}
		switch {
		case len(tops) < cap(tops):
			tops = append(tops, c)
		default:
			worstIdx := 0
			for i, t := range tops {
				if t.prob < tops[worstIdx].prob {
					worstIdx = i
				}
			}
			if c.prob > tops[worstIdx].prob {
				tops[worstIdx] = c
			}
		}
	}

	fmt.Println("- top 5      :")
	for _, c := range tops {
		fmt.Printf("    %-6s %.4f\\n", c.code, c.prob)
	}

	return nil
}
`;

const buckyDiarExample = `// This example shows you how to perform channel-separated speaker
// diarization with the bucky SDK (whisper.cpp under the hood). When each
// speaker is recorded on a dedicated channel — common in call-center and
// meeting captures — TranscribeChannelsFile decodes the audio preserving
// its channel layout, transcribes every channel on its own, and merges
// the results into a single transcript where each segment is tagged with
// the channel (speaker) it came from.
//
// The bundled sample is a 16 kHz stereo WAV with one speaker on each
// channel (speaker 0 on the left, speaker 1 on the right), so the merged,
// time-sorted transcript reads as a back-and-forth conversation.
//
// The first time you run this program the system will download and
// install the whisper.cpp libraries and a small whisper model.
//
// Run the example like this from the root of the project:
// $ make example-bucky-diar

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/bucky"
	"github.com/ardanlabs/kronk/sdk/bucky/model"
	buckylibs "github.com/ardanlabs/kronk/sdk/tools/bucky/libs"
	buckymodels "github.com/ardanlabs/kronk/sdk/tools/bucky/models"
)

// modelSource names the bucky whisper model to download. Valid short
// names are listed by models.SupportedModels(). A multilingual model is
// used so each channel's language is auto-detected (the sample has an
// English speaker on the left and a Spanish speaker on the right).
const modelSource = "tiny"

// audioFile is a 16 kHz stereo WAV sample with one speaker per channel.
const audioFile = "samples/stereo-speakers.wav"

func main() {
	if err := run(); err != nil {
		fmt.Printf("\\nERROR: %s\\n", err)
		os.Exit(1)
	}
}

func run() error {
	mp, err := installSystem()
	if err != nil {
		return fmt.Errorf("install system: %w", err)
	}

	b, err := newBucky(mp)
	if err != nil {
		return fmt.Errorf("new bucky: %w", err)
	}
	defer func() {
		fmt.Println("\\nUnloading whisper")
		if err := b.Unload(context.Background()); err != nil {
			fmt.Printf("unload: %v\\n", err)
		}
	}()

	if err := diarize(b, audioFile); err != nil {
		return fmt.Errorf("diarize: %w", err)
	}

	return nil
}

// =============================================================================

// diarize decodes the audio file preserving its channel layout, transcribes
// each channel as its own speaker, and prints both the per-speaker
// transcripts and the merged, time-sorted segment stream.
func diarize(b *bucky.Bucky, path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %q: %w", path, err)
	}
	defer f.Close()

	fmt.Printf("\\nDiarizing %s (one speaker per channel)...\\n", path)
	start := time.Now()

	// No language is set so whisper auto-detects each channel's language
	// independently (English on the left, Spanish on the right).
	d, err := b.TranscribeChannelsFile(ctx, f)
	if err != nil {
		return err
	}

	// d.Channels holds one Transcription per source channel.
	fmt.Println("\\nPer-Speaker Transcripts")
	for _, ct := range d.Channels {
		fmt.Printf("- speaker %d: %s\\n", ct.Channel, ct.Text)
	}

	// d.Segments merges every channel's segments sorted by start time,
	// each tagged with the channel (speaker) it came from.
	fmt.Println("\\nMerged Timeline")
	for _, s := range d.Segments {
		fmt.Printf("  [%6dms → %6dms] speaker %d: %s\\n",
			s.StartMs, s.EndMs, s.Channel, s.Text)
	}

	fmt.Println("\\n- speakers   :", len(d.Channels))
	fmt.Println("- segments   :", len(d.Segments))
	fmt.Println("- elapsed    :", time.Since(start).Round(time.Millisecond))

	return nil
}

// =============================================================================

func installSystem() (buckymodels.Path, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	lib, err := buckylibs.New(
		buckylibs.WithDetect(ctx, bucky.FmtLogger),
	)
	if err != nil {
		return buckymodels.Path{}, fmt.Errorf("libs new: %w", err)
	}

	if _, err := lib.Download(ctx, bucky.FmtLogger); err != nil {
		return buckymodels.Path{}, fmt.Errorf("download whisper.cpp libs: %w", err)
	}
	if err := bucky.Init(bucky.WithLibPath(lib.LibsPath())); err != nil {
		return buckymodels.Path{}, fmt.Errorf("bucky init: %w", err)
	}

	mdls, err := buckymodels.New()
	if err != nil {
		return buckymodels.Path{}, fmt.Errorf("models new: %w", err)
	}

	fmt.Println("Downloading whisper model:", modelSource)

	mp, err := mdls.Download(ctx, bucky.FmtLogger, modelSource)
	if err != nil {
		return buckymodels.Path{}, fmt.Errorf("download model: %w", err)
	}

	return mp, nil
}

func newBucky(mp buckymodels.Path) (*bucky.Bucky, error) {
	fmt.Println("Initializing bucky / whisper.cpp")

	if len(mp.ModelFiles) == 0 {
		return nil, fmt.Errorf("no model files on disk")
	}

	b, err := bucky.New(
		model.WithModelPath(mp.ModelFiles[0]),
		model.WithUseGPU(true),
		model.WithLog(bucky.FmtLogger),
	)
	if err != nil {
		return nil, fmt.Errorf("create whisper handle: %w", err)
	}

	mi := b.ModelInfo()
	fmt.Println("- model           :", mi.ID)
	fmt.Println("- model type      :", mi.Type)
	fmt.Println("- multilingual    :", mi.IsMultilingual)
	fmt.Println("- active-streams  :", b.ActiveStreams())

	return b, nil
}
`;

const buckyStreamExample = `// This example is a LIVE MICROPHONE transcription demo for the bucky
// streaming API. It captures the default input device, streams the audio
// through a *model.Stream, and renders the transcript live: partial
// hypotheses are re-rendered in place (words appear, then get revised as
// you keep talking), and finals are committed on their own line — the same
// effect as whisper.cpp's stream example. Say "STOP" to end.
//
// The streaming SDK itself is pure Go (no CGO). This example adds CGO only
// for microphone capture via github.com/gen2brain/malgo (miniaudio), which
// lives entirely in the examples module.
//
// Run the example like this from the root of the project:
// $ make example-bucky-stream
// $ make example-bucky-stream-vad

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/sdk/bucky"
	"github.com/ardanlabs/kronk/sdk/bucky/model"
	buckylibs "github.com/ardanlabs/kronk/sdk/tools/bucky/libs"
	buckymodels "github.com/ardanlabs/kronk/sdk/tools/bucky/models"
	"github.com/gen2brain/malgo"
)

// modelSource names the bucky whisper model to download.
const modelSource = "ggml-tiny.bin"

// vadModelSource names the optional Silero VAD model to download.
const vadModelSource = "silero-vad"

// micRate / micChannels are the format we ask the capture device for.
// miniaudio converts the hardware's native format to this for us, so we
// hand the stream exactly the 16 kHz mono int16 it wants with no resample.
const (
	micRate     = 16000
	micChannels = 1
)

// ANSI helpers for the live-rewrite UX. eraseLine clears the current line
// and returns the cursor to column 0 so the next print overwrites it —
// this is what produces the "words change as you talk" effect.
const (
	eraseLine = "\\033[2K\\r"
	colYellow = "\\033[33m"
	colGreen  = "\\033[32m"
	colRed    = "\\033[31m"
	colReset  = "\\033[0m"
)

type installedModels struct {
	whisper buckymodels.Path
	vadPath string
}

func main() {
	sileroVAD := flag.Bool("silero-vad", false, "use the Silero model instead of energy-based VAD")
	vadThreshold := flag.Float64("vad-threshold", 0.5, "Silero speech probability threshold (0, 1]")
	flag.Parse()

	if err := run(*sileroVAD, float32(*vadThreshold)); err != nil {
		fmt.Printf("\\nERROR: %s\\n", err)
		os.Exit(1)
	}
}

func run(sileroVAD bool, vadThreshold float32) error {
	if sileroVAD && (vadThreshold <= 0 || vadThreshold > 1) {
		return fmt.Errorf("vad threshold %v is outside (0, 1]", vadThreshold)
	}

	models, err := installSystem(sileroVAD)
	if err != nil {
		return fmt.Errorf("install system: %w", err)
	}

	b, err := newBucky(models.whisper)
	if err != nil {
		return fmt.Errorf("new bucky: %w", err)
	}
	defer func() {
		fmt.Println("Unloading whisper")
		if err := b.Unload(context.Background()); err != nil {
			fmt.Printf("unload: %v\\n", err)
		}
	}()

	if err := liveTranscribe(b, models.vadPath, vadThreshold); err != nil {
		return fmt.Errorf("live transcribe: %w", err)
	}

	return nil
}

// =============================================================================

// liveTranscribe opens a streaming session, wires the default microphone
// into it, and renders the transcript live until the speaker says "STOP"
// (or presses Ctrl-C).
func liveTranscribe(b *bucky.Bucky, vadPath string, vadThreshold float32) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Energy-based VAD remains the zero-configuration default. When
	// --silero-vad is set, these options replace it with learned speech
	// probabilities from the downloaded Silero model.
	streamOpts := []model.StreamOption{
		model.WithStreamLanguage("en"),
		model.WithPartialEveryMs(700),
	}
	if vadPath != "" {
		streamOpts = append(streamOpts,
			model.WithVADModel(vadPath),
			model.WithVADSpeechThreshold(vadThreshold),
		)
	}

	stream, err := b.NewStream(ctx, streamOpts...)
	if err != nil {
		return fmt.Errorf("new stream: %w", err)
	}
	defer stream.Close()

	// Consumer: render partials in place, commit finals, and watch for the
	// spoken "STOP" command. Runs until Events closes (after stream.Close).
	saidStop := make(chan struct{})
	consumerDone := make(chan struct{})
	go func() {
		defer close(consumerDone)
		consume(stream, saidStop)
	}()

	// The audio callback runs on miniaudio's realtime thread; it must never
	// block. It copies the captured bytes and hands them to a buffered
	// channel that a pump goroutine drains into the stream.
	pcmC := make(chan []byte, 64)
	onFrames := func(_, in []byte, _ uint32) {
		select {
		case pcmC <- append([]byte(nil), in...):
		default: // pump is behind; drop rather than stall the audio thread
		}
	}

	device, mctx, err := openMic(onFrames)
	if err != nil {
		return fmt.Errorf("open mic: %w", err)
	}
	defer func() {
		device.Uninit()
		_ = mctx.Uninit()
		mctx.Free()
	}()

	// Pump: convert + feed raw mic PCM into the stream. FeedPCM does the
	// pure-Go int16 -> float32 conversion (and would downmix/resample if
	// the format differed from the engine's 16 kHz mono).
	micFormat := model.AudioFormat{SampleRate: micRate, Channels: micChannels, Sample: model.Int16LE}
	go func() {
		for buf := range pcmC {
			if err := stream.FeedPCM(ctx, buf, micFormat); err != nil {
				return
			}
		}
	}()

	if err := device.Start(); err != nil {
		return fmt.Errorf("start mic: %w", err)
	}

	fmt.Printf("\\n%s🎤 Mic is live — say something. Say \\"STOP\\" to end.%s\\n\\n", colGreen, colReset)

	// Wait for the spoken stop word or Ctrl-C.
	select {
	case <-saidStop:
	case <-ctx.Done():
	}

	fmt.Printf("\\n\\nStopping…\\n")
	_ = device.Stop()
	close(pcmC)        // pump exits
	_ = stream.Close() // final flush + closes Events
	<-consumerDone     // let the consumer print the closing Final

	return nil
}

// consume renders transcript events. Partials overwrite the live line;
// Finals commit on their own line. The moment the word "stop" appears in
// either a partial or a final, it signals saidStop once so the program
// ends immediately rather than waiting for the next pause.
func consume(stream *model.Stream, saidStop chan struct{}) {
	signaled := false
	signalStop := func(text string) {
		if !signaled && containsWord(text, "stop") {
			signaled = true
			close(saidStop)
		}
	}

	for ev := range stream.Events() {
		switch ev.Kind {
		case model.EventPartial:
			fmt.Printf("%s%s%s%s", eraseLine, colYellow, ev.Text, colReset)
			signalStop(ev.Text)

		case model.EventFinal:
			fmt.Printf("%s%s%s%s\\n", eraseLine, colGreen, ev.Text, colReset)
			signalStop(ev.Text)

		case model.EventError:
			fmt.Printf("\\n%serror: %v%s\\n", colRed, ev.Err, colReset)
		}
	}
}

// containsWord reports whether text contains word as a whole word,
// case-insensitively and ignoring surrounding punctuation (so "Stop.",
// "STOP!" and "stop" all match).
func containsWord(text, word string) bool {
	for f := range strings.FieldsSeq(strings.ToLower(text)) {
		if strings.Trim(f, ".,!?;:\\"'\`-") == word {
			return true
		}
	}
	return false
}

// =============================================================================

// openMic initializes the miniaudio context and the default capture device
// configured for 16 kHz mono int16, wiring onFrames as the data callback.
func openMic(onFrames malgo.DataProc) (*malgo.Device, *malgo.AllocatedContext, error) {
	mctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, func(string) {})
	if err != nil {
		return nil, nil, fmt.Errorf("init audio context: %w", err)
	}

	cfg := malgo.DefaultDeviceConfig(malgo.Capture)
	cfg.Capture.Format = malgo.FormatS16
	cfg.Capture.Channels = micChannels
	cfg.SampleRate = micRate
	cfg.Alsa.NoMMap = 1

	device, err := malgo.InitDevice(mctx.Context, cfg, malgo.DeviceCallbacks{Data: onFrames})
	if err != nil {
		_ = mctx.Uninit()
		mctx.Free()
		return nil, nil, fmt.Errorf("init capture device: %w", err)
	}

	return device, mctx, nil
}

// =============================================================================

func installSystem(sileroVAD bool) (installedModels, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	lib, err := buckylibs.New(
		buckylibs.WithDetect(ctx, bucky.FmtLogger),
	)
	if err != nil {
		return installedModels{}, fmt.Errorf("libs new: %w", err)
	}

	if _, err := lib.Download(ctx, bucky.FmtLogger); err != nil {
		return installedModels{}, fmt.Errorf("download whisper.cpp libs: %w", err)
	}
	if err := bucky.Init(bucky.WithLibPath(lib.LibsPath())); err != nil {
		return installedModels{}, fmt.Errorf("bucky init: %w", err)
	}

	mdls, err := buckymodels.New()
	if err != nil {
		return installedModels{}, fmt.Errorf("models new: %w", err)
	}

	fmt.Println("Downloading whisper model:", modelSource)

	mp, err := mdls.Download(ctx, bucky.FmtLogger, modelSource)
	if err != nil {
		return installedModels{}, fmt.Errorf("download model: %w", err)
	}

	models := installedModels{whisper: mp}
	if !sileroVAD {
		return models, nil
	}

	fmt.Println("Downloading VAD model:", vadModelSource)

	vad, err := mdls.Download(ctx, bucky.FmtLogger, vadModelSource)
	if err != nil {
		return installedModels{}, fmt.Errorf("download VAD model: %w", err)
	}
	if len(vad.ModelFiles) == 0 {
		return installedModels{}, fmt.Errorf("download VAD model: no model files on disk")
	}
	models.vadPath = vad.ModelFiles[0]

	return models, nil
}

func newBucky(mp buckymodels.Path) (*bucky.Bucky, error) {
	fmt.Println("Initializing bucky / whisper.cpp")

	if len(mp.ModelFiles) == 0 {
		return nil, fmt.Errorf("no model files on disk")
	}

	b, err := bucky.New(
		model.WithModelPath(mp.ModelFiles[0]),
		model.WithUseGPU(true),
		model.WithLog(bucky.FmtLogger),
	)
	if err != nil {
		return nil, fmt.Errorf("create whisper handle: %w", err)
	}

	mi := b.ModelInfo()
	fmt.Println("- model           :", mi.ID)
	fmt.Println("- multilingual    :", mi.IsMultilingual)
	fmt.Println("- active-streams  :", b.ActiveStreams())

	return b, nil
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
		fmt.Printf("\\nERROR: %s\\n", err)
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
		fmt.Println("\\nUnloading Kronk")
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

	fmt.Print("- system info:\\n\\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	fmt.Println("- contextWindow  :", krn.ModelConfig().ContextWindow())
	fmt.Printf("- k/v            : %s/%s\\n", krn.ModelConfig().CacheTypeK, krn.ModelConfig().CacheTypeV)
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

const systemPrompt = \`You are designed to help users answer questions, create
content, and provide information in a helpful and accurate manner. Always follow
the user's instructions carefully and respond with clear, concise, and
well-structured answers.\`

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
	fmt.Print("\\nUSER> ")

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

	fmt.Print("\\nMODEL> ")

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
					fmt.Printf("\\n\\n\\u001b[92mExecuting %s...\\u001b[0m", tool.Function.Name)
				}
			}

			switch {
			case delta.Reasoning != "":
				reasoning = true
				fmt.Printf("\\u001b[91m%s\\u001b[0m", delta.Reasoning)

			case delta.Content != "":
				if reasoning {
					reasoning = false
					fmt.Print("\\n\\n")
				}

				fmt.Print(delta.Content)
				content.WriteString(delta.Content)
			}
		}
	}

	if len(toolCalls) > 0 {
		return "", toolCalls, usage, nil
	}

	return strings.TrimLeft(content.String(), "\\n"), nil, usage, nil
}

func appendToolCalls(conversation []model.D, toolCalls []model.ResponseToolCall) []model.D {
	fmt.Print("\\n\\n")

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

	fmt.Print("\\n")
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

	fmt.Printf("\\n\\n\\u001b[90mInput: %d  Reasoning: %d  Completion: %d  Total: %d  Window: %d (%.0f%% of %.0fK) TPS: %.2f\\u001b[0m",
		usage.PromptTokens, usage.CompletionTokensDetails.ReasoningTokens, usage.CompletionTokens, usage.TotalTokens, contextTokens, percentage, of, usage.TokensPerSecond)
}

func callTools(toolCalls []model.ResponseToolCall) []model.D {
	results := make([]model.D, 0, len(toolCalls))

	for _, toolCall := range toolCalls {
		fmt.Printf("\\u001b[92m%s(%v)\\u001b[0m: \\u001b[90mok\\u001b[0m\\n",
			toolCall.Function.Name, toolCall.Function.Arguments)

		// This example hard-codes the tool execution and its result. The agent
		// example replaces this with registered Tool implementations.
		results = append(results, model.D{
			"role":         "tool",
			"name":         toolCall.Function.Name,
			"tool_call_id": toolCall.ID,
			"content":      \`{"temperature": "72°F", "condition": "sunny"}\`,
		})
	}

	return results
}
`;

const concurrencyExample = `// This example shows you how to leverage Kronk's batch processing by running
// multiple inference requests concurrently against a single loaded model. It
// classifies trail-cam images using a small vision model.
//
// The first time you run this program the system will download and install
// the model and libraries.
//
// Run the example like this from the root of the project:
// $ make example-concurrency

package main

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"uuid"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

const (
	modelSource    = "unsloth/Qwen3.5-0.8B-Q8_0"
	imageLocation  = "samples/deer"
	numWorkers     = 2
	numRequests    = 1500
	requestTimeout = 60 * time.Second
)

func main() {
	if err := run(); err != nil {
		fmt.Println(err)
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
			fmt.Printf("failed to unload model: %v\\n", err)
		}
	}()

	return classifyImages(krn)
}

func installSystem() (models.Path, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
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
	fmt.Println("Loading model...")

	krn, err := kronk.New(
		model.WithModelFiles(mp.ModelFiles),
		model.WithProjFile(mp.ProjFile),
		model.WithAutoTune(true),
		model.WithIncrementalCache(false),
		model.WithContextWindow(8*1024),
		model.WithNSeqMax(2),
		model.WithLog(kronk.FmtLogger),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to create inference model: %w", err)
	}

	printModelInfo(krn)

	return krn, nil
}

func printModelInfo(krn *kronk.Kronk) {
	info := krn.SystemInfo()
	keys := make([]string, 0, len(info))
	for k := range info {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fmt.Print("- system info:\\n\\t")
	for _, k := range keys {
		fmt.Printf("%s:%v, ", k, info[k])
	}
	fmt.Println()

	cfg := krn.ModelConfig()
	mi := krn.ModelInfo()

	fmt.Println("- contextWindow  :", cfg.ContextWindow())
	fmt.Printf("- k/v            : %s/%s\\n", cfg.CacheTypeK, cfg.CacheTypeV)
	fmt.Println("- flashAttention :", cfg.FlashAttention())
	fmt.Println("- prefill batch  :", cfg.PrefillBatchSize())
	fmt.Println("- modelType      :", mi.Type)
	fmt.Println("- template       :", mi.Template.FileName)
	fmt.Println("- grammar        :", cfg.DefaultParams.Grammar != "")
	fmt.Println("- nSeqMax        :", cfg.NSeqMax())
	fmt.Println("- vramTotal      :", mi.VRAMTotal/(1024*1024), "MiB")
	fmt.Println("- slotMemory     :", mi.SlotMemory/(1024*1024), "MiB")
	fmt.Println("- modelSize      :", mi.Size/(1000*1000), "MB")
	fmt.Println("- imc            :", cfg.IncrementalCache())
	if n := cfg.PtrNGpuLayers; n != nil {
		fmt.Println("- nGPULayers     :", *n)
	} else {
		fmt.Println("- nGPULayers     : all")
	}
	if sm := cfg.PtrSplitMode; sm != nil {
		fmt.Println("- splitMode      :", sm)
	} else {
		fmt.Println("- splitMode      : auto")
	}
}

const prompt = \`Analyze the attached trail cam picture and determine if there
are any deer in that picture. If there are deer determine if any of the deer
have antlers. If there is a deer with antlers return: Buck. If there is deer but
none with antlers return: Doe. If there are no deer in the picture return: None.
Analyze carefully, because the deer can be behind some grasses or trees.
Sometimes the deer antlers can be obstructed by trees or grasses. You can only
respond with 1 of 3 possible values, value 1: Buck, value 2: Doe or value 3:
None. Do not return any other characters.
 \`

const systemPrompt = \`You are a helpful AI assistant. You are designed to help
users identify images and provide information in a helpful and accurate manner.
Always follow the user's instructions carefully.\`

func classifyImages(krn *kronk.Kronk) error {
	imageFiles, err := listImages(imageLocation)
	if err != nil {
		return fmt.Errorf("listImages: %w", err)
	}

	fmt.Printf("\\n- Number of images: %d\\n", len(imageFiles))

	if len(imageFiles) == 0 {
		return fmt.Errorf("no images to process")
	}

	// -------------------------------------------------------------------------
	// Start a pool of workers. Each worker pulls image paths off the channel,
	// runs inference, and prints the result.

	ch := make(chan string)
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for id := range numWorkers {
		go func() {
			defer func() {
				fmt.Printf("g[%d]: done\\n", id)
				wg.Done()
			}()

			for imageFile := range ch {
				processImage(krn, id, imageFile)
			}
		}()
	}

	// -------------------------------------------------------------------------
	// Send numRequests randomly chosen images through the pool, then close the
	// channel and wait for the workers to drain.

	for range numRequests {
		ch <- imageFiles[rand.IntN(len(imageFiles))]
	}

	close(ch)
	wg.Wait()

	return nil
}

func processImage(krn *kronk.Kronk, workerID int, imageFile string) {
	traceID := uuid.New().String()

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	ctx = applog.SetTraceID(ctx, traceID)

	imageData, err := os.ReadFile(imageFile)
	if err != nil {
		fmt.Printf("g[%d]: traceID %s: image %s: ERROR: read image: %s\\n", workerID, traceID, imageFile, err)
		return
	}

	imageType := "jpeg"
	if strings.EqualFold(filepath.Ext(imageFile), ".png") {
		imageType = "png"
	}

	params := model.D{
		"messages": model.Messages(
			model.TextMessage(model.RoleSystem, systemPrompt),
			model.ImageMessage(prompt, imageData, imageType),
		),
		"enable_thinking": false,
		"temperature":     1.0,
		"top_p":           0.95,
		"top_k":           64,
		"max_tokens":      2048,
	}

	resp, err := krn.Chat(ctx, params)
	if err != nil {
		fmt.Printf("g[%d]: traceID %s: image %s: ERROR: chat streaming: %s\\n", workerID, traceID, imageFile, err)
		return
	}

	fmt.Printf("g[%d]: traceID %s: image %s: Resp: %s\\n", workerID, traceID, imageFile, strings.Trim(resp.Choices[0].Message.Content, "\\n"))
}

func listImages(imageLocation string) ([]string, error) {
	entries, err := os.ReadDir(imageLocation)
	if err != nil {
		return nil, fmt.Errorf("unable to read directory %q: %w", imageLocation, err)
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		files = append(files, filepath.Join(imageLocation, entry.Name()))
	}

	return files, nil
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
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// modelSource is the model to download. It may be a HuggingFace URL,
// a canonical "provider/modelID", or a bare model id.
var modelSource = "Qwen/Qwen3-Embedding-0.6B-Q8_0.gguf"

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

	if err := embedding(krn); err != nil {
		return err
	}

	return nil
}

func installSystem() (models.Path, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
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

	fmt.Print("- system info:\\n\\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	fmt.Println("- contextWindow  :", krn.ModelConfig().ContextWindow())
	fmt.Printf("- k/v            : %s/%s\\n", krn.ModelConfig().CacheTypeK, krn.ModelConfig().CacheTypeV)
	fmt.Println("- flashAttention :", krn.ModelConfig().FlashAttention())
	fmt.Println("- prefill batch  :", krn.ModelConfig().PrefillBatchSize())
	fmt.Println("- embeddings     :", krn.ModelInfo().IsEmbedModel)
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
	fmt.Println("Created:", time.Unix(resp.Created, 0))
	fmt.Println("  Index    :", resp.Data[0].Index)
	fmt.Println("  Object   :", resp.Data[0].Object)
	fmt.Println("  Length   :", len(resp.Data[0].Embedding))
	fmt.Printf("  Embedding: [%v...%v]\\n", resp.Data[0].Embedding[:3], resp.Data[0].Embedding[len(resp.Data[0].Embedding)-3:])

	return nil
}
`;

const grammarExample = `// This example shows how to use GBNF grammars to constrain model output.
// Grammars force the model to only produce tokens that match the specified
// pattern, guaranteeing structured output.
//
// Run the example like this from the root of the project:
// $ make example-grammar

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

var grammarJSONObject = \`root ::= object
value ::= object | array | string | number | "true" | "false" | "null"
object ::= "{" ws ( string ":" ws value ("," ws string ":" ws value)* )? ws "}"
array ::= "[" ws ( value ("," ws value)* )? ws "]"
string ::= "\\"" ([^"\\\\] | "\\\\" ["\\\\bfnrt/] | "\\\\u" [0-9a-fA-F]{4})* "\\""
number ::= "-"? ("0" | [1-9][0-9]*) ("." [0-9]+)? ([eE] [+-]? [0-9]+)?
ws ::= [ \\t\\n\\r]*\`

// modelSource is the model to download. It may be a HuggingFace URL,
// a canonical "provider/modelID", or a bare model id.
var modelSource = "unsloth/Qwen3-0.6B-Q8_0"

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

	// -------------------------------------------------------------------------
	// Example 1: Using a grammar preset (GrammarJSONObject)

	fmt.Println("=== Example 1: Grammar Preset (JSON Object) ===")
	if err := grammarPreset(krn); err != nil {
		fmt.Println(err)
	}

	// -------------------------------------------------------------------------
	// Example 2: Using a JSON Schema to auto-generate grammar

	fmt.Println("\\n=== Example 2: JSON Schema ===")
	if err := jsonSchema(krn); err != nil {
		fmt.Println(err)
	}

	// -------------------------------------------------------------------------
	// Example 3: Custom grammar for constrained choices

	fmt.Println("\\n=== Example 3: Custom Grammar (Sentiment Analysis) ===")
	if err := customGrammar(krn); err != nil {
		fmt.Println(err)
	}

	return nil
}

func installSystem() (models.Path, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
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

	fmt.Print("- system info:\\n\\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	fmt.Println("- contextWindow  :", krn.ModelConfig().ContextWindow())
	fmt.Printf("- k/v            : %s/%s\\n", krn.ModelConfig().CacheTypeK, krn.ModelConfig().CacheTypeV)
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

// grammarPreset demonstrates using a built-in grammar preset to force JSON output.
func grammarPreset(krn *kronk.Kronk) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	prompt := "List 3 programming languages with their year of creation. Respond in JSON format."

	fmt.Println("PROMPT:", prompt)
	fmt.Println()

	d := model.D{
		"messages": model.DocumentArray(
			model.TextMessage(model.RoleUser, prompt),
		),
		"grammar":         grammarJSONObject,
		"enable_thinking": false, // Grammar requires output to match from first token
		"temperature":     0.7,
		"max_tokens":      512,
	}

	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		return fmt.Errorf("chat streaming: %w", err)
	}

	fmt.Print("RESPONSE: ")

	for resp := range ch {
		switch resp.Choices[0].FinishReason() {
		case model.FinishReasonError:
			return fmt.Errorf("error from model: %s", resp.Choices[0].Delta.Content)

		case model.FinishReasonStop:
			fmt.Println()
			return nil

		default:
			fmt.Print(resp.Choices[0].Delta.Content)
		}
	}

	return nil
}

// jsonSchema demonstrates using a JSON Schema to auto-generate a grammar.
// This gives you more control over the exact structure of the output.
func jsonSchema(krn *kronk.Kronk) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	prompt := "Describe the Go programming language."

	fmt.Println("PROMPT:", prompt)
	fmt.Println()

	// Define the expected output structure using JSON Schema.
	schema := model.D{
		"type": "object",
		"properties": model.D{
			"name": model.D{
				"type": "string",
			},
			"year": model.D{
				"type": "integer",
			},
			"paradigm": model.D{
				"type": "string",
				"enum": []string{"procedural", "object-oriented", "functional", "concurrent"},
			},
			"compiled": model.D{
				"type": "boolean",
			},
		},
		"required": []string{"name", "year", "paradigm", "compiled"},
	}

	d := model.D{
		"messages": model.DocumentArray(
			model.TextMessage(model.RoleUser, prompt),
		),
		"json_schema":     schema,
		"enable_thinking": false, // Grammar requires output to match from first token
		"temperature":     0.7,
		"max_tokens":      256,
	}

	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		return fmt.Errorf("chat streaming: %w", err)
	}

	fmt.Print("RESPONSE: ")

	for resp := range ch {
		switch resp.Choices[0].FinishReason() {
		case model.FinishReasonError:
			return fmt.Errorf("error from model: %s", resp.Choices[0].Delta.Content)

		case model.FinishReasonStop:
			fmt.Println()
			return nil

		default:
			fmt.Print(resp.Choices[0].Delta.Content)
		}
	}

	return nil
}

// customGrammar demonstrates writing a custom GBNF grammar to constrain
// output to specific choices. This is useful for classification tasks.
func customGrammar(krn *kronk.Kronk) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Custom grammar that only allows specific sentiment values.
	// The model MUST output one of these exact strings.
	sentimentGrammar := \`root ::= sentiment
sentiment ::= "positive" | "negative" | "neutral"\`

	prompt := \`Analyze the sentiment of this text and respond with exactly one word.

Text: "I absolutely love this product! It exceeded all my expectations and I would recommend it to everyone."

Sentiment:\`

	fmt.Println("PROMPT:", prompt)
	fmt.Println()

	d := model.D{
		"messages": model.DocumentArray(
			model.TextMessage(model.RoleUser, prompt),
		),
		"grammar":         sentimentGrammar,
		"enable_thinking": false, // Grammar requires output to match from first token
		"temperature":     0.0,
		"max_tokens":      16,
	}

	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		return fmt.Errorf("chat streaming: %w", err)
	}

	fmt.Print("RESPONSE: ")

	for resp := range ch {
		switch resp.Choices[0].FinishReason() {
		case model.FinishReasonError:
			return fmt.Errorf("error from model: %s", resp.Choices[0].Delta.Content)

		case model.FinishReasonStop:
			fmt.Println()
			return nil

		default:
			fmt.Print(resp.Choices[0].Delta.Content)
		}
	}

	return nil
}
`;

const malinaExample = `// This example shows you how to generate an image with the Malina SDK
// (stable-diffusion.cpp under the hood).
//
// Experimental: The Malina SDK public API is subject to change.
//
// The first time you run this program the system will download and install the
// stable-diffusion.cpp libraries and a Stable Diffusion 1.5 model bundle.
//
// Run the example like this from the root of the project:
// $ make example-malina

package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ardanlabs/kronk/sdk/malina"
	"github.com/ardanlabs/kronk/sdk/malina/model"
	"github.com/ardanlabs/kronk/sdk/tools/malina/libs"
	"github.com/ardanlabs/kronk/sdk/tools/malina/models"
)

// modelSource names the curated Malina model bundle to download. Valid names
// are listed by models.SupportedBundles().
var (
	modelSource = models.BundleSD15.String()
	progressMu  sync.Mutex
)

const (
	outputFile = "malina.png"
	prompt     = "a small red sailboat crossing a calm mountain lake at sunrise"
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

	mln, err := newMalina(mp)
	if err != nil {
		return fmt.Errorf("unable to init Malina: %w", err)
	}
	defer func() {
		fmt.Println("\\nUnloading Malina")
		if err := mln.Unload(context.Background()); err != nil {
			fmt.Printf("unload: %v\\n", err)
		}
	}()

	if err := generate(mln); err != nil {
		return fmt.Errorf("generate: %w", err)
	}

	return nil
}

// =============================================================================

func installSystem() (models.Path, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	libs, err := libs.New(
		libs.WithDetect(ctx, malina.FmtLogger),
		libs.WithValidation(true),
	)
	if err != nil {
		return models.Path{}, err
	}

	if _, err := libs.Download(ctx, malina.FmtLogger); err != nil {
		return models.Path{}, fmt.Errorf("unable to install stable-diffusion.cpp: %w", err)
	}

	if err := malina.Init(
		malina.WithLibPath(libs.LibsPath()),
		malina.WithProgress(progress),
	); err != nil {
		return models.Path{}, fmt.Errorf("unable to init Malina: %w", err)
	}

	// -------------------------------------------------------------------------

	mdls, err := models.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to init models: %w", err)
	}

	fmt.Println("Downloading model bundle:", modelSource)

	mp, err := mdls.Download(ctx, malina.FmtLogger, modelSource)
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install model bundle: %w", err)
	}

	return mp, nil
}

func newMalina(mp models.Path) (*malina.Malina, error) {
	fmt.Println("Loading model...")

	if len(mp.ModelFiles) == 0 {
		return nil, fmt.Errorf("no model files on disk")
	}

	mln, err := malina.New(
		model.WithModelPath(mp.ModelFiles[0]),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to create image generation model: %w", err)
	}

	si := mln.SystemInfo()
	cfg := mln.ModelConfig()
	mi := mln.ModelInfo()

	fmt.Println("- native version    :", si.NativeVersion)
	fmt.Println("- physical cores    :", si.PhysicalCores)
	fmt.Println("- backend devices   :", si.BackendDeviceCount)
	fmt.Println("- model             :", mi.ModelPath)
	fmt.Println("- cpu threads       :", cfg.CPUThreads)
	fmt.Println("- concurrency       :", cfg.Concurrency)
	fmt.Println("- queue depth       :", cfg.QueueDepth)
	fmt.Println("- active generations:", mln.ActiveGenerations())

	return mln, nil
}

// =============================================================================

func generate(mln *malina.Malina) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	fmt.Println("\\nGenerating image...")
	fmt.Println("- prompt           :", prompt)

	params := model.NewGenerateParams()
	params.Prompt = prompt
	params.Seed = 42

	start := time.Now()
	image, err := mln.Generate(ctx, params)
	if err != nil {
		return err
	}

	if err := os.WriteFile(outputFile, image.PNG, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", outputFile, err)
	}

	fmt.Println("- dimensions       :", fmt.Sprintf("%dx%d", image.Width, image.Height))
	fmt.Println("- seed             :", image.Seed)
	fmt.Println("- elapsed          :", time.Since(start).Round(time.Millisecond))
	fmt.Println("- output           :", outputFile)

	return nil
}

// progress renders model loading and image generation progress reported by
// stable-diffusion.cpp.
func progress(step int, steps int, secondsPerStep float32) {
	if step <= 0 || steps <= 0 {
		return
	}

	progressMu.Lock()
	defer progressMu.Unlock()

	const width = 50
	current := min(step, steps)
	filled := min(current*width/steps, width)
	bar := strings.Repeat("=", filled)
	if filled < width {
		bar += ">"
	}

	speed := fmt.Sprintf("%.2fs/it", secondsPerStep)
	if secondsPerStep > 0 && secondsPerStep < 1 {
		speed = fmt.Sprintf("%.2fit/s", 1/secondsPerStep)
	}

	fmt.Printf("\\r  |%-50s| %d/%d - %s\\x1b[K", bar, current, steps, speed)
	if current == steps {
		fmt.Println()
	}
}
`;

const malinaFlux2Example = `// This example shows you how to generate an image with the Malina SDK and a
// multi-file FLUX.2 model.
//
// Experimental: The Malina SDK public API is subject to change.
//
// The first time you run this program the system will download and install the
// stable-diffusion.cpp libraries and the FLUX.2 Klein 9B model bundle.
//
// Run the example like this from the root of the project:
// $ make example-malina-flux2

package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ardanlabs/kronk/sdk/malina"
	"github.com/ardanlabs/kronk/sdk/malina/model"
	"github.com/ardanlabs/kronk/sdk/tools/malina/libs"
	"github.com/ardanlabs/kronk/sdk/tools/malina/models"
)

var (
	modelSource = models.BundleFlux2Klein9B
	progressMu  sync.Mutex
)

const outputFile = "malina-flux2.png"

func main() {
	if err := run(); err != nil {
		fmt.Printf("\\nERROR: %s\\n", err)
		os.Exit(1)
	}
}

func run() error {
	manifest, err := installSystem()
	if err != nil {
		return fmt.Errorf("unable to install system: %w", err)
	}

	mln, err := newMalina(manifest)
	if err != nil {
		return fmt.Errorf("unable to init Malina: %w", err)
	}
	defer func() {
		fmt.Println("\\nUnloading Malina")
		if err := mln.Unload(context.Background()); err != nil {
			fmt.Printf("unload: %v\\n", err)
		}
	}()

	if err := generate(mln); err != nil {
		return fmt.Errorf("generate: %w", err)
	}

	return nil
}

// =============================================================================

func installSystem() (models.Manifest, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()

	libs, err := libs.New(
		libs.WithDetect(ctx, malina.FmtLogger),
		libs.WithValidation(true),
	)
	if err != nil {
		return models.Manifest{}, err
	}

	if _, err := libs.Download(ctx, malina.FmtLogger); err != nil {
		return models.Manifest{}, fmt.Errorf("unable to install stable-diffusion.cpp: %w", err)
	}

	if err := malina.Init(
		malina.WithLibPath(libs.LibsPath()),
		malina.WithProgress(progress),
	); err != nil {
		return models.Manifest{}, fmt.Errorf("unable to init Malina: %w", err)
	}

	// -------------------------------------------------------------------------

	mdls, err := models.New()
	if err != nil {
		return models.Manifest{}, fmt.Errorf("unable to init models: %w", err)
	}

	fmt.Println("Downloading model bundle:", modelSource)

	manifest, err := mdls.DownloadBundle(ctx, modelSource)
	if err != nil {
		return models.Manifest{}, fmt.Errorf("unable to install model bundle: %w", err)
	}

	return manifest, nil
}

func newMalina(manifest models.Manifest) (*malina.Malina, error) {
	fmt.Println("Loading model...")

	mln, err := malina.New(
		model.WithDiffusionModelPath(manifest.Files[string(models.RoleDiffusion)]),
		model.WithVAEPath(manifest.Files[string(models.RoleVAE)]),
		model.WithLLMPath(manifest.Files[string(models.RoleLLM)]),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to create image generation model: %w", err)
	}

	si := mln.SystemInfo()
	cfg := mln.ModelConfig()
	mi := mln.ModelInfo()

	fmt.Println("- native version    :", si.NativeVersion)
	fmt.Println("- physical cores    :", si.PhysicalCores)
	fmt.Println("- backend devices   :", si.BackendDeviceCount)
	fmt.Println("- model             :", mi.DiffusionModelPath)
	fmt.Println("- cpu threads       :", cfg.CPUThreads)
	fmt.Println("- active generations:", mln.ActiveGenerations())

	return mln, nil
}

// =============================================================================

func generate(mln *malina.Malina) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	params := model.NewGenerateParams()
	params.Prompt = "an orange cat on a tropical beach playing with oranges"
	params.NegativePrompt = "mascots, watermark, signature"
	params.Steps = 4

	fmt.Println("\\nGenerating FLUX.2 image...")
	start := time.Now()

	image, err := mln.Generate(ctx, params)
	if err != nil {
		return err
	}
	if err := os.WriteFile(outputFile, image.PNG, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", outputFile, err)
	}

	fmt.Printf("Wrote %s (%dx%d) in %s\\n", outputFile, image.Width, image.Height, time.Since(start).Round(time.Millisecond))

	return nil
}

// progress renders model loading and image generation progress reported by
// stable-diffusion.cpp.
func progress(step int, steps int, secondsPerStep float32) {
	if step <= 0 || steps <= 0 {
		return
	}

	progressMu.Lock()
	defer progressMu.Unlock()

	const width = 50
	current := min(step, steps)
	filled := min(current*width/steps, width)
	bar := strings.Repeat("=", filled)
	if filled < width {
		bar += ">"
	}

	speed := fmt.Sprintf("%.2fs/it", secondsPerStep)
	if secondsPerStep > 0 && secondsPerStep < 1 {
		speed = fmt.Sprintf("%.2fit/s", 1/secondsPerStep)
	}

	fmt.Printf("\\r  |%-50s| %d/%d - %s\\x1b[K", bar, current, steps, speed)
	if current == steps {
		fmt.Println()
	}
}
`;

const malinaImg2imgExample = `// This example shows you how to transform an existing image with the Malina
// SDK.
//
// Experimental: The Malina SDK public API is subject to change.
//
// The first time you run this program the system will download and install the
// stable-diffusion.cpp libraries and a Stable Diffusion 1.5 model bundle.
//
// Run the example like this from the root of the project:
// $ make example-malina-img2img

package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ardanlabs/kronk/sdk/malina"
	"github.com/ardanlabs/kronk/sdk/malina/model"
	"github.com/ardanlabs/kronk/sdk/tools/malina/libs"
	"github.com/ardanlabs/kronk/sdk/tools/malina/models"
)

var (
	modelSource = models.BundleSD15.String()
	progressMu  sync.Mutex
)

type config struct {
	input    string
	output   string
	prompt   string
	strength float64
	steps    int
	seed     int64
}

func main() {
	var cfg config
	flag.StringVar(&cfg.input, "in", "samples/giraffe.jpg", "source PNG or JPEG path")
	flag.StringVar(&cfg.output, "out", "malina-img2img.png", "output PNG path")
	flag.StringVar(&cfg.prompt, "prompt", "a watercolor painting at sunset", "prompt that steers the image")
	flag.Float64Var(&cfg.strength, "strength", 0.6, "noise strength in (0,1]")
	flag.IntVar(&cfg.steps, "steps", 20, "denoising steps")
	flag.Int64Var(&cfg.seed, "seed", 42, "RNG seed (-1 selects a random seed)")
	flag.Parse()

	if err := run(cfg); err != nil {
		fmt.Printf("\\nERROR: %s\\n", err)
		os.Exit(1)
	}
}

func run(cfg config) error {
	mp, err := installSystem()
	if err != nil {
		return fmt.Errorf("unable to install system: %w", err)
	}

	mln, err := newMalina(mp)
	if err != nil {
		return fmt.Errorf("unable to init Malina: %w", err)
	}
	defer func() {
		fmt.Println("\\nUnloading Malina")
		if err := mln.Unload(context.Background()); err != nil {
			fmt.Printf("unload: %v\\n", err)
		}
	}()

	if err := transform(mln, cfg); err != nil {
		return fmt.Errorf("transform: %w", err)
	}

	return nil
}

// =============================================================================

func installSystem() (models.Path, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	libs, err := libs.New(
		libs.WithDetect(ctx, malina.FmtLogger),
		libs.WithValidation(true),
	)
	if err != nil {
		return models.Path{}, err
	}

	if _, err := libs.Download(ctx, malina.FmtLogger); err != nil {
		return models.Path{}, fmt.Errorf("unable to install stable-diffusion.cpp: %w", err)
	}

	if err := malina.Init(
		malina.WithLibPath(libs.LibsPath()),
		malina.WithProgress(progress),
	); err != nil {
		return models.Path{}, fmt.Errorf("unable to init Malina: %w", err)
	}

	// -------------------------------------------------------------------------

	mdls, err := models.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to init models: %w", err)
	}

	fmt.Println("Downloading model bundle:", modelSource)

	mp, err := mdls.Download(ctx, malina.FmtLogger, modelSource)
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install model bundle: %w", err)
	}

	return mp, nil
}

func newMalina(mp models.Path) (*malina.Malina, error) {
	fmt.Println("Loading model...")

	if len(mp.ModelFiles) == 0 {
		return nil, fmt.Errorf("no model files on disk")
	}

	mln, err := malina.New(
		model.WithModelPath(mp.ModelFiles[0]),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to create image generation model: %w", err)
	}

	si := mln.SystemInfo()
	cfg := mln.ModelConfig()
	mi := mln.ModelInfo()

	fmt.Println("- native version    :", si.NativeVersion)
	fmt.Println("- physical cores    :", si.PhysicalCores)
	fmt.Println("- backend devices   :", si.BackendDeviceCount)
	fmt.Println("- model             :", mi.ModelPath)
	fmt.Println("- cpu threads       :", cfg.CPUThreads)
	fmt.Println("- concurrency       :", cfg.Concurrency)
	fmt.Println("- queue depth       :", cfg.QueueDepth)
	fmt.Println("- active generations:", mln.ActiveGenerations())

	return mln, nil
}

// =============================================================================

func transform(mln *malina.Malina, cfg config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	source, err := loadImage(cfg.input)
	if err != nil {
		return err
	}

	params := model.NewGenerateParams()
	params.Prompt = cfg.prompt
	params.InitImage = source
	params.Strength = float32(cfg.strength)
	params.Steps = cfg.steps
	params.Seed = cfg.seed
	params.Width, params.Height, err = generationSize(source.Bounds())
	if err != nil {
		return err
	}

	fmt.Printf("\\nTransforming %s with strength %.2f...\\n", cfg.input, cfg.strength)
	start := time.Now()

	generated, err := mln.Generate(ctx, params)
	if err != nil {
		return err
	}
	if err := os.WriteFile(cfg.output, generated.PNG, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", cfg.output, err)
	}

	fmt.Printf("Wrote %s (%dx%d) in %s\\n", cfg.output, generated.Width, generated.Height, time.Since(start).Round(time.Millisecond))

	return nil
}

// progress renders model loading and image generation progress reported by
// stable-diffusion.cpp.
func progress(step int, steps int, secondsPerStep float32) {
	if step <= 0 || steps <= 0 {
		return
	}

	progressMu.Lock()
	defer progressMu.Unlock()

	const width = 50
	current := min(step, steps)
	filled := min(current*width/steps, width)
	bar := strings.Repeat("=", filled)
	if filled < width {
		bar += ">"
	}

	speed := fmt.Sprintf("%.2fs/it", secondsPerStep)
	if secondsPerStep > 0 && secondsPerStep < 1 {
		speed = fmt.Sprintf("%.2fit/s", 1/secondsPerStep)
	}

	fmt.Printf("\\r  |%-50s| %d/%d - %s\\x1b[K", bar, current, steps, speed)
	if current == steps {
		fmt.Println()
	}
}

func loadImage(filename string) (image.Image, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read source image: %w", err)
	}

	image, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode source image: %w", err)
	}

	return image, nil
}

func generationSize(bounds image.Rectangle) (int, int, error) {
	const (
		alignment    = 8
		minDimension = 64
		maxDimension = 1024
	)

	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return 0, 0, errors.New("source image dimensions must be positive")
	}

	scale := min(1, min(float64(maxDimension)/float64(width), float64(maxDimension)/float64(height)))
	width = int(float64(width)*scale) / alignment * alignment
	height = int(float64(height)*scale) / alignment * alignment
	if width < minDimension || height < minDimension {
		return 0, 0, fmt.Errorf("source aspect ratio produces dimensions below %d pixels", minDimension)
	}

	return width, height, nil
}
`;

const malinaSdEncodeExample = `// This example shows you how to encode PNG and JPEG frames into a Motion-JPEG
// AVI with the Malina SDK.
//
// Experimental: The Malina SDK public API is subject to change.
//
// No model or native library is required.
//
// Run the example like this from the root of the project:
// $ make example-malina-sd-encode

package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ardanlabs/kronk/sdk/malina/model"
	"golang.org/x/image/draw"
)

type config struct {
	inputDir string
	output   string
	fps      int
	quality  int
}

func main() {
	var cfg config
	flag.StringVar(&cfg.inputDir, "i", "samples/deer", "directory containing PNG and JPEG frames")
	flag.StringVar(&cfg.output, "o", "malina-output.avi", "output AVI path")
	flag.IntVar(&cfg.fps, "fps", 24, "frames per second")
	flag.IntVar(&cfg.quality, "quality", 90, "JPEG quality from 1 to 100")
	flag.Parse()

	if err := run(cfg); err != nil {
		fmt.Printf("\\nERROR: %s\\n", err)
		os.Exit(1)
	}
}

func run(cfg config) error {
	paths, err := imagePaths(cfg.inputDir)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return fmt.Errorf("no PNG or JPEG files found in %s", cfg.inputDir)
	}

	frames := make([]image.Image, 0, len(paths))
	var target image.Rectangle
	for _, path := range paths {
		frame, err := loadImage(path)
		if err != nil {
			return err
		}
		if target.Empty() {
			target = image.Rect(0, 0, frame.Bounds().Dx(), frame.Bounds().Dy())
		}
		frames = append(frames, resize(frame, target))
	}

	if err := model.SaveAVI(cfg.output, frames, cfg.fps, cfg.quality); err != nil {
		return fmt.Errorf("save AVI: %w", err)
	}

	fmt.Printf("Wrote %s (%d frames, %dx%d at %d fps)\\n", cfg.output, len(frames), target.Dx(), target.Dy(), cfg.fps)

	return nil
}

func imagePaths(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read frames directory: %w", err)
	}

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".jpg", ".jpeg", ".png":
			paths = append(paths, filepath.Join(dir, entry.Name()))
		}
	}
	slices.Sort(paths)

	return paths, nil
}

func loadImage(filename string) (image.Image, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filename, err)
	}

	frame, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", filename, err)
	}

	return frame, nil
}

func resize(source image.Image, target image.Rectangle) image.Image {
	if source.Bounds().Dx() == target.Dx() && source.Bounds().Dy() == target.Dy() {
		return source
	}

	frame := image.NewRGBA(target)
	draw.BiLinear.Scale(frame, target, source, source.Bounds(), draw.Src, nil)

	return frame
}
`;

const malinaSystemExample = `// This example prints Malina and stable-diffusion.cpp system information.
// Compatible libraries are downloaded automatically.
//
// Experimental: The Malina SDK public API is subject to change.
//
// The first time you run this program the system will download and install the
// stable-diffusion.cpp libraries.
//
// Run the example like this from the root of the project:
// $ make example-malina-system

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/malina"
	"github.com/ardanlabs/kronk/sdk/tools/malina/libs"
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("\\nERROR: %s\\n", err)
		os.Exit(1)
	}
}

func run() error {
	if err := installSystem(); err != nil {
		return fmt.Errorf("unable to install system: %w", err)
	}

	info, err := malina.SystemInfo()
	if err != nil {
		return fmt.Errorf("system info: %w", err)
	}

	fmt.Println("-- stable-diffusion.cpp --")
	fmt.Println("version:              ", info.NativeVersion)
	fmt.Println("physical cores:       ", info.PhysicalCores)
	fmt.Println("GGML backend devices: ", info.BackendDeviceCount)
	fmt.Println()
	fmt.Println("-- System info --")
	fmt.Println(info.Description)

	return nil
}

// =============================================================================

func installSystem() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	libs, err := libs.New(
		libs.WithDetect(ctx, malina.FmtLogger),
		libs.WithValidation(true),
	)
	if err != nil {
		return err
	}

	if _, err := libs.Download(ctx, malina.FmtLogger); err != nil {
		return fmt.Errorf("unable to install stable-diffusion.cpp: %w", err)
	}

	if err := malina.Init(malina.WithLibPath(libs.LibsPath())); err != nil {
		return fmt.Errorf("unable to init Malina: %w", err)
	}

	return nil
}
`;

const poolExample = `// This example shows you how to use the pool package to manage multiple
// models in memory at the same time. The pool will load models on demand,
// keep them resident up to a configured cap, and unload them after a TTL
// of inactivity.
//
// The first time you run this program the system will download and install
// the models and libraries.
//
// Run the example like this from the root of the project:
// $ make example-pool

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/pool"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

const (
	questionModel = "unsloth/Qwen3-0.6B-Q8_0"
	visionModel   = "unsloth/Qwen3.5-0.8B-Q8_0"
	imageFile     = "samples/giraffe.jpg"
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("\\nERROR: %s\\n", err)
		os.Exit(1)
	}
}

func run() error {
	mdls, err := installSystem()
	if err != nil {
		return fmt.Errorf("unable to install system: %w", err)
	}

	// -------------------------------------------------------------------------

	const cacheTTL = 15 * time.Second

	cfg := pool.Config{
		Log:           kronk.FmtLogger,
		KronkModels:   mdls,
		BudgetPercent: 95,
		TTL:           cacheTTL,
	}

	p, err := pool.New(cfg)
	if err != nil {
		return fmt.Errorf("unable to create pool: %w", err)
	}

	defer func() {
		fmt.Println("\\nShutting down pool")
		if err := p.Shutdown(context.Background()); err != nil {
			fmt.Printf("failed to shutdown pool: %v\\n", err)
		}
	}()

	// -------------------------------------------------------------------------

	if err := acquireAndAsk(p); err != nil {
		return fmt.Errorf("acquire and ask: %w", err)
	}

	printStatus(p, "after question model")

	if err := acquireAndSee(p); err != nil {
		return fmt.Errorf("acquire and see: %w", err)
	}

	printStatus(p, "after vision model")

	// -------------------------------------------------------------------------

	wait := cacheTTL + 5*time.Second
	fmt.Printf("\\nWaiting %s for TTL to expire...\\n", wait)
	time.Sleep(wait)

	printStatus(p, "after TTL expiry")

	return nil
}

func installSystem() (*models.Models, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	libs, err := libs.New(
		libs.WithDetect(ctx, kronk.FmtLogger),
	)
	if err != nil {
		return nil, err
	}

	if _, err := libs.Download(ctx, kronk.FmtLogger); err != nil {
		return nil, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	if err := kronk.Init(kronk.WithLibPath(libs.LibsPath())); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	// -------------------------------------------------------------------------

	mdls, err := models.New()
	if err != nil {
		return nil, fmt.Errorf("unable to create models system: %w", err)
	}

	for _, src := range []string{questionModel, visionModel} {
		fmt.Println("Downloading model:", src)
		if _, err := mdls.Download(ctx, kronk.FmtLogger, src); err != nil {
			return nil, fmt.Errorf("unable to install model %q: %w", src, err)
		}
	}

	if err := mdls.BuildIndex(kronk.FmtLogger, false); err != nil {
		return nil, fmt.Errorf("unable to build model index: %w", err)
	}

	return mdls, nil
}

func acquireAndAsk(p *pool.Pool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	fmt.Println("\\nAcquiring question model:", questionModel)

	krn, err := p.Kronk.AquireModel(ctx, questionModel)
	if err != nil {
		return fmt.Errorf("acquire model: %w", err)
	}

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

	return streamResponse(ch)
}

func acquireAndSee(p *pool.Pool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	fmt.Println("\\nAcquiring vision model:", visionModel)

	krn, err := p.Kronk.AquireModel(ctx, visionModel)
	if err != nil {
		return fmt.Errorf("acquire model: %w", err)
	}

	image, err := readImage(imageFile)
	if err != nil {
		return fmt.Errorf("read image: %w", err)
	}

	question := "What is in this picture?"

	fmt.Printf("\\nQuestion: %s\\n", question)

	d := model.D{
		"messages":    model.ImageMessage(question, image, "jpeg"),
		"temperature": 0.7,
		"top_p":       0.9,
		"top_k":       40,
		"max_tokens":  2048,
	}

	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		return fmt.Errorf("vision streaming: %w", err)
	}

	return streamResponse(ch)
}

func streamResponse(ch <-chan model.ChatResponse) error {
	fmt.Print("\\nMODEL> ")

	var reasoning bool

	for resp := range ch {
		switch resp.Choices[0].FinishReason() {
		case model.FinishReasonError:
			return fmt.Errorf("error from model: %s", resp.Choices[0].Delta.Content)

		case model.FinishReasonStop:
			fmt.Println()
			return nil

		default:
			if resp.Choices[0].Delta.Reasoning != "" {
				reasoning = true
				fmt.Printf("\\u001b[91m%s\\u001b[0m", resp.Choices[0].Delta.Reasoning)
				continue
			}

			if reasoning {
				reasoning = false
				fmt.Println()
				continue
			}

			fmt.Printf("%s", resp.Choices[0].Delta.Content)
		}
	}

	return nil
}

func printStatus(p *pool.Pool, label string) {
	details, err := p.Kronk.ModelStatus()
	if err != nil {
		fmt.Printf("\\nModelStatus error: %v\\n", err)
		return
	}

	fmt.Printf("\\n--- pool status (%s) ---\\n", label)
	fmt.Printf("models in cache: %d\\n", len(details))
	for _, d := range details {
		fmt.Printf("  - id=%s family=%s vram=%dMiB slots=%d active=%d expires=%s\\n",
			d.ID,
			d.ModelFamily,
			d.VRAMTotal/(1024*1024),
			d.Slots,
			d.ActiveStreams,
			d.ExpiresAt.Format(time.RFC3339),
		)
	}
	fmt.Println("------------------------")
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
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

const modelSource = "unsloth/Qwen3-0.6B-Q8_0"

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

	if err := question(krn); err != nil {
		fmt.Println(err)
	}

	return nil
}

func installSystem() (models.Path, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
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

	// -------------------------------------------------------------------------

	mdls, err := models.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to init models: %w", err)
	}

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

	fmt.Print("- system info:\\n\\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	fmt.Println("- contextWindow  :", krn.ModelConfig().ContextWindow())
	fmt.Printf("- k/v            : %s/%s\\n", krn.ModelConfig().CacheTypeK, krn.ModelConfig().CacheTypeV)
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
		switch resp.Choices[0].FinishReason() {
		case model.FinishReasonError:
			return fmt.Errorf("error from model: %s", resp.Choices[0].Delta.Content)

		case model.FinishReasonStop:
			return nil

		default:
			if resp.Choices[0].Delta.Reasoning != "" {
				reasoning = true
				fmt.Printf("\\u001b[91m%s\\u001b[0m", resp.Choices[0].Delta.Reasoning)
				continue
			}

			if reasoning {
				reasoning = false
				fmt.Println()
				continue
			}

			fmt.Printf("%s", resp.Choices[0].Delta.Content)
		}
	}

	return nil
}
`;

const ragExample = `// This example shows you a complete RAG application using DuckDB as an embedding
// DB and an embedding model to generate embeddings, and a chat model for
// answering a question using the Kronk SDK.
//
// # Running the example:
//
//	$ make example-rag

package main

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/examples/rag/duck"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

const (
	modelChatSource  = "ornith-ai/Ornith-1.5-9B-Q4_K_M"
	modelEmbedSource = "Qwen/Qwen3-Embedding-0.6B-Q8_0.gguf"
	dbPath           = "rag/docs/duck-rag.db" // ":memory:"
	chunksFile       = "rag/docs/book.chunks"
	dimensions       = 1024
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("\\nERROR: %s\\n", err)
		os.Exit(1)
	}
}

func run() error {
	infoEmbed, infoChat, err := installSystem()
	if err != nil {
		return fmt.Errorf("unable to install system: %w", err)
	}

	krnEmbed, err := newKronk(infoEmbed)
	if err != nil {
		return fmt.Errorf("unable to create embedding model: %w", err)
	}
	defer func() {
		fmt.Println("\\nUnloading embedding model")
		if err := krnEmbed.Unload(context.Background()); err != nil {
			fmt.Printf("failed to unload embedding model: %v", err)
		}
	}()

	krnChat, err := newKronk(infoChat)
	if err != nil {
		return fmt.Errorf("unable to create chat model: %w", err)
	}
	defer func() {
		fmt.Println("\\nUnloading chat model")
		if err := krnChat.Unload(context.Background()); err != nil {
			fmt.Printf("failed to unload chat model: %v", err)
		}
	}()

	// -------------------------------------------------------------------------

	db, err := duck.LoadData(dbPath, krnEmbed, dimensions, chunksFile)
	if err != nil {
		return fmt.Errorf("error connecting to database: %w", err)
	}
	defer db.Close()

	// -------------------------------------------------------------------------

	var messages []model.D

	for {
		messages, err = userInput(messages)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("unable to get user input: %w", err)
		}

		// ---------------------------------------------------------------------

		docs, err := func() ([]duck.Document, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			docs, err := vectorSearch(ctx, krnEmbed, db, messages)
			if err != nil {
				return nil, fmt.Errorf("unable to get vector search results: %w", err)
			}

			return docs, nil
		}()

		if err != nil {
			return fmt.Errorf("unable to get vector search results: %w", err)
		}

		// ---------------------------------------------------------------------

		messages, err = func() ([]model.D, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()

			d := model.D{
				"messages":    addContextPrompt(docs, messages),
				"max_tokens":  2048,
				"temperature": 0.7,
				"top_p":       0.9,
				"top_k":       40,
				"stream_options": model.D{
					"include_usage": true,
				},
			}

			ch, err := performChat(ctx, krnChat, d)
			if err != nil {
				return nil, fmt.Errorf("unable to perform chat: %w", err)
			}

			messages, err = modelResponse(krnChat, messages, ch)
			if err != nil {
				return nil, fmt.Errorf("unable to get model response: %w", err)
			}

			return messages, nil
		}()

		if err != nil {
			return fmt.Errorf("unable to perform chat: %w", err)
		}
	}
}

func installSystem() (models.Path, models.Path, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	libs, err := libs.New(
		libs.WithDetect(ctx, kronk.FmtLogger),
	)
	if err != nil {
		return models.Path{}, models.Path{}, err
	}

	if _, err := libs.Download(ctx, kronk.FmtLogger); err != nil {
		return models.Path{}, models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	if err := kronk.Init(kronk.WithLibPath(libs.LibsPath())); err != nil {
		return models.Path{}, models.Path{}, fmt.Errorf("unable to init kronk: %w", err)
	}

	mdls, err := models.New()
	if err != nil {
		return models.Path{}, models.Path{}, fmt.Errorf("unable to create models api: %w", err)
	}

	infoEmbed, err := mdls.Download(context.Background(), kronk.FmtLogger, modelEmbedSource)
	if err != nil {
		return models.Path{}, models.Path{}, fmt.Errorf("unable to install model: %w", err)
	}

	infoChat, err := mdls.Download(context.Background(), kronk.FmtLogger, modelChatSource)
	if err != nil {
		return models.Path{}, models.Path{}, fmt.Errorf("unable to install model: %w", err)
	}

	return infoEmbed, infoChat, nil
}

func newKronk(mp models.Path) (*kronk.Kronk, error) {
	krn, err := kronk.New(
		model.WithModelFiles(mp.ModelFiles),
		model.WithAutoTune(true),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to create inference model: %w", err)
	}

	fmt.Print("- system info:\\n\\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	fmt.Println("- contextWindow:", krn.ModelConfig().ContextWindow())
	fmt.Println("- embeddings   :", krn.ModelInfo().IsEmbedModel)
	fmt.Println("- template     :", krn.ModelInfo().Template.FileName)

	return krn, nil
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

	messages = append(messages, model.TextMessage("user", userInput))

	return messages, nil
}

func vectorSearch(ctx context.Context, krnEmbed *kronk.Kronk, db *sql.DB, messages []model.D) ([]duck.Document, error) {
	fmt.Print("\\n--- Vector Search ---\\n\\n")

	d := model.D{
		"input": messages[len(messages)-1]["content"].(string),
	}

	resp, err := krnEmbed.Embeddings(ctx, d)
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}

	if len(resp.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("empty query vector")
	}

	docs, err := duck.Search(db, resp.Data[0].Embedding, 5)
	if err != nil {
		return nil, fmt.Errorf("error searching database: %w", err)
	}

	for _, doc := range docs {
		fmt.Printf("Doc: %f: %s\\n", doc.Similarity, strings.ReplaceAll(doc.Text, "\\n", " ")[:100])
	}

	return docs, nil
}

func addContextPrompt(documents []duck.Document, messages []model.D) []model.D {
	const prompt = \`
		- Use the following Context to answer the user's question.
		- If you don't know the answer, say that you don't know.
		- Responses should be properly formatted to be easily read.
		- Share code if code is presented in the context.
		- Do not include any additional information not present in the context.

		Context:
		
		%s

		Question: %s
		\`

	var count int
	var content strings.Builder
	for _, doc := range documents {
		content.WriteString(fmt.Sprintf("%s\\n%s\\n", doc.Text, doc.Text))
		count++
		if count == 2 {
			break
		}
	}

	lastUserInput := messages[len(messages)-1]["content"].(string)
	finalPrompt := fmt.Sprintf(prompt, content.String(), lastUserInput)

	messages = append(messages, model.TextMessage("user", finalPrompt))

	return messages
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

	for resp := range ch {
		lr = resp
		if len(resp.Choices) == 0 {
			continue
		}

		switch resp.Choices[0].FinishReason() {
		case model.FinishReasonError:
			return messages, fmt.Errorf("error from model: %s", resp.Choices[0].Delta.Content)

		case model.FinishReasonStop, model.FinishReasonLength:
			continue

		case model.FinishReasonTool:
			fmt.Println()
			toolCall := resp.Choices[0].Message.ToolCalls[0]

			fmt.Printf("\\u001b[92mModel Asking For Tool Call:\\nToolID[%s]: %s(%s)\\u001b[0m\\n",
				toolCall.ID,
				toolCall.Function.Name,
				toolCall.Function.Arguments,
			)

			messages = append(messages,
				model.TextMessage("tool", fmt.Sprintf("Tool call %s: %s(%v)",
					toolCall.ID,
					toolCall.Function.Name,
					toolCall.Function.Arguments),
				),
			)
			continue

		default:
			if resp.Choices[0].Delta.Reasoning != "" {
				fmt.Printf("\\u001b[91m%s\\u001b[0m", resp.Choices[0].Delta.Reasoning)
				reasoning = true
				continue
			}

			if reasoning {
				reasoning = false

				fmt.Println()
			}

			fmt.Printf("%s", resp.Choices[0].Delta.Content)
		}
	}

	// -------------------------------------------------------------------------
	if lr.Usage == nil {
		return messages, fmt.Errorf("stream ended without usage")
	}

	contextTokens := lr.Usage.PromptTokens + lr.Usage.CompletionTokens
	contextWindow := krn.ModelConfig().ContextWindow()
	percentage := (float64(contextTokens) / float64(contextWindow)) * 100
	of := float32(contextWindow) / float32(1024)

	fmt.Printf("\\n\\n\\u001b[90mPrompt: %d  Reasoning: %d  Completion: %d  Total: %d  Window: %d (%.0f%% of %.0fK) TPS: %.2f\\u001b[0m\\n",
		lr.Usage.PromptTokens, lr.Usage.CompletionTokensDetails.ReasoningTokens, lr.Usage.CompletionTokens, lr.Usage.TotalTokens, contextTokens, percentage, of, lr.Usage.TokensPerSecond)

	return messages, nil
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
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// modelSource is the model to download. It may be a HuggingFace URL,
// a canonical "provider/modelID", or a bare model id.
var modelSource = "gpustack/bge-reranker-v2-m3-Q8_0"

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

	if err := rerank(krn); err != nil {
		return err
	}

	return nil
}

func installSystem() (models.Path, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
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
		return nil, fmt.Errorf("unable to create reranker model: %w", err)
	}

	fmt.Print("- system info:\\n\\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	fmt.Println("- contextWindow  :", krn.ModelConfig().ContextWindow())
	fmt.Printf("- k/v            : %s/%s\\n", krn.ModelConfig().CacheTypeK, krn.ModelConfig().CacheTypeV)
	fmt.Println("- flashAttention :", krn.ModelConfig().FlashAttention())
	fmt.Println("- prefill batch  :", krn.ModelConfig().PrefillBatchSize())
	fmt.Println("- embeddings     :", krn.ModelInfo().IsEmbedModel)
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
	fmt.Println("Question: What is the capital of France?")
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
	"encoding/json"
	"errors"
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
		fmt.Printf("\\nERROR: %s\\n", err)
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
		fmt.Println("\\nUnloading Kronk")
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

	fmt.Print("- system info:\\n\\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	fmt.Println("- contextWindow  :", krn.ModelConfig().ContextWindow())
	fmt.Printf("- k/v            : %s/%s\\n", krn.ModelConfig().CacheTypeK, krn.ModelConfig().CacheTypeV)
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

const systemPrompt = \`You are designed to help users answer questions, create
content, and provide information in a helpful and accurate manner. Always follow
the user's instructions carefully and respond with clear, concise, and
well-structured answers.\`

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
	fmt.Print("\\nUSER> ")

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

func streamModelTurn(ctx context.Context, krn *kronk.Kronk, conversation []model.D) (string, []model.ResponseToolCall, *kronk.ResponseUsage, error) {
	d := model.D{
		"input":             conversation,
		"tools":             toolDocuments(),
		"max_output_tokens": 2048,
		"stream_options":    model.D{"include_usage": true},
	}

	fmt.Print("\\nMODEL> ")

	callCtx, cancelCall := context.WithTimeout(ctx, 2*time.Minute)
	defer cancelCall()

	ch, err := krn.ResponseStreaming(callCtx, d)
	if err != nil {
		return "", nil, nil, fmt.Errorf("response streaming: %w", err)
	}

	var content strings.Builder
	var toolCalls []model.ResponseToolCall
	var usage *kronk.ResponseUsage
	reasoning := false

	for event := range ch {
		switch event.Type {
		case "response.reasoning_summary_text.delta":
			reasoning = true
			fmt.Printf("\\u001b[91m%s\\u001b[0m", event.Delta)

		case "response.output_text.delta":
			if reasoning {
				reasoning = false
				fmt.Print("\\n\\n")
			}

			fmt.Print(event.Delta)
			content.WriteString(event.Delta)

		case "response.output_item.added":
			if event.Item != nil && event.Item.Type == "function_call" {
				fmt.Printf("\\n\\n\\u001b[92mExecuting %s...\\u001b[0m", event.Item.Name)
			}

		case "response.function_call_arguments.done":
			var arguments model.ToolCallArguments
			if err := json.Unmarshal([]byte(event.Arguments), &arguments); err != nil {
				return "", nil, usage, fmt.Errorf("decode tool arguments: %w", err)
			}

			toolCalls = append(toolCalls, model.ResponseToolCall{
				ID:    event.ItemID,
				Index: len(toolCalls),
				Type:  "function",
				Function: model.ResponseToolCallFunction{
					Name:      event.Name,
					Arguments: arguments,
				},
			})

		case "response.completed", "response.incomplete":
			if event.Response != nil {
				usage = &event.Response.Usage
			}

		case "response.failed":
			if event.Response != nil && event.Response.Error != nil {
				return "", nil, usage, fmt.Errorf("error from model: %s", event.Response.Error.Message)
			}
			return "", nil, usage, errors.New("error from model: response failed")
		}
	}

	return strings.TrimLeft(content.String(), "\\n"), toolCalls, usage, nil
}

func appendToolCalls(conversation []model.D, toolCalls []model.ResponseToolCall) []model.D {
	fmt.Print("\\n\\n")

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

	fmt.Print("\\n")
	return append(conversation, model.D{"role": "assistant", "content": content})
}

func printUsage(krn *kronk.Kronk, usage *kronk.ResponseUsage) {
	if usage == nil {
		return
	}

	reasoningTokens := usage.OutputTokenDetail.ReasoningTokens
	completionTokens := usage.OutputTokens - reasoningTokens
	contextWindow := krn.ModelConfig().ContextWindow()
	percentage := (float64(usage.TotalTokens) / float64(contextWindow)) * 100
	of := float32(contextWindow) / float32(1024)

	fmt.Printf("\\n\\n\\u001b[90mInput: %d  Reasoning: %d  Completion: %d  Output: %d  Window: %d (%.0f%% of %.0fK)\\u001b[0m",
		usage.InputTokens, reasoningTokens, completionTokens, usage.OutputTokens, usage.TotalTokens, percentage, of)
}

func callTools(toolCalls []model.ResponseToolCall) []model.D {
	results := make([]model.D, 0, len(toolCalls))

	for _, toolCall := range toolCalls {
		fmt.Printf("\\u001b[92m%s(%v)\\u001b[0m: \\u001b[90mok\\u001b[0m\\n",
			toolCall.Function.Name, toolCall.Function.Arguments)

		// This example hard-codes the tool execution and its result. The agent
		// example replaces this with registered Tool implementations.
		results = append(results, model.D{
			"role":         "tool",
			"name":         toolCall.Function.Name,
			"tool_call_id": toolCall.ID,
			"content":      \`{"temperature": "72°F", "condition": "sunny"}\`,
		})
	}

	return results
}
`;

const sessionStoreExample = `// This example shows how SDK users can implement and inject a custom IMC
// session-store factory. The included disk store is intentionally temporary:
// it creates anonymous files and removes them on Close. It demonstrates the
// extension contract, not durable session persistence. Production stores need
// atomic replacement, integrity validation, resource limits, and cleanup that
// does not depend exclusively on Close.
//
// The first time you run this program the system will download and install
// the model and libraries.
//
// Run the example like this from the root of the project:
// $ make example-session-store

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

const modelSource = "unsloth/Qwen3-0.6B-Q8_0"

func main() {
	if err := run(); err != nil {
		fmt.Printf("\\nERROR: %s\\n", err)
		os.Exit(1)
	}
}

func run() error {
	mp, err := installSystem()
	if err != nil {
		return fmt.Errorf("install system: %w", err)
	}

	storeDir, err := os.MkdirTemp("", "kronk-session-store-")
	if err != nil {
		return fmt.Errorf("create session-store directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(storeDir); err != nil {
			fmt.Printf("remove session-store directory: %v\\n", err)
		}
	}()

	factory, err := NewFactory(context.Background(), storeDir)
	if err != nil {
		return fmt.Errorf("construct session-store factory: %w", err)
	}

	krn, err := kronk.New(
		model.WithModelFiles(mp.ModelFiles),
		model.WithAutoTune(true),
		model.WithIncrementalCache(true),
		model.WithSessionStoreFactory(factory),

		// Need this to trigger session usage for this
		// small example.
		model.WithCacheMinTokens(5),
	)
	if err != nil {
		return fmt.Errorf("create kronk: %w", err)
	}
	defer func() {
		if err := krn.Unload(context.Background()); err != nil {
			fmt.Printf("unload model: %v\\n", err)
		}
	}()

	fmt.Println("temporary session files:", storeDir)
	return askQuestion(krn)
}

func installSystem() (models.Path, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	libraryManager, err := libs.New(
		libs.WithDetect(ctx, kronk.FmtLogger),
	)
	if err != nil {
		return models.Path{}, err
	}

	if _, err := libraryManager.Download(ctx, kronk.FmtLogger); err != nil {
		return models.Path{}, fmt.Errorf("install llama.cpp: %w", err)
	}
	if err := kronk.Init(kronk.WithLibPath(libraryManager.LibsPath())); err != nil {
		return models.Path{}, fmt.Errorf("initialize kronk: %w", err)
	}

	modelManager, err := models.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("initialize models: %w", err)
	}

	mp, err := modelManager.Download(ctx, kronk.FmtLogger, modelSource)
	if err != nil {
		return models.Path{}, fmt.Errorf("install model: %w", err)
	}

	return mp, nil
}

func askQuestion(krn *kronk.Kronk) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	doc := model.D{
		"messages": model.DocumentArray(
			model.TextMessage(model.RoleUser, "Explain custom session storage in one sentence."),
		),
		"max_tokens": 128,
	}

	ch, err := krn.ChatStreaming(ctx, doc)
	if err != nil {
		return fmt.Errorf("chat streaming: %w", err)
	}

	for resp := range ch {
		if resp.Choices[0].FinishReason() == model.FinishReasonError {
			return fmt.Errorf("model: %s", resp.Choices[0].Delta.Content)
		}
		fmt.Print(resp.Choices[0].Delta.Content)
	}
	fmt.Println()

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
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// modelSource is the model to download. It may be a HuggingFace URL,
// a canonical "provider/modelID", or a bare model id.
//
// modelProjURL is the optional companion mmproj URL. It is honored only
// when modelSource is a direct URL; for an id, the resolver auto-discovers
// the mmproj.
var modelSource = "unsloth/Qwen3.5-0.8B-Q8_0"

const imageFile = "samples/giraffe.jpg"

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
		model.WithProjFile(mp.ProjFile),
		model.WithAutoTune(true),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to create inference model: %w", err)
	}

	fmt.Print("- system info:\\n\\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	fmt.Println("- contextWindow  :", krn.ModelConfig().ContextWindow())
	fmt.Printf("- k/v            : %s/%s\\n", krn.ModelConfig().CacheTypeK, krn.ModelConfig().CacheTypeV)
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
		"messages":    model.ImageMessage(question, image, "jpg"),
		"temperature": 0.7,
		"top_p":       0.9,
		"top_k":       40,
		"max_tokens":  2048,
		"stream_options": model.D{
			"include_usage": true,
		},
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

	for resp := range ch {
		lr = resp
		if len(resp.Choices) == 0 {
			continue
		}

		switch resp.Choices[0].FinishReason() {
		case model.FinishReasonStop, model.FinishReasonLength:
			continue

		case model.FinishReasonError:
			return fmt.Errorf("error from model: %s", resp.Choices[0].Delta.Content)
		}

		if resp.Choices[0].Delta.Reasoning != "" {
			fmt.Printf("\\u001b[91m%s\\u001b[0m", resp.Choices[0].Delta.Reasoning)
			reasoning = true
			continue
		}

		if reasoning {
			reasoning = false
			fmt.Print("\\n\\n")
		}

		fmt.Printf("%s", resp.Choices[0].Delta.Content)
	}

	// -------------------------------------------------------------------------
	if lr.Usage == nil {
		return fmt.Errorf("stream ended without usage")
	}

	contextTokens := lr.Usage.PromptTokens + lr.Usage.CompletionTokens
	contextWindow := krn.ModelConfig().ContextWindow()
	percentage := (float64(contextTokens) / float64(contextWindow)) * 100
	of := float32(contextWindow) / float32(1024)

	fmt.Printf("\\n\\n\\u001b[90mInput: %d  Reasoning: %d  Completion: %d  Total: %d  Window: %d (%.0f%% of %.0fK) TPS: %.2f\\u001b[0m\\n",
		lr.Usage.PromptTokens, lr.Usage.CompletionTokensDetails.ReasoningTokens, lr.Usage.CompletionTokens, lr.Usage.TotalTokens, contextTokens, percentage, of, lr.Usage.TokensPerSecond)

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
  const location = useLocation();

  useEffect(() => {
    const container = document.querySelector('.main-content');
    if (!container) return;
    if (!location.hash) {
      container.scrollTo({ top: 0 });
      return;
    }
    const id = location.hash.slice(1);
    requestAnimationFrame(() => {
      const element = document.getElementById(id);
      if (!element) return;
      const containerRect = container.getBoundingClientRect();
      const elementRect = element.getBoundingClientRect();
      const offset = elementRect.top - containerRect.top + container.scrollTop;
      container.scrollTo({ top: offset - 20, behavior: 'smooth' });
    });
  }, [location.key, location.hash]);

  return (
    <div>
      <div className="page-header">
        <h2>SDK Examples</h2>
        <p>Complete working examples demonstrating how to use the Kronk SDK</p>
      </div>

      <div className="doc-layout">
        <div className="doc-content">

          <div className="card" id="example-agent">
            <h3>Agent</h3>
            <p className="doc-description">This example shows you how to create a simple agent application against an</p>
            <CodeBlock code={agentExample} language="go" />
          </div>

          <div className="card" id="example-audio">
            <h3>Audio</h3>
            <p className="doc-description">This example shows you how to execute a simple prompt against an audio model.</p>
            <CodeBlock code={audioExample} language="go" />
          </div>

          <div className="card" id="example-bucky">
            <h3>Bucky</h3>
            <p className="doc-description">This example shows you how to transcribe an audio file with the</p>
            <CodeBlock code={buckyExample} language="go" />
          </div>

          <div className="card" id="example-bucky-diar">
            <h3>Bucky-Diar</h3>
            <p className="doc-description">This example shows you how to perform channel-separated speaker</p>
            <CodeBlock code={buckyDiarExample} language="go" />
          </div>

          <div className="card" id="example-bucky-stream">
            <h3>Bucky-Stream</h3>
            <p className="doc-description">This example is a LIVE MICROPHONE transcription demo for the bucky</p>
            <CodeBlock code={buckyStreamExample} language="go" />
          </div>

          <div className="card" id="example-chat">
            <h3>Chat</h3>
            <p className="doc-description">This example shows you how to create a simple chat application against an</p>
            <CodeBlock code={chatExample} language="go" />
          </div>

          <div className="card" id="example-concurrency">
            <h3>Concurrency</h3>
            <p className="doc-description">This example shows you how to leverage Kronk's batch processing by running</p>
            <CodeBlock code={concurrencyExample} language="go" />
          </div>

          <div className="card" id="example-embedding">
            <h3>Embedding</h3>
            <p className="doc-description">This example shows you how to use an embedding model.</p>
            <CodeBlock code={embeddingExample} language="go" />
          </div>

          <div className="card" id="example-grammar">
            <h3>Grammar</h3>
            <p className="doc-description">This example shows how to use GBNF grammars to constrain model output.</p>
            <CodeBlock code={grammarExample} language="go" />
          </div>

          <div className="card" id="example-malina">
            <h3>Malina</h3>
            <p className="doc-description">This example shows you how to generate an image with the Malina SDK</p>
            <CodeBlock code={malinaExample} language="go" />
          </div>

          <div className="card" id="example-malina-flux2">
            <h3>Malina-Flux2</h3>
            <p className="doc-description">This example shows you how to generate an image with the Malina SDK and a</p>
            <CodeBlock code={malinaFlux2Example} language="go" />
          </div>

          <div className="card" id="example-malina-img2img">
            <h3>Malina-Img2img</h3>
            <p className="doc-description">This example shows you how to transform an existing image with the Malina</p>
            <CodeBlock code={malinaImg2imgExample} language="go" />
          </div>

          <div className="card" id="example-malina-sd-encode">
            <h3>Malina-Sd-Encode</h3>
            <p className="doc-description">This example shows you how to encode PNG and JPEG frames into a Motion-JPEG</p>
            <CodeBlock code={malinaSdEncodeExample} language="go" />
          </div>

          <div className="card" id="example-malina-system">
            <h3>Malina-System</h3>
            <p className="doc-description">This example prints Malina and stable-diffusion.cpp system information.</p>
            <CodeBlock code={malinaSystemExample} language="go" />
          </div>

          <div className="card" id="example-pool">
            <h3>Pool</h3>
            <p className="doc-description">This example shows you how to use the pool package to manage multiple</p>
            <CodeBlock code={poolExample} language="go" />
          </div>

          <div className="card" id="example-question">
            <h3>Question</h3>
            <p className="doc-description">This example shows you a basic program of using Kronk to ask a model a question.</p>
            <CodeBlock code={questionExample} language="go" />
          </div>

          <div className="card" id="example-rag">
            <h3>Rag</h3>
            <p className="doc-description">This example shows you a complete RAG application using DuckDB as an embedding</p>
            <CodeBlock code={ragExample} language="go" />
          </div>

          <div className="card" id="example-rerank">
            <h3>Rerank</h3>
            <p className="doc-description">This example shows you how to use a reranker model.</p>
            <CodeBlock code={rerankExample} language="go" />
          </div>

          <div className="card" id="example-response">
            <h3>Response</h3>
            <p className="doc-description">This example shows you how to create a simple chat application against an</p>
            <CodeBlock code={responseExample} language="go" />
          </div>

          <div className="card" id="example-session-store">
            <h3>Session-Store</h3>
            <p className="doc-description">This example shows how SDK users can implement and inject a custom IMC</p>
            <CodeBlock code={sessionStoreExample} language="go" />
          </div>

          <div className="card" id="example-vision">
            <h3>Vision</h3>
            <p className="doc-description">This example shows you how to execute a simple prompt against a vision model.</p>
            <CodeBlock code={visionExample} language="go" />
          </div>
        </div>

        <nav className="doc-sidebar">
          <div className="doc-sidebar-content">
            <div className="doc-index-section">
              <span className="doc-index-header">Examples</span>
              <ul>
                <li><a href="#example-agent">Agent</a></li>
                <li><a href="#example-audio">Audio</a></li>
                <li><a href="#example-bucky">Bucky</a></li>
                <li><a href="#example-bucky-diar">Bucky-Diar</a></li>
                <li><a href="#example-bucky-stream">Bucky-Stream</a></li>
                <li><a href="#example-chat">Chat</a></li>
                <li><a href="#example-concurrency">Concurrency</a></li>
                <li><a href="#example-embedding">Embedding</a></li>
                <li><a href="#example-grammar">Grammar</a></li>
                <li><a href="#example-malina">Malina</a></li>
                <li><a href="#example-malina-flux2">Malina-Flux2</a></li>
                <li><a href="#example-malina-img2img">Malina-Img2img</a></li>
                <li><a href="#example-malina-sd-encode">Malina-Sd-Encode</a></li>
                <li><a href="#example-malina-system">Malina-System</a></li>
                <li><a href="#example-pool">Pool</a></li>
                <li><a href="#example-question">Question</a></li>
                <li><a href="#example-rag">Rag</a></li>
                <li><a href="#example-rerank">Rerank</a></li>
                <li><a href="#example-response">Response</a></li>
                <li><a href="#example-session-store">Session-Store</a></li>
                <li><a href="#example-vision">Vision</a></li>
              </ul>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
