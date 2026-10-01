package main

import "testing"

func TestMatchesService(t *testing.T) {
	tests := []struct {
		name    string
		fields  map[string]any
		service string
		want    bool
	}{
		{name: "no filter", fields: map[string]any{}, want: true},
		{name: "matching", fields: map[string]any{"service": "KRONK"}, service: "kronk", want: true},
		{name: "different", fields: map[string]any{"service": "AUTH"}, service: "kronk"},
		{name: "missing", fields: map[string]any{}, service: "kronk"},
		{name: "not a string", fields: map[string]any{"service": 42}, service: "kronk"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesService(tt.fields, tt.service); got != tt.want {
				t.Errorf("matchesService: got %t, want %t", got, tt.want)
			}
		})
	}
}
