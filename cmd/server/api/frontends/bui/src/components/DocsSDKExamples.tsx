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
	SourceURL: "https://huggingface.co/unsloth/gpt-oss-20b-GGUF/resolve/main/gpt-oss-20b-Q8_0.gguf",
	// ModelID: "gpt-oss-20b-Q8_0",
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
		"stream":         true,
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
	reasonFirstChunk := true
	reasonThinking := false

	for resp := range ch {
		lastResp = resp

		if len(resp.Choice) == 0 {
			continue
		}

		// On the first real chunk, stop the latency printer.
		stopPrinter()

		switch resp.Choice[0].FinishReason() {
		case model.FinishReasonError:
			return "", nil, lastResp.Usage, fmt.Errorf("error from model: %s", resp.Choice[0].Delta.Content)

		case model.FinishReasonStop:
			text := strings.TrimLeft(strings.Join(chunks, " "), "\\n")
			return text, nil, lastResp.Usage, nil

		case model.FinishReasonTool:
			return "", resp.Choice[0].Delta.ToolCalls, lastResp.Usage, nil

		default:
			delta := resp.Choice[0].Delta

			switch {
			case delta.Reasoning != "":
				reasonThinking = true

				if reasonFirstChunk {
					reasonFirstChunk = false
					fmt.Print("\\n")
				}

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
	contextWindow := a.krn.ModelConfig().ContextWindow
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
			continue
		}

		fmt.Printf("\\u001b[92m%s(%v)\\u001b[0m:\\n", toolCall.Function.Name, toolCall.Function.Arguments)
		resps = append(resps, tool.Call(ctx, toolCall))
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

	fmt.Print("- system info:\\n\\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	fmt.Println("- contextWindow:", krn.ModelConfig().ContextWindow)
	fmt.Printf("- k/v          : %s/%s\\n", krn.ModelConfig().CacheTypeK, krn.ModelConfig().CacheTypeV)
	fmt.Println("- nBatch       :", krn.ModelConfig().NBatch)
	fmt.Println("- nuBatch      :", krn.ModelConfig().NUBatch)
	fmt.Println("- modelType    :", krn.ModelInfo().Type)
	fmt.Println("- isGPT        :", krn.ModelInfo().IsGPTModel)
	fmt.Println("- template     :", krn.ModelInfo().Template.FileName)
	fmt.Println("- grammar      :", krn.ModelConfig().DefaultParams.Grammar != "")

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
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// modelSpec defines how to obtain the model to download.
// - SourceURL: Download the model file directly from a HuggingFace URL
// - ProjURL  : Download the projection file directly from a HuggingFace URL
// - ModelID  : Download the model from the catalog by model ID
//
// To use a catalog model, comment out SourceURL/ProjURL and set ModelID.
// To use direct URLs, comment out ModelID and set SourceURL/ProjURL.
type modelSpec struct {
	SourceURL string
	ProjURL   string
	ModelID   string
}

// Configure this to switch between URL and catalog downloads.
// Set either (SourceURL and ProjURL) or ModelID, not both.
var modelSpecConfig = modelSpec{
	SourceURL: "https://huggingface.co/mradermacher/Qwen2-Audio-7B-GGUF/resolve/main/Qwen2-Audio-7B.Q8_0.gguf",
	ProjURL:   "https://huggingface.co/mradermacher/Qwen2-Audio-7B-GGUF/resolve/main/Qwen2-Audio-7B.mmproj-Q8_0.gguf",
	// ModelID: "Qwen2-Audio-7B-Q8_0",
}

const audioFile = "examples/samples/jfk.wav"

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

	// -------------------------------------------------------------------------

	ctlg, err := catalog.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to create catalog system: %w", err)
	}

	if err := ctlg.Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download catalog: %w", err)
	}

	// -------------------------------------------------------------------------

	mdls, err := models.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	// Download model based on spec config using switch/case
	var mp models.Path

	switch {
	case modelSpecConfig.SourceURL != "":
		fmt.Println("Downloading model from URL:", modelSpecConfig.SourceURL)
		fmt.Println("Downloading projection file:", modelSpecConfig.ProjURL)
		mp, err = mdls.Download(ctx, kronk.FmtLogger, modelSpecConfig.SourceURL, modelSpecConfig.ProjURL)

	case modelSpecConfig.ModelID != "":
		fmt.Println("Downloading model from catalog:", modelSpecConfig.ModelID)
		mp, err = ctlg.DownloadModel(ctx, kronk.FmtLogger, modelSpecConfig.ModelID)

	default:
		return models.Path{}, fmt.Errorf("modelSpecConfig requires either (SourceURL and ProjURL) or ModelID to be set")
	}

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
		ModelFiles: mp.ModelFiles,
		ProjFile:   mp.ProjFile,
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
	fmt.Printf("- k/v          : %s/%s\\n", krn.ModelConfig().CacheTypeK, krn.ModelConfig().CacheTypeV)
	fmt.Println("- nBatch       :", krn.ModelConfig().NBatch)
	fmt.Println("- nuBatch      :", krn.ModelConfig().NUBatch)
	fmt.Println("- modelType    :", krn.ModelInfo().Type)
	fmt.Println("- isGPT        :", krn.ModelInfo().IsGPTModel)
	fmt.Println("- template     :", krn.ModelInfo().Template.FileName)

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

func performChat(ctx context.Context, krn *kronk.Kronk, question string, audioFile string) (<-chan model.ChatResponse, error) {
	audio, err := readImage(audioFile)
	if err != nil {
		return nil, fmt.Errorf("read image: %w", err)
	}

	fmt.Printf("\\nQuestion: %s\\n", question)

	d := model.D{
		"messages":    model.RawMediaMessage(question, audio),
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
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// modelSpec defines how to obtain the model to download.
// - SourceURL: Download the model file directly from a HuggingFace URL
// - ModelID  : Download the model from the catalog by model ID
//
// To use a catalog model, comment out SourceURL and set ModelID.
// To use a direct URL, comment out ModelID and set SourceURL.
type modelSpec struct {
	SourceURL string
	ModelID   string
}

// Configure this to switch between URL and catalog downloads.
// Set either SourceURL or ModelID, not both.
var modelSpecConfig = modelSpec{
	SourceURL: "https://huggingface.co/unsloth/Qwen3-0.6B-GGUF/resolve/main/Qwen3-0.6B-Q8_0.gguf",
	// ModelID: "Qwen3-0.6B-Q8_0",
}

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

	ctlg, err := catalog.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to create catalog system: %w", err)
	}

	if err := ctlg.Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download catalog: %w", err)
	}

	// -------------------------------------------------------------------------

	mdls, err := models.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	// Download model based on spec config using switch/case
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

	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	cfg := model.Config{
		ModelFiles: mp.ModelFiles,
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
	fmt.Printf("- k/v          : %s/%s\\n", krn.ModelConfig().CacheTypeK, krn.ModelConfig().CacheTypeV)
	fmt.Println("- nBatch       :", krn.ModelConfig().NBatch)
	fmt.Println("- nuBatch      :", krn.ModelConfig().NUBatch)
	fmt.Println("- modelType    :", krn.ModelInfo().Type)
	fmt.Println("- isGPT        :", krn.ModelInfo().IsGPTModel)
	fmt.Println("- template     :", krn.ModelInfo().Template.FileName)
	fmt.Println("- grammar      :", krn.ModelConfig().DefaultParams.Grammar != "")

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

loop:
	for resp := range ch {
		lr = resp

		if len(resp.Choice) == 0 {
			continue
		}

		switch resp.Choice[0].FinishReason() {
		case model.FinishReasonError:
			return messages, fmt.Errorf("error from model: %s", resp.Choice[0].Delta.Content)

		case model.FinishReasonStop:
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
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// modelSpec defines how to obtain the model to download.
// - SourceURL: Download the model file directly from a HuggingFace URL
// - ModelID  : Download the model from the catalog by model ID
//
// To use a catalog model, comment out SourceURL and set ModelID.
// To use a direct URL, comment out ModelID and set SourceURL.
type modelSpec struct {
	SourceURL string
	ModelID   string
}

// Configure this to switch between URL and catalog downloads.
// Set either SourceURL or ModelID, not both.
var modelSpecConfig = modelSpec{
	SourceURL: "https://huggingface.co/ggml-org/embeddinggemma-300m-qat-q8_0-GGUF/resolve/main/embeddinggemma-300m-qat-Q8_0.gguf",
	// ModelID: "embeddinggemma-300m-qat-Q8_0",
}

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

	ctlg, err := catalog.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to create catalog system: %w", err)
	}

	if err := ctlg.Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download catalog: %w", err)
	}

	// -------------------------------------------------------------------------

	mdls, err := models.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	// Download model based on spec config using switch/case
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

	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	cfg := model.Config{
		ModelFiles: mp.ModelFiles,
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
	fmt.Printf("- k/v          : %s/%s\\n", krn.ModelConfig().CacheTypeK, krn.ModelConfig().CacheTypeV)
	fmt.Println("- nBatch       :", krn.ModelConfig().NBatch)
	fmt.Println("- nuBatch      :", krn.ModelConfig().NUBatch)
	fmt.Println("- embeddings   :", krn.ModelInfo().IsEmbedModel)
	fmt.Println("- modelType    :", krn.ModelInfo().Type)
	fmt.Println("- isGPT        :", krn.ModelInfo().IsGPTModel)
	fmt.Println("- template     :", krn.ModelInfo().Template.FileName)

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
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
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

// modelSpec defines how to obtain the model to download.
// - SourceURL: Download the model file directly from a HuggingFace URL
// - ModelID  : Download the model from the catalog by model ID
//
// To use a catalog model, comment out SourceURL and set ModelID.
// To use a direct URL, comment out ModelID and set SourceURL.
type modelSpec struct {
	SourceURL string
	ModelID   string
}

// Configure this to switch between URL and catalog downloads.
// Set either SourceURL or ModelID, not both.
var modelSpecConfig = modelSpec{
	SourceURL: "https://huggingface.co/unsloth/Qwen3-0.6B-GGUF/resolve/main/Qwen3-0.6B-Q8_0.gguf",
	// ModelID: "Qwen3-0.6B-Q8_0",
}

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

	ctlg, err := catalog.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to create catalog system: %w", err)
	}

	if err := ctlg.Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download catalog: %w", err)
	}

	// -------------------------------------------------------------------------

	mdls, err := models.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	// Download model based on spec config using switch/case
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

	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	cfg := model.Config{
		ModelFiles: mp.ModelFiles,
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
	fmt.Printf("- k/v          : %s/%s\\n", krn.ModelConfig().CacheTypeK, krn.ModelConfig().CacheTypeV)
	fmt.Println("- nBatch       :", krn.ModelConfig().NBatch)
	fmt.Println("- nuBatch      :", krn.ModelConfig().NUBatch)
	fmt.Println("- modelType    :", krn.ModelInfo().Type)
	fmt.Println("- isGPT        :", krn.ModelInfo().IsGPTModel)
	fmt.Println("- template     :", krn.ModelInfo().Template.FileName)

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
		"grammar":     grammarJSONObject,
		"temperature": 0.7,
		"max_tokens":  512,
	}

	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		return fmt.Errorf("chat streaming: %w", err)
	}

	fmt.Print("RESPONSE: ")

	for resp := range ch {
		switch resp.Choice[0].FinishReason() {
		case model.FinishReasonError:
			return fmt.Errorf("error from model: %s", resp.Choice[0].Delta.Content)

		case model.FinishReasonStop:
			fmt.Println()
			return nil

		default:
			fmt.Print(resp.Choice[0].Delta.Content)
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
		switch resp.Choice[0].FinishReason() {
		case model.FinishReasonError:
			return fmt.Errorf("error from model: %s", resp.Choice[0].Delta.Content)

		case model.FinishReasonStop:
			fmt.Println()
			return nil

		default:
			fmt.Print(resp.Choice[0].Delta.Content)
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
		switch resp.Choice[0].FinishReason() {
		case model.FinishReasonError:
			return fmt.Errorf("error from model: %s", resp.Choice[0].Delta.Content)

		case model.FinishReasonStop:
			fmt.Println()
			return nil

		default:
			fmt.Print(resp.Choice[0].Delta.Content)
		}
	}

	return nil
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

const modelURL = "https://huggingface.co/unsloth/Qwen3-0.6B-GGUF/resolve/main/Qwen3-0.6B-Q8_0.gguf"

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
	// mp, err := templates.Catalog().DownloadModel(ctx, kronk.FmtLogger, "Qwen3-8B-Q8_0")
	// if err != nil {
	// 	return models.Path{}, fmt.Errorf("unable to download model: %w", err)
	// }

	return mp, nil
}

func newKronk(mp models.Path) (*kronk.Kronk, error) {
	fmt.Println("loading model...")

	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	cfg := model.Config{
		ModelFiles: mp.ModelFiles,
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
	fmt.Printf("- k/v          : %s/%s\\n", krn.ModelConfig().CacheTypeK, krn.ModelConfig().CacheTypeV)
	fmt.Println("- nBatch       :", krn.ModelConfig().NBatch)
	fmt.Println("- nuBatch      :", krn.ModelConfig().NUBatch)
	fmt.Println("- modelType    :", krn.ModelInfo().Type)
	fmt.Println("- isGPT        :", krn.ModelInfo().IsGPTModel)
	fmt.Println("- template     :", krn.ModelInfo().Template.FileName)

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
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// modelSpec defines how to obtain the model to download.
// - SourceURL: Download the model file directly from a HuggingFace URL
// - ModelID  : Download the model from the catalog by model ID
//
// To use a catalog model, comment out SourceURL and set ModelID.
// To use a direct URL, comment out ModelID and set SourceURL.
type modelSpec struct {
	SourceURL string
	ModelID   string
}

// Configure this to switch between URL and catalog downloads.
// Set either SourceURL or ModelID, not both.
var modelSpecConfig = modelSpec{
	SourceURL: "https://huggingface.co/gpustack/bge-reranker-v2-m3-GGUF/resolve/main/bge-reranker-v2-m3-Q8_0.gguf",
	// ModelID: "bge-reranker-v2-m3-Q8_0",
}

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

	ctlg, err := catalog.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to create catalog system: %w", err)
	}

	if err := ctlg.Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download catalog: %w", err)
	}

	// -------------------------------------------------------------------------

	mdls, err := models.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	// Download model based on spec config using switch/case
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

	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	cfg := model.Config{
		ModelFiles: mp.ModelFiles,
	}

	krn, err := kronk.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("unable to create reranker model: %w", err)
	}

	fmt.Print("- system info:\\n\\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	fmt.Println("- contextWindow:", krn.ModelConfig().ContextWindow)
	fmt.Printf("- k/v          : %s/%s\\n", krn.ModelConfig().CacheTypeK, krn.ModelConfig().CacheTypeV)
	fmt.Println("- nBatch       :", krn.ModelConfig().NBatch)
	fmt.Println("- nuBatch      :", krn.ModelConfig().NUBatch)
	fmt.Println("- embeddings   :", krn.ModelInfo().IsEmbedModel)
	fmt.Println("- modelType    :", krn.ModelInfo().Type)
	fmt.Println("- isGPT        :", krn.ModelInfo().IsGPTModel)
	fmt.Println("- template     :", krn.ModelInfo().Template.FileName)

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
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// modelSpec defines how to obtain the model to download.
// - SourceURL: Download the model file directly from a HuggingFace URL
// - ModelID  : Download the model from the catalog by model ID
//
// To use a catalog model, comment out SourceURL and set ModelID.
// To use a direct URL, comment out ModelID and set SourceURL.
type modelSpec struct {
	SourceURL string
	ModelID   string
}

// Configure this to switch between URL and catalog downloads.
// Set either SourceURL or ModelID, not both.
var modelSpecConfig = modelSpec{
	SourceURL: "https://huggingface.co/unsloth/Qwen3-0.6B-GGUF/resolve/main/Qwen3-0.6B-Q8_0.gguf",
	// ModelID: "Qwen3-0.6B-Q8_0",
}

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

	ctlg, err := catalog.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to create catalog system: %w", err)
	}

	if err := ctlg.Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download catalog: %w", err)
	}

	// -------------------------------------------------------------------------

	mdls, err := models.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	// Download model based on spec config using switch/case
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

	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	cfg := model.Config{
		ModelFiles: mp.ModelFiles,
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
	fmt.Printf("- k/v          : %s/%s\\n", krn.ModelConfig().CacheTypeK, krn.ModelConfig().CacheTypeV)
	fmt.Println("- nBatch       :", krn.ModelConfig().NBatch)
	fmt.Println("- nuBatch      :", krn.ModelConfig().NUBatch)
	fmt.Println("- modelType    :", krn.ModelInfo().Type)
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
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// modelSpec defines how to obtain the model to download.
// - SourceURL: Download the model file directly from a HuggingFace URL
// - ProjURL  : Download the projection file directly from a HuggingFace URL
// - ModelID  : Download the model from the catalog by model ID
//
// To use a catalog model, comment out SourceURL and set ModelID.
// To use a direct URL, comment out ModelID and set SourceURL/ProjURL.
type modelSpec struct {
	SourceURL string
	ProjURL   string
	ModelID   string
}

// Configure this to switch between URL and catalog downloads.
// Set either SourceURL (with ProjURL) or ModelID, not both.
var modelSpecConfig = modelSpec{
	SourceURL: "https://huggingface.co/unsloth/Qwen3.5-0.8B-GGUF/resolve/main/Qwen3.5-0.8B-Q8_0.gguf",
	ProjURL:   "https://huggingface.co/unsloth/Qwen3.5-0.8B-GGUF/resolve/main/mmproj-F16.gguf",
	// ModelID: "Qwen3.5-0.8B-Q8_0",
}

const imageFile = "examples/samples/giraffe.jpg"

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

	ctlg, err := catalog.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to create catalog system: %w", err)
	}

	if err := ctlg.Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download catalog: %w", err)
	}

	// -------------------------------------------------------------------------

	mdls, err := models.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	// Download model based on spec config using switch/case
	var mp models.Path

	switch {
	case modelSpecConfig.SourceURL != "":
		fmt.Println("Downloading model from URL:", modelSpecConfig.SourceURL)
		fmt.Println("Downloading projection file:", modelSpecConfig.ProjURL)
		mp, err = mdls.Download(ctx, kronk.FmtLogger, modelSpecConfig.SourceURL, modelSpecConfig.ProjURL)

	case modelSpecConfig.ModelID != "":
		fmt.Println("Downloading model from catalog:", modelSpecConfig.ModelID)
		mp, err = ctlg.DownloadModel(ctx, kronk.FmtLogger, modelSpecConfig.ModelID)

	default:
		return models.Path{}, fmt.Errorf("modelSpecConfig requires either (SourceURL and ProjURL) or ModelID to be set")
	}

	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install model: %w", err)
	}

	return mp, nil
}

func newKronk(mp models.Path) (*kronk.Kronk, error) {
	fmt.Println("loading model...")

	if err := kronk.Init(kronk.WithLogLevel(kronk.LogNormal)); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	cfg := model.Config{
		ModelFiles:    mp.ModelFiles,
		ProjFile:      mp.ProjFile,
		ContextWindow: 8196,
		CacheTypeK:    model.GGMLTypeF16,
		CacheTypeV:    model.GGMLTypeBF16,
		JinjaFile:     "/Users/bill/code/go/src/github.com/ardanlabs/kronk_catalogs/templates/qwen3.5.jinja",
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
	fmt.Printf("- k/v          : %s/%s\\n", krn.ModelConfig().CacheTypeK, krn.ModelConfig().CacheTypeV)
	fmt.Println("- nBatch       :", krn.ModelConfig().NBatch)
	fmt.Println("- nuBatch      :", krn.ModelConfig().NUBatch)
	fmt.Println("- modelType    :", krn.ModelInfo().Type)
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

export default function DocsSDKExamples() {
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

          <div className="card" id="example-chat">
            <h3>Chat</h3>
            <p className="doc-description">This example shows you how to create a simple chat application against an</p>
            <CodeBlock code={chatExample} language="go" />
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

          <div className="card" id="example-question">
            <h3>Question</h3>
            <p className="doc-description">This example shows you a basic program of using Kronk to ask a model a question.</p>
            <CodeBlock code={questionExample} language="go" />
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
                <li><a href="#example-chat">Chat</a></li>
                <li><a href="#example-embedding">Embedding</a></li>
                <li><a href="#example-grammar">Grammar</a></li>
                <li><a href="#example-question">Question</a></li>
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
