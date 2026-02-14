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
