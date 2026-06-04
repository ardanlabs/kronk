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
	"sort"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("\nERROR: %s\n", err)
		os.Exit(1)
	}
}

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

	// Check if the selected model is already downloaded and valid on disk.
	// The catalog may resolve to a different quantization than what's on disk,
	// so we verify the file actually exists before trusting the catalog lookup.
	mp, err := mdls.FullPath(source)
	if err == nil {
		allExist := true
		for _, f := range mp.ModelFiles {
			if _, statErr := os.Stat(f); statErr != nil {
				allExist = false
				break
			}
		}
		if allExist {
			fmt.Println("Using existing model:", mp.ModelFiles[0])
			return mp, nil
		}
	}

	// Model not found or files missing — download it
	fmt.Println("Downloading model:", source)
	mp, err = mdls.Download(ctx, kronk.FmtLogger, source)
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

	var systemPrompt = `You will be given source code for one identifier from a 
	program. Return the source code in a JavaScript code block.`

	// WRITE SOME CODE / FUNCTION READS THE FILE
	code, err := os.ReadFile(codeFile)
	if err != nil {
		return fmt.Errorf("chat: read code.chunk: %w", err)
	}

	identifiers, err := columnZeroIdentifiers(codeFile)
	if err != nil {
		return fmt.Errorf("chat: index identifiers: %w", err)
	}

	type ident struct {
		name string
		typ  string
		line int
	}

	var idents []ident
	nameWidth := len("Identifier")
	for name, info := range identifiers {
		if info.typ != "function" {
			continue
		}
		idents = append(idents, ident{name, info.typ, info.line})
		if len(name) > nameWidth {
			nameWidth = len(name)
		}
	}
	sort.Slice(idents, func(i, j int) bool { return idents[i].line < idents[j].line })

	fmt.Println("\nAvailable functions:")
	fmt.Printf("  %4s | %-*s | %s\n", "Line", nameWidth, "Identifier", "Type")
	fmt.Printf("  %s-+-%s-+-%s\n", strings.Repeat("-", 4), strings.Repeat("-", nameWidth), strings.Repeat("-", 8))
	for _, id := range idents {
		fmt.Printf("  %4d | %-*s | %s\n", id.line, nameWidth, id.name, id.typ)
	}

	messages = append(messages,
		model.TextMessage(model.RoleSystem, systemPrompt),
		// USER MESSAGE WITH THE CONTEXT OF THE FILE
		model.TextMessage("user", "Here is the code to analyze:\n\n"+string(code)),
	)

	for {
		var err error
		var originalCode string
		messages, originalCode, err = userInput(messages, codeFile, identifiers)
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

			messages, err = modelResponse(krn, messages, ch, originalCode)

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

func userInput(messages []model.D, codeFile string, identifiers map[string]identInfo) ([]model.D, string, error) {
	fmt.Print("\nUSER> ")

	reader := bufio.NewReader(os.Stdin)

	userInput, err := reader.ReadString('\n')

	if err != nil {
		return messages, "", fmt.Errorf("unable to read user input: %w", err)
	}

	if strings.TrimSpace(userInput) == "quit" || userInput == "quit\n" {
		return nil, "", io.EOF
	}

	var originalCode string

	identifier := strings.TrimSpace(userInput)
	info, exists := identifiers[identifier]
	if !exists {
		for _, token := range regexp.MustCompile(`[A-Za-z_$][A-Za-z0-9_$]*`).FindAllString(userInput, -1) {
			if info, exists = identifiers[token]; exists {
				identifier = token
				break
			}
		}
	}

	if exists {
		fmt.Printf("\033[90mFound %s %q at %s:%d\033[0m\n", info.typ, identifier, codeFile, info.line)

		code, err := extractColumnZeroIdentifierCode(codeFile, identifier)
		if err != nil {
			return messages, "", err
		}

		originalCode = code

		// Don't include the source in the prompt — rely on the full file
		// loaded as context at startup so this tests the model's recall.
		userInput = fmt.Sprintf(`Return the full body code for this %s named %s.`, info.typ, identifier)
	}

	messages = append(messages,
		model.TextMessage(model.RoleUser, userInput),
	)

	return messages, originalCode, nil
}

func performChat(ctx context.Context, krn *kronk.Kronk, d model.D) (model.ChatResponse, error) {
	ch, err := krn.Chat(ctx, d)

	if err != nil {
		return model.ChatResponse{}, fmt.Errorf("chat streaming: %w", err)
	}

	return ch, nil
}

func modelResponse(krn *kronk.Kronk, messages []model.D, resp model.ChatResponse, originalCode string) ([]model.D, error) {
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

	if originalCode != "" {
		modelCode := strings.TrimSpace(firstCodeBlock(content))
		percent := codeMatchPercent(modelCode, originalCode)

		fmt.Printf("\nCode match: %.2f%%\n", percent)

		if diff := firstCodeDifference(modelCode, originalCode); diff != -1 {
			line := 1 + strings.Count(normalizeIndent(originalCode)[:diff], "\n")
			fmt.Printf("First difference at line: %d\n", line)
			printCodeDiff(originalCode, modelCode)
		}
	}

	reasoning := resp.Choices[0].Message.Reasoning

	if reasoning != "" {
		fmt.Printf("\033[91m%s\033[0m", reasoning)
	}

	fmt.Printf("\n\033[90mTokens: %d input, %d output | TPS: %.2f\033[0m\n",
		resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TokensPerSecond)

	return messages, nil
}

func firstCodeBlock(content string) string {
	start := strings.Index(content, "```")
	if start == -1 {
		return ""
	}

	content = content[start+3:]
	if newline := strings.Index(content, "\n"); newline != -1 {
		content = content[newline+1:]
	}

	before, _, ok := strings.Cut(content, "```")
	if !ok {
		return content
	}

	return before
}

func normalizeIndent(s string) string {
	var result []rune
	for _, r := range s {
		if r == '\t' {
			result = append(result, ' ', ' ', ' ', ' ')
		} else {
			result = append(result, r)
		}
	}
	return string(result)
}

func codeMatchPercent(modelCode, originalCode string) float64 {
	modelCode = normalizeIndent(modelCode)
	originalCode = normalizeIndent(originalCode)

	maxLen := max(len(modelCode), len(originalCode))
	if maxLen == 0 {
		return 100
	}

	var matches int
	for i := range maxLen {
		if i < len(modelCode) && i < len(originalCode) && modelCode[i] == originalCode[i] {
			matches++
		}
	}

	return float64(matches) / float64(maxLen) * 100
}

func firstCodeDifference(modelCode, originalCode string) int {
	modelCode = normalizeIndent(modelCode)
	originalCode = normalizeIndent(originalCode)

	maxLen := max(len(modelCode), len(originalCode))
	for i := range maxLen {
		if i >= len(modelCode) || i >= len(originalCode) || modelCode[i] != originalCode[i] {
			return i
		}
	}

	return -1
}

func printCodeDiff(originalCode, modelCode string) {
	modelCode = normalizeIndent(modelCode)
	originalCode = normalizeIndent(originalCode)

	origLines := strings.Split(originalCode, "\n")
	modelLines := strings.Split(modelCode, "\n")

	diffByte := firstCodeDifference(modelCode, originalCode)

	lineIdx := 0
	accum := 0
	for i, line := range origLines {
		lineWithNewline := len(line) + 1
		if accum+lineWithNewline > diffByte {
			lineIdx = i
			break
		}
		accum += lineWithNewline
	}

	if lineIdx >= len(origLines) {
		lineIdx = len(origLines) - 1
	}

	fmt.Println("\n--- Code Diff ---")
	fmt.Printf("%-45s | %s\n", "ORIGINAL", "MODEL")
	fmt.Println(strings.Repeat("-", 80))

	for i := 0; i < max(len(origLines), len(modelLines)); i++ {
		origLine := ""
		if i < len(origLines) {
			origLine = origLines[i]
		}
		modelLine := ""
		if i < len(modelLines) {
			modelLine = modelLines[i]
		}

		if i == lineIdx {
			fmt.Printf("  %-43s | %s\n", origLine, modelLine)
		} else {
			fmt.Printf("  %-43s | %s\n", origLine, modelLine)
		}
	}
	fmt.Println("--- End Diff ---")
}

type identInfo struct {
	typ  string
	line int
}

func columnZeroIdentifiers(filename string) (map[string]identInfo, error) {
	b, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filename, err)
	}

	identifiers := make(map[string]identInfo)
	declRE := regexp.MustCompile(`^(function|var|let|const)\s+([A-Za-z_$][A-Za-z0-9_$]*)`)

	lineNum := 0
	for line := range strings.SplitSeq(string(b), "\n") {
		lineNum++

		if line != strings.TrimLeft(line, " \t") {
			continue
		}

		if matches := declRE.FindStringSubmatch(line); len(matches) == 3 {
			info := identInfo{typ: "variable", line: lineNum}
			if matches[1] == "function" {
				info.typ = "function"
			}
			identifiers[matches[2]] = info
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
