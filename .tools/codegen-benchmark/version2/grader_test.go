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
					e := executeScenario(context.Background(), binary, s)
					if e.Passed {
						t.Fatal("broken fixture passed")
					}
					return
				}
			}
			t.Fatal("unknown scenario")
		})
	}
}

func TestAcceptanceBoundedExecution(t *testing.T) {
	for _, source := range []string{
		`package main; import "fmt"; func main() { for { fmt.Print("unexpected output") } }`,
		`package main; import "time"; func main() { time.Sleep(time.Hour) }`,
	} {
		binary := fixtureBinary(t, source)
		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		e := executeScenario(ctx, binary, acceptanceScenarios()[0])
		cancel()
		if e.Passed || len(e.Actual) > 65539 || e.Error == "" {
			t.Fatalf("unbounded execution: %+v", e)
		}
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
