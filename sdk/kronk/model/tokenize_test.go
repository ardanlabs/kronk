package model

import (
	"errors"
	"testing"
)

func TestTokenizeInputValidation(t *testing.T) {
	tests := []struct {
		name  string
		input D
	}{
		{name: "missing", input: D{}},
		{name: "empty", input: D{"input": ""}},
		{name: "wrong type", input: D{"input": 42}},
	}

	m := Model{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := m.Tokenize(t.Context(), tt.input); !errors.Is(err, ErrInvalidRequest) {
				t.Errorf("Tokenize: got %v, want ErrInvalidRequest", err)
			}
		})
	}
}
