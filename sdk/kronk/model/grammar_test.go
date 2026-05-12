package model

import (
	"strings"
	"testing"
)

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
	schema := D{
		"type": "object",
		"properties": D{
			"status": D{
				"type": "string",
				"enum": []any{"pending", "active", "completed"},
			},
		},
	}

	grammar, err := fromJSONSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(grammar, "pending") {
		t.Error("grammar should contain enum value 'pending'")
	}
	if !strings.Contains(grammar, "active") {
		t.Error("grammar should contain enum value 'active'")
	}
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

	if _, err := fromResponseFormat(rf); err == nil {
		t.Error("expected error when json_schema field is missing")
	}
}

func TestFromResponseFormat_UnsupportedType(t *testing.T) {
	rf := D{"type": "xml"}

	if _, err := fromResponseFormat(rf); err == nil {
		t.Error("expected error for unsupported type")
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
