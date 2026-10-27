package model

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTokenizeIDsValidation(t *testing.T) {
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	expired, cancelDeadline := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancelDeadline()

	tests := []struct {
		name  string
		ctx   context.Context
		input string
		want  error
	}{
		{name: "empty", ctx: t.Context(), want: ErrInvalidRequest},
		{name: "canceled", ctx: canceled, input: "<bos>formatted text", want: context.Canceled},
		{name: "expired", ctx: expired, input: "text", want: context.DeadlineExceeded},
	}

	// A zero model ensures these public checks do not access native vocabulary.
	m := Model{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := m.TokenizeIDs(tt.ctx, tt.input)
			if !errors.Is(err, tt.want) {
				t.Errorf("TokenizeIDs: got error %v, want %v", err, tt.want)
			}
			if got != nil {
				t.Errorf("TokenizeIDs: got %v, want nil", got)
			}
		})
	}
}

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
