package source

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTemplateVariables(t *testing.T) {
	for name, tt := range map[string]struct {
		script string
		want   string
	}{
		"none": {
			script: "Hello {# enable_thinking #} world",
			want:   "",
		},
		"thinking and effort": {
			script: "{%- if enable_thinking is defined and enable_thinking is false %}<think></think>{% endif %}" +
				"{{ reasoning_effort | default('medium') }}",
			want: "enable_thinking,reasoning_effort",
		},
		"read before its default is set": {
			script: "{% if add_generation_prompt is not defined %}{% set add_generation_prompt = true %}{% endif %}",
			want:   "add_generation_prompt",
		},
		"locals, attributes, filters, tests and calls": {
			script: "{%- set ns = namespace(found=false, last=-1) %}" +
				"{%- for message in messages[::-1] %}{{ message.content | trim }}{% set ns.found = true %}{% endfor %}" +
				"{%- macro render(item, sep=', ') %}{{ item | join(sep) }}{% endmacro %}" +
				"{{ render(tools) }}{% if tools is iterable and tools is not none %}{{ raise_exception('bad') }}{% endif %}" +
				"{{ loop.index if loop is defined }}",
			want: "messages,tools",
		},
		"strings, comments and raw": {
			script: "{{ 'enable_thinking' }}{# reasoning_effort #}{% raw %}{{ preserve_thinking }}{% endraw %}{{ bos_token }}",
			want:   "bos_token",
		},
		"set from itself": {
			script: "{%- set enable_thinking = enable_thinking | default(false) -%}{% if enable_thinking %}x{% endif %}",
			want:   "enable_thinking",
		},
		"for with two targets": {
			script: "{% for k, v in kwargs.items() %}{{ k }}={{ v }}{% endfor %}",
			want:   "kwargs",
		},
	} {
		if got := strings.Join(templateVariables(tt.script), ","); got != tt.want {
			t.Errorf("%s: TemplateVariables = %q, want %q", name, got, tt.want)
		}
	}
}

func TestFindTemplateFile(t *testing.T) {
	base := t.TempDir()
	jinjaDir := filepath.Join(base, "jinja")
	if err := os.MkdirAll(jinjaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(path, body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(jinjaDir, "Exact.Q8_0.jinja"), "exact")
	write(filepath.Join(jinjaDir, "Stripped.jinja"), "stripped")
	own := filepath.Join(base, "own.jinja")
	write(own, "own")

	for _, tt := range []struct {
		file, id string
		want     string
		found    bool
		err      bool
	}{
		{own, "Exact.Q8_0", "own", true, false},                            // template: wins
		{"", "Exact.Q8_0", "exact", true, false},                           // exact id
		{"", "Stripped-UD-Q4_K_XL", "stripped", true, false},               // quant suffix removed
		{"", "Missing", "", false, false},                                  // the GGUF's template
		{filepath.Join(base, "nope.jinja"), "Exact.Q8_0", "", false, true}, // as at model load
	} {
		got, found, err := findTemplateFile(tt.file, tt.id, base)
		if got != tt.want || found != tt.found || (err != nil) != tt.err {
			t.Errorf("findTemplateFile(%q, %q) = %q, %v, %v", tt.file, tt.id, got, found, err)
		}
	}
}
