package questions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/kronk/config/source"
)

func TestTemplateQuestions(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "jinja"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, script string) string {
		t.Helper()
		p := filepath.Join(base, name)
		if err := os.WriteFile(p, []byte(script), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	const gguf = "{{ messages }}"
	write("jinja/Model-Q8_0.jinja", "{% if enable_thinking %}{% endif %}{{ preserve_thinking }}")
	effort := write("effort.jinja", "{{ reasoning_effort }}")

	qs := Chat()
	shown := func(f source.Facts, a *Answers, key string) bool { return a.Applies(f, a.ByKey[key]) }

	f := sampleFacts()
	f.Templates = source.NewTemplates(source.Choice{ID: "owner/Model-Q8_0"}, base, true, gguf)
	a := NewAnswers(qs)

	// This machine: the jinja directory's template is used over the GGUF's.
	if !shown(f, a, "sampling-parameters.enable_thinking") || shown(f, a, "sampling-parameters.reasoning_effort") {
		t.Fatal("jinja directory template: thinking asked, reasoning not")
	}
	if got := strings.Join(f.Template(a.Value("template")).Custom(), ","); got != "preserve_thinking" {
		t.Fatalf("custom variables = %q", got)
	}

	// The template answer wins, as template: does on the server.
	*a.Sel["template"] = effort
	if shown(f, a, "sampling-parameters.enable_thinking") || !shown(f, a, "sampling-parameters.reasoning_effort") {
		t.Fatal("template file: reasoning asked, thinking not")
	}

	// A template file that can't be read here: ask both.
	*a.Sel["template"] = filepath.Join(base, "missing.jinja")
	if !shown(f, a, "sampling-parameters.enable_thinking") || !shown(f, a, "sampling-parameters.reasoning_effort") {
		t.Fatal("unreadable template file: both asked")
	}

	// Another machine: this machine's jinja directory doesn't apply.
	f.Templates = source.NewTemplates(source.Choice{ID: "owner/Model-Q8_0"}, base, false, gguf)
	*a.Sel["template"] = ""
	if shown(f, a, "sampling-parameters.enable_thinking") {
		t.Fatal("another machine: the GGUF template, which doesn't read enable_thinking")
	}
}

func TestNUMAQuestion(t *testing.T) {
	f := sampleFacts()
	a := NewAnswers(Chat())
	*a.Sel["ngpu-layers"] = "-1" // the model runs on the CPU

	for nodes, want := range map[int]bool{0: true, 1: false, 2: true} {
		f.NUMANodes = nodes
		if got := a.Applies(f, a.ByKey["numa"]); got != want {
			t.Errorf("%d NUMA nodes: asked = %v, want %v", nodes, got, want)
		}
	}
}
