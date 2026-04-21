package mcpapp

import (
	"testing"
)

func TestTieredReplace_ExactMatch(t *testing.T) {
	content := "func main() {\n\tfmt.Println(\"hello\")\n}\n"
	oldStr := "func main() {\n\tfmt.Println(\"hello\")\n}"
	newStr := "func main() {\n\tfmt.Println(\"world\")\n}"

	result, ok := tieredReplace(content, oldStr, newStr)
	if !ok {
		t.Fatal("expected match")
	}

	expected := "func main() {\n\tfmt.Println(\"world\")\n}\n"
	if result != expected {
		t.Errorf("got:\n%q\nwant:\n%q", result, expected)
	}
}

func TestTieredReplace_LineEndingNormalization(t *testing.T) {
	content := "line1\r\nline2\r\nline3\r\n"
	oldStr := "line1\nline2\nline3"
	newStr := "replaced"

	result, ok := tieredReplace(content, oldStr, newStr)
	if !ok {
		t.Fatal("expected match with line ending normalization")
	}

	if result != "replaced\r\n" {
		t.Errorf("got: %q", result)
	}
}

func TestTieredReplace_IndentInsensitive(t *testing.T) {
	content := "package main\n\n\tfunc hello() {\n\t\tfmt.Println(\"hi\")\n\t}\n"
	oldStr := "func hello() {\n    fmt.Println(\"hi\")\n}"
	newStr := "func hello() {\n\tfmt.Println(\"bye\")\n}"

	result, ok := tieredReplace(content, oldStr, newStr)
	if !ok {
		t.Fatal("expected match with indentation-insensitive matching")
	}

	if result != "package main\n\nfunc hello() {\n\tfmt.Println(\"bye\")\n}\n" {
		t.Errorf("got:\n%q", result)
	}
}

func TestTieredReplace_NoMatch(t *testing.T) {
	content := "func main() {\n\tfmt.Println(\"hello\")\n}\n"
	oldStr := "func doesNotExist() {}"
	newStr := "replaced"

	_, ok := tieredReplace(content, oldStr, newStr)
	if ok {
		t.Fatal("expected no match")
	}
}

func TestTieredReplace_AmbiguousMatch(t *testing.T) {
	content := "fmt.Println(\"a\")\nfmt.Println(\"a\")\n"
	oldStr := "fmt.Println(\"a\")"
	newStr := "fmt.Println(\"b\")"

	_, ok := tieredReplace(content, oldStr, newStr)
	if ok {
		t.Fatal("expected ambiguous match to fail")
	}
}

func TestTieredReplace_IndentInsensitive_Ambiguous(t *testing.T) {
	content := "\tfmt.Println(\"a\")\n    fmt.Println(\"a\")\n"
	oldStr := "fmt.Println(\"a\")"
	newStr := "fmt.Println(\"b\")"

	_, ok := tieredReplace(content, oldStr, newStr)
	if ok {
		t.Fatal("expected ambiguous indent-insensitive match to fail")
	}
}
