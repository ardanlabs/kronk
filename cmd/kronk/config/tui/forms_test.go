package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/ardanlabs/kronk/cmd/kronk/config/questions"
	"github.com/ardanlabs/kronk/cmd/kronk/config/source"
	"github.com/ardanlabs/kronk/sdk/tools/devices"
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

func TestPickRowSource(t *testing.T) {
	tests := map[string]struct {
		c    source.Choice
		want string
	}{
		"installed":  {source.Choice{ID: "a/b", Installed: true}, "installed"},
		"catalog":    {source.Choice{ID: "a/b"}, "catalog"},
		"incomplete": {source.Choice{ID: "a/b", Incomplete: true}, "incomplete"},
	}
	for name, tt := range tests {
		if got := newPickRow(tt.c).sourceText; got != tt.want {
			t.Errorf("%s: source = %q, want %q", name, got, tt.want)
		}
	}
}

func TestExistingChoicesRemainSelected(t *testing.T) {
	a := newApp(t.Context(), nil, true)
	a.facts = source.Facts{Endpoint: source.EndpointChat, Devices: devices.Devices{Devices: []devices.DeviceInfo{
		{Name: "CUDA0", Type: "gpu_cuda"}, {Name: "CUDA1", Type: "gpu_cuda"},
	}}}
	a.qs = questions.Chat()
	a.ans = questions.NewAnswers(a.qs)
	a.existing = map[string]any{
		"devices":              []any{"CUDA1", "CUDA0"},
		"draft-model.devices":  []any{"RemoteGPU", "CUDA1"},
		"draft-model.model-id": "owner/draft",
		"cache-type-k":         "f32",
	}
	a.ans.Prefill(a.facts, a.existing)
	for key, want := range map[string]string{
		"devices": "CUDA1,CUDA0", "draft-model.devices": "RemoteGPU,CUDA1",
		"draft-model.model-id": "owner/draft", "cache-type-k": "f32",
	} {
		field := a.field(a.ans.ByKey[key])
		field.Focus()
		field.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if a.ans.ByKey[key].Kind == questions.KindMulti {
			for device := range strings.SplitSeq(want, ",") {
				if !strings.Contains(field.View(), device) {
					t.Errorf("%s does not display configured GPU %q", key, device)
				}
			}
		}
		if got := a.ans.Value(key); got != want {
			t.Errorf("unchanged %s submitted %q, want %q", key, got, want)
		}
	}
}
