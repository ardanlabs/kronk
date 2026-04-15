package jinja_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/jinja"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// =============================================================================
// Test infrastructure
// =============================================================================

var (
	modelPaths map[string]models.Path
	mdls       *models.Models
)

func TestMain(m *testing.M) {
	// Initialize the Kronk library.
	if err := kronk.Init(); err != nil {
		fmt.Printf("Failed to init kronk: %v\n", err)
		os.Exit(1)
	}

	// Initialize the models system.
	var err error
	mdls, err = models.New()
	if err != nil {
		fmt.Printf("Failed to init models: %v\n", err)
		os.Exit(1)
	}

	modelPaths = make(map[string]models.Path)

	modelNames := []string{
		"Qwen3-8B-Q8_0",
		"gpt-oss-20b-Q8_0",
		"Qwen3-VL-30B-A3B-Instruct-Q8_0",
		"Qwen3.5-35B-A3B-Q8_0",
		"Qwen2-Audio-7B.Q8_0",

		// Local-only models (not available in GitHub Actions CI).
		"gemma-4-26B-A4B-it-UD-Q8_K_XL",
		"Ministral-3-14B-Instruct-2512-Q4_0",
		"rnj-1-instruct-Q6_K",
		"LFM2.5-VL-1.6B-Q8_0",
	}

	for _, name := range modelNames {
		if mp, err := mdls.FullPath(name); err == nil {
			modelPaths[name] = mp
			fmt.Printf("Found model: %s\n", name)
		} else {
			fmt.Printf("Model not found (skipping): %s\n", name)
		}
	}

	os.Exit(m.Run())
}

// loadTemplate loads a chat template from a GGUF model by name.
func loadTemplate(t *testing.T, modelName string) string {
	t.Helper()

	mp, ok := modelPaths[modelName]
	if !ok || len(mp.ModelFiles) == 0 {
		t.Skipf("model %s not available", modelName)
	}

	mparams := llama.ModelDefaultParams()
	mparams.VocabOnly = 1

	mdl, err := llama.ModelLoadFromFile(mp.ModelFiles[0], mparams)
	if err != nil {
		t.Fatalf("failed to load model %s: %v", modelName, err)
	}
	defer llama.ModelFree(mdl)

	template := llama.ModelChatTemplate(mdl, "")
	if template == "" {
		template, _ = llama.ModelMetaValStr(mdl, "tokenizer.chat_template")
	}

	if template == "" {
		t.Fatalf("model %s has no chat template", modelName)
	}

	return template
}

// skipCI skips the test when running in GitHub Actions.
func skipCI(t *testing.T) {
	t.Helper()
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("local-only test, skipping in CI")
	}
}

// =============================================================================
// Basic engine tests (no model dependency)
// =============================================================================

func TestBasicRender(t *testing.T) {
	tmpl, err := jinja.Compile("Hello {{ name }}!")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	result, err := tmpl.Render(map[string]any{
		"name": "World",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if result != "Hello World!" {
		t.Errorf("expected 'Hello World!', got %q", result)
	}
}

func TestForLoop(t *testing.T) {
	source := `{% for item in items %}{{ item }}{% if not loop.last %}, {% endif %}{% endfor %}`
	tmpl, err := jinja.Compile(source)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	result, err := tmpl.Render(map[string]any{
		"items": []any{"a", "b", "c"},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if result != "a, b, c" {
		t.Errorf("expected 'a, b, c', got %q", result)
	}
}

func TestIfElse(t *testing.T) {
	source := `{% if x %}yes{% else %}no{% endif %}`
	tmpl, err := jinja.Compile(source)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	result, err := tmpl.Render(map[string]any{"x": true})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if result != "yes" {
		t.Errorf("expected 'yes', got %q", result)
	}

	result, err = tmpl.Render(map[string]any{"x": false})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if result != "no" {
		t.Errorf("expected 'no', got %q", result)
	}
}

func TestTojsonFilter(t *testing.T) {
	source := `{{ data | tojson }}`
	tmpl, err := jinja.Compile(source)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	result, err := tmpl.Render(map[string]any{
		"data": map[string]any{"name": "test", "value": 42},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if !strings.Contains(result, `"name"`) || !strings.Contains(result, `"test"`) {
		t.Errorf("tojson output unexpected: %q", result)
	}
}

func TestNamespace(t *testing.T) {
	source := `{%- set ns = namespace(found=false) -%}
{%- for item in items -%}
{%- if item == "target" -%}
{%- set ns.found = true -%}
{%- endif -%}
{%- endfor -%}
{{ ns.found }}`

	tmpl, err := jinja.Compile(source)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	result, err := tmpl.Render(map[string]any{
		"items": []any{"a", "target", "b"},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	result = strings.TrimSpace(result)
	if result != "True" {
		t.Errorf("expected 'True', got %q", result)
	}
}

func TestStringMethods(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{"strip", `{{ "  hello  ".strip() }}`, "hello"},
		{"split", `{{ "a,b,c".split(",") | join("-") }}`, "a-b-c"},
		{"startswith", `{{ "hello".startswith("hel") }}`, "True"},
		{"endswith", `{{ "hello".endswith("llo") }}`, "True"},
		{"upper", `{{ "hello".upper() }}`, "HELLO"},
		{"lower", `{{ "HELLO".lower() }}`, "hello"},
		{"replace", `{{ "hello world".replace("world", "jinja") }}`, "hello jinja"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := jinja.Compile(tt.source)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			result, err := tmpl.Render(nil)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if result != tt.want {
				t.Errorf("expected %q, got %q", tt.want, result)
			}
		})
	}
}

func TestDictMethods(t *testing.T) {
	source := `{%- for key, value in data.items() -%}{{ key }}={{ value }} {% endfor -%}`
	tmpl, err := jinja.Compile(source)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	result, err := tmpl.Render(map[string]any{
		"data": map[string]any{"a": 1, "b": 2},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	// Keys are sorted for deterministic output.
	if !strings.Contains(result, "a=1") || !strings.Contains(result, "b=2") {
		t.Errorf("unexpected dict items output: %q", result)
	}
}

func TestInlineIf(t *testing.T) {
	source := `{{ "yes" if x else "no" }}`
	tmpl, err := jinja.Compile(source)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	result, err := tmpl.Render(map[string]any{"x": true})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if result != "yes" {
		t.Errorf("expected 'yes', got %q", result)
	}
}

func TestIsTests(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{"is defined", `{{ "yes" if x is defined else "no" }}`, "yes"},
		{"is not defined", `{{ "yes" if y is not defined else "no" }}`, "yes"},
		{"is string", `{{ "yes" if x is string else "no" }}`, "yes"},
		{"is none", `{{ "yes" if n is none else "no" }}`, "yes"},
		{"is true", `{{ "yes" if t is true else "no" }}`, "yes"},
		{"is false", `{{ "yes" if f is false else "no" }}`, "yes"},
	}

	data := map[string]any{
		"x": "hello",
		"n": nil,
		"t": true,
		"f": false,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := jinja.Compile(tt.source)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			result, err := tmpl.Render(data)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if result != tt.want {
				t.Errorf("expected %q, got %q", tt.want, result)
			}
		})
	}
}

func TestWhitespaceControl(t *testing.T) {
	source := "A\n  {%- if true %}\nB\n  {%- endif %}\nC"
	tmpl, err := jinja.Compile(source)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	result, err := tmpl.Render(nil)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if !strings.Contains(result, "A") && !strings.Contains(result, "B") && !strings.Contains(result, "C") {
		t.Errorf("whitespace control produced unexpected output: %q", result)
	}
}

func TestSlicing(t *testing.T) {
	source := `{{ items[::-1] | join(",") }}`
	tmpl, err := jinja.Compile(source)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	result, err := tmpl.Render(map[string]any{
		"items": []any{"a", "b", "c"},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if result != "c,b,a" {
		t.Errorf("expected 'c,b,a', got %q", result)
	}
}

func TestStringConcat(t *testing.T) {
	source := `{{ "Hello" ~ " " ~ "World" }}`
	tmpl, err := jinja.Compile(source)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	result, err := tmpl.Render(nil)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if result != "Hello World" {
		t.Errorf("expected 'Hello World', got %q", result)
	}
}

func TestLoopVariables(t *testing.T) {
	source := `{%- for item in items -%}{{ loop.index0 }}:{{ item }} {% endfor -%}`
	tmpl, err := jinja.Compile(source)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	result, err := tmpl.Render(map[string]any{
		"items": []any{"x", "y", "z"},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if result != "0:x 1:y 2:z " {
		t.Errorf("expected '0:x 1:y 2:z ', got %q", result)
	}
}

// =============================================================================
// Chat template compilation tests (verify templates from GGUF models compile)
// =============================================================================

func TestCompile_Qwen3_8B(t *testing.T) {
	template := loadTemplate(t, "Qwen3-8B-Q8_0")
	_, err := jinja.Compile(template)
	if err != nil {
		t.Fatalf("failed to compile Qwen3-8B template: %v", err)
	}
}

func TestCompile_GPT_OSS_20B(t *testing.T) {
	template := loadTemplate(t, "gpt-oss-20b-Q8_0")
	_, err := jinja.Compile(template)
	if err != nil {
		t.Fatalf("failed to compile gpt-oss-20b template: %v", err)
	}
}

func TestCompile_Qwen3_VL_30B(t *testing.T) {
	template := loadTemplate(t, "Qwen3-VL-30B-A3B-Instruct-Q8_0")
	_, err := jinja.Compile(template)
	if err != nil {
		t.Fatalf("failed to compile Qwen3-VL-30B template: %v", err)
	}
}

func TestCompile_Qwen35_35B(t *testing.T) {
	template := loadTemplate(t, "Qwen3.5-35B-A3B-Q8_0")
	_, err := jinja.Compile(template)
	if err != nil {
		t.Fatalf("failed to compile Qwen3.5-35B template: %v", err)
	}
}

func TestCompile_Qwen2_Audio(t *testing.T) {
	template := loadTemplate(t, "Qwen2-Audio-7B.Q8_0")
	_, err := jinja.Compile(template)
	if err != nil {
		t.Fatalf("failed to compile Qwen2-Audio template: %v", err)
	}
}

// =============================================================================
// Chat template render tests with simple messages
// =============================================================================

func TestRender_Qwen3_8B_SimpleChat(t *testing.T) {
	template := loadTemplate(t, "Qwen3-8B-Q8_0")

	tmpl, err := jinja.Compile(template)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	data := map[string]any{
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": "Hello!",
			},
		},
		"add_generation_prompt": true,
		"enable_thinking":       true,
	}

	result, err := tmpl.Render(data)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	t.Logf("Qwen3-8B output:\n%s", result)

	// Verify key structural elements.
	if !strings.Contains(result, "Hello!") {
		t.Error("output missing user message content")
	}
	if !strings.Contains(result, "user") {
		t.Error("output missing user role marker")
	}
	if !strings.Contains(result, "assistant") {
		t.Error("output missing assistant generation prompt")
	}
}

func TestRender_GPT_OSS_20B_SimpleChat(t *testing.T) {
	template := loadTemplate(t, "gpt-oss-20b-Q8_0")

	tmpl, err := jinja.Compile(template)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	data := map[string]any{
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": "Hello!",
			},
		},
		"add_generation_prompt": true,
	}

	result, err := tmpl.Render(data)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	t.Logf("gpt-oss-20b output:\n%s", result)

	if !strings.Contains(result, "Hello!") {
		t.Error("output missing user message content")
	}
}

func TestRender_Qwen35_35B_SimpleChat(t *testing.T) {
	template := loadTemplate(t, "Qwen3.5-35B-A3B-Q8_0")

	tmpl, err := jinja.Compile(template)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	data := map[string]any{
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": "Hello!",
			},
		},
		"add_generation_prompt": true,
		"enable_thinking":       true,
	}

	result, err := tmpl.Render(data)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	t.Logf("Qwen3.5-35B output:\n%s", result)

	if !strings.Contains(result, "Hello!") {
		t.Error("output missing user message content")
	}
}

// =============================================================================
// Tool calling template tests
// =============================================================================

func TestRender_Qwen3_8B_ToolCalling(t *testing.T) {
	template := loadTemplate(t, "Qwen3-8B-Q8_0")

	tmpl, err := jinja.Compile(template)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	data := map[string]any{
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": "What is the weather in London?",
			},
		},
		"tools": []any{
			map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        "get_weather",
					"description": "Get the current weather for a location",
					"parameters": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"location": map[string]any{
								"type":        "string",
								"description": "The location to get weather for",
							},
						},
						"required": []any{"location"},
					},
				},
			},
		},
		"add_generation_prompt": true,
		"enable_thinking":       true,
	}

	result, err := tmpl.Render(data)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	t.Logf("Qwen3-8B tool calling output:\n%s", result)

	if !strings.Contains(result, "get_weather") {
		t.Error("output missing tool name")
	}
	if !strings.Contains(result, "London") {
		t.Error("output missing user message")
	}
}

func TestRender_GPT_OSS_20B_ToolCalling(t *testing.T) {
	template := loadTemplate(t, "gpt-oss-20b-Q8_0")

	tmpl, err := jinja.Compile(template)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	data := map[string]any{
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": "What is the weather in London?",
			},
		},
		"tools": []any{
			map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        "get_weather",
					"description": "Get the current weather for a location",
					"parameters": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"location": map[string]any{
								"type":        "string",
								"description": "The location to get weather for",
							},
						},
						"required": []any{"location"},
					},
				},
			},
		},
		"add_generation_prompt": true,
	}

	result, err := tmpl.Render(data)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	t.Logf("gpt-oss-20b tool calling output:\n%s", result)

	if !strings.Contains(result, "get_weather") {
		t.Error("output missing tool name")
	}
}

func TestRender_Qwen35_35B_ToolCalling(t *testing.T) {
	template := loadTemplate(t, "Qwen3.5-35B-A3B-Q8_0")

	tmpl, err := jinja.Compile(template)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	data := map[string]any{
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": "What is the weather in London?",
			},
		},
		"tools": []any{
			map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        "get_weather",
					"description": "Get the current weather for a location",
					"parameters": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"location": map[string]any{
								"type":        "string",
								"description": "The location to get weather for",
							},
						},
						"required": []any{"location"},
					},
				},
			},
		},
		"add_generation_prompt": true,
		"enable_thinking":       true,
	}

	result, err := tmpl.Render(data)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	t.Logf("Qwen3.5-35B tool calling output:\n%s", result)

	if !strings.Contains(result, "get_weather") {
		t.Error("output missing tool name")
	}
}

// =============================================================================
// Local-only model tests (skipped in GitHub Actions CI)
// =============================================================================

func TestCompile_Local_Gemma4(t *testing.T) {
	skipCI(t)
	template := loadTemplate(t, "gemma-4-26B-A4B-it-UD-Q8_K_XL")
	_, err := jinja.Compile(template)
	if err != nil {
		t.Fatalf("failed to compile gemma-4 template: %v", err)
	}
}

func TestCompile_Local_Ministral3(t *testing.T) {
	skipCI(t)
	template := loadTemplate(t, "Ministral-3-14B-Instruct-2512-Q4_0")
	_, err := jinja.Compile(template)
	if err != nil {
		t.Fatalf("failed to compile Ministral-3 template: %v", err)
	}
}

func TestCompile_Local_RNJ1(t *testing.T) {
	skipCI(t)
	template := loadTemplate(t, "rnj-1-instruct-Q6_K")
	_, err := jinja.Compile(template)
	if err != nil {
		t.Fatalf("failed to compile rnj-1 template: %v", err)
	}
}

func TestCompile_Local_LFM25(t *testing.T) {
	skipCI(t)
	template := loadTemplate(t, "LFM2.5-VL-1.6B-Q8_0")
	_, err := jinja.Compile(template)
	if err != nil {
		t.Fatalf("failed to compile LFM2.5-VL template: %v", err)
	}
}

func TestRender_Local_Gemma4_SimpleChat(t *testing.T) {
	skipCI(t)
	template := loadTemplate(t, "gemma-4-26B-A4B-it-UD-Q8_K_XL")

	tmpl, err := jinja.Compile(template)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	data := map[string]any{
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": "Hello!",
			},
		},
		"add_generation_prompt": true,
		"enable_thinking":       true,
	}

	result, err := tmpl.Render(data)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	t.Logf("gemma-4 output:\n%s", result)

	if !strings.Contains(result, "Hello!") {
		t.Error("output missing user message content")
	}
	if !strings.Contains(result, "model") || !strings.Contains(result, "user") {
		t.Error("output missing role markers")
	}
}

func TestRender_Local_Ministral3_SimpleChat(t *testing.T) {
	skipCI(t)
	template := loadTemplate(t, "Ministral-3-14B-Instruct-2512-Q4_0")

	tmpl, err := jinja.Compile(template)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	data := map[string]any{
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": "Hello!",
			},
		},
		"add_generation_prompt": true,
	}

	result, err := tmpl.Render(data)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	t.Logf("Ministral-3 output:\n%s", result)

	if !strings.Contains(result, "Hello!") {
		t.Error("output missing user message content")
	}
}

func TestRender_Local_RNJ1_SimpleChat(t *testing.T) {
	skipCI(t)
	template := loadTemplate(t, "rnj-1-instruct-Q6_K")

	tmpl, err := jinja.Compile(template)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	data := map[string]any{
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": "Hello!",
			},
		},
		"add_generation_prompt": true,
	}

	result, err := tmpl.Render(data)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	t.Logf("rnj-1 output:\n%s", result)

	if !strings.Contains(result, "Hello!") {
		t.Error("output missing user message content")
	}
}

func TestRender_Local_LFM25_SimpleChat(t *testing.T) {
	skipCI(t)
	template := loadTemplate(t, "LFM2.5-VL-1.6B-Q8_0")

	tmpl, err := jinja.Compile(template)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	data := map[string]any{
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": "Hello!",
			},
		},
		"add_generation_prompt": true,
	}

	result, err := tmpl.Render(data)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	t.Logf("LFM2.5-VL output:\n%s", result)

	if !strings.Contains(result, "Hello!") {
		t.Error("output missing user message content")
	}
}

func TestRender_Local_Gemma4_ToolCalling(t *testing.T) {
	skipCI(t)
	template := loadTemplate(t, "gemma-4-26B-A4B-it-UD-Q8_K_XL")

	tmpl, err := jinja.Compile(template)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	data := map[string]any{
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": "What is the weather in London?",
			},
		},
		"tools": []any{
			map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        "get_weather",
					"description": "Get the current weather for a location",
					"parameters": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"location": map[string]any{
								"type":        "string",
								"description": "The location to get weather for",
							},
						},
						"required": []any{"location"},
					},
				},
			},
		},
		"add_generation_prompt": true,
		"enable_thinking":       true,
	}

	result, err := tmpl.Render(data)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	t.Logf("gemma-4 tool calling output:\n%s", result)

	if !strings.Contains(result, "get_weather") {
		t.Error("output missing tool name")
	}
	if !strings.Contains(result, "London") {
		t.Error("output missing user message")
	}
}

// TestRender_Local_Gemma4_MultiTurnToolCall exercises the exact scenario that
// fails in gonja: multi-turn conversation with assistant tool_calls followed by
// tool responses. The template uses message.get('reasoning'), message.get('tool_calls'),
// message.get('content'), etc.
func TestRender_Local_Gemma4_MultiTurnToolCall(t *testing.T) {
	skipCI(t)
	template := loadTemplate(t, "gemma-4-26B-A4B-it-UD-Q8_K_XL")

	tmpl, err := jinja.Compile(template)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	data := map[string]any{
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": "What is the weather in London?",
			},
			map[string]any{
				"role":    "assistant",
				"content": "",
				"tool_calls": []any{
					map[string]any{
						"id":   "call_1",
						"type": "function",
						"function": map[string]any{
							"name":      "get_weather",
							"arguments": `{"location": "London"}`,
						},
					},
				},
			},
			map[string]any{
				"role":         "tool",
				"tool_call_id": "call_1",
				"name":         "get_weather",
				"content":      `{"temperature": 15, "condition": "cloudy"}`,
			},
			map[string]any{
				"role":    "assistant",
				"content": "The weather in London is 15°C and cloudy.",
			},
			map[string]any{
				"role":    "user",
				"content": "What about Paris?",
			},
		},
		"tools": []any{
			map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        "get_weather",
					"description": "Get the current weather for a location",
					"parameters": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"location": map[string]any{
								"type":        "string",
								"description": "The location to get weather for",
							},
						},
						"required": []any{"location"},
					},
				},
			},
		},
		"add_generation_prompt": true,
		"enable_thinking":       true,
	}

	result, err := tmpl.Render(data)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	t.Logf("gemma-4 multi-turn tool call output:\n%s", result)

	if !strings.Contains(result, "get_weather") {
		t.Error("output missing tool name")
	}
	if !strings.Contains(result, "London") {
		t.Error("output missing first user message")
	}
	if !strings.Contains(result, "Paris") {
		t.Error("output missing second user message")
	}
	if !strings.Contains(result, "tool_call") || !strings.Contains(result, "tool_response") {
		t.Error("output missing tool call/response markers")
	}
}

// TestFromGoValue_NamedTypes verifies that FromGoValue handles named types
// like model.D (map[string]any) and []model.D ([]map[string]any) that don't
// match bare type switches. This is the exact pattern that caused empty prompts
// in production — messages and tools passed as []D silently became Undefined.
func TestFromGoValue_NamedTypes(t *testing.T) {
	// Use model.D directly — it's a defined type (not an alias), so
	// []model.D won't match []map[string]any in a type switch.
	source := `{%- for msg in messages -%}[{{ msg.role }}:{{ msg.content }}]{% endfor -%}`
	tmpl, err := jinja.Compile(source)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	messages := []model.D{
		{"role": "system", "content": "You are helpful."},
		{"role": "user", "content": "Hello!"},
	}

	result, err := tmpl.Render(map[string]any{
		"messages": messages,
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	expected := "[system:You are helpful.][user:Hello!]"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// TestFromGoValue_NamedMapType verifies that a named map type (model.D)
// is correctly converted via the reflection fallback.
func TestFromGoValue_NamedMapType(t *testing.T) {
	source := `{{ d.name }}`
	tmpl, err := jinja.Compile(source)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	result, err := tmpl.Render(map[string]any{
		"d": model.D{"name": "Alice"},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if result != "Alice" {
		t.Errorf("expected %q, got %q", "Alice", result)
	}
}

// TestDictGet verifies dict.get() works correctly — the exact method that
// fails in gonja with "unknown method 'get' for ”".
func TestDictGet(t *testing.T) {
	tests := []struct {
		name   string
		source string
		data   map[string]any
		want   string
	}{
		{
			name:   "get existing key",
			source: `{{ d.get('name') }}`,
			data:   map[string]any{"d": map[string]any{"name": "Alice"}},
			want:   "Alice",
		},
		{
			name:   "get missing key returns none",
			source: `{{ d.get('missing') }}`,
			data:   map[string]any{"d": map[string]any{"name": "Alice"}},
			want:   "",
		},
		{
			name:   "get missing key with default",
			source: `{{ d.get('missing', 'fallback') }}`,
			data:   map[string]any{"d": map[string]any{"name": "Alice"}},
			want:   "fallback",
		},
		{
			name:   "get in or chain",
			source: `{{ d.get('a') or d.get('b') or 'none' }}`,
			data:   map[string]any{"d": map[string]any{"b": "found_b"}},
			want:   "found_b",
		},
		{
			name:   "get in if condition",
			source: `{% if d.get('tool_calls') %}yes{% else %}no{% endif %}`,
			data:   map[string]any{"d": map[string]any{"name": "Alice"}},
			want:   "no",
		},
		{
			name:   "get truthy check",
			source: `{% if d.get('tool_calls') %}yes{% else %}no{% endif %}`,
			data:   map[string]any{"d": map[string]any{"tool_calls": []any{"call1"}}},
			want:   "yes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := jinja.Compile(tt.source)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			result, err := tmpl.Render(tt.data)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if result != tt.want {
				t.Errorf("expected %q, got %q", tt.want, result)
			}
		})
	}
}
