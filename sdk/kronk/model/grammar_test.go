package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
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
	if !strings.Contains(grammar, `"\"name\""`) {
		t.Errorf("grammar missing name property:\n%s", grammar)
	}
	if !strings.Contains(grammar, `"\"age\""`) {
		t.Errorf("grammar missing age property:\n%s", grammar)
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
				`root ::= "{" ws "\"verdict\"" ws ":" ws rule-1 ws "}"`,
				`rule-1 ::= ( "\"yes\"" | "\"no\"" | "\"maybe\"" )`,
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
				`root ::= "[" ws ( rule-1 ( ws "," ws rule-1 )* )? ws "]"`,
				`rule-1 ::= ( "\"yes\"" | "\"no\"" )`,
			},
		},
		{
			name: "top level",
			schema: D{
				"type": "string",
				"enum": []any{"yes", "no"},
			},
			wantRules: []string{
				`root ::= ( "\"yes\"" | "\"no\"" )`,
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

func TestJSONValueToRuleEscapesString(t *testing.T) {
	got, err := jsonValueToRule("a\"b\\c\n雪")
	if err != nil {
		t.Fatalf("jsonValueToRule: %v", err)
	}

	want := `"\"a\\\"b\\\\c\\n雪\""`
	if got != want {
		t.Errorf("jsonValueToRule: got %q, want %q", got, want)
	}
}

func TestFromJSONSchema_PropertyNamesAreNotRuleNames(t *testing.T) {
	schema := D{
		"type": "object",
		"properties": D{
			"bad name_\"": D{
				"type": "string",
				"enum": []any{"a\"b\\c\n雪", "no"},
			},
		},
	}

	grammar, err := fromJSONSchema(schema)
	if err != nil {
		t.Fatalf("fromJSONSchema: %v", err)
	}

	if !strings.Contains(grammar, `"\"bad name_\\\"\""`) {
		t.Errorf("grammar missing escaped property name:\n%s", grammar)
	}
	if !strings.Contains(grammar, `"\"a\\\"b\\\\c\\n雪\""`) {
		t.Errorf("grammar missing escaped enum value:\n%s", grammar)
	}
	if strings.Contains(grammar, "root-bad name") {
		t.Errorf("grammar used property text as a rule name:\n%s", grammar)
	}
	if !strings.Contains(grammar, "rule-1 ::=") {
		t.Errorf("grammar missing generated rule name:\n%s", grammar)
	}
}

func TestFromJSONSchema_PatternUnsupported(t *testing.T) {
	schema := D{
		"type":    "string",
		"pattern": `^[a-z]+$`,
	}

	if _, err := fromJSONSchema(schema); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("fromJSONSchema: got %v, want ErrInvalidRequest", err)
	}
}

func TestFromJSONSchema_EnumGrammarInitializes(t *testing.T) {
	schema := D{
		"type": "object",
		"properties": D{
			"verdict\"\\\n雪": D{
				"type": "string",
				"enum": []any{"yes\"\\\n雪", "no", "maybe"},
			},
		},
		"required": []string{"verdict\"\\\n雪"},
	}

	grammar, err := fromJSONSchema(schema)
	if err != nil {
		t.Fatalf("fromJSONSchema: unexpected error: %v", err)
	}

	vocab := grammarTestVocab(t)
	sampler := llama.SamplerInitGrammar(vocab, grammar, "root")
	if sampler == 0 {
		t.Fatal("SamplerInitGrammar: got zero sampler, want initialized sampler")
	}
	t.Cleanup(func() {
		llama.SamplerFree(sampler)
	})
}

func grammarTestVocab(t *testing.T) llama.Vocab {
	t.Helper()
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
	if err := llama.Init(); err != nil {
		t.Fatalf("initialize llama: %v", err)
	}
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

	return llama.ModelGetVocab(mdl)
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
	if !strings.Contains(grammar, `"\"user\""`) {
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
	if !strings.Contains(grammar, `"\"name\""`) {
		t.Error("grammar should contain name property")
	}
	if !strings.Contains(grammar, `"\"age\""`) {
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
	if !strings.Contains(grammar, `"\"id\""`) {
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
	if !strings.Contains(grammar, `"\"flag\""`) {
		t.Error("grammar should contain flag property")
	}
}

func TestFromResponseFormat_SiblingSchemaPrecedence(t *testing.T) {
	tests := []struct {
		name string
		rf   D
		want D
	}{
		{"sibling", D{"type": "json_schema", "schema": D{"type": "integer"}}, D{"type": "integer"}},
		{"wrapper wins", D{"type": "json_schema", "schema": D{"type": "integer"}, "json_schema": D{"schema": D{"type": "boolean"}}}, D{"type": "boolean"}},
		{"empty wrapper wins", D{"type": "json_schema", "schema": D{"type": "integer"}, "json_schema": D{}}, D{}},
		{"empty wrapped schema wins", D{"type": "json_schema", "schema": D{"type": "integer"}, "json_schema": D{"schema": D{}}}, D{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := fromResponseFormat(tt.rf)
			if err != nil {
				t.Fatal(err)
			}
			want, err := fromJSONSchema(tt.want)
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Errorf("grammar: got %s, want %s", got, want)
			}
		})
	}
	for _, wrapper := range []any{nil, false, "bad", []any{}} {
		_, err := fromResponseFormat(D{"type": "json_schema", "json_schema": wrapper, "schema": D{}})
		if !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("wrapper %v: got %v, want ErrInvalidRequest", wrapper, err)
		}
	}
}

func TestFromJSONSchema_UnsupportedKeywords(t *testing.T) {
	for _, keyword := range []string{
		"allOf", "anyOf", "oneOf", "not", "$ref", "$dynamicRef", "$defs", "definitions",
		"if", "then", "else", "const", "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "multipleOf",
		"minLength", "maxLength", "pattern", "format",
		"minItems", "maxItems", "uniqueItems", "contains", "minContains", "maxContains", "prefixItems", "additionalItems",
		"minProperties", "maxProperties", "patternProperties", "propertyNames", "dependencies", "dependentRequired", "dependentSchemas",
		"unevaluatedProperties", "unevaluatedItems", "nullable", "unknownAssertion",
	} {
		t.Run(keyword, func(t *testing.T) {
			schema := D{"type": "string", keyword: nil}
			for _, input := range []D{schema, {"type": "string", "enum": []string{"yes"}, keyword: nil}, {"type": "object", "properties": D{"child": schema}}, {"type": "array", "items": schema}} {
				if _, err := fromJSONSchema(input); !errors.Is(err, ErrInvalidRequest) {
					t.Errorf("schema %v: got %v, want ErrInvalidRequest", input, err)
				}
			}
		})
	}
	for _, schema := range []D{
		{"allOf": []D{{"type": "string"}, {"minLength": 2}}},
		{"anyOf": []D{{"type": "string"}, {"type": "null"}}},
		{"oneOf": []D{{"type": "string"}, {"type": "integer"}}},
		{"not": D{"type": "null"}}, {"$ref": "#/$defs/entry"},
		{"type": "string", "minLength": 2}, {"type": "number", "minimum": 1},
		{"type": "array", "minItems": 1}, {"type": "object", "minProperties": 1},
	} {
		if _, err := fromJSONSchema(schema); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("schema %v: got %v, want ErrInvalidRequest", schema, err)
		}
	}
}

func TestFromJSONSchema_Malformed(t *testing.T) {
	for _, schema := range []any{
		nil, true, "string", []any{}, D{"type": "unknown"}, D{"type": nil}, D{"type": []string{"string", "null"}},
		D{"enum": "yes"}, D{"enum": []any{}}, D{"type": "string", "enum": []any{1}},
		D{"type": "object", "properties": nil}, D{"type": "object", "properties": []any{}},
		D{"type": "object", "properties": D{"x": true}}, D{"type": "object", "properties": D{"x": nil}},
		D{"type": "object", "required": "x"}, D{"type": "object", "required": []any{1}},
		D{"type": "object", "required": []string{"x"}},
		D{"type": "object", "properties": D{"x": D{}}, "required": []string{"x", "x"}},
		D{"type": "object", "additionalProperties": D{}}, D{"type": "object", "additionalProperties": nil},
		D{"type": "array", "items": []any{}}, D{"type": "array", "items": true}, D{"type": "array", "items": nil},
		D{"properties": D{"x": D{}}}, D{"items": D{}},
		D{"type": "object", "enum": []any{D{}}, "properties": D{}},
		D{"type": "array", "enum": []any{[]any{}}, "items": D{}},
	} {
		t.Run(fmt.Sprint(schema), func(t *testing.T) {
			if _, err := fromJSONSchema(schema); !errors.Is(err, ErrInvalidRequest) {
				t.Errorf("got %v, want ErrInvalidRequest", err)
			}
		})
	}
}

func TestFromJSONSchema_AnnotationsAndTypedEnum(t *testing.T) {
	schema := D{"type": "string", "enum": []string{"yes", "no"}}
	for _, key := range []string{"$schema", "$id", "$comment", "title", "description", "default", "examples", "deprecated", "readOnly", "writeOnly"} {
		schema[key] = "annotation"
	}
	schema["contentEncoding"] = "base64"
	schema["contentMediaType"] = "application/json"
	schema["contentSchema"] = D{"allOf": []D{{"type": "object"}}}
	grammar, err := fromJSONSchema(schema)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(grammar, `root ::= ( "\"yes\"" | "\"no\"" )`) {
		t.Errorf("got %s, want string enum root", grammar)
	}
	grammar, err = fromJSONSchema(D{"type": "integer", "enum": []any{json.Number("9007199254740993"), json.Number("1.00000000000000001"), "no"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(grammar, `root ::= "9007199254740993"`+"\n") {
		t.Errorf("got %s, want exact integer enum root", grammar)
	}
}

// grammarAccepts checks every token's mask, including EOS. Accept alone does
// not validate a string, and a valid prefix is not necessarily a complete JSON value.
func grammarAccepts(t *testing.T, vocab llama.Vocab, grammar, input string) bool {
	t.Helper()
	gs, err := newGrammarSampler(vocab, grammar)
	if err != nil {
		t.Fatal(err)
	}
	defer gs.Free()
	tokens := append(llama.Tokenize(vocab, input, false, false), llama.VocabEOS(vocab))
	for _, token := range tokens {
		candidate := llama.TokenData{Id: token}
		data := llama.TokenDataArray{Data: &candidate, Size: 1, Selected: -1}
		llama.SamplerApply(gs.sampler, &data)
		if math.IsInf(float64(candidate.Logit), -1) {
			return false
		}
		llama.SamplerAccept(gs.sampler, token)
	}
	return true
}

func TestFromJSONSchema_PropertyPresenceNative(t *testing.T) {
	vocab := grammarTestVocab(t)
	// Exhaust all required/present combinations: optional properties before,
	// between, and after required ones, including the all-optional empty object.
	for required := range 8 {
		schema := D{"type": "object", "properties": D{"a": D{"type": "integer"}, "b": D{"type": "integer"}, "c": D{"type": "integer"}}}
		var keys []string
		for i, key := range []string{"a", "b", "c"} {
			if required&(1<<i) != 0 {
				keys = append(keys, key)
			}
		}
		if len(keys) > 0 {
			schema["required"] = keys
		}
		grammar, err := fromJSONSchema(schema)
		if err != nil {
			t.Fatal(err)
		}
		for present := range 8 {
			var pairs []string
			for i, key := range []string{"a", "b", "c"} {
				if present&(1<<i) != 0 {
					pairs = append(pairs, fmt.Sprintf(`%q:1`, key))
				}
			}
			input := "{" + strings.Join(pairs, ",") + "}"
			want := present&required == required
			if got := grammarAccepts(t, vocab, grammar, input); got != want {
				t.Errorf("required %v input %s: got %t, want %t", keys, input, got, want)
			}
		}
		for _, input := range []string{`{,}`, `{"a":1,}`, `{,"c":1}`, `{"a":1,,"c":1}`, `{"c":1,"b":1,"a":1}`, `{"a":1,"a":1}`, `{"a":1`, `{"a":"bad"}`} {
			if grammarAccepts(t, vocab, grammar, input) {
				t.Errorf("required %v: accepted %s", keys, input)
			}
		}
	}
	for _, tt := range []struct {
		name   string
		schema D
		allow  []string
		deny   []string
	}{
		{"closed empty", D{"type": "object", "additionalProperties": false}, []string{`{}`, `{ }`}, []string{`{"x":1}`, `[]`}},
		{"closed empty properties", D{"type": "object", "properties": D{}, "additionalProperties": false}, []string{`{}`}, []string{`{"x":1}`}},
		{"open object", D{"type": "object", "additionalProperties": true}, []string{`{}`, `{"x":1}`}, []string{`[]`}},
		{"array of optional objects", D{"type": "array", "items": D{"type": "object", "properties": D{"a": D{"type": "boolean"}, "b": D{"type": "null"}}}}, []string{`[]`, `[{}, {"b":null}, {"a":true,"b":null}]`}, []string{`[{,"b":null}]`, `[{"a":true,}]`, `[{"b":false}]`}},
		{"typed enum", D{"type": "string", "enum": []any{"yes", "no", 1}}, []string{`"yes"`, `"no"`}, []string{`1`, `"maybe"`, `"ye"`}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			grammar, err := fromJSONSchema(tt.schema)
			if err != nil {
				t.Fatal(err)
			}
			for _, input := range tt.allow {
				if !json.Valid([]byte(input)) || !grammarAccepts(t, vocab, grammar, input) {
					t.Errorf("rejected allowed JSON %s", input)
				}
			}
			for _, input := range tt.deny {
				if grammarAccepts(t, vocab, grammar, input) {
					t.Errorf("accepted rejected input %s", input)
				}
			}
		})
	}
}
