package model

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/hybridgroup/yzma/pkg/llama"
)

const grammarTestModelFile = "Qwen3-0.6B-Q8_0.gguf"

func TestGrammarSamplerInitialization(t *testing.T) {
	gs, err := newGrammarSampler(0, "")
	if err != nil {
		t.Fatalf("newGrammarSampler: got %v, want nil", err)
	}
	if gs != nil {
		t.Fatalf("newGrammarSampler: got %v, want nil", gs)
	}

	gs, err = newGrammarSampler(0, `root ::= "ok"`)
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("newGrammarSampler: got %v, want ErrInvalidRequest", err)
	}
	if gs != nil {
		t.Fatalf("newGrammarSampler: got %v, want nil", gs)
	}
}

func TestFromJSONSchema_SimpleObject(t *testing.T) {
	schema := D{
		"type": "object",
		"properties": D{
			"name": D{"type": "string"},
			"age":  D{"type": "integer"},
		},
		"required": []string{"name", "age"},
	}

	grammar, err := fromJSONSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(grammar, "root ::=") {
		t.Error("grammar should contain root rule")
	}
	if !strings.Contains(grammar, `"name"`) {
		t.Error("grammar should contain name property")
	}
	if !strings.Contains(grammar, `"age"`) {
		t.Error("grammar should contain age property")
	}
}

func TestFromJSONSchema_WithEnum(t *testing.T) {
	tests := []struct {
		name       string
		schema     D
		wantRules  []string
		rootHasAlt bool
	}{
		{
			name: "object property",
			schema: D{
				"type": "object",
				"properties": D{
					"verdict": D{
						"type": "string",
						"enum": []any{"yes", "no", "maybe"},
					},
				},
				"required": []string{"verdict"},
			},
			wantRules: []string{
				`root ::= "{" ws "\"" "verdict" "\"" ws ":" ws root-verdict ws "}"`,
				`root-verdict ::= ( "\"" "yes" "\"" | "\"" "no" "\"" | "\"" "maybe" "\"" )`,
			},
		},
		{
			name: "array items",
			schema: D{
				"type": "array",
				"items": D{
					"type": "string",
					"enum": []any{"yes", "no"},
				},
			},
			wantRules: []string{
				`root ::= "[" ws ( root-item ( ws "," ws root-item )* )? ws "]"`,
				`root-item ::= ( "\"" "yes" "\"" | "\"" "no" "\"" )`,
			},
		},
		{
			name: "top level",
			schema: D{
				"type": "string",
				"enum": []any{"yes", "no"},
			},
			wantRules: []string{
				`root ::= ( "\"" "yes" "\"" | "\"" "no" "\"" )`,
			},
			rootHasAlt: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			grammar, err := fromJSONSchema(tt.schema)
			if err != nil {
				t.Fatalf("fromJSONSchema: unexpected error: %v", err)
			}

			for _, want := range tt.wantRules {
				if !strings.Contains(grammar, want) {
					t.Errorf("grammar: got\n%s\nwant rule %q", grammar, want)
				}
			}

			root, _, _ := strings.Cut(grammar, "\n")
			if got := strings.Contains(root, " | "); got != tt.rootHasAlt {
				t.Errorf("root alternation: got %t, want %t in %q", got, tt.rootHasAlt, root)
			}
		})
	}
}

func TestFromJSONSchema_EnumGrammarInitializes(t *testing.T) {
	schema := D{
		"type": "object",
		"properties": D{
			"verdict": D{
				"type": "string",
				"enum": []any{"yes", "no", "maybe"},
			},
		},
		"required": []string{"verdict"},
	}

	grammar, err := fromJSONSchema(schema)
	if err != nil {
		t.Fatalf("fromJSONSchema: unexpected error: %v", err)
	}

	pattern := filepath.Join(defaults.BaseDir(""), "models", "*", "*", grammarTestModelFile)
	modelFiles, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("find model vocabulary: %v", err)
	}
	if len(modelFiles) == 0 {
		t.Skipf("model %s not downloaded", grammarTestModelFile)
	}

	if err := llama.Load(libs.Path("")); err != nil {
		t.Fatalf("load llama library: %v", err)
	}
	llama.Init()
	llama.LogSet(llama.LogSilent())

	params := llama.ModelDefaultParams()
	params.VocabOnly = 1

	mdl, err := llama.ModelLoadFromFile(modelFiles[0], params)
	if err != nil {
		t.Fatalf("load model vocabulary: %v", err)
	}
	t.Cleanup(func() {
		if err := llama.ModelFree(mdl); err != nil {
			t.Errorf("free model: %v", err)
		}
	})

	sampler := llama.SamplerInitGrammar(llama.ModelGetVocab(mdl), grammar, "root")
	if sampler == 0 {
		t.Fatal("SamplerInitGrammar: got zero sampler, want initialized sampler")
	}
	t.Cleanup(func() {
		llama.SamplerFree(sampler)
	})
}

func TestFromJSONSchema_Array(t *testing.T) {
	schema := D{
		"type": "array",
		"items": D{
			"type": "string",
		},
	}

	grammar, err := fromJSONSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(grammar, "root ::=") {
		t.Error("grammar should contain root rule")
	}
	if !strings.Contains(grammar, "[") {
		t.Error("grammar should contain array brackets")
	}
}

func TestFromJSONSchema_NestedObject(t *testing.T) {
	schema := D{
		"type": "object",
		"properties": D{
			"user": D{
				"type": "object",
				"properties": D{
					"email": D{"type": "string"},
				},
			},
		},
	}

	grammar, err := fromJSONSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(grammar, "root ::=") {
		t.Error("grammar should contain root rule")
	}
	if !strings.Contains(grammar, `"user"`) {
		t.Error("grammar should contain user property")
	}
}

func TestFromJSONSchema_BooleanType(t *testing.T) {
	schema := D{
		"type": "boolean",
	}

	grammar, err := fromJSONSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(grammar, "root ::= boolean") {
		t.Errorf("expected root to be boolean, got: %s", grammar)
	}
}

func TestFromJSONSchema_MapStringAny(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{"type": "integer"},
		},
	}

	grammar, err := fromJSONSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(grammar, "root ::=") {
		t.Error("grammar should contain root rule")
	}
}

func TestFromResponseFormat_Text(t *testing.T) {
	grammar, err := fromResponseFormat(D{"type": "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if grammar != "" {
		t.Errorf("expected empty grammar for text, got %q", grammar)
	}
}

func TestFromResponseFormat_Empty(t *testing.T) {
	grammar, err := fromResponseFormat(D{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if grammar != "" {
		t.Errorf("expected empty grammar for empty type, got %q", grammar)
	}
}

func TestFromResponseFormat_NotAMap(t *testing.T) {
	grammar, err := fromResponseFormat("text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if grammar != "" {
		t.Errorf("expected empty grammar for non-map input, got %q", grammar)
	}
}

func TestFromResponseFormat_JSONObject(t *testing.T) {
	grammar, err := fromResponseFormat(D{"type": "json_object"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(grammar, "root ::=") {
		t.Error("grammar should contain root rule")
	}
	if !strings.Contains(grammar, "object") {
		t.Error("grammar should reference the object rule")
	}
}

func TestFromResponseFormat_JSONSchema_OpenAIWrapped(t *testing.T) {
	rf := D{
		"type": "json_schema",
		"json_schema": D{
			"name":   "Person",
			"strict": true,
			"schema": D{
				"type": "object",
				"properties": D{
					"name": D{"type": "string"},
					"age":  D{"type": "integer"},
				},
				"required": []string{"name", "age"},
			},
		},
	}

	grammar, err := fromResponseFormat(rf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(grammar, "root ::=") {
		t.Error("grammar should contain root rule")
	}
	if !strings.Contains(grammar, `"name"`) {
		t.Error("grammar should contain name property")
	}
	if !strings.Contains(grammar, `"age"`) {
		t.Error("grammar should contain age property")
	}
}

func TestFromResponseFormat_JSONSchema_DirectSchema(t *testing.T) {
	rf := D{
		"type": "json_schema",
		"json_schema": D{
			"type": "object",
			"properties": D{
				"id": D{"type": "integer"},
			},
		},
	}

	grammar, err := fromResponseFormat(rf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(grammar, "root ::=") {
		t.Error("grammar should contain root rule")
	}
	if !strings.Contains(grammar, `"id"`) {
		t.Error("grammar should contain id property")
	}
}

func TestFromResponseFormat_JSONSchema_MissingJSONSchema(t *testing.T) {
	rf := D{"type": "json_schema"}

	if _, err := fromResponseFormat(rf); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("fromResponseFormat: got %v, want ErrInvalidRequest", err)
	}
}

func TestFromResponseFormat_UnsupportedType(t *testing.T) {
	rf := D{"type": "xml"}

	if _, err := fromResponseFormat(rf); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("fromResponseFormat: got %v, want ErrInvalidRequest", err)
	}
}

func TestFromResponseFormat_MapStringAny(t *testing.T) {
	rf := map[string]any{
		"type": "json_schema",
		"json_schema": map[string]any{
			"schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"flag": map[string]any{"type": "boolean"},
				},
			},
		},
	}

	grammar, err := fromResponseFormat(rf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(grammar, `"flag"`) {
		t.Error("grammar should contain flag property")
	}
}
