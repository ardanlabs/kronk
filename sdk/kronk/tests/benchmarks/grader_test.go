package benchmarks_test

import (
	"context"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const graderTimeout = 20 * time.Second

type gradeMode string

const (
	gradeBase gradeMode = "base"
	gradeUndo gradeMode = "undo"
)

type gradeCheck struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail,omitempty"`
}

type gradeResult struct {
	Passed int          `json:"passed"`
	Total  int          `json:"total"`
	Checks []gradeCheck `json:"checks"`
}

type scenario struct {
	name    string
	input   string
	want    []string
	count   map[string]int
	ordered []string
}

func (gr gradeResult) Percentage() float64 {
	if gr.Total == 0 {
		return 0
	}
	return 100 * float64(gr.Passed) / float64(gr.Total)
}

func (gr gradeResult) Failures(turn string) []string {
	var failures []string
	for _, check := range gr.Checks {
		if !check.Passed {
			failures = append(failures, fmt.Sprintf("%s/%s: %s", turn, check.Name, check.Detail))
		}
	}
	return failures
}

func (gr *gradeResult) add(name string, passed bool, detail string) {
	if passed {
		gr.Passed++
	}
	gr.Checks = append(gr.Checks, gradeCheck{Name: name, Passed: passed, Detail: detail})
}

func extractGoSource(response string) string {
	response = strings.TrimSpace(response)

	for _, marker := range []string{"```go", "```golang", "```"} {
		for remainder := response; ; {
			start := strings.Index(remainder, marker)
			if start < 0 {
				break
			}
			body := remainder[start+len(marker):]
			before, after, ok := strings.Cut(body, "```")
			if !ok {
				break
			}
			candidate := strings.TrimSpace(before)
			if strings.HasPrefix(candidate, "package main") {
				return candidate + "\n"
			}
			remainder = after
		}
	}

	if start := strings.Index(response, "package main"); start >= 0 {
		return strings.TrimSpace(response[start:]) + "\n"
	}
	return ""
}

func gradeProgram(parent context.Context, source string, mode gradeMode) gradeResult {
	result := gradeResult{Total: 19}
	if mode == gradeUndo {
		result.Total = 24
	}
	result.add("source-extracted", source != "", "no complete package main source found")
	if source == "" {
		return result
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", source, parser.AllErrors)
	result.add("go-parse", err == nil, errorDetail(err))
	if err != nil {
		return result
	}

	formatted, err := format.Source([]byte(source))
	result.add("gofmt", err == nil, errorDetail(err))
	if err != nil {
		return result
	}
	source = string(formatted)

	checks := inspectStructure(file, mode)
	for _, check := range checks {
		result.add(check.Name, check.Passed, check.Detail)
	}

	dir, err := prepareProgram(source)
	result.add("prepare-build", err == nil, errorDetail(err))
	if err != nil {
		return result
	}
	defer os.RemoveAll(dir)

	ctx, cancel := context.WithTimeout(parent, graderTimeout)
	defer cancel()

	buildOutput, err := runCommand(ctx, dir, "go", "build", "-o", "tictactoe", ".")
	result.add("go-build", err == nil, commandDetail(ctx, err, buildOutput))
	if err != nil {
		return result
	}

	vetOutput, err := runCommand(ctx, dir, "go", "vet", "./...")
	result.add("go-vet", err == nil, commandDetail(ctx, err, vetOutput))

	if err := os.WriteFile(filepath.Join(dir, "main_test.go"), []byte(graderUnitTests), 0644); err != nil {
		result.add("logic-tests", false, err.Error())
	} else {
		testOutput, testErr := runCommand(ctx, dir, "go", "test", "-run", "TestGenerated", "-count=1")
		result.add("logic-tests", testErr == nil, commandDetail(ctx, testErr, testOutput))
	}

	for _, scenario := range scenarios(mode) {
		output, runErr := runProgram(parent, filepath.Join(dir, "tictactoe"), scenario.input)
		passed, detail := assessScenario(output, runErr, scenario)
		result.add("scenario-"+scenario.name, passed, detail)
	}

	return result
}

func inspectStructure(file *ast.File, mode gradeMode) []gradeCheck {
	checks := []gradeCheck{
		{Name: "type-board", Detail: "want type Board [9]byte"},
		{Name: "shared-stdin", Detail: "want package-level stdin initialized with bufio.NewReader(os.Stdin)"},
	}
	required := map[string]string{
		"renderBoard": "func renderBoard(b *Board, xWins, oWins, draws int)",
		"hasWinner":   "func hasWinner(b *Board) bool",
		"boardFull":   "func boardFull(b *Board) bool",
		"playerX":     "func playerX(b *Board) int",
		"playerO":     "func playerO(b *Board) int",
	}
	for name, signature := range required {
		checks = append(checks, gradeCheck{Name: "function-" + name, Detail: "want " + signature})
	}
	if mode == gradeUndo {
		checks = append(checks, gradeCheck{Name: "one-move-state", Detail: "want lastMove initialized to -1"})
	}

	ast.Inspect(file, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.TypeSpec:
			array, ok := node.Type.(*ast.ArrayType)
			if node.Name.Name == "Board" && ok && integerLiteral(array.Len, "9") && identifier(array.Elt, "byte") {
				passCheck(checks, "type-board")
			}
		case *ast.ValueSpec:
			if len(node.Names) == 1 && node.Names[0].Name == "stdin" && len(node.Values) == 1 && isStdinReader(node.Values[0]) {
				passCheck(checks, "shared-stdin")
			}
		case *ast.FuncDecl:
			if signature, exists := required[node.Name.Name]; exists && exactFunctionSignature(node, node.Name.Name) {
				passCheck(checks, "function-"+node.Name.Name)
				_ = signature
			}
		case *ast.AssignStmt:
			if mode == gradeUndo && node.Tok == token.DEFINE && len(node.Lhs) == 1 && len(node.Rhs) == 1 && identifier(node.Lhs[0], "lastMove") && negativeOne(node.Rhs[0]) {
				passCheck(checks, "one-move-state")
			}
		}
		return true
	})

	return checks
}

func exactFunctionSignature(fn *ast.FuncDecl, name string) bool {
	params := flattenFields(fn.Type.Params)
	results := flattenFields(fn.Type.Results)

	switch name {
	case "renderBoard":
		return len(params) == 4 && pointerTo(params[0], "Board") && identifier(params[1], "int") && identifier(params[2], "int") && identifier(params[3], "int") && len(results) == 0
	case "hasWinner", "boardFull":
		return len(params) == 1 && pointerTo(params[0], "Board") && len(results) == 1 && identifier(results[0], "bool")
	case "playerX", "playerO":
		return len(params) == 1 && pointerTo(params[0], "Board") && len(results) == 1 && identifier(results[0], "int")
	}
	return false
}

func flattenFields(fields *ast.FieldList) []ast.Expr {
	if fields == nil {
		return nil
	}
	var expressions []ast.Expr
	for _, field := range fields.List {
		count := max(len(field.Names), 1)
		for range count {
			expressions = append(expressions, field.Type)
		}
	}
	return expressions
}

func isStdinReader(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	fun, ok := call.Fun.(*ast.SelectorExpr)
	arg, argOK := call.Args[0].(*ast.SelectorExpr)
	return ok && argOK && identifier(fun.X, "bufio") && fun.Sel.Name == "NewReader" && identifier(arg.X, "os") && arg.Sel.Name == "Stdin"
}

func pointerTo(expr ast.Expr, name string) bool {
	star, ok := expr.(*ast.StarExpr)
	return ok && identifier(star.X, name)
}

func identifier(expr ast.Expr, name string) bool {
	id, ok := expr.(*ast.Ident)
	return ok && id.Name == name
}

func integerLiteral(expr ast.Expr, value string) bool {
	literal, ok := expr.(*ast.BasicLit)
	return ok && literal.Kind == token.INT && literal.Value == value
}

func negativeOne(expr ast.Expr) bool {
	unary, ok := expr.(*ast.UnaryExpr)
	return ok && unary.Op == token.SUB && integerLiteral(unary.X, "1")
}

func passCheck(checks []gradeCheck, name string) {
	for idx := range checks {
		if checks[idx].Name == name {
			checks[idx].Passed = true
			checks[idx].Detail = ""
		}
	}
}

func prepareProgram(source string) (string, error) {
	dir, err := os.MkdirTemp("", "kronk-codegen-grade-")
	if err != nil {
		return "", fmt.Errorf("creating grader directory: %w", err)
	}

	files := map[string]string{
		"go.mod":  "module tictactoe\n\ngo 1.24\n",
		"main.go": source,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			os.RemoveAll(dir)
			return "", fmt.Errorf("writing %s: %w", name, err)
		}
	}
	return dir, nil
}

func runCommand(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func runProgram(parent context.Context, binary, input string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, graderTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary)
	cmd.Stdin = strings.NewReader(input)
	cmd.Env = append(os.Environ(), "TERM=dumb")
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return string(output), ctx.Err()
	}
	return string(output), err
}

func scenarios(mode gradeMode) []scenario {
	initialBoard := "Score: X: 0 | O: 0 | Draws: 0\n\n 1 | 2 | 3\n-----------\n 4 | 5 | 6\n-----------\n 7 | 8 | 9\n\n"
	base := []scenario{
		{
			name:  "x-win",
			input: "1\n4\n2\n5\n3\nn\n",
			want:  []string{initialBoard, "Score: X: 1 | O: 0 | Draws: 0", "Player X wins!", "Play again? (y/n): "},
			ordered: []string{
				" X | X | X",
				"Player X wins!",
				"Play again? (y/n): ",
			},
		},
		{
			name:  "o-win",
			input: "1\n2\n3\n5\n4\n8\nn\n",
			want:  []string{"Score: X: 0 | O: 1 | Draws: 0", "Player O wins!"},
		},
		{
			name:  "draw",
			input: "1\n2\n3\n5\n4\n6\n8\n7\n9\nn\n",
			want:  []string{"Score: X: 0 | O: 0 | Draws: 1", "It's a draw."},
		},
		{
			name:  "invalid-input",
			input: "0\nword\n1\n1\n4\n2\n5\n3\nn\n",
			count: map[string]int{"Invalid move. Enter an empty position from 1 to 9.": 3},
		},
		{
			name:  "replay-score",
			input: "1\n4\n2\n5\n3\ny\n1\n2\n3\n5\n4\n8\nn\n",
			want:  []string{"Score: X: 1 | O: 1 | Draws: 0"},
		},
	}
	if mode == gradeBase {
		return base
	}

	return append(base,
		scenario{
			name:  "undo-x",
			input: "5\n0\n1\n2\n4\n3\n7\nn\n",
			want: []string{
				"Player O's turn. Enter a number (1-9), or 0 to undo the last move:",
				"Player X's turn. Enter a number (1-9), or 0 to undo the last move:",
				"Player X wins!",
			},
			ordered: []string{" 4 | X | 6", " 4 | 5 | 6"},
		},
		scenario{
			name:    "undo-o",
			input:   "1\n2\n0\n5\n2\n9\n3\nn\n",
			want:    []string{"Player X wins!"},
			ordered: []string{" X | O | 3", " X | 2 | 3", " X | X | X"},
		},
		scenario{
			name:  "undo-unavailable",
			input: "0\n1\n2\n4\n3\n7\n5\nn\n",
			count: map[string]int{"Invalid move. Enter an empty position from 1 to 9.": 1},
		},
		scenario{
			name:  "undo-twice",
			input: "5\n0\n0\n1\n2\n4\n3\n7\nn\n",
			count: map[string]int{"Invalid move. Enter an empty position from 1 to 9.": 1},
			want:  []string{"Player X wins!"},
		},
	)
}

func assessScenario(output string, runErr error, scenario scenario) (bool, string) {
	clean := stripANSI(output)
	if runErr != nil {
		return false, fmt.Sprintf("run failed: %v; output: %s", runErr, truncate(clean, 600))
	}
	for _, want := range scenario.want {
		if !strings.Contains(clean, want) {
			return false, fmt.Sprintf("missing %q; output: %s", want, truncate(clean, 600))
		}
	}
	for text, count := range scenario.count {
		if got := strings.Count(clean, text); got != count {
			return false, fmt.Sprintf("%q count: got %d, want %d; output: %s", text, got, count, truncate(clean, 600))
		}
	}
	position := 0
	for _, want := range scenario.ordered {
		idx := strings.Index(clean[position:], want)
		if idx < 0 {
			return false, fmt.Sprintf("ordered output missing %q; output: %s", want, truncate(clean, 600))
		}
		position += idx + len(want)
	}
	return true, ""
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

func stripANSI(value string) string {
	return ansiPattern.ReplaceAllString(value, "")
}

func commandDetail(ctx context.Context, err error, output string) string {
	if err == nil {
		return ""
	}
	if ctx.Err() != nil {
		return ctx.Err().Error()
	}
	return truncate(strings.TrimSpace(output), 2000)
}

func errorDetail(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func truncate(value string, size int) string {
	if len(value) <= size {
		return value
	}
	return value[:size] + "..."
}

const graderUnitTests = `package main

import "testing"

func TestGeneratedWinnerLines(t *testing.T) {
	lines := [][3]int{
		{0, 1, 2}, {3, 4, 5}, {6, 7, 8},
		{0, 3, 6}, {1, 4, 7}, {2, 5, 8},
		{0, 4, 8}, {2, 4, 6},
	}
	for _, line := range lines {
		var board Board
		for _, index := range line {
			board[index] = 'X'
		}
		if !hasWinner(&board) {
			t.Fatalf("hasWinner(%v): got false, want true", line)
		}
	}

	board := Board{'X', 'O', 'X', 'O', 'X', 'O', 'O', 'X'}
	if hasWinner(&board) {
		t.Fatal("hasWinner(non-winning board): got true, want false")
	}
}

func TestGeneratedBoardFull(t *testing.T) {
	board := Board{'X', 'O', 'X', 'O', 'X', 'O', 'O', 'X', 'O'}
	if !boardFull(&board) {
		t.Fatal("boardFull(full board): got false, want true")
	}
	board[4] = 0
	if boardFull(&board) {
		t.Fatal("boardFull(non-full board): got true, want false")
	}
}
`
