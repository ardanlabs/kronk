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
	"slices"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

var selectedModel string

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
		selectedModel = files[0].ID
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

		selectedModel = files[n-1].ID
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

		again, err := askTestAnother()
		if err != nil {
			return fmt.Errorf("askTestAnother: %w", err)
		}

		if !again {
			return nil
		}
	}
}

func askTestAnother() (bool, error) {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("\nTest another function? (y/n): ")

		line, err := reader.ReadString('\n')
		if errors.Is(err, io.EOF) {
			return false, nil
		}

		if err != nil {
			return false, err
		}

		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			fmt.Println("Please enter y or n.")
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
		return a.line - b.line
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
	if len(resp.Choices) != 0 {
		choice := resp.Choices[0]
		if choice.FinishReason() == model.FinishReasonError {
			return fmt.Errorf("error from model: %s", choice.Message.Content)
		}

		content := choice.Message.Content

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

	want := strings.Split(originalCode, "\n")
	got := strings.Split(modelCode, "\n")

	// Exact comparison, whitespace included.
	lcs := lcsLines(want, got)

	// LCS-based similarity ratio (matching lines), not positional chars.
	total := len(want) + len(got)
	percent := 100.0
	if total > 0 {
		percent = float64(2*lcs) / float64(total) * 100
	}
	fmt.Printf("\nCode match: %.2f%%\n", percent)

	if selectedModel != "" {
		fmt.Printf("\nModel: %s\n", selectedModel)
	}

	fmt.Printf("\nCode diff (-want +got):\n%s\n", lineDiff(want, got))
}

// lcsLines returns the length of the longest common subsequence of two
// line slices.
func lcsLines(a, b []string) int {
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)

	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			if a[i-1] == b[j-1] {
				curr[j] = prev[j-1] + 1
			} else {
				curr[j] = max(prev[j], curr[j-1])
			}
		}
		prev, curr = curr, prev
	}

	return prev[len(b)]
}

// lineDiff renders a line-by-line diff. Lines are aligned with an LCS table
// so inserted or removed lines don't shift the rest of the comparison.
// Matching lines use " " context; within a changed block each removed "-"
// line is paired next to its added "+" line.
func lineDiff(a, b []string) string {
	// LCS table for alignment (tolerates inserted/removed lines).
	dp := make([][]int, len(a)+1)
	for i := range dp {
		dp[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else {
				dp[i][j] = max(dp[i+1][j], dp[i][j+1])
			}
		}
	}

	var sb strings.Builder
	var dels, adds []string

	// flush writes a changed block, pairing each removed line with the
	// added line at the same offset so "-" sits next to its "+".
	flush := func() {
		for k := range max(len(dels), len(adds)) {
			if k < len(dels) {
				fmt.Fprintf(&sb, "- %s\n", dels[k])
			}
			if k < len(adds) {
				fmt.Fprintf(&sb, "\033[91m+ %s\033[0m\n", adds[k])
			}
		}
		dels, adds = nil, nil
	}

	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			flush()
			sb.WriteString("  " + a[i] + "\n")
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			dels = append(dels, a[i])
			i++
		default:
			adds = append(adds, b[j])
			j++
		}
	}
	for ; i < len(a); i++ {
		dels = append(dels, a[i])
	}
	for ; j < len(b); j++ {
		adds = append(adds, b[j])
	}
	flush()

	return sb.String()
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
