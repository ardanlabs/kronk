package main

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/creack/pty"
)

const graderTimeout = 20 * time.Second

type gradeCheck struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail,omitempty"`
}

type gradeResult struct {
	Passed   int                 `json:"passed"`
	Total    int                 `json:"total"`
	Checks   []gradeCheck        `json:"checks"`
	Evidence []executionEvidence `json:"evidence,omitempty"`
}

type executionEvidence struct {
	Scenario string `json:"scenario"`
	Input    string `json:"input"`
	Expected string `json:"expected_raw_output"`
	Actual   string `json:"actual_raw_output"`
	Error    string `json:"error,omitempty"`
	Passed   bool   `json:"passed"`
}

type scenario struct {
	name  string
	input string
	steps []string
}

func (gr gradeResult) Percentage() float64 {
	if gr.Total == 0 {
		return 0
	}
	return 100 * float64(gr.Passed) / float64(gr.Total)
}

func (gr gradeResult) PassedCheck(name string) bool {
	for _, check := range gr.Checks {
		if check.Name == name {
			return check.Passed
		}
	}
	return false
}

func (gr *gradeResult) add(name string, passed bool, detail string) {
	if passed {
		gr.Passed++
	}
	gr.Checks = append(gr.Checks, gradeCheck{Name: name, Passed: passed, Detail: detail})
}

func gradeProgram(parent context.Context, source string) gradeResult {
	result := gradeResult{Total: 15 + len(acceptanceScenarios())}
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
	result.add("gofmt", err == nil && bytes.Equal(formatted, []byte(source)), "submitted source must already be gofmt formatted")
	if err != nil {
		return result
	}

	checks := inspectStructure(file)
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

	for _, scenario := range acceptanceScenarios() {
		evidence := executeScenario(parent, filepath.Join(dir, "tictactoe"), scenario)
		result.Evidence = append(result.Evidence, evidence)
		result.add("scenario-"+scenario.name, evidence.Passed, evidence.Error)
	}

	return result
}

func inspectStructure(file *ast.File) []gradeCheck {
	checks := []gradeCheck{
		{Name: "type-board", Detail: "want type Board [9]byte"},
		{Name: "shared-stdin", Detail: "want package-level stdin initialized with bufio.NewReader(os.Stdin)"},
		{Name: "no-structs", Passed: true, Detail: "task forbids additional structs"},
	}
	for _, decl := range file.Decls {
		if decl, ok := decl.(*ast.GenDecl); ok && decl.Tok == token.VAR {
			for _, spec := range decl.Specs {
				node := spec.(*ast.ValueSpec)
				if len(node.Names) == 1 && node.Names[0].Name == "stdin" && len(node.Values) == 1 && isStdinReader(node.Values[0]) {
					passCheck(checks, "shared-stdin")
				}
			}
		}
	}
	required := map[string]string{
		"renderBoard": "func renderBoard(b *Board, xWins, oWins, draws int)",
		"hasWinner":   "func hasWinner(b *Board) bool",
		"boardFull":   "func boardFull(b *Board) bool",
		"playerX":     "func playerX(b *Board) int",
		"playerO":     "func playerO(b *Board) int",
	}
	for _, name := range []string{"renderBoard", "hasWinner", "boardFull", "playerX", "playerO"} {
		checks = append(checks, gradeCheck{Name: "function-" + name, Detail: "want " + required[name]})
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.TypeSpec:
			array, ok := node.Type.(*ast.ArrayType)
			if node.Name.Name == "Board" && !node.Assign.IsValid() && ok && integerLiteral(array.Len, "9") && identifier(array.Elt, "byte") {
				passCheck(checks, "type-board")
			}
		case *ast.StructType:
			for i := range checks {
				if checks[i].Name == "no-structs" {
					checks[i].Passed = false
				}
			}
		case *ast.FuncDecl:
			if signature, exists := required[node.Name.Name]; exists && exactFunctionSignature(node, node.Name.Name) {
				passCheck(checks, "function-"+node.Name.Name)
				_ = signature
			}
		}
		return true
	})

	return checks
}

func exactFunctionSignature(fn *ast.FuncDecl, name string) bool {
	if fn.Recv != nil || fn.Type.TypeParams != nil {
		return false
	}
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
		"go.mod":  "module tictactoe\n\ngo 1.27.0\n",
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
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local", "GOPROXY=off", "GOSUMDB=off")
	output := &boundedOutput{}
	cmd.Stdout, cmd.Stderr = output, output
	err := cmd.Run()
	return output.String(), err
}

type boundedOutput struct{ bytes.Buffer }

func (b *boundedOutput) Write(p []byte) (int, error) {
	n := len(p)
	if remaining := 65536 - b.Len(); remaining > 0 {
		_, _ = b.Buffer.Write(p[:min(len(p), remaining)])
	}
	return n, nil
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

// The oracle models only the frozen step-one contract, never candidate helpers.
var winningLines = [][3]int{{0, 1, 2}, {3, 4, 5}, {6, 7, 8}, {0, 3, 6}, {1, 4, 7}, {2, 5, 8}, {0, 4, 8}, {2, 4, 6}}

func oracleWinner(b [9]byte) bool {
	for _, l := range winningLines {
		if b[l[0]] != 0 && b[l[0]] == b[l[1]] && b[l[1]] == b[l[2]] {
			return true
		}
	}
	return false
}

func oracleBoard(b [9]byte, scores [3]int) string {
	var out strings.Builder
	fmt.Fprintf(&out, "\033[2J\033[H\nScore: X: %d | O: %d | Draws: %d\n\n", scores[0], scores[1], scores[2])
	for i, cell := range b {
		if i%3 == 0 {
			out.WriteByte(' ')
		} else {
			out.WriteString(" \033[32m|\033[0m ")
		}
		switch cell {
		case 'X':
			out.WriteString("\033[1;31mX\033[0m")
		case 'O':
			out.WriteString("\033[1;32mO\033[0m")
		default:
			fmt.Fprintf(&out, "\033[37m%d\033[0m", i+1)
		}
		if i%3 == 2 {
			out.WriteByte('\n')
			if i != 8 {
				out.WriteString("\033[32m-----------\033[0m\n")
			}
		}
	}
	out.WriteByte('\n')
	return out.String()
}

func transcript(name, input string) scenario {
	s := scenario{name: name, input: input}
	var b [9]byte
	var scores [3]int
	player := byte('X')
	prompt := func() string { return fmt.Sprintf("Player %c's turn. Enter a number (1-9):", player) }
	s.steps = append(s.steps, oracleBoard(b, scores)+prompt())
	finished := false
	for line := range strings.SplitSeq(strings.TrimSuffix(input, "\n"), "\n") {
		v := strings.TrimSpace(line)
		if finished {
			if strings.EqualFold(v, "y") {
				b = [9]byte{}
				player = 'X'
				finished = false
				s.steps = append(s.steps, oracleBoard(b, scores)+prompt())
			} else {
				s.steps = append(s.steps, "")
			}
			continue
		}
		if len(v) != 1 || v[0] < '1' || v[0] > '9' || b[v[0]-'1'] != 0 {
			s.steps = append(s.steps, "Invalid move. Enter an empty position from 1 to 9.\n"+prompt())
			continue
		}
		b[v[0]-'1'] = player
		full := true
		for _, c := range b {
			if c == 0 {
				full = false
			}
		}
		result := ""
		if oracleWinner(b) {
			if player == 'X' {
				scores[0]++
			} else {
				scores[1]++
			}
			result = fmt.Sprintf("Player %c wins!\n", player)
		} else if full {
			scores[2]++
			result = "It's a draw.\n"
		}
		if result != "" {
			finished = true
			s.steps = append(s.steps, oracleBoard(b, scores)+result+"Play again? (y/n): ")
		} else {
			if player == 'X' {
				player = 'O'
			} else {
				player = 'X'
			}
			s.steps = append(s.steps, oracleBoard(b, scores)+prompt())
		}
	}
	return s
}

// Search legal gameplay for each player's line; stop at any earlier result.
func lineGame(target [3]int, mark byte) string {
	var search func([9]byte, byte, string) string
	search = func(b [9]byte, p byte, moves string) string {
		if oracleWinner(b) {
			if b[target[0]] == mark && b[target[1]] == mark && b[target[2]] == mark {
				return moves
			}
			return ""
		}
		for i, c := range b {
			if c != 0 {
				continue
			}
			next := b
			next[i] = p
			other := byte('X')
			if p == 'X' {
				other = 'O'
			}
			if found := search(next, other, moves+fmt.Sprintf("%d\n", i+1)); found != "" {
				return found
			}
		}
		return ""
	}
	return search([9]byte{}, 'X', "")
}

func acceptanceScenarios() []scenario {
	var out []scenario
	for _, p := range []byte{'X', 'O'} {
		for i, l := range winningLines {
			out = append(out, transcript(fmt.Sprintf("%c-line-%d", p, i+1), lineGame(l, p)+"n\n"))
		}
	}
	draw := "1\n2\n3\n5\n4\n6\n8\n7\n9\n"
	x := "1\n4\n2\n5\n3\n"
	o := "1\n2\n3\n5\n4\n8\n"
	out = append(out,
		transcript("draw", draw+"n\n"),
		transcript("last-move-win", "1\n2\n3\n4\n5\n6\n8\n7\n9\nn\n"),
		transcript("invalid-input", "\n0\n10\n01\nword\n-1\n9x\n:\n９\n1 2\n  \n\t1 \r\n1\n0\n\n 4 \n4\n2\n5\n3\nn\n"),
		transcript("replay-score", x+" y \n"+o+"Y\n"+draw+"n\n"),
		transcript("other-exit", x+"yes\n"),
		transcript("empty-exit", x+"\n"),
	)
	return out
}

// A real terminal with echo and output newline translation disabled preserves
// the exact bytes specified by the task. Input is sent only after each expected
// state has been observed, so programs which wait for all input cannot pass.
func executeScenario(parent context.Context, binary string, s scenario) executionEvidence {
	e := executionEvidence{Scenario: s.name, Input: s.input, Expected: strings.Join(s.steps, "")}
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", `stty -echo -onlcr -icrnl || exit; exec "$1"`, "grader", binary)
	cmd.Dir = filepath.Dir(binary)
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.Env = append(os.Environ(), "TERM=xterm", "LC_ALL=C")
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		e.Error = err.Error()
		return e
	}
	defer f.Close()
	defer func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }()
	fd := int(f.Fd())
	if err := syscall.SetNonblock(fd, true); err != nil {
		cancel()
		_ = cmd.Wait()
		e.Error = err.Error()
		return e
	}
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	chunks := make(chan []byte, 8)
	go func() {
		defer close(chunks)
		buf := make([]byte, 4096)
		for {
			n, err := syscall.Read(fd, buf)
			if n > 0 {
				select {
				case chunks <- bytes.Clone(buf[:n]):
				case <-ctx.Done():
					return
				}
			}
			if err == syscall.EAGAIN || err == syscall.EWOULDBLOCK {
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Millisecond):
					continue
				}
			}
			if err != nil || n == 0 {
				return
			}
		}
	}()
	var actual []byte
	readUntil := func(want string) error {
		for len(actual) < len(want) {
			select {
			case chunk, ok := <-chunks:
				if !ok {
					return fmt.Errorf("terminal closed before expected state")
				}
				actual = append(actual, chunk...)
				if len(actual) > 65536 {
					return fmt.Errorf("output limit exceeded")
				}
			case <-ctx.Done():
				return ctx.Err()
			}
			if !strings.HasPrefix(want, string(actual)) {
				return fmt.Errorf("raw terminal transcript mismatch")
			}
		}
		if string(actual) != want {
			return fmt.Errorf("raw terminal transcript mismatch")
		}
		return nil
	}
	var want strings.Builder
	inputs := strings.Split(strings.TrimSuffix(s.input, "\n"), "\n")
	for i, step := range s.steps {
		want.WriteString(step)
		if err = readUntil(want.String()); err != nil {
			break
		}
		if i < len(inputs) {
			_, err = f.Write([]byte(inputs[i] + "\n"))
			if err != nil {
				break
			}
		}
	}
	if err != nil {
		cancel()
	}
	// Drain through exit, rejecting duplicate boards, extra prompts and runaway output.
	for chunk := range chunks {
		if len(actual)+len(chunk) > 65536 {
			err = fmt.Errorf("output limit exceeded")
			cancel()
			break
		}
		actual = append(actual, chunk...)
	}
	waitErr := <-waited
	if err == nil {
		err = waitErr
	}
	if err == nil && ctx.Err() != nil {
		err = ctx.Err()
	}
	if err == nil && string(actual) != e.Expected {
		err = fmt.Errorf("raw terminal transcript mismatch")
	}
	e.Actual = truncate(string(actual), 65536)
	e.Error = errorDetail(err)
	if err != nil && e.Actual != e.Expected {
		index := 0
		for index < len(e.Actual) && index < len(e.Expected) && e.Actual[index] == e.Expected[index] {
			index++
		}
		start := max(0, index-30)
		e.Error += fmt.Sprintf("; first difference at byte %d: expected %q, actual %q (ANSI escapes shown literally)", index,
			e.Expected[start:min(len(e.Expected), index+100)], e.Actual[start:min(len(e.Actual), index+100)])
	}
	e.Passed = err == nil
	return e
}

const graderUnitTests = `package main

import (
	"bufio"
	"strings"
	"testing"
)

func TestGeneratedPlayers(t *testing.T) {
	saved := stdin
	defer func() { stdin = saved }()
	for _, player := range []func(*Board) int{playerX, playerO} {
		for index := range 9 {
			var b Board
			input := "word\n0\n10\n\n01\n \t" + string(byte('1'+index)) + " \r\n"
			stdin = bufio.NewReader(strings.NewReader(input))
			if got := player(&b); got != index { t.Fatalf("move: got %d want %d", got, index) }
			if b != (Board{}) { t.Fatal("player function changed board") }
		}
	}
	b := Board{'X'}
	stdin = bufio.NewReader(strings.NewReader("1\n2\n3\n"))
	if playerO(&b) != 1 || playerX(&b) != 2 { t.Fatal("occupied rejection or shared buffered input failed") }
	if b != (Board{'X'}) { t.Fatal("player function changed occupied board") }
}

func TestGeneratedWinnerLines(t *testing.T) {
	lines := [][3]int{
		{0, 1, 2}, {3, 4, 5}, {6, 7, 8},
		{0, 3, 6}, {1, 4, 7}, {2, 5, 8},
		{0, 4, 8}, {2, 4, 6},
	}
	for _, mark := range []byte{'X', 'O'} {
		for _, line := range lines {
			var board Board
			for _, index := range line {
				board[index] = mark
			}
			if !hasWinner(&board) {
				t.Fatalf("hasWinner(%v, %c): got false, want true", line, mark)
			}
		}
	}
	var empty Board
	if hasWinner(&empty) { t.Fatal("empty board won") }

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
	for i := range board {
		partial := board
		partial[i] = 0
		if boardFull(&partial) {
			t.Fatalf("boardFull(empty cell %d): got true, want false", i)
		}
	}
}
`
