package questions

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ardanlabs/kronk/cmd/kronk/config/configfile"
	"github.com/ardanlabs/kronk/sdk/tools/modelconfig"
)

func TestParseVars(t *testing.T) {
	vars, err := parseVars("preserve_thinking=true\n\n  max_turns = 3\nstyle=short answers\nlevels=[low, high]\n")
	if err != nil {
		t.Fatal(err)
	}
	if vars["preserve_thinking"] != true || vars["max_turns"] != 3 || vars["style"] != "short answers" {
		t.Fatalf("vars = %#v", vars)
	}
	if l, ok := vars["levels"].([]any); !ok || len(l) != 2 {
		t.Fatalf("levels = %#v", vars["levels"])
	}

	for _, bad := range []string{
		"no equals sign",
		"1bad=x",
		"name=",
		"enable_thinking=true",
		"chat_template_kwargs=x",
		"a=1\na=2",
	} {
		if _, err := parseVars(bad); err == nil {
			t.Errorf("%q: no error", bad)
		}
	}
}

func TestVarsRoundTrip(t *testing.T) {
	existing := map[string]any{
		"chat-template-kwargs.preserve_thinking": true,
		"chat-template-kwargs.levels":            []any{"low", "high"},
		"chat-template-kwargs.style.tone":        "dry",
		"nseq-max":                               2,
	}

	text := formatVars(existingVars(existing))
	if want := "levels=[low, high]\npreserve_thinking=true\nstyle={tone: dry}"; text != want {
		t.Fatalf("formatVars = %q, want %q", text, want)
	}

	changes, err := varChanges(text, existing)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("unchanged variables produced changes: %+v", changes)
	}

	changes, err = varChanges("preserve_thinking=false\nnew_one=1\nstyle={tone: dry}", existing)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, c := range changes {
		switch {
		case c.Del:
			got[c.Key] = "del"
		case c.Old != nil:
			got[c.Key] = "change"
		default:
			got[c.Key] = "add"
		}
	}
	want := map[string]string{
		"chat-template-kwargs.levels":            "del",
		"chat-template-kwargs.preserve_thinking": "change",
		"chat-template-kwargs.new_one":           "add",
	}
	if len(got) != len(want) {
		t.Fatalf("changes = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("changes = %v, want %v", got, want)
		}
	}

	// Removing a map variable removes its flattened sub keys too.
	flat := Apply(existing, []configfile.Change{{Key: "chat-template-kwargs.style", Del: true}})
	if _, ok := flat["chat-template-kwargs.style.tone"]; ok {
		t.Fatal("sub key left after removing its variable")
	}
}

func TestVarsSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model_config.yaml")

	changes, err := varChanges("preserve_thinking=true\nlevels=[low, high]", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := configfile.SaveEntry(path, configfile.SectionModels, "a/b", changes); err != nil {
		t.Fatal(err)
	}

	doc, err := modelconfig.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	kw := doc.Models["a/b"].ChatTemplateKwargs
	if kw["preserve_thinking"] != true || len(kw["levels"].([]any)) != 2 {
		data, _ := os.ReadFile(path)
		t.Fatalf("kwargs = %#v\nfile:\n%s", kw, data)
	}
}
