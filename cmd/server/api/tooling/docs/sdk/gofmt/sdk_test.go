package gofmt

import (
	"strings"
	"testing"
)

func TestParseTypePreservesParagraphs(t *testing.T) {
	scanner := lineScanner{
		lines: []string{
			"}",
			"    Config describes the configuration across",
			"    wrapped lines.",
			"",
			"    NThreads describes the thread count.",
			"",
			"func NewConfig() Config",
		},
	}

	got := parseType("type Config struct {", &scanner)
	want := "Config describes the configuration across wrapped lines.\n\nNThreads describes the thread count."
	if got.comment != want {
		t.Errorf("comment: got %q, want %q", got.comment, want)
	}
	if next := scanner.peek(); next != "func NewConfig() Config" {
		t.Errorf("next line: got %q, want %q", next, "func NewConfig() Config")
	}
}

func TestWriteDescriptionRendersParagraphs(t *testing.T) {
	var b strings.Builder
	writeDescription(&b, "  ", "doc-description", "First paragraph.\n\nSecond <paragraph>.")

	want := "  <p className=\"doc-description\">First paragraph.</p>\n" +
		"  <p className=\"doc-description\">Second &lt;paragraph&gt;.</p>\n"
	if got := b.String(); got != want {
		t.Errorf("description: got %q, want %q", got, want)
	}
}
