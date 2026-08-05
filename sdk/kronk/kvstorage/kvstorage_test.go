package kvstorage

import (
	"encoding/json"
	"testing"
)

func TestKind(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    Kind
		wantErr bool
	}{
		{"RAM", "ram", RAM, false},
		{"unknown", "unknown", Kind{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Parse() error = %v, wantErr %t", err, tt.wantErr)
			}
			if !got.Equal(tt.want) {
				t.Errorf("Parse() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestKindJSON(t *testing.T) {
	type config struct {
		Kind Kind `json:"kind,omitzero"`
	}

	data, err := json.Marshal(config{Kind: RAM})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v, want nil", err)
	}
	if got, want := string(data), `{"kind":"ram"}`; got != want {
		t.Errorf("json.Marshal() = %s, want %s", got, want)
	}

	var cfg config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, want nil", err)
	}
	if !cfg.Kind.Equal(RAM) {
		t.Errorf("json.Unmarshal() kind = %q, want %q", cfg.Kind, RAM)
	}

	if err := json.Unmarshal([]byte(`{"kind":"unknown"}`), &cfg); err == nil {
		t.Fatal("json.Unmarshal() error = nil, want error")
	}

	data, err = json.Marshal(config{})
	if err != nil {
		t.Fatalf("json.Marshal() zero value error = %v, want nil", err)
	}
	if got, want := string(data), `{}`; got != want {
		t.Errorf("json.Marshal() zero value = %s, want %s", got, want)
	}
}
