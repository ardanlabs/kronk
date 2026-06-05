//

package main

import (
	"bufio"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

type identInfo struct {
	typ  string
	line int
}

// =============================================================================

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

	// -------------------------------------------------------------------------

	libMgr, err := libs.New(
		libs.WithVersion(defaults.LibVersion("")),
	)
	if err != nil {
		return models.Path{}, err
	}

	if _, err := libMgr.Download(ctx, kronk.FmtLogger); err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	// -------------------------------------------------------------------------

	mdls, err := models.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to init models: %w", err)
	}

	modelID, err := getModelID()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to get model id: %w", err)
	}

	mp, err := mdls.FullPath(modelID)
	if err != nil {
		fmt.Println("Downloading model:", modelID)
		mp, err = mdls.Download(ctx, kronk.FmtLogger, modelID)
		if err != nil {
			return models.Path{}, fmt.Errorf("unable to install model: %w", err)
		}

		return mp, nil
	}

	fmt.Println("Using existing model:", mp.ModelFiles[0])
	return mp, nil
}

func getModelID() (string, error) {
	mdls, err := models.New()
	if err != nil {
		return "", fmt.Errorf("models.new: %w", err)
	}

	files, err := mdls.Files()
	if err != nil || len(files) == 0 {
		return "", fmt.Errorf("mdls.files: %w", err)
	}

	// If only one model, use it
	if len(files) == 1 {
		return files[0].ID, nil
	}

	// Multiple models — let user choose
	fmt.Println("\nAvailable models:")
	for i, f := range files {
		fmt.Printf("  %d: %s\n", i+1, f.ID)
	}

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Printf("Select a model (1-%d): ", len(files))

		// Scanning into an int forces numeric input; any non-numeric
		// token returns an error.
		var n int
		_, err := fmt.Fscan(reader, &n)

		// Discard the rest of the line so the next scan starts clean.
		if _, derr := reader.ReadString('\n'); derr != nil && err == nil {
			err = derr
		}

		if errors.Is(err, io.EOF) {
			return "", io.EOF
		}

		if err != nil {
			fmt.Println("Please enter a number.")
			continue
		}

		if n < 1 || n > len(files) {
			fmt.Printf("Please enter a number between 1 and %d.\n", len(files))
			continue
		}

		return files[n-1].ID, nil
	}
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
	const codeFile = "kaleah/code.chunk"
	identifiers, code, err := retrieveIdentifiers(codeFile)
	if err != nil {
		return fmt.Errorf("chat: index identifiers: %w", err)
	}

	for {
		identifier, err := selectIdentifier(identifiers)
		if err != nil {
			return fmt.Errorf("run:user input: %w", err)
		}

		codeBlock, err := extractIdentifierCode(code, identifier)
		if err != nil {
			return fmt.Errorf("extractIdentifierCode: %w", err)
		}

		info := identifiers[identifier]
		fmt.Printf("\033[90mFound %s %q at %s:%d\033[0m\n", info.typ, identifier, codeFile, info.line)

		// Don't include the source in the prompt — rely on the full file
		// loaded as context at startup so this tests the model's recall.
		userInput := fmt.Sprintf(`Return the full body code for this %s named %s.`, info.typ, identifier)

		messages := createInitialMessages(code)
		messages = append(messages,
			model.TextMessage(model.RoleUser, userInput),
		)

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		err = generateResponse(ctx, krn, messages, codeBlock)
		if err != nil {
			return fmt.Errorf("generateResponse: %w", err)

		}
	}
}

func retrieveIdentifiers(codeFile string) (map[string]identInfo, []byte, error) {
	code, err := os.ReadFile(codeFile)
	if err != nil {
		return nil, nil, fmt.Errorf("chat: read code.chunk: %w", err)
	}

	identifiers := make(map[string]identInfo)
	declRE := regexp.MustCompile(`^(function|var|let|const)\s+([A-Za-z_$][A-Za-z0-9_$]*)`)

	lineNum := 0
	for line := range strings.SplitSeq(string(code), "\n") {
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

	return identifiers, code, nil
}

func selectIdentifier(identifiers map[string]identInfo) (string, error) {
	idents := sortIdentifiers(identifiers)

	printIdentifiers(identifiers)

	reader := bufio.NewReader(os.Stdin)

	var identifier string
	for {
		fmt.Printf("Select a function (1-%d, 0 to quit): ", len(idents))

		// Scanning into an int forces numeric input; any non-numeric
		// token returns an error.
		var n int
		_, err := fmt.Fscan(reader, &n)

		// Discard the rest of the line so the next scan starts clean.
		if _, derr := reader.ReadString('\n'); derr != nil && err == nil {
			err = derr
		}

		if errors.Is(err, io.EOF) {
			return "", io.EOF
		}

		if err != nil {
			fmt.Println("Please enter a number.")
			continue
		}

		if n == 0 {
			return "", io.EOF
		}

		if n < 1 || n > len(idents) {
			fmt.Printf("Please enter a number between 1 and %d.\n", len(idents))
			continue
		}

		identifier = idents[n-1].name
		break
	}

	return identifier, nil
}

func printIdentifiers(identifiers map[string]identInfo) {
	idents := sortIdentifiers(identifiers)

	var identLabelWidth int
	for _, id := range idents {
		if len(id.name) > identLabelWidth {
			identLabelWidth = len(id.name)
		}
	}

	fmt.Println("\nWhich Function Do We Test?")
	fmt.Printf("  %4s | %4s | %-*s\n", "Num", "Line", identLabelWidth, "Identifier")
	fmt.Printf("  %s-+-%s-+-%s-\n", strings.Repeat("-", 4), strings.Repeat("-", 4), strings.Repeat("-", identLabelWidth))

	for i, id := range idents {
		fmt.Printf("  %4d | %4d | %-*s\n", i+1, id.line, identLabelWidth, id.name)
	}
}

type ident struct {
	name string
	typ  string
	line int
}

// sortIdentifiers returns the function identifiers ordered by source line.
func sortIdentifiers(identifiers map[string]identInfo) []ident {
	var idents []ident
	for name, info := range identifiers {
		if info.typ != "function" {
			continue
		}
		idents = append(idents, ident{name, info.typ, info.line})
	}

	slices.SortFunc(idents, func(a, b ident) int {
		return cmp.Compare(a.line, b.line)
	})

	return idents
}

func extractIdentifierCode(code []byte, identifier string) (string, error) {
	src := string(code)
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

func createInitialMessages(code []byte) []model.D {
	var systemPrompt = `You will be given source code for one identifier from a 
	program. Return the source code in a JavaScript code block.`

	return append(model.DocumentArray(),
		model.TextMessage(model.RoleSystem, systemPrompt),
		model.TextMessage("user", "Here is the code to analyze:\n\n"+string(code)),
	)
}

func generateResponse(ctx context.Context, krn *kronk.Kronk, messages []model.D, codeBlock string) error {
	fmt.Print("\nModel is loading... ")
	d := model.D{
		"messages":        messages,
		"max_tokens":      4096,
		"enable_thinking": false,
	}

	ch, err := krn.Chat(ctx, d)
	if err != nil {
		return fmt.Errorf("chat: %w", err)
	}

	return modelResponse(ch, codeBlock)
}

func modelResponse(resp model.ChatResponse, codeBlock string) error {
	fmt.Print("\nMODEL> ")

	if len(resp.Choices) != 0 {
		choice := resp.Choices[0]
		if choice.FinishReason() == model.FinishReasonError {
			return fmt.Errorf("error from model: %s", choice.Message.Content)
		}

		content := choice.Message.Content
		if content != "" {
			fmt.Print(content)
		}

		if codeBlock != "" {
			compareCode(content, codeBlock)
		}

		if choice.Message.Reasoning != "" {
			fmt.Printf("\033[91m%s\033[0m", choice.Message.Reasoning)
		}

		fmt.Printf("\n\033[90mTokens: %d input, %d output | TPS: %.2f\033[0m\n",
			resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TokensPerSecond)
	}
	return nil
}

func compareCode(content, originalCode string) {
	modelCode := strings.TrimSpace(parseCodeBlock(content))

	percent := calculateMatchPercent(modelCode, originalCode)

	fmt.Printf("\nCode match: %.2f%%\n", percent)

	if diff := firstCodeDifference(modelCode, originalCode); diff != -1 {
		line := 1 + strings.Count(normalizeIndent(originalCode)[:diff], "\n")
		fmt.Printf("First difference at line: %d\n", line)

		printCodeDiff(originalCode, modelCode)
	}
}

func parseCodeBlock(content string) string {
	start := strings.Index(content, "```")
	if start == -1 {
		return ""
	}

	content = content[start+3:]
	if newline := strings.Index(content, "\n"); newline != -1 {
		content = content[newline+1:]
	}

	parts := strings.SplitN(content, "```", 2)
	if len(parts) < 2 {
		return content
	}

	return parts[0]
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

func calculateMatchPercent(modelCode, originalCode string) float64 {
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

	fmt.Println("\n--- Code Diff ---")
	fmt.Printf("%-45s | %s\n", "ORIGINAL", "MODEL")
	fmt.Println(strings.Repeat("-", 80))

	const w = 43
	for i := 0; i < max(len(origLines), len(modelLines)); i++ {
		origLine := ""
		if i < len(origLines) {
			origLine = origLines[i]
		}

		modelLine := ""
		if i < len(modelLines) {
			modelLine = modelLines[i]
		}

		// Wrap long lines at the column width so neither side pushes
		// into the other column.
		for len(origLine) > 0 || len(modelLine) > 0 {
			origChunk := origLine
			if len(origChunk) > w {
				origChunk, origLine = origChunk[:w], origChunk[w:]
			} else {
				origLine = ""
			}

			modelChunk := modelLine
			if len(modelChunk) > w {
				modelChunk, modelLine = modelChunk[:w], modelChunk[w:]
			} else {
				modelLine = ""
			}

			fmt.Printf("  %-*s | %s\n", w, origChunk, modelChunk)
		}
	}

	fmt.Println("--- End Diff ---")
}
