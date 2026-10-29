package tui

import (
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/kronk/config/source"
)

func TestUpperFirst(t *testing.T) {
	for in, want := range map[string]string{
		"auto":               "Auto",
		"llama.cpp's choice": "Llama.cpp's choice",
		"AutoTune's pick: 1": "AutoTune's pick: 1",
		"0":                  "0",
		"":                   "",
	} {
		if got := upperFirst(in); got != want {
			t.Errorf("upperFirst(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestKindColumnOnlyInAllModels(t *testing.T) {
	a := groupedApp()
	row := newPickRow(a.choices[0]) // unsloth/Qwen3-0.6B-Q8_0, chat
	w := pickWidths{id: 30, source: 9, kind: 10, size: 6}

	a.group = string(source.GroupChat)
	if label := a.pickLabel(row, w); strings.Contains(label, "chat") {
		t.Fatalf("chat list shows the kind: %q", label)
	}
	if line := a.groupLine(); !strings.Contains(line, "Chat") || !strings.Contains(line, "2 models") {
		t.Fatalf("group line = %q", line)
	}

	a.group = groupAll
	if label := a.pickLabel(row, w); !strings.Contains(label, "chat") {
		t.Fatalf("All models doesn't show the kind: %q", label)
	}
	if line := a.groupLine(); !strings.Contains(line, "All models") {
		t.Fatalf("group line = %q", line)
	}
}
