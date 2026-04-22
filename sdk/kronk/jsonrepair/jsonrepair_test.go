package jsonrepair

import (
	"encoding/json"
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
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
		goCode    map[string]string // key → "file" or "snippet": parse the repaired value as Go source
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
		// Gemma <|"|> content then standard JSON filePath — must compile
		// =================================================================
		{
			// Reproduces production failure: after parseGemmaToolCallFormat
			// strips call:write and outer braces from the double-brace
			// format call:write{{...}}, jsonrepair receives this input.
			// The Go source has \n escapes in fmt.Print string literals
			// that must survive as literal \n in the output so the code
			// compiles. Real newlines (line breaks) must become actual
			// newlines, not literal \n.
			name:  "gemma content with source escapes then json filePath compiles",
			input: "{\"content:<|\"|>package main\n\nimport (\n\t\"bufio\"\n\t\"fmt\"\n\t\"os\"\n\t\"strconv\"\n\t\"strings\"\n)\n\ntype Game struct {\n\tboard  [9]string\n\treader *bufio.Reader\n}\n\nfunc NewGame() *Game {\n\tg := &Game{\n\t\treader: bufio.NewReader(os.Stdin),\n\t}\n\tfor i := 0; i < 9; i++ {\n\t\tg.board[i] = strconv.Itoa(i + 1)\n\t}\n\treturn g\n}\n\nfunc (g *Game) renderBoard() {\n\tfmt.Println()\n\tfmt.Printf(\"%s | %s | %s\\n\", g.board[0], g.board[1], g.board[2])\n\tfmt.Println(\"----------\")\n\tfmt.Printf(\"%s | %s | %s\\n\", g.board[3], g.board[4], g.board[5])\n\tfmt.Println(\"----------\")\n\tfmt.Printf(\"%s | %s | %s\\n\", g.board[6], g.board[7], g.board[8])\n}\n\nfunc (g *Game) checkWinner() string {\n\twins := [8][3]int{\n\t\t{0, 1, 2}, {3, 4, 5}, {6, 7, 8},\n\t\t{0, 3, 6}, {1, 4, 7}, {2, 5, 8},\n\t\t{0, 4, 8}, {2, 4, 6},\n\t}\n\n\tfor _, combo := range wins {\n\t\tif g.board[combo[0]] == g.board[combo[1]] && g.board[combo[1]] == g.board[combo[2]] {\n\t\t\treturn g.board[combo[0]]\n\t\t}\n\t}\n\n\tfull := true\n\tfor i := 0; i < 9; i++ {\n\t\tcell := g.board[i]\n\t\tif cell != \"X\" && cell != \"O\" {\n\t\t\tfull = false\n\t\t\tbreak\n\t\t}\n\t}\n\tif full {\n\t\treturn \"Draw\"\n\t}\n\n\treturn \"\"\n}\n\nfunc playerX(g *Game) int {\n\tfor {\n\t\tfmt.Print(\"\\nPlayer X's turn. Enter a number (1-9): \")\n\t\tinput, err := g.reader.ReadString('\\n')\n\t\tif err != nil {\n\t\t\tcontinue\n\t\t}\n\t\tinput = strings.TrimSpace(input)\n\t\tval, err := strconv.Atoi(input)\n\t\tif err != nil || val < 1 || val > 9 {\n\t\t\tcontinue\n\t\t}\n\n\t\tidx := val - 1\n\t\tcell := g.board[idx]\n\t\tisNumber := false\n\t\tfor i := 1; i <= 9; i++ {\n\t\t\tif cell == strconv.Itoa(i) {\n\t\t\t\tisNumber = true\n\t\t\t\tbreak\n\t\t\t}\n\t\t}\n\n\t\tif isNumber {\n\t\t\treturn idx\n\t\t}\n\t}\n}\n\nfunc playerO(g *Game) int {\n\tfor {\n\t\tfmt.Print(\"\\nPlayer O's turn. Enter a number (1-9): \")\n\t\tinput, err := g.reader.ReadString('\\n')\n\t\tif err != nil {\n\t\t\tcontinue\n\t\t}\n\t\tinput = strings.TrimSpace(input)\n\t\tval, err := strconv.Atoi(input)\n\t\tif err != nil || val < 1 || val > 9 {\n\t\t\tcontinue\n\t\t}\n\n\t\tidx := val - 1\n\t\tcell := g.board[idx]\n\t\tisNumber := false\n\t\tfor i := 1; i <= 9; i++ {\n\t\t\tif cell == strconv.Itoa(i) {\n\t\t\t\tisNumber = true\n\t\t\t\tbreak\n\t\t\t}\n\t\t}\n\n\t\tif isNumber {\n\t\t\treturn idx\n\t\t}\n\t}\n}\n\nfunc main() {\n\tfor {\n\t\tgame := NewGame()\n\t\tturnX := true\n\n\t\tfor {\n\t\t\tgame.renderBoard()\n\n\t\t\tvar idx int\n\t\t\tif turnX {\n\t\t\t\tidx = playerX(game)\n\t\t\t\tgame.board[idx] = \"X\"\n\t\t\t} else {\n\t\t\t\tidx = playerO(game)\n\t\t\t\tgame.board[idx] = \"O\"\n\t\t\t}\n\n\t\t\tresult := game.checkWinner()\n\t\t\tif result != \"\" {\n\t\t\t\tgame.renderBoard()\n\t\t\t\tif result == \"Draw\" {\n\t\t\t\t\tfmt.Println(\"It's a draw!\")\n\t\t\t\t} else {\n\t\t\t\t\tfmt.Printf(\"Player %s wins!\\n\", result)\n\t\t\t\t}\n\t\t\t\tbreak\n\t\t\t}\n\t\t\tturnX = !turnX\n\t\t}\n\n\t\tfmt.Print(\"\\nPlay again? (y/n): \")\n\t\tinput, err := game.reader.ReadString('\\n')\n\t\tif err != nil {\n\t\t\tbreak\n\t\t}\n\t\tif strings.TrimSpace(strings.ToLower(input)) != \"y\" {\n\t\t\tbreak\n\t\t}\n\t}\n}\n<|\"|>,\"filePath\":\"examples/talks/tictactoe/main.go\"}",
			keys:  map[string]string{"content": "package main", "filePath": "examples/talks/tictactoe/main.go"},
			goCode: map[string]string{
				"content": "file",
			},
		},

		{
			// Reproduces production failure: model wraps the second arg in
			// a nested object instead of listing it as a flat sibling:
			//   call:bash{{"command":<|"|>ls -a<|"|>,{"description":<|"|>...<|"|>}}}
			// After parseGemmaToolCallFormat strips the outer braces, jsonrepair
			// receives this input. The nested {"description":...} must be unwrapped
			// so both keys appear at the top level.
			name:  "gemma nested object wrapping second arg",
			input: `{"command":<|"|>ls -a<|"|>,{"description":<|"|>list all files including hidden ones in the current directory<|"|>}}`,
			keys:  map[string]string{"command": "ls -a", "description": "list all files including hidden ones in the current directory"},
		},

		// =================================================================
		// repairQuotes must preserve valid JSON escapes (\n, \t, etc.)
		// =================================================================
		{
			// Reproduces production failure: edit tool call has raw tab
			// characters (invalid JSON) triggering repairQuotes, which
			// was incorrectly double-escaping the valid \n and \t JSON
			// escapes between statements.
			name: "repairQuotes preserves valid json escapes newline tab",
			// Raw tab chars make the JSON invalid, triggering repairQuotes.
			// The \n and \t between statements are valid JSON escapes and
			// must NOT be double-escaped.
			input: "{\"filePath\":\"main.go\",\"newString\":\"\tfmt.Printf(\\\"%s | %s | %s\\\\n\\\", board[6], board[7], board[8])\\n\\tfmt.Println()\",\"oldString\":\"\tfmt.Printf(\\\"%s | %s | %s\\\\n\\\", board[6], board[7], board[8])\"}",
			wantExact: map[string]string{
				"filePath":  "main.go",
				"oldString": "\tfmt.Printf(\"%s | %s | %s\\n\", board[6], board[7], board[8])",
				"newString": "\tfmt.Printf(\"%s | %s | %s\\n\", board[6], board[7], board[8])\n\tfmt.Println()",
			},
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
			name:  "gemma open backtick close with malformed key quote",
			input: "{content:<|\"|>package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n`,filePath\":\"examples/talks/tictactoe/main.go\"}",
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
		// Mixed JSON quotes + Gemma <|"|> boundary token
		// =================================================================
		{
			// Reproduces production failure: model uses JSON " quotes for
			// code content but emits a single <|"|> at the boundary between
			// content and filePath. The content value is a full Go source
			// file with unescaped { } braces. filePath is in a nested JSON
			// object: ,"filePath":"path"}.
			name:  "gemma boundary token between JSON quoted code and filePath",
			input: "{\"content\":\"package main\n\nimport (\n\t\"fmt\"\n\t\"os\"\n)\n\nvar board = [9]string{\n\t\"1\", \"2\", \"3\",\n\t\"4\", \"5\", \"6\",\n\t\"7\", \"8\", \"9\",\n}\n\nfunc main() {\n\tfor {\n\t\tfmt.Println(\"hello\")\n\t\tos.Exit(0)\n\t}\n}\n<|\"|>,{\"filePath\":\"examples/talks/tictactoe/main.go\"}}",
			keys:  map[string]string{"content": "package main", "filePath": "examples/talks/tictactoe/main.go"},
		},

		// =================================================================
		// Go code verification — repaired values must parse as valid Go
		// =================================================================
		{
			// Full Go source file via Gemma <|"|> tokens.
			name:  "go code file gemma tokens",
			input: "{content:<|\"|>package main\n\nimport (\n\t\"bufio\"\n\t\"fmt\"\n\t\"os\"\n)\n\nfunc main() {\n\treader := bufio.NewReader(os.Stdin)\n\tfmt.Print(\"Enter text: \")\n\ttext, _ := reader.ReadString('\\n')\n\tfmt.Println(text)\n}<|\"|>,filePath:<|\"|>main.go<|\"|>}",
			goCode: map[string]string{
				"content": "file",
			},
		},
		{
			// Full Go source file with unescaped quotes.
			name:  "go code file unescaped quotes",
			input: "{\"content\":\"package main\n\nimport (\n\t\"fmt\"\n)\n\nfunc main() {\n\tboard := [9]string{\"1\", \"2\", \"3\", \"4\", \"5\", \"6\", \"7\", \"8\", \"9\"}\n\tfmt.Println(board)\n}\n\",\"filePath\":\"main.go\"}",
			goCode: map[string]string{
				"content": "file",
			},
		},
		{
			// Full Go source file with Gemma boundary token.
			name:  "go code file gemma boundary token",
			input: "{\"content\":\"package main\n\nimport (\n\t\"fmt\"\n\t\"os\"\n)\n\nvar board = [9]string{\n\t\"1\", \"2\", \"3\",\n\t\"4\", \"5\", \"6\",\n\t\"7\", \"8\", \"9\",\n}\n\nfunc main() {\n\tfor {\n\t\tfmt.Println(\"hello\")\n\t\tos.Exit(0)\n\t}\n}\n<|\"|>,{\"filePath\":\"main.go\"}}",
			goCode: map[string]string{
				"content": "file",
			},
		},
		{
			// Edit snippet: add fmt.Println() after fmt.Printf().
			// Reproduces the production failure where \n\t was
			// double-escaped to \\n\\t, producing literal backslash
			// characters that won't parse as Go.
			name:  "go code snippet edit add println",
			input: "{\"filePath\":\"main.go\",\"newString\":\"\tfmt.Printf(\\\"%s | %s | %s\\\\n\\\", board[6], board[7], board[8])\\n\\tfmt.Println()\",\"oldString\":\"\tfmt.Printf(\\\"%s | %s | %s\\\\n\\\", board[6], board[7], board[8])\"}",
			goCode: map[string]string{
				"newString": "snippet",
				"oldString": "snippet",
			},
		},
		{
			// Edit snippet: multi-statement with if block.
			name:  "go code snippet edit if block",
			input: "{\"filePath\":\"main.go\",\"newString\":\"\\t\\tif winner != \\\"\\\" {\\n\\t\\t\\tfmt.Println()\\n\\t\\t\\tfmt.Println(\\\"done\\\")\\n\\t\\t}\",\"oldString\":\"\\t\\tif winner != \\\"\\\" {\\n\\t\\t\\tfmt.Println(\\\"done\\\")\\n\\t\\t}\"}",
			goCode: map[string]string{
				"newString": "snippet",
				"oldString": "snippet",
			},
		},
		{
			// Edit snippet via Gemma <|"|> tokens with ANSI codes.
			name:  "go code snippet gemma ansi edit",
			input: "{\"filePath\":\"main.go\",\"newString:<|\"|>const (\n\tcolorReset = \"\\033[0m\"\n\tcolorRed   = \"\\033[31m\"\n)\n<|\"|>,\"oldString\":\"import (\\n\\t\\\"fmt\\\"\\n)\"}",
			goCode: map[string]string{
				"newString": "snippet",
			},
		},
		{
			// Full file via Gemma with backslash-n in string literals.
			name:  "go code file gemma preserves format escapes",
			input: "{content:<|\"|>package main\n\nimport (\n\t\"fmt\"\n)\n\nfunc main() {\n\tfor {\n\t\tplayGame()\n\t\tfmt.Print(\"\\nPlay again? (y/n): \")\n\t\tvar choice string\n\t\tfmt.Scanln(&choice)\n\t}\n}<|\"|>,filePath:<|\"|>main.go<|\"|>}",
			goCode: map[string]string{
				"content": "file",
			},
		},
		{
			// Edit snippet: unescaped quotes in both old and new values.
			name:  "go code snippet edit unescaped quotes",
			input: "{\"filePath\":\"main.go\",\"newString\":\"fmt.Printf(\"Player %s wins!\\n\", winner)\n\t\tfmt.Println(\"done\")\",\"oldString\":\"fmt.Printf(\"Player %s wins!\\n\", winner)\"}",
			goCode: map[string]string{
				"newString": "snippet",
				"oldString": "snippet",
			},
		},

		// =================================================================
		// Spurious " after closing <|"|> before comma separator
		// =================================================================
		{
			// Reproduces production failure: model writes <|"|>",filePath:
			// instead of <|"|>,filePath: — the " after the closing gemma
			// token is a spurious artifact from the token boundary. Both
			// keys use bare (unquoted) identifiers with <|"|> values.
			name:  "gemma spurious quote after close before comma key",
			input: "{\"content:<|\"|>package main\n\nimport (\n\t\"bufio\"\n\t\"fmt\"\n\t\"os\"\n\t\"strconv\"\n\t\"strings\"\n)\n\nfunc printBoard(board []string) {\n\tfmt.Println()\n\tfmt.Printf(\"%s | %s | %s\\n\", board[0], board[1], board[2])\n\tfmt.Println(\"----------\")\n\tfmt.Printf(\"%s | %s | %s\\n\", board[3], board[4], board[5])\n\tfmt.Println(\"----------\")\n\tfmt.Printf(\"%s | %s | %s\\n\", board[6], board[7], board[8])\n}\n\nfunc checkWinner(board []string) string {\n\twins := [][]int{\n\t\t{0, 1, 2}, {3, 4, 5}, {6, 7, 8},\n\t\t{0, 3, 6}, {1, 4, 7}, {2, 5, 8},\n\t\t{0, 4, 8}, {2, 4, 6},\n\t}\n\n\tfor _, win := range wins {\n\t\tif board[win[0]] == board[win[1]] && board[win[1]] == board[win[2]] {\n\t\t\tres := board[win[0]]\n\t\t\tif res == \"X\" || res == \"O\" {\n\t\t\t\treturn res\n\t\t\t}\n\t\t}\n\t}\n\treturn \"\"\n}\n\nfunc isDraw(board []string) bool {\n\tfor _, val := range board {\n\t\tif val != \"X\" && val != \"O\" {\n\t\t\treturn false\n\t\t}\n\t}\n\treturn true\n}\n\nfunc getMove(prompt string, board []string) int {\n\treader := bufio.NewReader(os.Stdin)\n\tfor {\n\t\tfmt.Print(prompt)\n\t\tinput, err := reader.ReadString('\\n')\n\t\tif err != nil {\n\t\t\tcontinue\n\t\t}\n\t\tinput = strings.TrimSpace(input)\n\t\tnum, err := strconv.Atoi(input)\n\t\tif err != nil || num < 1 || num > 9 {\n\t\t\tcontinue\n\t\t}\n\t\tidx := num - 1\n\t\tif board[idx] == \"X\" || board[idx] == \"O\" {\n\t\t\tcontinue\n\t\t}\n\t\treturn idx\n\t}\n}\n\nfunc main() {\n\tfor {\n\t\tboard := []string{\"1\", \"2\", \"3\", \"4\", \"5\", \"6\", \"7\", \"8\", \"9\"}\n\t\tcurrentPlayer := \"X\"\n\n\t\tfor {\n\t\t\tprintBoard(board)\n\t\t\tprompt := fmt.Sprintf(\"\\nPlayer %s's turn. Enter a number (1-9): \", currentPlayer)\n\t\t\tidx := getMove(prompt, board)\n\t\t\tboard[idx] = currentPlayer\n\n\t\t\tif w := checkWinner(board); w != \"\" {\n\t\t\t\tprintBoard(board)\n\t\t\t\tfmt.Printf(\"Player %s wins!\\n\", w)\n\t\t\t\tbreak\n\t\t\t}\n\t\t\tif isDraw(board) {\n\t\t\t\tprintBoard(board)\n\t\t\t\tfmt.Println(\"It's a draw!\")\n\t\t\t\tbreak\n\t\t\t}\n\t\t\tif currentPlayer == \"X\" {\n\t\t\t\tcurrentPlayer = \"O\"\n\t\t\t} else {\n\t\t\t\tcurrentPlayer = \"X\"\n\t\t\t}\n\t\t}\n\n\t\tfmt.Print(\"\\nPlay again? (y/n): \")\n\t\tvar answer string\n\t\tfmt.Scanln(&answer)\n\t\tif strings.ToLower(answer) != \"y\" {\n\t\t\tbreak\n\t\t}\n\t}\n}\n<|\"|>\",filePath:<|\"|>examples/talks/tictactoe/main.go<|\"|>}",
			keys:  map[string]string{"content": "package main", "filePath": "examples/talks/tictactoe/main.go"},
			goCode: map[string]string{
				"content": "file",
			},
		},

		// =================================================================
		// Spurious " between closing <|"|> and already-quoted next key
		// =================================================================
		{
			// Reproduces production failure: closing <|"|> is followed by
			// a spurious " before the comma and the next key is already
			// quoted: <|"|>","filePath":"...". The spurious " must be
			// stripped so it doesn't corrupt the content value.
			name:  "gemma paired close token spurious quote before quoted key",
			input: `{"content:<|"|>hello world<|"|>","filePath":"main.go"}`,
			wantExact: map[string]string{
				"content":  "hello world",
				"filePath": "main.go",
			},
		},
		{
			// Same pattern but with Go source containing ANSI escape codes.
			name:  "gemma ansi content paired close then quoted key",
			input: "{\"content:<|\"|>package main\n\nconst colorRed = \"\\033[31m\"\n<|\"|>\",\"filePath\":\"main.go\"}",
			keys:  map[string]string{"content": "package main", "filePath": "main.go"},
		},

		// =================================================================
		// Missing closing quote on key — "oldString: instead of "oldString":
		// =================================================================
		{
			// Reproduces production failure: model outputs "oldString: (colon
			// without closing quote on key) instead of "oldString":" for the
			// edit tool. The newString key is correct but oldString is malformed,
			// causing the JSON parser to absorb the key name into the previous
			// value string, leaving oldString undefined.
			name:  "missing closing quote on key oldString colon",
			input: `{"filePath":"examples/talks/tictactoe/main.go","newString":"const (\n\tcolorReset = \"\\033[0m\"\n\tcolorRed   = \"\\033[31m\"\n\tcolorBlue  = \"\\033[34m\"\n)\n\nfunc formatCell(cell string) string {\n\tswitch cell {\n\tcase \"X\":\n\t\treturn colorBlue + cell + colorReset\n\tcase \"O\":\n\t\treturn colorRed + cell + colorReset\n\tdefault:\n\t\treturn cell\n\t}\n}\n\nfunc printBoard(board []string) {\n\tfmt.Println()\n\tfmt.Printf(\"%s | %s | %s\\n\", formatCell(board[0]), formatCell(board[1]), formatCell(board[2]))\n\tfmt.Println(\"----------\")\n\tfmt.Printf(\"%s | %s | %s\\n\", formatCell(board[3]), formatCell(board[4]), formatCell(board[5]))\n\tfmt.Println(\"----------\")\n\tfmt.Printf(\"%s | %s | %s\\n\", formatCell(board[6]), formatCell(board[7]), formatCell(board[8]))\n}","oldString:"func printBoard(board []string) {\n\tfmt.Println()\n\tfmt.Printf(\"%s | %s | %s\\n\", board[0], board[1], board[2])\n\tfmt.Println(\"----------\")\n\tfmt.Printf(\"%s | %s | %s\\n\", board[3], board[4], board[5])\n\tfmt.Println(\"----------\")\n\tfmt.Printf(\"%s | %s | %s\\n\", board[6], board[7], board[8])\n}","replaceAll":true}`,
			keys:  map[string]string{"filePath": "main.go", "newString": "formatCell", "oldString": "printBoard"},
		},

		// =================================================================
		// Backslash followed by raw control character
		// =================================================================
		{
			// Reproduces production failure: model emits \<TAB> (backslash
			// followed by a literal tab byte 0x09) instead of the two-char
			// sequence \t. The jsonrepair default escape handler wrote the
			// raw tab byte into the output, producing invalid JSON.
			name:  "backslash followed by raw tab byte in unescaped source",
			input: "{\"content\":\"func foo() {\n\treturn bar\n\\\t}\",\"filePath\":\"main.go\"}",
			keys:  map[string]string{"content": "foo", "filePath": "main.go"},
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

			// Verify Go code values parse as valid Go source.
			for key, mode := range tt.goCode {
				val, ok := m[key]
				if !ok {
					t.Errorf("missing key %q for Go parse check\n  repaired: %q", key, got)
					continue
				}

				src, isStr := val.(string)
				if !isStr {
					t.Errorf("key %q is not a string: %T", key, val)
					continue
				}

				if err := parseGoSource(src, mode); err != nil {
					t.Errorf("key %q does not parse as Go (%s):\n  error: %v\n  source:\n%s", key, mode, err, src)
				}
			}
		})
	}
}

func TestRepairWriteAndCompile(t *testing.T) {
	// Model output literal \n and \t (two-char escape sequences), NOT real
	// newline/tab bytes, inside the <|"|> delimited value.
	input := "{\"content:<|\"|>package main\\n\\nimport (\\n\\t\"bufio\"\\n\\t\"fmt\"\\n\\t\"os\"\\n\\t\"strconv\"\\n\\t\"strings\"\\n)\\n\\ntype Game struct {\\n\\tboard  [9]string\\n\\treader *bufio.Reader\\n}\\n\\nfunc NewGame() *Game {\\n\\tg := &Game{\\n\\t\\treader: bufio.NewReader(os.Stdin),\\n\\t}\\n\\tfor i := 0; i < 9; i++ {\\n\\t\\tg.board[i] = strconv.Itoa(i + 1)\\n\\t}\\n\\treturn g\\n}\\n\\nfunc (g *Game) renderBoard() {\\n\\tfmt.Println()\\n\\tfmt.Printf(\"%s | %s | %s\\\\n\", g.board[0], g.board[1], g.board[2])\\n\\tfmt.Println(\"----------\")\\n\\tfmt.Printf(\"%s | %s | %s\\\\n\", g.board[3], g.board[4], g.board[5])\\n\\tfmt.Println(\"----------\")\\n\\tfmt.Printf(\"%s | %s | %s\\\\n\", g.board[6], g.board[7], g.board[8])\\n}\\n\\nfunc (g *Game) checkWinner() string {\\n\\twins := [8][3]int{\\n\\t\\t{0, 1, 2}, {3, 4, 5}, {6, 7, 8},\\n\\t\\t{0, 3, 6}, {1, 4, 7}, {2, 5, 8},\\n\\t\\t{0, 4, 8}, {2, 4, 6},\\n\\t}\\n\\n\\tfor _, combo := range wins {\\n\\t\\tif g.board[combo[0]] == g.board[combo[1]] && g.board[combo[1]] == g.board[combo[2]] {\\n\\t\\t\\treturn g.board[combo[0]]\\n\\t\\t}\\n\\t}\\n\\n\\tfull := true\\n\\tfor i := 0; i < 9; i++ {\\n\\t\\tcell := g.board[i]\\n\\t\\tif cell != \"X\" && cell != \"O\" {\\n\\t\\t\\tfull = false\\n\\t\\t\\tbreak\\n\\t\\t}\\n\\t}\\n\\tif full {\\n\\t\\treturn \"Draw\"\\n\\t}\\n\\n\\treturn \"\"\\n}\\n\\nfunc playerX(g *Game) int {\\n\\tfor {\\n\\t\\tfmt.Print(\"\\\\nPlayer X's turn. Enter a number (1-9): \")\\n\\t\\tinput, err := g.reader.ReadString('\\\\n')\\n\\t\\tif err != nil {\\n\\t\\t\\tcontinue\\n\\t\\t}\\n\\t\\tinput = strings.TrimSpace(input)\\n\\t\\tval, err := strconv.Atoi(input)\\n\\t\\tif err != nil || val < 1 || val > 9 {\\n\\t\\t\\tcontinue\\n\\t\\t}\\n\\n\\t\\tidx := val - 1\\n\\t\\tcell := g.board[idx]\\n\\t\\tisNumber := false\\n\\t\\tfor i := 1; i <= 9; i++ {\\n\\t\\t\\tif cell == strconv.Itoa(i) {\\n\\t\\t\\t\\tisNumber = true\\n\\t\\t\\t\\tbreak\\n\\t\\t\\t}\\n\\t\\t}\\n\\n\\t\\tif isNumber {\\n\\t\\t\\treturn idx\\n\\t\\t}\\n\\t}\\n}\\n\\nfunc main() {\\n\\tfor {\\n\\t\\tgame := NewGame()\\n\\t\\tturnX := true\\n\\n\\t\\tfor {\\n\\t\\t\\tgame.renderBoard()\\n\\n\\t\\t\\tvar idx int\\n\\t\\t\\tif turnX {\\n\\t\\t\\t\\tidx = playerX(game)\\n\\t\\t\\t\\tgame.board[idx] = \"X\"\\n\\t\\t\\t} else {\\n\\t\\t\\t\\tgame.board[idx] = \"O\"\\n\\t\\t\\t}\\n\\n\\t\\t\\tresult := game.checkWinner()\\n\\t\\t\\tif result != \"\" {\\n\\t\\t\\t\\tgame.renderBoard()\\n\\t\\t\\t\\tif result == \"Draw\" {\\n\\t\\t\\t\\t\\tfmt.Println(\"It's a draw!\")\\n\\t\\t\\t\\t} else {\\n\\t\\t\\t\\t\\tfmt.Printf(\"Player %s wins!\\\\n\", result)\\n\\t\\t\\t\\t}\\n\\t\\t\\t\\tbreak\\n\\t\\t\\t}\\n\\t\\t\\tturnX = !turnX\\n\\t\\t}\\n\\n\\t\\tfmt.Print(\"\\\\nPlay again? (y/n): \")\\n\\t\\tinput, err := game.reader.ReadString('\\\\n')\\n\\t\\tif err != nil {\\n\\t\\t\\tbreak\\n\\t\\t}\\n\\t\\tif strings.TrimSpace(strings.ToLower(input)) != \"y\" {\\n\\t\\t\\tbreak\\n\\t\\t}\\n\\t}\\n}\\n<|\"|>,\"filePath\":\"examples/talks/tictactoe/main.go\"}"

	// Step 1: Repair the JSON.
	repaired, err := Repair(input)
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}
	t.Logf("Repaired JSON:\n%s", repaired)

	// Step 2: Unmarshal and extract content.
	var m map[string]any
	if err := json.Unmarshal([]byte(repaired), &m); err != nil {
		t.Fatalf("Unmarshal failed: %v\n  repaired: %s", err, repaired)
	}

	content, ok := m["content"].(string)
	if !ok {
		t.Fatalf("content key missing or not a string: %v", m["content"])
	}

	// Step 3: Verify it compiles as Go.
	if err := parseGoSource(content, "file"); err != nil {
		t.Errorf("Go parse failed: %v\n  content:\n%s", err, content)
	}

	// Step 4: Write to file and run gofmt.
	outFile := t.TempDir() + "/main.go"
	if err := os.WriteFile(outFile, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	t.Logf("Wrote %d bytes to %s", len(content), outFile)
	t.Logf("Content written:\n%s", content)

	out, err := exec.Command("gofmt", "-e", outFile).CombinedOutput()
	if err != nil {
		t.Errorf("gofmt failed: %v\n  output: %s", err, out)
	} else {
		t.Logf("gofmt output:\n%s", out)
	}
}

// parseGoSource parses src as Go source code and verifies it can be formatted
// by gofmt (format.Source). When mode is "file", src must be a complete Go
// source file (with package clause). When mode is "snippet", src is wrapped
// in a function body before parsing.
func parseGoSource(src, mode string) error {
	fset := token.NewFileSet()

	var full string
	switch mode {
	case "file":
		full = src
	case "snippet":
		full = fmt.Sprintf("package main\nfunc _() {\n%s\n}", src)
	default:
		return fmt.Errorf("unknown Go parse mode: %q", mode)
	}

	if _, err := parser.ParseFile(fset, "test.go", full, parser.AllErrors); err != nil {
		return err
	}

	if _, err := format.Source([]byte(full)); err != nil {
		return fmt.Errorf("format.Source: %w", err)
	}

	return nil
}
