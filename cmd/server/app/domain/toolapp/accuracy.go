package toolapp

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// accuracyCode is the fixed source the Accuracy app tests against. The
// code.chunk file is checked in alongside this package and embedded into the
// binary so the feature works with no upload or external file.
//
//go:embed code.chunk
var accuracyCode []byte

// accuracyContextWindow is the context window used when loading the model for
// a test. The whole source file is sent as context (~33k tokens), so the model
// must be loaded with a large window using WithContextWindow(131072). Without
// this, a model resolved with the default 8K window overflows and the
// inference call fails.
const accuracyContextWindow = 131072

// =============================================================================
// Requests
//
// The Accuracy app drives a small interactive flow in the BUI:
//
//  1. Pick a model     (served by GET /v1/kronk/models).
//  2. Pick a function  (GET /v1/accuracy/functions — from the fixed source).
//  3. Compare recall   (POST /v1/accuracy/test with model + function).

// AccuracyRequest is the body for asking a model to recall a single
// function and comparing the result against the fixed source.
type AccuracyRequest struct {
	Model    string `json:"model"`
	Function string `json:"function"`
}

// =============================================================================
// Responses

// AccuracyFunction describes a single top-level function declaration. Loc is
// the number of lines in the function's source.
type AccuracyFunction struct {
	Num        int    `json:"num"`
	Line       int    `json:"line"`
	Loc        int    `json:"loc"`
	Identifier string `json:"identifier"`
}

// AccuracyFunctionsResponse is the list of functions found in the source.
type AccuracyFunctionsResponse struct {
	Object string             `json:"object"`
	Data   []AccuracyFunction `json:"data"`
}

// Encode implements the web.Encoder interface.
func (r AccuracyFunctionsResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(r)
	return data, "application/json", err
}

// AccuracyDiffLine is a single line of a code diff. Op is one of "context",
// "del" (present in got, missing from want) or "add" (present in want,
// missing from got) so the UI can colorize the output.
type AccuracyDiffLine struct {
	Op   string `json:"op"`
	Text string `json:"text"`
}

// AccuracyUsage reports token accounting for a single test run.
type AccuracyUsage struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TokensPerSecond  float64 `json:"tokens_per_second"`
}

// AccuracyResponse is the result of asking a model to recall a function.
type AccuracyResponse struct {
	Model        string             `json:"model"`
	Function     string             `json:"function"`
	Line         int                `json:"line"`
	MatchPercent float64            `json:"match_percent"`
	Want         string             `json:"want"`
	Got          string             `json:"got"`
	Diff         []AccuracyDiffLine `json:"diff"`
	Usage        AccuracyUsage      `json:"usage"`
}

// Encode implements the web.Encoder interface.
func (r AccuracyResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(r)
	return data, "application/json", err
}

// =============================================================================
// Handlers

// listAccuracyFunctions returns the top-level function declarations found in
// the fixed source code, ordered by their line number.
func (a *app) listAccuracyFunctions(ctx context.Context, r *http.Request) web.Encoder {
	identifiers := retrieveIdentifiers(accuracyCode)
	idents := sortIdentifiers(identifiers)

	resp := AccuracyFunctionsResponse{
		Object: "list",
		Data:   make([]AccuracyFunction, 0, len(idents)),
	}

	for i, id := range idents {
		loc := 0
		if block, err := extractIdentifierCode(accuracyCode, id.name); err == nil {
			loc = strings.Count(block, "\n") + 1
		}

		resp.Data = append(resp.Data, AccuracyFunction{
			Num:        i + 1,
			Line:       id.line,
			Loc:        loc,
			Identifier: id.name,
		})
	}

	return resp
}

// runAccuracy asks the chosen model to recall a single function from
// memory and compares the answer against the source code, returning a match
// percentage and a line-by-line diff.
func (a *app) runAccuracy(ctx context.Context, r *http.Request) web.Encoder {
	var req AccuracyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	switch {
	case strings.TrimSpace(req.Model) == "":
		return errs.Errorf(errs.InvalidArgument, "missing model field")
	case strings.TrimSpace(req.Function) == "":
		return errs.Errorf(errs.InvalidArgument, "missing function field")
	}

	code := accuracyCode

	identifiers := retrieveIdentifiers(code)
	info, exists := identifiers[req.Function]
	if !exists || info.typ != "function" {
		return errs.Errorf(errs.InvalidArgument, "function %q not found", req.Function)
	}

	codeBlock, err := extractIdentifierCode(code, req.Function)
	if err != nil {
		return errs.Errorf(errs.Internal, "unable to extract function: %s", err)
	}

	// Load the model with a large context window and incremental caching
	// disabled. Resolve the model's normal config, override those two settings,
	// and acquire a dedicated instance.
	cfg, err := a.models.KronkResolvedConfig(req.Model, a.pool.Kronk.ModelConfig())
	if err != nil {
		return errs.New(errs.InvalidArgument, fmt.Errorf("resolving model config: %w", err))
	}

	cw := accuracyContextWindow
	imc := false
	cfg.PtrContextWindow = &cw
	cfg.PtrIncrementalCache = &imc

	krn, err := a.pool.Kronk.AquireCustom(ctx, req.Model+"/accuracy", cfg)
	if err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// Don't include the source in the prompt for the question itself; rely on
	// the full file provided as context so this tests the model's recall.
	userInput := fmt.Sprintf("Return the full body code for this %s named %s.", info.typ, req.Function)

	messages := append(createInitialMessages(code),
		model.TextMessage(model.RoleUser, userInput),
	)

	d := model.D{
		"messages":        messages,
		"max_tokens":      4096,
		"enable_thinking": false,
	}

	resp, err := krn.Chat(ctx, d)
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	if len(resp.Choices) == 0 || resp.Choices[0].Message == nil {
		return errs.Errorf(errs.Internal, "model returned no choices")
	}

	choice := resp.Choices[0]
	if choice.FinishReason() == model.FinishReasonError {
		return errs.Errorf(errs.Internal, "error from model: %s", choice.Message.Content)
	}

	modelCode := strings.TrimSpace(parseCodeBlock(choice.Message.Content))

	want := strings.Split(codeBlock, "\n")
	got := strings.Split(modelCode, "\n")

	out := AccuracyResponse{
		Model:        req.Model,
		Function:     req.Function,
		Line:         info.line,
		MatchPercent: matchPercent(want, got),
		Want:         codeBlock,
		Got:          modelCode,
		Diff:         lineDiff(got, want),
	}

	if resp.Usage != nil {
		out.Usage = AccuracyUsage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TokensPerSecond:  resp.Usage.TokensPerSecond,
		}
	}

	return out
}

// =============================================================================
// Source indexing and code extraction

type accuracyIdentInfo struct {
	typ  string
	line int
}

type accuracyIdent struct {
	name string
	typ  string
	line int
}

var accuracyDeclRE = regexp.MustCompile(`^(function|var|let|const)\s+([A-Za-z_$][A-Za-z0-9_$]*)`)

// retrieveIdentifiers indexes every top-level (non-indented) declaration in
// the source, mapping its name to its kind and line number.
func retrieveIdentifiers(code []byte) map[string]accuracyIdentInfo {
	identifiers := make(map[string]accuracyIdentInfo)

	lineNum := 0
	for line := range strings.SplitSeq(string(code), "\n") {
		lineNum++

		if line != strings.TrimLeft(line, " \t") {
			continue
		}

		if matches := accuracyDeclRE.FindStringSubmatch(line); len(matches) == 3 {
			info := accuracyIdentInfo{typ: "variable", line: lineNum}
			if matches[1] == "function" {
				info.typ = "function"
			}
			identifiers[matches[2]] = info
		}
	}

	return identifiers
}

// sortIdentifiers returns the function identifiers ordered by source line.
func sortIdentifiers(identifiers map[string]accuracyIdentInfo) []accuracyIdent {
	var idents []accuracyIdent
	for name, info := range identifiers {
		if info.typ != "function" {
			continue
		}
		idents = append(idents, accuracyIdent{name, info.typ, info.line})
	}

	slices.SortFunc(idents, func(a, b accuracyIdent) int {
		return a.line - b.line
	})

	return idents
}

// extractIdentifierCode returns the full source text for a single
// declaration, brace-matching function bodies.
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

// =============================================================================
// Prompting and diffing

func createInitialMessages(code []byte) []model.D {
	const systemPrompt = `You will be given source code for one identifier from a 
	program. Return the source code in a JavaScript code block.`

	return append(model.DocumentArray(),
		model.TextMessage(model.RoleSystem, systemPrompt),
		model.TextMessage(model.RoleUser, "Here is the code to analyze:\n\n"+string(code)),
	)
}

// parseCodeBlock extracts the contents of the first fenced code block.
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

// matchPercent reports an LCS-based similarity ratio (matching lines) between
// the wanted and received line slices.
func matchPercent(want, got []string) float64 {
	lcs := lcsLines(want, got)

	total := len(want) + len(got)
	if total == 0 {
		return 100.0
	}

	return float64(2*lcs) / float64(total) * 100
}

// lcsLines returns the length of the longest common subsequence of two line
// slices.
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

// lineDiff renders a line-by-line diff. Lines are aligned with an LCS table so
// inserted or removed lines don't shift the rest of the comparison. Matching
// lines are "context"; within a changed block each removed "del" line is
// paired next to its added "add" line.
func lineDiff(a, b []string) []AccuracyDiffLine {
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

	var out []AccuracyDiffLine
	var dels, adds []string

	// flush writes a changed block, pairing each removed line with the added
	// line at the same offset so "del" sits next to its "add".
	flush := func() {
		for k := range max(len(dels), len(adds)) {
			if k < len(dels) {
				out = append(out, AccuracyDiffLine{Op: "del", Text: dels[k]})
			}
			if k < len(adds) {
				out = append(out, AccuracyDiffLine{Op: "add", Text: adds[k]})
			}
		}
		dels, adds = nil, nil
	}

	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			flush()
			out = append(out, AccuracyDiffLine{Op: "context", Text: a[i]})
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

	return out
}
