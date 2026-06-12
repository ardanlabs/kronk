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
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// modelSource is the model to download. It may be a HuggingFace URL,
// a canonical "provider/modelID", or a bare model id.
var modelSource = "unsloth/gemma-4-E4B-it-qat-UD-Q4_K_XL"

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
	fmt.Print("\\u001b[94m\\nYou\\u001b[0m: ")

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
		"tools":          a.toolDocuments,
		"tool_selection": "auto",
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
			text := strings.TrimLeft(strings.Join(chunks, " "), "\\n")
			return text, nil, lastResp.Usage, nil

		case model.FinishReasonTool:
			return "", resp.Choices[0].Delta.ToolCalls, lastResp.Usage, nil

		default:
			delta := resp.Choices[0].Delta

			switch {
			case delta.Reasoning != "":
				reasonThinking = true

				fmt.Printf("\\u001b[91m%s\\u001b[0m", delta.Reasoning)

			case delta.Content != "":
				if reasonThinking {
					reasonThinking = false
					fmt.Print("\\n\\n")
				}

				fmt.Print(delta.Content)
				chunks = append(chunks, delta.Content)
			}
		}
	}

	// Stream ended without an explicit finish reason.
	text := strings.TrimLeft(strings.Join(chunks, " "), "\\n")
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
func (a *Agent) appendToolCalls(conversation *[]model.D, toolCalls []model.ResponseToolCall) {
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

	fmt.Print("\\n")
	*conversation = append(*conversation, model.D{"role": "assistant", "content": content})
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

	fmt.Printf("\\n\\n\\u001b[90mInput: %d  Reasoning: %d  Completion: %d  Output: %d  Window: %d (%.0f%% of %.0fK) TPS: %.2f\\u001b[0m",
		usage.PromptTokens, usage.ReasoningTokens, usage.CompletionTokens, usage.OutputTokens, contextTokens, percentage, of, usage.TokensPerSecond)
}

// callTools looks up requested tools by name and executes them.
func (a *Agent) callTools(ctx context.Context, toolCalls []model.ResponseToolCall) []model.D {
	resps := make([]model.D, 0, len(toolCalls))

	for _, toolCall := range toolCalls {
		tool, exists := a.tools[toolCall.Function.Name]
		if !exists {
			fmt.Printf("\\u001b[91mUnknown tool: %s\\u001b[0m\\n", toolCall.Function.Name)
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
	libs, err := libs.New(libs.WithVersion(defaults.LibVersion("")))
	if err != nil {
		return models.Path{}, err
	}
	if _, err := libs.Download(ctx, kronk.FmtLogger); err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
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
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
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
		libs.WithVersion(defaults.LibVersion("")),
	)
	if err != nil {
		return models.Path{}, err
	}

	if _, err := libs.Download(ctx, kronk.FmtLogger); err != nil {
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
		model.WithProjFile(mp.ProjFile),
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

		switch resp.Choices[0].FinishReason() {
		case model.FinishReasonStop:
			break loop

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

	contextTokens := lr.Usage.PromptTokens + lr.Usage.CompletionTokens
	contextWindow := krn.ModelConfig().ContextWindow()
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

	"github.com/ardanlabs/bucky/pkg/audio"
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

	lib, err := buckylibs.New()
	if err != nil {
		return buckymodels.Path{}, fmt.Errorf("libs new: %w", err)
	}

	if _, err := lib.Download(ctx, bucky.FmtLogger); err != nil {
		return buckymodels.Path{}, fmt.Errorf("download whisper.cpp libs: %w", err)
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

	if err := bucky.Init(); err != nil {
		return nil, fmt.Errorf("bucky init: %w", err)
	}

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

	samples, err := audio.Decode(f)
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

	lib, err := buckylibs.New()
	if err != nil {
		return buckymodels.Path{}, fmt.Errorf("libs new: %w", err)
	}

	if _, err := lib.Download(ctx, bucky.FmtLogger); err != nil {
		return buckymodels.Path{}, fmt.Errorf("download whisper.cpp libs: %w", err)
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

	if err := bucky.Init(); err != nil {
		return nil, fmt.Errorf("bucky init: %w", err)
	}

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

package main

import (
	"context"
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
		fmt.Println("Unloading whisper")
		if err := b.Unload(context.Background()); err != nil {
			fmt.Printf("unload: %v\\n", err)
		}
	}()

	if err := liveTranscribe(b); err != nil {
		return fmt.Errorf("live transcribe: %w", err)
	}

	return nil
}

// =============================================================================

// liveTranscribe opens a streaming session, wires the default microphone
// into it, and renders the transcript live until the speaker says "STOP"
// (or presses Ctrl-C).
func liveTranscribe(b *bucky.Bucky) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// VAD is on by default, so Finals commit when you pause — cuts land in
	// the gaps between phrases instead of mid-word. PartialEveryMs is kept
	// short for a snappy live feel.
	stream, err := b.NewStream(ctx,
		model.WithStreamLanguage("en"),
		model.WithPartialEveryMs(700),
	)
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
	for _, f := range strings.Fields(strings.ToLower(text)) {
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

func installSystem() (buckymodels.Path, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	lib, err := buckylibs.New()
	if err != nil {
		return buckymodels.Path{}, fmt.Errorf("libs new: %w", err)
	}

	if _, err := lib.Download(ctx, bucky.FmtLogger); err != nil {
		return buckymodels.Path{}, fmt.Errorf("download whisper.cpp libs: %w", err)
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

	if err := bucky.Init(); err != nil {
		return nil, fmt.Errorf("bucky init: %w", err)
	}

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
				"messages":   messages,
				"tools":      toolDocuments(),
				"max_tokens": 2048,
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
	var content strings.Builder

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

		case model.FinishReasonTool:
			fmt.Println()
			if krn.ModelInfo().IsGPTModel {
				fmt.Println()
			}

			fmt.Printf("\\u001b[92mModel Asking For Tool Calls:\\n\\u001b[0m")

			var toolCallDocs []model.D
			for _, tool := range resp.Choices[0].Delta.ToolCalls {
				fmt.Printf("\\u001b[92mToolID[%s]: %s(%s)\\n\\u001b[0m",
					tool.ID,
					tool.Function.Name,
					tool.Function.Arguments,
				)

				argsJSON, _ := json.Marshal(tool.Function.Arguments)
				toolCallDocs = append(toolCallDocs, model.D{
					"id":   tool.ID,
					"type": "function",
					"function": model.D{
						"name":      tool.Function.Name,
						"arguments": string(argsJSON),
					},
				})
			}

			messages = append(messages, model.D{
				"role":       "assistant",
				"tool_calls": toolCallDocs,
			})

			for _, tool := range resp.Choices[0].Delta.ToolCalls {
				messages = append(messages, model.D{
					"role":         "tool",
					"tool_call_id": tool.ID,
					"name":         tool.Function.Name,
					"content":      \`{"temperature": "72°F", "condition": "sunny"}\`,
				})
			}

			break loop

		default:
			if resp.Choices[0].Delta.Reasoning != "" {
				fmt.Printf("\\u001b[91m%s\\u001b[0m", resp.Choices[0].Delta.Reasoning)
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

			content.WriteString(resp.Choices[0].Delta.Content)
			fmt.Printf("%s", resp.Choices[0].Delta.Content)
		}
	}

	// Append the assistant's response to conversation history.
	if content.Len() > 0 {
		messages = append(messages, model.TextMessage(model.RoleAssistant, content.String()))
	}

	// -------------------------------------------------------------------------

	contextTokens := lr.Usage.PromptTokens + lr.Usage.CompletionTokens
	contextWindow := krn.ModelConfig().ContextWindow()
	percentage := (float64(contextTokens) / float64(contextWindow)) * 100
	of := float32(contextWindow) / float32(1024)

	fmt.Printf("\\n\\n\\u001b[90mInput: %d  Reasoning: %d  Completion: %d  Output: %d  Window: %d (%.0f%% of %.0fK) TPS: %.2f\\u001b[0m\\n",
		lr.Usage.PromptTokens, lr.Usage.ReasoningTokens, lr.Usage.CompletionTokens, lr.Usage.OutputTokens, contextTokens, percentage, of, lr.Usage.TokensPerSecond)

	return messages, nil
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
	"log"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/google/uuid"
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
		log.Fatal(err)
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
		libs.WithVersion(defaults.LibVersion("")),
	)
	if err != nil {
		return models.Path{}, err
	}

	if _, err := libs.Download(ctx, kronk.FmtLogger); err != nil {
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
	fmt.Println("Loading model...")

	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	krn, err := kronk.New(
		model.WithModelFiles(mp.ModelFiles),
		model.WithProjFile(mp.ProjFile),
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
	fmt.Println("- flashAttention :", cfg.FlashAttention)
	fmt.Println("- nBatch         :", cfg.NBatch())
	fmt.Println("- nuBatch        :", cfg.NUBatch())
	fmt.Println("- modelType      :", mi.Type)
	fmt.Println("- isGPT          :", mi.IsGPTModel)
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
	traceID := uuid.NewString()

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
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// modelSource is the model to download. It may be a HuggingFace URL,
// a canonical "provider/modelID", or a bare model id.
var modelSource = "ggml-org/embeddinggemma-300m-qat-Q8_0"

// var modelSource = "embeddinggemma-300m-qat-Q8_0"

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
	fmt.Println("- flashAttention :", krn.ModelConfig().FlashAttention)
	fmt.Println("- nBatch         :", krn.ModelConfig().NBatch())
	fmt.Println("- nuBatch        :", krn.ModelConfig().NUBatch())
	fmt.Println("- embeddings     :", krn.ModelInfo().IsEmbedModel)
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
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
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
		libs.WithVersion(defaults.LibVersion("")),
	)
	if err != nil {
		return models.Path{}, err
	}

	if _, err := libs.Download(ctx, kronk.FmtLogger); err != nil {
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
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
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

	if err := kronk.Init(); err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}

	// -------------------------------------------------------------------------

	const cacheTTL = 15 * time.Second

	cfg := pool.Config{
		Log:           kronk.FmtLogger,
		KronkModels:   mdls,
		BudgetPercent: 80,
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
		libs.WithVersion(defaults.LibVersion("")),
	)
	if err != nil {
		return nil, err
	}

	if _, err := libs.Download(ctx, kronk.FmtLogger); err != nil {
		return nil, fmt.Errorf("unable to install llama.cpp: %w", err)
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
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
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

	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	krn, err := kronk.New(
		model.WithModelFiles(mp.ModelFiles),
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
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

const (
	modelChatSource  = "unsloth/gemma-4-E4B-it-qat-UD-Q4_K_XL"
	modelEmbedSource = "ggml-org/embeddinggemma-300m-qat-Q8_0"
	dbPath           = "rag/docs/duck-rag.db" // ":memory:"
	chunksFile       = "rag/docs/book.chunks"
	dimentions       = 768
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

	db, err := duck.LoadData(dbPath, krnEmbed, dimentions, chunksFile)
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
		libs.WithVersion(defaults.LibVersion("")),
	)
	if err != nil {
		return models.Path{}, models.Path{}, err
	}

	if _, err := libs.Download(ctx, kronk.FmtLogger); err != nil {
		return models.Path{}, models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
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
	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	krn, err := kronk.New(
		model.WithModelFiles(mp.ModelFiles),
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
	fmt.Println("- isGPT        :", krn.ModelInfo().IsGPTModel)
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

loop:
	for resp := range ch {
		lr = resp

		switch resp.Choices[0].FinishReason() {
		case model.FinishReasonError:
			return messages, fmt.Errorf("error from model: %s", resp.Choices[0].Delta.Content)

		case model.FinishReasonStop:
			break loop

		case model.FinishReasonTool:
			fmt.Println()
			if krn.ModelInfo().IsGPTModel {
				fmt.Println()
			}

			fmt.Printf("\\u001b[92mModel Asking For Tool Call:\\nToolID[%s]: %s(%s)\\u001b[0m\\n",
				resp.Choices[0].Delta.ToolCalls[0].ID,
				resp.Choices[0].Delta.ToolCalls[0].Function.Name,
				resp.Choices[0].Delta.ToolCalls[0].Function.Arguments,
			)

			messages = append(messages,
				model.TextMessage("tool", fmt.Sprintf("Tool call %s: %s(%v)",
					resp.Choices[0].Delta.ToolCalls[0].ID,
					resp.Choices[0].Delta.ToolCalls[0].Function.Name,
					resp.Choices[0].Delta.ToolCalls[0].Function.Arguments),
				),
			)
			break loop

		default:
			if resp.Choices[0].Delta.Reasoning != "" {
				fmt.Printf("\\u001b[91m%s\\u001b[0m", resp.Choices[0].Delta.Reasoning)
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

			fmt.Printf("%s", resp.Choices[0].Delta.Content)
		}
	}

	// -------------------------------------------------------------------------

	contextTokens := lr.Usage.PromptTokens + lr.Usage.CompletionTokens
	contextWindow := krn.ModelConfig().ContextWindow()
	percentage := (float64(contextTokens) / float64(contextWindow)) * 100
	of := float32(contextWindow) / float32(1024)

	fmt.Printf("\\n\\n\\u001b[90mPrompt: %d  Reasoning: %d  Completion: %d  Output: %d  Window: %d (%.0f%% of %.0fK) TPS: %.2f\\u001b[0m\\n",
		lr.Usage.PromptTokens, lr.Usage.ReasoningTokens, lr.Usage.CompletionTokens, lr.Usage.OutputTokens, contextTokens, percentage, of, lr.Usage.TokensPerSecond)

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
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
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
	fmt.Println("- flashAttention :", krn.ModelConfig().FlashAttention)
	fmt.Println("- nBatch         :", krn.ModelConfig().NBatch())
	fmt.Println("- nuBatch        :", krn.ModelConfig().NUBatch())
	fmt.Println("- embeddings     :", krn.ModelInfo().IsEmbedModel)
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

	type pendingToolCall struct {
		id        string
		name      string
		arguments string
	}
	var pendingCalls []pendingToolCall

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

			pendingCalls = append(pendingCalls, pendingToolCall{
				id:        event.ItemID,
				name:      event.Name,
				arguments: event.Arguments,
			})

		case "response.completed":
			finalResp = event.Response
		}
	}

	if fullText != "" {
		messages = append(messages,
			model.TextMessage("assistant", fullText),
		)
	}

	if len(pendingCalls) > 0 {
		var toolCallDocs []model.D
		for _, tc := range pendingCalls {
			toolCallDocs = append(toolCallDocs, model.D{
				"id":   tc.id,
				"type": "function",
				"function": model.D{
					"name":      tc.name,
					"arguments": tc.arguments,
				},
			})
		}

		messages = append(messages, model.D{
			"role":       "assistant",
			"tool_calls": toolCallDocs,
		})

		for _, tc := range pendingCalls {
			messages = append(messages, model.D{
				"role":         "tool",
				"tool_call_id": tc.id,
				"name":         tc.name,
				"content":      \`{"temperature": "72°F", "condition": "sunny"}\`,
			})
		}
	}

	// -------------------------------------------------------------------------

	if finalResp != nil {
		contextTokens := finalResp.Usage.InputTokens + finalResp.Usage.OutputTokens
		contextWindow := krn.ModelConfig().ContextWindow()
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
		libs.WithVersion(defaults.LibVersion("")),
	)
	if err != nil {
		return models.Path{}, err
	}

	if _, err := libs.Download(ctx, kronk.FmtLogger); err != nil {
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
		model.WithProjFile(mp.ProjFile),
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

		switch resp.Choices[0].FinishReason() {
		case model.FinishReasonStop:
			break loop

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

	contextTokens := lr.Usage.PromptTokens + lr.Usage.CompletionTokens
	contextWindow := krn.ModelConfig().ContextWindow()
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
                <li><a href="#example-pool">Pool</a></li>
                <li><a href="#example-question">Question</a></li>
                <li><a href="#example-rag">Rag</a></li>
                <li><a href="#example-rerank">Rerank</a></li>
                <li><a href="#example-response">Response</a></li>
                <li><a href="#example-vision">Vision</a></li>
              </ul>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
