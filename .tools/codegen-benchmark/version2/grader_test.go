package main

import (
	"context"
	"encoding/json"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixtureSource(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/correct.go")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func fixtureBinary(t *testing.T, source string) string {
	t.Helper()
	dir, err := prepareProgram(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	out, err := runCommand(context.Background(), dir, "go", "build", "-o", "game", ".")
	if err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}
	return filepath.Join(dir, "game")
}

func TestAcceptanceCorrect(t *testing.T) {
	r := gradeProgram(context.Background(), fixtureSource(t))
	if r.Passed != r.Total {
		for _, c := range r.Checks {
			if !c.Passed {
				t.Errorf("%s: %s", c.Name, c.Detail)
			}
		}
	}
	if len(r.Findings) != 0 {
		t.Fatalf("passing program produced failure findings: %+v", r.Findings)
	}
	if len(r.Evidence) != 22 {
		t.Fatalf("evidence: %d", len(r.Evidence))
	}
	for _, e := range r.Evidence {
		if !e.Passed {
			t.Errorf("%s: %s\nexpected %q\nactual %q", e.Scenario, e.Error, e.Expected, e.Actual)
		}
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if dir := os.Getenv("KRONK_GRADER_EVIDENCE_DIR"); dir != "" {
		if err := os.WriteFile(filepath.Join(dir, "correct-result.json"), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
}

// Each mutation is a runnable broken fixture, built once and tested against the
// public terminal interface rather than inspected for suspicious source text.
func TestAcceptanceRegressions(t *testing.T) {
	source := fixtureSource(t)
	cases := []struct{ name, old, new, scenario string }{
		{"colors", `\033[37m`, ``, "X-line-1"},
		{"reset", `\033[0m`, ``, "X-line-1"},
		{"clear", `\033[2J\033[H`, ``, "X-line-1"},
		{"newline", `\033[H\n`, `\033[H`, "X-line-1"},
		{"occupied", ` && b[line[0]-'1'] == 0`, ``, "invalid-input"},
		{"multichar", `len(line) == 1`, `len(line) >= 1`, "invalid-input"},
		{"trim", `line = strings.TrimSpace(line)`, `line = strings.TrimSuffix(line, "\n")`, "invalid-input"},
		{"turn", `player = 'O'`, `player = 'X'`, "X-line-1"},
		{"result-order", `renderBoard(&b, xWins, oWins, draws)
				fmt.Printf("Player %c wins!\n", player)`, `fmt.Printf("Player %c wins!\n", player)
				renderBoard(&b, xWins, oWins, draws)`, "X-line-1"},
		{"replay", `strings.EqualFold(strings.TrimSpace(answer), "y")`, `(strings.TrimSpace(answer) == "y")`, "replay-score"},
		{"scores", `var b Board`, `xWins, oWins, draws = 0, 0, 0; var b Board`, "replay-score"},
		{"prompt", `(1-9):`, `(1-9): `, "X-line-1"},
		{"duplicate", `b[index] = player`, `renderBoard(&b, xWins, oWins, draws); b[index] = player`, "X-line-1"},
		{"win-precedence", `if hasWinner(&b) {`, `if hasWinner(&b) && !boardFull(&b) {`, "last-move-win"},
		{"other-exit", `strings.EqualFold(strings.TrimSpace(answer), "y")`, `(strings.TrimSpace(answer) != "n")`, "other-exit"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !strings.Contains(source, c.old) {
				t.Fatal("mutation does not match fixture")
			}
			binary := fixtureBinary(t, strings.ReplaceAll(source, c.old, c.new))
			for _, s := range acceptanceScenarios() {
				if s.name == c.scenario {
					e, _ := executeScenario(context.Background(), binary, s)
					if e.Passed {
						t.Fatal("broken fixture passed")
					}
					switch c.name {
					case "colors", "reset", "clear", "newline", "prompt":
						if !strings.Contains(e.Actual, "Player X wins!\nPlay again? (y/n): ") {
							t.Fatalf("presentation defect blocked later gameplay: %s", e.Error)
						}
					}
					return
				}
			}
			t.Fatal("unknown scenario")
		})
	}
}

func TestAcceptanceFormattingFeedback(t *testing.T) {
	source := fixtureSource(t)
	// An initial rendering defect must not hide a separate end-of-game defect.
	source = strings.Replace(source, `Draws: %d\n\n`, `Draws: %d\n`, 1)
	source = strings.Replace(source, `fmt.Printf("Player %c wins!\n", player)`, `fmt.Printf("\nPlayer %c wins!\n", player)`, 1)
	grade := gradeProgram(t.Context(), source)
	s := acceptanceScenarios()[0]
	e := grade.Evidence[0]
	if e.Passed || !strings.Contains(e.Actual, "Player X wins!\nPlay again? (y/n): ") {
		t.Fatalf("formatting defects must fail without preventing gameplay: %s\n%q", e.Error, e.Actual)
	}
	feedback := graderFeedback(grade)
	for _, want := range []string{
		`Input sent: "1\n4\n2\n5\n3\nn\n"`,
		`Draws: 0\n\n \x1b[37m1\x1b[0m`, `Draws: 0\n \x1b[37m1\x1b[0m`,
		`\n\nPlayer X wins!\n`, `\n\n\nPlayer X wins!\n`,
	} {
		if !strings.Contains(feedback, want) {
			t.Errorf("feedback omitted %q: %s", want, feedback)
		}
	}
	if e.SentInput != s.input {
		t.Fatalf("completed gameplay lost sent inputs: %q", e.SentInput)
	}
	finding := grade.Findings[0]
	if finding.Location != "scenario-"+s.name || finding.Kind != "transcript" ||
		finding.Expected != e.Expected || finding.Actual != e.Actual || finding.Input != e.SentInput {
		t.Fatalf("feedback must retain complete transcripts, not inferred edits: %+v", finding)
	}
}

func TestAcceptanceStopsOnUnexpectedState(t *testing.T) {
	for _, tc := range []struct{ name, old, replacement string }{
		{"player", `player = 'O'`, `player = 'X'`},
		{"board", `b[index] = player`, `b[index] = 'O'`},
		{"score", `xWins, oWins, draws := 0, 0, 0`, `xWins, oWins, draws := 2, 0, 0`},
		{"prompt", `Player %c's turn. Enter a number (1-9):`, `Play again? (y/n): Player %c`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := strings.Replace(fixtureSource(t), tc.old, tc.replacement, 1)
			source = strings.Replace(source, `Draws: %d\n\n`, `Draws: %d\n`, 1)
			e, _ := executeScenario(t.Context(), fixtureBinary(t, source), acceptanceScenarios()[0])
			if e.Passed || !strings.Contains(e.Error, "unexpected game state or prompt") {
				t.Fatalf("unexpected state must stop progression: %s", e.Error)
			}
			if strings.Contains(e.Actual, "wins!") {
				t.Fatalf("continued gameplay after unexpected state: %q", e.Actual)
			}
		})
	}
}

// A formatting defect must not hide a missing prompt, and identical timeout
// reasons must not merge interactions that were waiting for different output.
func TestAcceptanceFeedbackPreservesStoppedContext(t *testing.T) {
	const invalid = "Invalid move. Enter an empty position from 1 to 9.\n"
	const prompt = "Player X's turn. Enter a number (1-9):"
	grade := gradeResult{Total: 2}
	for _, tc := range []struct {
		name, output, missing string
	}{
		{"missing-prompt", `fmt.Println("Invalid move. Enter an empty position from 1 to 9.")`, prompt},
		{"missing-message-and-prompt", "", invalid + prompt},
	} {
		source := strings.Replace(fixtureSource(t), `Draws: %d\n\n`, `Draws: %d\n`, 1)
		source = strings.Replace(source, `fmt.Println("Invalid move. Enter an empty position from 1 to 9.")`,
			tc.output+`; for { _, _ = stdin.ReadString('\n') }`, 1)
		s := transcript(tc.name, "\n1\n") // The second input must not be sent after the missing prompt.
		e, findings := executeScenario(t.Context(), fixtureBinary(t, source), s)
		if e.Passed || !strings.Contains(e.Error, "deadline exceeded") {
			t.Fatalf("missing output must stop the scenario: %+v", e)
		}
		if len(findings) != 1 {
			t.Fatalf("want one complete scenario observation: %+v", findings)
		}
		stopped := findings[0]
		wantActual := strings.Replace(s.steps[0], "Draws: 0\n\n", "Draws: 0\n", 1) + strings.TrimSuffix(invalid+prompt, tc.missing)
		if stopped.Kind != "interaction" || stopped.Step != 2 || stopped.Expected != s.steps[0]+invalid+prompt ||
			stopped.Actual != wantActual || stopped.Input != "\n" || e.SentInput != "\n" {
			t.Fatalf("lost preceding formatting, stopped interaction, or actual inputs: %+v", stopped)
		}
		grade.add("scenario-"+s.name, e.Passed, e.Error, findings...)
	}
	feedback := graderFeedback(grade)
	for _, want := range []string{"missing-prompt [interaction], step 2", "missing-message-and-prompt [interaction], step 2", prompt, `Draws: 0\n\n`, `Draws: 0\n `} {
		if !strings.Contains(feedback, want) {
			t.Errorf("feedback omitted %q: %s", want, feedback)
		}
	}
	if strings.Count(feedback, "[interaction]") != 2 {
		t.Fatalf("distinct scenario observations merged: %s", feedback)
	}
}

func TestAcceptanceBoundedExecution(t *testing.T) {
	for _, source := range []string{
		`package main; import "fmt"; func main() { for { fmt.Print("unexpected output") } }`,
		`package main; import "time"; func main() { time.Sleep(time.Hour) }`,
	} {
		binary := fixtureBinary(t, source)
		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		e, _ := executeScenario(ctx, binary, acceptanceScenarios()[0])
		cancel()
		if e.Passed || len(e.Actual) > 65539 || e.Error == "" {
			t.Fatalf("unbounded execution: %+v", e)
		}
	}
}

func TestCompilerDiagnosticsPreserved(t *testing.T) {
	var source strings.Builder
	source.WriteString("package main\nvar (\n")
	for i := range 8 {
		source.WriteString("_ = " + strings.Repeat("unknown", 50) + string(rune('a'+i)) + "\n")
	}
	source.WriteString("_ = missingTailDiagnostic\n)\nfunc main() {}\n")
	formatted, err := format.Source([]byte(source.String()))
	if err != nil {
		t.Fatal(err)
	}
	grade := gradeProgram(t.Context(), string(formatted))
	feedback := graderFeedback(grade)
	if grade.PassedCheck("go-build") || !strings.Contains(feedback, "undefined: missingTailDiagnostic") {
		t.Fatalf("feedback lost the compiler diagnostic after the first 2000 bytes: %s", feedback)
	}
}

func TestSubmittedFormatting(t *testing.T) {
	source := fixtureSource(t)
	formatted, err := format.Source([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	if string(formatted) != source {
		t.Fatal("correct fixture not formatted")
	}
	r := gradeProgram(context.Background(), strings.Replace(source, "type Board [9]byte", "type Board    [9]byte", 1))
	if r.PassedCheck("gofmt") {
		t.Fatal("unformatted submission passed")
	}
}
