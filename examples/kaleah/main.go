//

package main

import (
	"bufio"
	"context"
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

func getModelSource() string {
	// If user specified a model via env var, use it
	if model := os.Getenv("KRONK_MODEL"); model != "" {
		return model
	}

	mdls, err := models.New()
	if err != nil {
		return "unsloth/Qwen3.6-35B-A3B-UD-Q8_K_XL"
	}

	files, err := mdls.Files()
	if err != nil || len(files) == 0 {
		return "unsloth/Qwen3.6-35B-A3B-UD-Q8_K_XL"
	}

	// If only one model, use it
	if len(files) == 1 {
		return files[0].ID
	}

	// Multiple models — let user choose
	fmt.Println("\nAvailable models:")
	for i, f := range files {
		fmt.Printf("  %d: %s\n", i+1, f.ID)
	}
	fmt.Print("Select a model (number or name): ")

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return files[0].ID
	}

	// Check if input is a number
	for i, f := range files {
		if fmt.Sprintf("%d", i+1) == input {
			return f.ID
		}
	}

	// Check if input matches a model ID
	for _, f := range files {
		if strings.EqualFold(f.ID, input) {
			return f.ID
		}
	}

	// Input didn't match — return first
	return files[0].ID
}

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

	source := getModelSource()

	// Check if the selected model is already downloaded
	if mp, err := mdls.FullPath(source); err == nil {
		fmt.Println("Using existing model:", mp.ModelFiles[0])
		return mp, nil
	}

	// Model not found — download it
	fmt.Println("Downloading model:", source)
	mp, err := mdls.Download(ctx, kronk.FmtLogger, source)
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
		model.WithIncrementalCache(false),
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
	codeFile := "kaleah/code.chunk"

	var systemPrompt = `You will be given source code for one identifier from a program.
Return the source code first, then create a side-by-side index of the identifiers used in that code and their type or kind.
Use a JavaScript code block for the source code, followed by a markdown table with the columns Identifier and Type / Kind.`

	// WRITE SOME CODE / FUNCTION READS THE FILE
	code, err := os.ReadFile(codeFile)
	if err != nil {
		return fmt.Errorf("chat: read code.chunk: %w", err)
	}

	identifiers, err := columnZeroIdentifiers(codeFile)
	if err != nil {
		return fmt.Errorf("chat: index identifiers: %w", err)
	}

	messages = append(messages,
		model.TextMessage(model.RoleSystem, systemPrompt),
		// USER MESSAGE WITH THE CONTEXT OF THE FILE
		model.TextMessage("user", "Here is the code to analyze:\n\n"+string(code)),
	)

	for {
		var err error
		messages, err = userInput(messages, codeFile, identifiers)
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

func userInput(messages []model.D, codeFile string, identifiers map[string]string) ([]model.D, error) {
	fmt.Print("\nUSER> ")

	reader := bufio.NewReader(os.Stdin)

	userInput, err := reader.ReadString('\n')

	if err != nil {
		return messages, fmt.Errorf("unable to read user input: %w", err)
	}

	if strings.TrimSpace(userInput) == "quit" || userInput == "quit\n" {
		return nil, io.EOF
	}

	identifier := strings.TrimSpace(userInput)
	typ, exists := identifiers[identifier]
	if !exists {
		for _, token := range regexp.MustCompile(`[A-Za-z_$][A-Za-z0-9_$]*`).FindAllString(userInput, -1) {
			if typ, exists = identifiers[token]; exists {
				identifier = token
				break
			}
		}
	}

	if exists {
		code, err := extractColumnZeroIdentifierCode(codeFile, identifier)
		if err != nil {
			return messages, err
		}

		userInput = fmt.Sprintf(`Return the full body code for this %s named %s, then create a side-by-side identifier/type index for it.

Code:
%s`, typ, identifier, code)
	}

	messages = append(messages,
		model.TextMessage(model.RoleUser, userInput),
	)

	return messages, nil
}

func performChat(ctx context.Context, krn *kronk.Kronk, d model.D) (model.ChatResponse, error) {
	ch, err := krn.Chat(ctx, d)

	if err != nil {
		return model.ChatResponse{}, fmt.Errorf("chat streaming: %w", err)
	}

	return ch, nil
}

func modelResponse(krn *kronk.Kronk, messages []model.D, resp model.ChatResponse) ([]model.D, error) {
	fmt.Print("\nMODEL> ")

	if len(resp.Choices) == 0 {
		return messages, nil
	}

	switch resp.Choices[0].FinishReason() {
	case model.FinishReasonError:
		return messages, fmt.Errorf("error from model: %s", resp.Choices[0].Message.Content)
	case model.FinishReasonStop:
	}

	content := resp.Choices[0].Message.Content

	if content != "" {
		fmt.Print(content)
		messages = append(messages, model.TextMessage(model.RoleAssistant, content))
	}

	reasoning := resp.Choices[0].Message.Reasoning

	if reasoning != "" {
		fmt.Printf("\033[91m%s\033[0m", reasoning)
	}

	fmt.Printf("\n\033[90mTokens: %d input, %d output | TPS: %.2f\033[0m\n",
		resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TokensPerSecond)

	return messages, nil
}

func columnZeroIdentifiers(filename string) (map[string]string, error) {
	b, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filename, err)
	}

	identifiers := make(map[string]string)
	declRE := regexp.MustCompile(`^(function|var|let|const)\s+([A-Za-z_$][A-Za-z0-9_$]*)`)

	for line := range strings.SplitSeq(string(b), "\n") {
		if line != strings.TrimLeft(line, " \t") {
			continue
		}

		if matches := declRE.FindStringSubmatch(line); len(matches) == 3 {
			identifiers[matches[2]] = "variable"
			if matches[1] == "function" {
				identifiers[matches[2]] = "function"
			}
		}
	}

	return identifiers, nil
}

func extractColumnZeroIdentifierCode(filename, identifier string) (string, error) {
	b, err := os.ReadFile(filename)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", filename, err)
	}

	src := string(b)
	declRE := regexp.MustCompile(`^(function|var|let|const)\s+` + regexp.QuoteMeta(identifier) + `(?:\s|\(|=|,|;)`)
	offset := 0

	for _, line := range strings.SplitAfter(src, "\n") {
		if line != strings.TrimLeft(line, " \t") || !declRE.MatchString(line) {
			offset += len(line)
			continue
		}

		if !strings.HasPrefix(line, "function ") {
			return strings.TrimSpace(line), nil
		}

		open := strings.Index(src[offset:], "{")
		if open == -1 {
			return "", fmt.Errorf("function %s has no opening brace", identifier)
		}

		open += offset
		depth := 0
		for i := open; i < len(src); i++ {
			switch src[i] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					return strings.TrimSpace(src[offset : i+1]), nil
				}
			}
		}

		return "", fmt.Errorf("function %s has no closing brace", identifier)
	}

	return "", fmt.Errorf("identifier %s not found", identifier)
}
