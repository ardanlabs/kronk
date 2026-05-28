//

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

const (
	codeFile = "code.chunk"
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

	var systemPrompt = `You are a helpful AI assistant specialized in code analysis. You have access to two tools:
	1. tool_read_file - Read the contents of any file
	2. tool_find_function - Search for a specific function by name in a JavaScript/jQuery file and return its body
	The default file is code.chunk (a 5000-line jQuery source file). When the user asks about a function:
	- Use tool_find_function to locate the function
	- Present the function code clearly to the user
	- If the tool cannot find the function, suggest alternatives or offer to read the file manually`

	messages = append(messages,
		model.TextMessage(model.RoleSystem, systemPrompt),
	)

	// Register available tools
	tools := make(map[string]Tool)
	RegisterReadFile(tools)
	RegisterFindFunction(tools)
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

			toolDocs := []model.D{
				RegisterReadFile(nil),
				RegisterFindFunction(nil),
			}

			d := model.D{
				"messages":   messages,
				"tools":      toolDocs,
				"max_tokens": 4096,
			}

			ch, err := performChat(ctx, krn, d)

			if err != nil {
				return nil, fmt.Errorf("run: unable to perform chat: %w", err)
			}

			messages, err = modelResponse(krn, messages, ch, tools)

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

func modelResponse(krn *kronk.Kronk, messages []model.D, ch <-chan model.ChatResponse, tools map[string]Tool) ([]model.D, error) {
	fmt.Print("\nMODEL> ")

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
			fmt.Printf("\033[92mModel is calling tools:\033[0m\n")

			for _, tool := range resp.Choices[0].Delta.ToolCalls {
				fmt.Printf("\033[92m  Tool: %s\033[0m\n", tool.Function.Name)

				var args map[string]any

				argsJSON, _ := json.Marshal(tool.Function.Arguments)

				if err := json.Unmarshal(argsJSON, &args); err != nil {
					fmt.Printf("  Error parsing args: %v\n", err)
					continue
				}

				if toolFn, ok := tools[tool.Function.Name]; ok {
					toolResult := toolFn.Call(context.Background(), tool)
					contentMap, _ := toolResult["content"].(string)
					var resultData map[string]any
					json.Unmarshal([]byte(contentMap), &resultData)
					fmt.Printf("  \033[90mResult: %v\033[0m\n\n", resultData)
					messages = append(messages, toolResult)
				} else {
					fmt.Printf("  \033[91mTool not found: %s\033[0m\n", tool.Function.Name)
				}
			}

			break loop
		default:
			if resp.Choices[0].Delta.Reasoning != "" {
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

type Tool interface {
	Call(ctx context.Context, toolCall model.ResponseToolCall) model.D
}

func toolSuccessResponse(toolID string, toolName string, keyValues ...any) model.D {
	data := make(map[string]any, len(keyValues)/2)

	for i := 0; i < len(keyValues); i += 2 {
		data[keyValues[i].(string)] = keyValues[i+1]
	}

	return toolResponse(toolID, toolName, data, "SUCCESS")
}

func toolErrorResponse(toolID string, toolName string, err error) model.D {
	data := map[string]any{"error": err.Error()}

	return toolResponse(toolID, toolName, data, "FAILED")
}

func toolResponse(toolID string, toolName string, data map[string]any, status string) model.D {
	info := struct {
		Status string         `json:"status"`
		Data   map[string]any `json:"data"`
	}{
		Status: status,
		Data:   data,
	}

	content, err := json.Marshal(info)

	if err != nil {
		return model.D{
			"role":         "tool",
			"name":         toolName,
			"tool_call_id": toolID,
			"content":      `{"status": "FAILED", "data": "error marshaling tool response"}`,
		}
	}

	return model.D{
		"role":         "tool",
		"name":         toolName,
		"tool_call_id": toolID,
		"content":      string(content),
	}
}

type ReadFile struct {
	name string
}

func RegisterReadFile(tools map[string]Tool) model.D {
	rf := ReadFile{name: "tool_read_file"}

	tools[rf.name] = &rf

	return rf.toolDocuments()
}

func (rf *ReadFile) toolDocuments() model.D {
	return model.D{
		"type": "function",
		"function": model.D{
			"name":        rf.name,
			"description": "Read the contents of a given file path. Returns the full file content.",
			"parameters": model.D{
				"type": "object",
				"properties": model.D{
					"path": model.D{
						"type":        "string",
						"description": "The relative path of a file in the working directory.",
					},
				},
				"required": []string{"path"},
			},
		},
	}
}

func (rf *ReadFile) Call(ctx context.Context, toolCall model.ResponseToolCall) (resp model.D) {
	defer func() {
		if r := recover(); r != nil {
			resp = toolErrorResponse(toolCall.ID, toolCall.Function.Name, fmt.Errorf("%s", r))
		}
	}()

	path, _ := toolCall.Function.Arguments["path"].(string)

	if path == "" {
		path = codeFile
	}

	content, err := os.ReadFile(path)

	if err != nil {
		return toolErrorResponse(toolCall.ID, toolCall.Function.Name, err)
	}

	return toolSuccessResponse(toolCall.ID, toolCall.Function.Name, "file_contents", string(content))
}

type FindFunction struct {
	name string
}

func RegisterFindFunction(tools map[string]Tool) model.D {
	ff := FindFunction{name: "tool_find_function"}

	tools[ff.name] = &ff

	return ff.toolDocuments()
}

func (ff *FindFunction) toolDocuments() model.D {
	return model.D{
		"type": "function",
		"function": model.D{
			"name":        ff.name,
			"description": "Search for a function by name in a JavaScript/jQuery source file. Returns the function definition with line numbers, and its full body.",
			"parameters": model.D{
				"type": "object",
				"properties": model.D{
					"path": model.D{
						"type":        "string",
						"description": "The relative path of the JavaScript source file to search in. Defaults to code.chunk.",
					},
					"function_name": model.D{
						"type":        "string",
						"description": "The exact or partial name of the function to find.",
					},
				},
				"required": []string{"function_name"},
			},
		},
	}
}

func (ff *FindFunction) Call(ctx context.Context, toolCall model.ResponseToolCall) (resp model.D) {
	defer func() {
		if r := recover(); r != nil {
			resp = toolErrorResponse(toolCall.ID, ff.name, fmt.Errorf("%v", r))
		}
	}()

	path, _ := toolCall.Function.Arguments["path"].(string)

	funcName, _ := toolCall.Function.Arguments["function_name"].(string)

	if path == "" {
		path = codeFile
	}

	if funcName == "" {
		return toolErrorResponse(toolCall.ID, ff.name, fmt.Errorf("function_name is required"))
	}

	content, err := os.ReadFile(path)

	if err != nil {
		return toolErrorResponse(toolCall.ID, ff.name, err)
	}

	lines := strings.Split(string(content), "\n")

	var funcLineNum int

	var funcLine string

	found := false

	patterns := []string{
		`(?m)^function\s+` + regexp.QuoteMeta(funcName) + `\s*\(`,
		`(?m)^\s+\` + regexp.QuoteMeta(funcName) + `:\s*function\s*\(`,
		`(?m)^\s+\` + regexp.QuoteMeta(funcName) + `:\s*function\s*\(\)\s*\{`,
		`(?m)^\s+var\s+` + regexp.QuoteMeta(funcName) + `\s*=\s*function\s*\(`,
		`(?m)^\s+(?:this\.|\$\.)?` + regexp.QuoteMeta(funcName) + `\s*=\s*function\s*\(`,
	}

	for _, pat := range patterns {
		re := regexp.MustCompile(pat)
		for i, line := range lines {
			if re.MatchString(line) {
				funcLineNum = i + 1
				funcLine = line
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		return toolSuccessResponse(toolCall.ID, ff.name,
			"found", false,
			"message", fmt.Sprintf("Function '%s' not found in %s", funcName, path),
		)
	}
	funcBody, totalLines := extractFunctionBody(lines, funcLineNum-1)
	return toolSuccessResponse(toolCall.ID, ff.name,
		"found", true,
		"function_name", funcName,
		"file", path,
		"line", funcLineNum,
		"total_lines", totalLines,
		"signature", funcLine,
		"body", strings.Join(funcBody, "\n"),
	)
}

func extractFunctionBody(lines []string, startIdx int) ([]string, int) {
	var body []string

	depth := 0

	inBody := false

	for i := startIdx; i < len(lines); i++ {
		line := lines[i]
		body = append(body, line)
		for _, ch := range line {
			switch ch {
			case '{':
				depth++
				inBody = true
			case '}':
				depth--
				if inBody && depth == 0 {
					return body, i - startIdx + 1
				}
			}
		}
	}
	return body, len(lines) - startIdx
}
