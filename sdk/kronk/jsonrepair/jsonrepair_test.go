package jsonrepair

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRepair(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		fail      bool              // true if repair should fail (irrecoverable)
		keys      map[string]string // expected key → substring-of-value (empty string = just check key exists)
		wantExact map[string]string // expected key → exact value (for precise output verification)
	}{
		// =================================================================
		// Valid JSON — no repair needed
		// =================================================================
		{
			name:  "valid simple",
			input: `{"content":"hello","filePath":"main.go"}`,
			keys:  map[string]string{"content": "hello", "filePath": "main.go"},
		},
		{
			name:  "valid with escaped quotes",
			input: `{"content":"board := [9]string{\"1\", \"2\", \"3\", \"4\", \"5\", \"6\", \"7\", \"8\", \"9\"}\nfmt.Println(board)","filePath":"main.go"}`,
			keys:  map[string]string{"filePath": "main.go"},
		},
		{
			name:  "valid edit with escaped quotes",
			input: `{"filePath":"main.go","oldString":"fmt.Println(\"hello\")","newString":"fmt.Println(\"goodbye\")"}`,
			keys:  map[string]string{"filePath": "main.go", "oldString": "", "newString": ""},
		},
		{
			name:  "valid with boolean",
			input: `{"content":"new text","filePath":"main.go","replaceAll":false}`,
			keys:  map[string]string{"content": "new text", "filePath": "main.go"},
		},

		// =================================================================
		// Bare keys
		// =================================================================
		{
			name:  "bare keys",
			input: `{content:"hello",filePath:"main.go"}`,
			keys:  map[string]string{"content": "hello", "filePath": "main.go"},
		},

		// =================================================================
		// Gemma4 <|"|> tokens
		// =================================================================
		{
			name:  "gemma simple tokens",
			input: `{"content":<|"|>import "fmt"<|"|>,"filePath":"main.go"}`,
			keys:  map[string]string{"content": `import "fmt"`, "filePath": "main.go"},
		},
		{
			name:  "gemma bare keys and tokens",
			input: `{content:<|"|>import "os"<|"|>,filePath:<|"|>main.go<|"|>}`,
			keys:  map[string]string{"content": `import "os"`, "filePath": "main.go"},
		},
		{
			name:  "gemma bare keys with code and newlines",
			input: "{content:<|\"|>package main\n\nimport (\n\t\"bufio\"\n\t\"fmt\"\n)\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}<|\"|>,filePath:<|\"|>examples/talks/tictactoe/main.go<|\"|>}",
			keys:  map[string]string{"content": "package main", "filePath": "examples/talks/tictactoe/main.go"},
		},
		{
			// Reproduces production failure: model outputs \n (backslash-n) as
			// a Go escape sequence inside fmt.Print("\nPlay again?"). The repair
			// must preserve it as literal \n, not convert to a real newline.
			name:  "gemma preserves backslash-n in string literals",
			input: "{content:<|\"|>package main\n\nimport (\n\t\"fmt\"\n)\n\nfunc main() {\n\tfor {\n\t\tplayGame()\n\t\tfmt.Print(\"\\nPlay again? (y/n): \")\n\t\tvar choice string\n\t\tfmt.Scanln(&choice)\n\t}\n}<|\"|>,filePath:<|\"|>examples/talks/tictactoe/main.go<|\"|>}",
			wantExact: map[string]string{
				"content":  "package main\n\nimport (\n\t\"fmt\"\n)\n\nfunc main() {\n\tfor {\n\t\tplayGame()\n\t\tfmt.Print(\"\\nPlay again? (y/n): \")\n\t\tvar choice string\n\t\tfmt.Scanln(&choice)\n\t}\n}",
				"filePath": "examples/talks/tictactoe/main.go",
			},
		},

		{
			// Reproduces production failure: model writes "content:<|"|> instead
			// of "content":<|"|> (missing closing " on key), opens with <|"|>
			// but closes with " (mixed delimiters), and has extra trailing }
			// from Gemma's call:write{{...}} double-brace wrapping.
			name:  "gemma missing key quote mixed delimiters trailing brace",
			input: "{\"content:<|\"|>package main\n\nimport (\n\t\"fmt\"\n)\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n\",\"filePath\":\"examples/talks/tictactoe/main.go\"}}",
			keys:  map[string]string{"content": "package main", "filePath": "examples/talks/tictactoe/main.go"},
		},

		{
			// Reproduces production failure: model uses <|"|> for edit values
			// containing ANSI \033 codes. The \0 must be double-escaped to
			// survive JSON parsing.
			name:  "gemma token with ANSI escapes in edit",
			input: "{\"filePath\":\"main.go\",\"newString:<|\"|>const (\n\tcolorReset = \"\\033[0m\"\n\tcolorRed   = \"\\033[31m\"\n)\n<|\"|>,\"oldString\":\"import (\\n\\t\\\"fmt\\\"\\n)\"}",
			keys:  map[string]string{"filePath": "main.go", "newString": "colorReset", "oldString": ""},
		},
		{
			// Reproduces production failure: model uses <|"|> for content value
			// but standard quotes for filePath. The closing <|"|> token boundary
			// swallows the opening " of filePath, producing ,filePath": instead
			// of ,"filePath":.
			name:  "gemma token value then bare key missing open quote",
			input: "{\"content:<|\"|>package main\n\nimport (\n\t\"fmt\"\n)\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n<|\"|>,filePath\":\"/Users/bill/test/main.go\"}",
			keys:  map[string]string{"content": "package main", "filePath": "/Users/bill/test/main.go"},
		},

		// =================================================================
		// Mixed delimiters — model opens with " but closes with <|"|>
		// =================================================================
		{
			name:  "mixed delimiters quote to gemma",
			input: `{"filePath":"main.go","newString":"import (\n\t\"bufio\"\n\t\"fmt\"\n)<|"|>,"oldString":"import (\n\t\"os\"\n)"}`,
			keys:  map[string]string{"filePath": "main.go", "newString": "", "oldString": ""},
		},

		// =================================================================
		// Unescaped quotes in content — the core failure case
		// =================================================================
		{
			name:  "unescaped Go imports",
			input: `{"content":"package main\nimport (\n\t"bufio"\n\t"fmt"\n)\n","filePath":"main.go"}`,
			keys:  map[string]string{"content": "bufio", "filePath": "main.go"},
		},
		{
			name:  "unescaped board init tictactoe",
			input: `{"content":"board := [9]string{"1", "2", "3", "4", "5", "6", "7", "8", "9"}\nfmt.Println(board)","filePath":"main.go"}`,
			keys:  map[string]string{"content": "board", "filePath": "main.go"},
		},
		{
			name:  "multiple unescaped quotes",
			input: `{"content":"a "b" c "d" e","filePath":"x.go"}`,
			keys:  map[string]string{"filePath": "x.go"},
		},
		{
			name:  "bare keys and unescaped quotes",
			input: `{content:"say "hello" world",filePath:"test.go"}`,
			keys:  map[string]string{"filePath": "test.go"},
		},
		{
			name:  "unescaped edit oldString newString",
			input: `{"filePath":"main.go","oldString":"import "fmt"","newString":"import (\n\t"fmt"\n\t"os"\n)"}`,
			keys:  map[string]string{"filePath": "main.go", "oldString": "", "newString": ""},
		},
		{
			// Reproduces production failure: Go source code with \n in
			// fmt.Printf format strings. The \n must be preserved as
			// literal backslash-n, not converted to an actual newline.
			name:  "unescaped quotes preserve backslash-n in source",
			input: "{\"content\":\"func printBoard(board []string) {\n\tfmt.Printf(\"%s | %s | %s\\n\", board[0], board[1], board[2])\n}\",\"filePath\":\"main.go\"}",
			wantExact: map[string]string{
				"content":  "func printBoard(board []string) {\n\tfmt.Printf(\"%s | %s | %s\\n\", board[0], board[1], board[2])\n}",
				"filePath": "main.go",
			},
		},

		// =================================================================
		// Backtick delimiters
		// =================================================================
		{
			name:  "backtick comma separator",
			input: "{\"content\":\"hello" + "`,`" + "filePath\":\"main.go\"}",
			keys:  map[string]string{"content": "hello", "filePath": "main.go"},
		},
		{
			name:  "gemma open backtick close with bare keys",
			input: "{content:<|\"|>package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n`, filePath: \"examples/talks/tictactoe/main.go\"}",
			keys:  map[string]string{"content": "package main", "filePath": "examples/talks/tictactoe/main.go"},
		},
		{
			name:  "backtick value delimiters",
			input: "{\"content\":`hello world`,\"filePath\":`main.go`}",
			keys:  map[string]string{"content": "hello world", "filePath": "main.go"},
		},

		// =================================================================
		// Mixed delimiters — oldString closes with <|"|> + double brace
		// =================================================================
		{
			// Reproduces production failure: newString has partially
			// escaped quotes (model escaped some but not all), oldString
			// opens with " but closes with <|"|>, and the whole thing
			// ends with }} from Gemma's double-brace wrapping. The
			// findKeyAwareClosingQuote must accept "}} at end-of-string.
			name:  "edit with mixed escaping and oldString closing with gemma token double brace",
			input: "{\"filePath\":\"main.go\",\"newString\":\"\\t\\tif winner != \\\"\\\" {\\n\\t\\t\\tfmt.Println()\\n\\t\\t\\tfmt.Println(\\\"done\\\")\\n\\t\\t}\",\"oldString\":\"\\t\\tif winner != \\\"\\\" {\\n\\t\\t\\tfmt.Println(\\\"done\\\")\\n\\t\\t}<|\"|>}}",
			keys:  map[string]string{"filePath": "main.go", "newString": "", "oldString": ""},
		},
		{
			// Reproduces production failure: newString opens with " and
			// has partially escaped quotes (model uses \" for some but
			// raw " for others), oldString has fully unescaped quotes
			// and closes with <|"|>. Double brace }} at end.
			name:  "edit with unescaped quotes in both values and gemma close",
			input: "{\"filePath\":\"main.go\",\"newString\":\"fmt.Printf(\"Player %s wins!\\n\", winner)\n\t\tfmt.Println(\"done\")\",\"oldString\":\"fmt.Printf(\"Player %s wins!\\n\", winner)<|\"|>}}",
			keys:  map[string]string{"filePath": "main.go", "newString": "", "oldString": ""},
		},

		// =================================================================
		// ANSI escape sequences — \033 octal escapes in Go source
		// =================================================================
		{
			// Reproduces production failure: Go source contains ANSI codes
			// like \033[31m. The \0 is not a valid JSON escape and must be
			// double-escaped to \\0 so the literal backslash survives.
			name:  "unescaped quotes with ANSI escapes",
			input: "{\"content\":\"package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Printf(\"\\033[31m%s\\033[0m\\n\", \"hello\")\n}\n\",\"filePath\":\"main.go\"}",
			keys:  map[string]string{"content": "package main", "filePath": "main.go"},
		},
		{
			// Full tic-tac-toe game with ANSI colors, unescaped quotes,
			// board init arrays, and multi-function Go source.
			name:  "full tictactoe with ANSI codes and unescaped quotes",
			input: "{\"content\":\"package main\n\nimport (\n\t\"fmt\"\n)\n\nfunc main() {\n\tfor {\n\t\tplayGame()\n\t\tfmt.Print(\"\\nPlay again? (y/n): \")\n\t\tvar response string\n\t\tfmt.Scanln(&response)\n\t\tif response != \"y\" {\n\t\t\tbreak\n\t\t}\n\t\tfmt.Println()\n\t}\n}\n\nfunc playGame() {\n\tboard := [9]string{\"1\", \"2\", \"3\", \"4\", \"5\", \"6\", \"7\", \"8\", \"9\"}\n\tcurrentPlayer := \"X\"\n\n\tfor {\n\t\tprintBoard(board)\n\n\t\tvar choice int\n\t\tif currentPlayer == \"X\" {\n\t\t\tchoice = playerX(board)\n\t\t} else {\n\t\t\tchoice = playerO(board)\n\t\t}\n\n\t\tindex := choice - 1\n\t\tif index < 0 || index > 8 || board[index] == \"X\" || board[index] == \"O\" {\n\t\t\tfmt.Println(\"Invalid move, try again.\")\n\t\t\tcontinue\n\t\t}\n\n\t\tboard[index] = currentPlayer\n\n\t\tif checkWinner(board, currentPlayer) {\n\t\t\tprintBoard(board)\n\t\t\tfmt.Printf(\"\\nPlayer %s wins!\\n\", currentPlayer)\n\t\t\treturn\n\t\t}\n\n\t\tif checkDraw(board) {\n\t\t\tprintBoard(board)\n\t\t\tfmt.Println(\"\\nIt's a draw!\")\n\t\t\treturn\n\t\t}\n\n\t\tif currentPlayer == \"X\" {\n\t\t\tcurrentPlayer = \"O\"\n\t\t} else {\n\t\t\tcurrentPlayer = \"X\"\n\t\t}\n\t}\n}\n\nfunc printBoard(board [9]string) {\n\tfmt.Println()\n\tfmt.Printf(\"\\033[31m%s\\033[0m | %s | %s\\n\", board[0], board[1], board[2])\n\tfmt.Println(\"----------\")\n\tfmt.Printf(\"%s | %s | %s\\n\", board[3], board[4], board[5])\n\tfmt.Println(\"----------\")\n\tfmt.Printf(\"%s | %s | %s\\n\", board[6], board[7], board[8])\n}\n\nfunc playerX(board [9]string) int {\n\tfmt.Printf(\"\\n\\033[31mPlayer X\\033[0m's turn. Enter a number (1-9): \")\n\tvar choice int\n\tfmt.Scanln(&choice)\n\treturn choice\n}\n\nfunc playerO(board [9]string) int {\n\tfmt.Printf(\"\\n\\033[34mPlayer O\\033[0m's turn. Enter a number (1-9): \")\n\tvar choice int\n\tfmt.Scanln(&choice)\n\treturn choice\n}\n\nfunc checkWinner(board [9]string, player string) bool {\n\twins := [8][3]int{\n\t\t{0, 1, 2}, {3, 4, 5}, {6, 7, 8},\n\t\t{0, 3, 6}, {1, 4, 7}, {2, 5, 8},\n\t\t{0, 4, 8}, {2, 4, 6},\n\t}\n\n\tfor _, win := range wins {\n\t\tif board[win[0]] == player && board[win[1]] == player && board[win[2]] == player {\n\t\t\treturn true\n\t\t}\n\t}\n\treturn false\n}\n\nfunc checkDraw(board [9]string) bool {\n\tfor _, cell := range board {\n\t\tif cell != \"X\" && cell != \"O\" {\n\t\t\treturn false\n\t\t}\n\t}\n\treturn true\n}\n\",\"filePath\":\"/Users/bill/code/go/src/github.com/ardanlabs/kronk/examples/talks/tictactoe/main.go\"}",
			keys:  map[string]string{"content": "package main", "filePath": "/Users/bill/code/go/src/github.com/ardanlabs/kronk/examples/talks/tictactoe/main.go"},
		},

		// =================================================================
		// Irrecoverable — should return error
		// =================================================================
		{
			name:  "garbage input",
			input: "not json at all",
			fail:  true,
		},
		{
			name:  "empty string",
			input: "",
			fail:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Repair(tt.input)

			if tt.fail {
				if err == nil {
					t.Fatalf("Repair() should have failed\n  input: %q\n  got:   %q", tt.input, got)
				}
				return
			}

			if err != nil {
				t.Fatalf("Repair() failed: %v\n  input: %q", err, tt.input)
			}

			var m map[string]any
			if err := json.Unmarshal([]byte(got), &m); err != nil {
				t.Fatalf("repaired JSON should parse: %v\n  input:    %q\n  repaired: %q", err, tt.input, got)
			}

			for key, wantSub := range tt.keys {
				val, ok := m[key]
				if !ok {
					t.Errorf("missing key %q in repaired JSON\n  repaired: %q", key, got)
					continue
				}
				if wantSub == "" {
					continue
				}

				str, isStr := val.(string)
				if isStr {
					if !strings.Contains(str, wantSub) {
						t.Errorf("key %q = %q, want substring %q", key, str, wantSub)
					}
				}
			}

			for key, wantVal := range tt.wantExact {
				val, ok := m[key]
				if !ok {
					t.Errorf("missing key %q in repaired JSON\n  repaired: %q", key, got)
					continue
				}

				str, isStr := val.(string)
				if !isStr {
					t.Errorf("key %q is not a string: %T", key, val)
					continue
				}

				if str != wantVal {
					t.Errorf("key %q value mismatch\n  got:  %q\n  want: %q", key, str, wantVal)
				}
			}
		})
	}
}
