package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ardanlabs/kronk/cmd/kronk/config/source"
)

func groupedApp() *app {
	a := newApp(context.Background(), nil, true)
	a.choices = []source.Choice{
		{ID: "unsloth/Qwen3-0.6B-Q8_0", Installed: true},
		{ID: "unsloth/gpt-oss-20b-Q8_0"},
		{ID: "nomic-ai/nomic-embed-text-v1.5.Q8_0", Endpoint: "embeddings"},
		{ID: "gpustack/bge-reranker-v2-m3-Q8_0", Endpoint: "embeddings"},
		{ID: "tiny", Kind: source.KindAudioModel},
	}
	return a
}

func TestGroupsAskedFirst(t *testing.T) {
	a := groupedApp()
	a.openGroups()
	if a.step != stepGroup || a.group != string(source.GroupChat) {
		t.Fatalf("step = %v, group = %q; want the groups, on chat", a.step, a.group)
	}

	groups, counts := a.groups()
	want := []source.Group{source.GroupChat, source.GroupEmbeddings, source.GroupRerank, source.GroupAudio}
	if len(groups) != len(want) {
		t.Fatalf("groups = %v, want %v", groups, want)
	}
	for i := range want {
		if groups[i] != want[i] {
			t.Fatalf("groups = %v, want %v", groups, want)
		}
	}
	if c := counts[source.GroupChat]; c.total != 2 || c.installed != 1 {
		t.Fatalf("chat count = %+v", c)
	}

	a.group = string(source.GroupRerank)
	if got := a.pickChoices(); len(got) != 1 || got[0].ID != "gpustack/bge-reranker-v2-m3-Q8_0" {
		t.Fatalf("rerank lists %v", got)
	}
	if q := a.pickQuestion(); q != "Which rerank model do you want to configure?" {
		t.Fatalf("question = %q", q)
	}

	a.group = groupAll
	if got := a.pickChoices(); len(got) != len(a.choices) {
		t.Fatalf("all lists %d of %d", len(got), len(a.choices))
	}
}

func TestOneGroupSkipsTheGroups(t *testing.T) {
	a := newApp(context.Background(), nil, true)
	a.choices = []source.Choice{{ID: "a/b", Installed: true}, {ID: "c/d"}}
	a.openGroups()
	if a.step != stepPick || a.groupAsked {
		t.Fatalf("step = %v; one group goes straight to the models", a.step)
	}
}

func TestEscClearsTheSearchInOnePress(t *testing.T) {
	a := groupedApp()
	a.group, a.groupAsked = string(source.GroupChat), true
	a.open(stepPick)

	a.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if !a.fieldUsesEsc() {
		t.Fatal("typing: the search is not open")
	}

	a.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if a.step != stepPick || a.fieldUsesEsc() {
		t.Fatalf("esc: step = %v, search still open or kept as a filter", a.step)
	}

	a.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if a.step != stepGroup {
		t.Fatalf("second esc: step = %v, want the groups", a.step)
	}
}

func TestDraftsAreInstalledModelsWithTheTokenizer(t *testing.T) {
	a := newApp(context.Background(), nil, true)
	a.choices = []source.Choice{
		{ID: "unsloth/Qwen3-1.7B-Q4_K_M", Installed: true, Tokenizer: "gpt2:qwen2"}, // the model itself
		{ID: "unsloth/Qwen3-0.6B-Q8_0", Installed: true, Tokenizer: "gpt2:qwen2"},
		{ID: "unsloth/Qwen3.5-0.8B-Q8_0", Installed: true, Tokenizer: "gpt2:qwen35"},
		{ID: "unsloth/Qwen3-4B-Q8_0", Tokenizer: ""}, // catalog: not installed
		{ID: "Qwen/Qwen3-Embedding-0.6B-Q8_0", Installed: true, Tokenizer: "gpt2:qwen2", Endpoint: "embeddings"},
	}

	f := source.Facts{ID: "unsloth/Qwen3-1.7B-Q4_K_M", Endpoint: source.EndpointChat}
	f.Info.Metadata = map[string]string{"tokenizer.ggml.model": "gpt2", "tokenizer.ggml.pre": "qwen2"}
	a.startQuestions(f)

	if got := a.facts.Drafts; len(got) != 1 || got[0] != "unsloth/Qwen3-0.6B-Q8_0" {
		t.Fatalf("drafts = %v; want only the installed chat model with the same tokenizer", got)
	}

	f.Info.Metadata = nil // no tokenizer in the metadata: nothing can be checked
	a.startQuestions(f)
	if len(a.facts.Drafts) != 0 {
		t.Fatalf("drafts = %v; want none when the tokenizer is unknown", a.facts.Drafts)
	}
}
