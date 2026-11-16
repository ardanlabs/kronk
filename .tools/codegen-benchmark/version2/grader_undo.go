package main

import (
	"context"
	"strings"
)

func gradeUndoProgram(parent context.Context, source string) gradeResult {
	// The ordinary player tests still cover every cell and buffered read-ahead.
	// In v2, exactly "0" is valid; use "00" as the invalid zero input instead.
	tests := strings.Replace(graderUnitTests, `word\n0\n10`, `word\n00\n10`, 1) + undoPlayerTests
	return gradeProgramWith(parent, source, undoAcceptanceScenarios(), tests)
}

const undoPlayerTests = `
func TestGeneratedUndoSentinel(t *testing.T) {
	saved := stdin
	defer func() { stdin = saved }()
	for _, player := range []func(*Board) int{playerX, playerO} {
		for _, board := range []Board{{}, {'X', 'O', 'X', 'O', 'X', 'O', 'O', 'X', 'O'}} {
			for _, input := range []string{"0\n", " \t0 \r\n", "00\n01\n-1\n10\nword\n0\n"} {
				b := board
				stdin = bufio.NewReader(strings.NewReader(input))
				if got := player(&b); got != -1 { t.Fatalf("undo: got %d want -1", got) }
				if b != board { t.Fatal("undo sentinel changed board") }
			}
		}
	}
}
`

// Ordinary games reuse the frozen v1 oracle with updated prompts. Undo screens
// are explicit observations, not a second implementation of the game loop.
func undoAcceptanceScenarios() []scenario {
	const xPrompt = "Player X's turn. Enter a number (1-9), or 0 to undo the last move:"
	const oPrompt = "Player O's turn. Enter a number (1-9), or 0 to undo the last move:"
	const invalid = "Invalid move. Enter an empty position from 1 to 9.\n"
	normal := func(name, input string) scenario {
		s := transcript(name, input)
		for i := range s.steps {
			s.steps[i] = strings.ReplaceAll(s.steps[i], "(1-9):", "(1-9), or 0 to undo the last move:")
		}
		return s
	}
	var cases []scenario
	for _, s := range acceptanceScenarios() {
		cases = append(cases, normal(s.name, strings.ReplaceAll(s.input, "\n0\n", "\n00\n")))
	}
	const xGame = "1\n4\n2\n5\n3\n"
	x := normal("X", xGame+"n\n")
	cases = append(cases,
		scenario{
			name:  "undo-X-and-reject-without-history",
			input: "0\n9\n \t0 \r\n0\n" + xGame + "n\n",
			steps: append([]string{
				x.steps[0], invalid + xPrompt,
				oracleBoard([9]byte{8: 'X'}, [3]int{}) + oPrompt,
				x.steps[0], invalid + xPrompt,
			}, x.steps[1:]...),
		},
		scenario{
			name:  "undo-replacement-in-the-same-cell",
			input: "1\n0\n1\n0\n" + xGame + "n\n",
			steps: append([]string{x.steps[0], x.steps[1], x.steps[0], x.steps[1], x.steps[0]}, x.steps[1:]...),
		},
		scenario{
			name:  "undo-replacement-in-a-different-cell",
			input: "9\n0\n1\n0\n" + xGame + "n\n",
			steps: append([]string{
				x.steps[0], oracleBoard([9]byte{8: 'X'}, [3]int{}) + oPrompt,
				x.steps[0], x.steps[1], x.steps[0],
			}, x.steps[1:]...),
		},
		scenario{
			name:  "undo-only-the-latest-move",
			input: "1\n4\n2\n0\n0\n2\n5\n3\nn\n",
			steps: append([]string{
				x.steps[0], x.steps[1], x.steps[2], x.steps[3], x.steps[2], invalid + xPrompt,
			}, x.steps[3:]...),
		},
		scenario{
			name:  "undo-O-and-reject-without-history",
			input: "1\n5\n0\n0\n4\n2\n5\n3\nn\n",
			steps: append([]string{
				x.steps[0], x.steps[1],
				oracleBoard([9]byte{0: 'X', 4: 'O'}, [3]int{}) + xPrompt,
				x.steps[1], invalid + oPrompt,
			}, x.steps[2:]...),
		},
	)
	o := normal("O", "9\n4\n1\n5\n2\n6\nn\n")
	cases = append(cases, scenario{
		name:  "undo-O-in-the-first-cell",
		input: "9\n1\n0\n4\n1\n5\n2\n6\nn\n",
		steps: append([]string{
			o.steps[0], o.steps[1],
			oracleBoard([9]byte{0: 'O', 8: 'X'}, [3]int{}) + xPrompt,
			o.steps[1],
		}, o.steps[2:]...),
	})
	replay := normal("undo-state-resets-after-replay", xGame+"y\n"+xGame+"n\n")
	steps := append([]string{}, replay.steps[:7]...)
	steps = append(steps, invalid+xPrompt,
		oracleBoard([9]byte{8: 'X'}, [3]int{1, 0, 0})+oPrompt, replay.steps[6])
	steps = append(steps, replay.steps[7:]...)
	replay.input = xGame + "y\n0\n9\n0\n" + xGame + "n\n"
	replay.steps = steps
	cases = append(cases, replay,
		normal("cannot-undo-a-win", xGame+"0\n"),
		normal("cannot-undo-a-draw", "1\n2\n3\n5\n4\n6\n8\n7\n9\n0\n"),
	)
	// Undo must preserve all scores, including nonzero O wins and draws. Reuse
	// the explicit X/O undo screens after building a session with all scores set.
	const oGame = "1\n2\n3\n5\n4\n8\n"
	const draw = "1\n2\n3\n5\n4\n6\n8\n7\n9\n"
	prefix := normal("scores", xGame+"y\n"+oGame+"y\n"+draw+"y\n")
	for _, undo := range cases {
		if undo.name != "undo-X-and-reject-without-history" && undo.name != "undo-O-and-reject-without-history" {
			continue
		}
		combined := scenario{name: undo.name + "-preserves-all-scores", input: prefix.input + undo.input}
		combined.steps = append(combined.steps, prefix.steps[:len(prefix.steps)-1]...)
		for _, step := range undo.steps {
			step = strings.ReplaceAll(step, "Score: X: 0 | O: 0 | Draws: 0", "Score: X: 1 | O: 1 | Draws: 1")
			step = strings.ReplaceAll(step, "Score: X: 1 | O: 0 | Draws: 0", "Score: X: 2 | O: 1 | Draws: 1")
			combined.steps = append(combined.steps, step)
		}
		cases = append(cases, combined)
	}
	return cases
}
