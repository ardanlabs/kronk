package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/tools/modelconfig"
)

const defaultConfig = "../../../sdk/tools/defaults/yaml/model_config.yaml"

func TestSaveEntryNewModel(t *testing.T) {
	path := copyDefault(t)

	changes := []change{
		{key: "context-window", new: 65536},
		{key: "cache-type-k", new: "q8_0"},
		{key: "cache-type-v", new: "q8_0"},
		{key: "moe.mode", new: "keep_top_n"},
		{key: "moe.keep-experts-top-n", new: 20},
		{key: "sampling-parameters.temperature", new: 0.7},
	}

	backup, err := saveEntry(path, "test/new-model", changes)
	if err != nil {
		t.Fatal(err)
	}
	if backup != path+".bak" {
		t.Fatalf("backup = %q", backup)
	}

	doc, err := modelconfig.Load(path)
	if err != nil {
		t.Fatal(err)
	}

	mc, ok := doc.Models["test/new-model"]
	if !ok {
		t.Fatal("entry not written")
	}
	if mc.PtrContextWindow == nil || *mc.PtrContextWindow != 65536 {
		t.Fatalf("context-window = %v", mc.PtrContextWindow)
	}
	if mc.MoE == nil || mc.MoE.Mode.String() != "keep_top_n" {
		t.Fatalf("moe = %+v", mc.MoE)
	}
	if mc.Sampling.Temperature != 0.7 {
		t.Fatalf("temperature = %v", mc.Sampling.Temperature)
	}

	orig, err := modelconfig.Load(defaultConfig)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Models) != len(orig.Models)+1 {
		t.Fatalf("models = %d, want %d", len(doc.Models), len(orig.Models)+1)
	}

	data, _ := os.ReadFile(path)
	for _, want := range []string{"# SERVER OPTIONS", "kms: {}", "# nseq-max: 1  ~89GB VRAM"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("lost %q", want)
		}
	}
}

func TestSaveEntryEditAndRemove(t *testing.T) {
	path := copyDefault(t)
	id := "ornith-ai/Ornith-1.5-9B-Q4_K_M/AGENT"

	changes := []change{
		{key: "context-window", old: 65536, new: 32768},
		{key: "sampling-parameters.max_tokens", old: 8192, del: true},
		{key: "sampling-parameters.temperature", old: 1, del: true},
		{key: "sampling-parameters.top_k", old: 20, del: true},
		{key: "sampling-parameters.top_p", old: 0.95, del: true},
	}

	if _, err := saveEntry(path, id, changes); err != nil {
		t.Fatal(err)
	}

	doc, err := modelconfig.Load(path)
	if err != nil {
		t.Fatal(err)
	}

	mc := doc.Models[id]
	if *mc.PtrContextWindow != 32768 {
		t.Fatalf("context-window = %d", *mc.PtrContextWindow)
	}

	data, _ := os.ReadFile(path)
	entry := string(data)[strings.Index(string(data), id):]
	entry = entry[:strings.Index(entry, "mradermacher/")]
	if strings.Contains(entry, "sampling-parameters") {
		t.Fatalf("empty sampling-parameters left behind:\n%s", entry)
	}
}

func TestSaveEntryNewFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "model_config.yaml")

	backup, err := saveEntry(path, "test/model", []change{{key: "nseq-max", new: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if backup != "" {
		t.Fatalf("backup for a new file = %q", backup)
	}

	doc, err := modelconfig.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if *doc.Models["test/model"].PtrNSeqMax != 2 {
		t.Fatal("nseq-max not written")
	}
}

func TestSaveEntryRejectsInvalid(t *testing.T) {
	path := copyDefault(t)
	before, _ := os.ReadFile(path)

	if _, err := saveEntry(path, "test/model", []change{{key: "flash-attention", new: "sometimes"}}); err == nil {
		t.Fatal("an invalid value must fail")
	}

	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("the file changed after a failed save")
	}
}

// TestEditEntryKeepsLayout checks that only the edited entry's lines change.
func TestEditEntryKeepsLayout(t *testing.T) {
	data, err := os.ReadFile(defaultConfig)
	if err != nil {
		t.Fatal(err)
	}
	orig := string(data)

	out, err := editEntry(data, "test/new", []change{{key: "context-window", new: 65536}})
	if err != nil {
		t.Fatal(err)
	}

	added := "\n  test/new:\n    context-window: 65536\n"
	if !strings.Contains(string(out), added) {
		t.Fatalf("new entry not found:\n%s", out)
	}
	if got := strings.Replace(string(out), added, "", 1); got != orig {
		t.Fatal("lines outside the new entry changed")
	}

	id := "mradermacher/Qwopus3.5-4B-Coder.Q8_0/AGENT"
	out, err = editEntry(data, id, []change{{key: "context-window", old: 65536, new: 32768}})
	if err != nil {
		t.Fatal(err)
	}

	want := strings.Replace(orig, id+":\n    context-window: 65536", id+":\n    context-window: 32768", 1)
	if string(out) != want {
		t.Fatalf("edit changed more than the value:\n%s", out)
	}
}

func TestEditEntryEmptyModels(t *testing.T) {
	out, err := editEntry([]byte("version: 2\n\nkms: {}\n\nmodels: {}\n\nbucky-models: {}\n"), "a/b", []change{{key: "nseq-max", new: 1}})
	if err != nil {
		t.Fatal(err)
	}

	want := "version: 2\n\nkms: {}\n\nmodels:\n  a/b:\n    nseq-max: 1\n\nbucky-models: {}\n"
	if string(out) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}
}

func copyDefault(t *testing.T) string {
	t.Helper()

	data, err := os.ReadFile(defaultConfig)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "model_config.yaml")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	return path
}
