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
