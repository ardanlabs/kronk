package configfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/tools/modelconfig"
)

const defaultConfig = "../../../../sdk/tools/defaults/yaml/model_config.yaml"

func TestSaveEntryNewModel(t *testing.T) {
	path := copyDefault(t)

	changes := []Change{
		{Key: "context-window", New: 65536},
		{Key: "cache-type-k", New: "q8_0"},
		{Key: "cache-type-v", New: "q8_0"},
		{Key: "moe.mode", New: "keep_top_n"},
		{Key: "moe.keep-experts-top-n", New: 20},
		{Key: "sampling-parameters.temperature", New: 0.7},
	}

	backup, err := SaveEntry(path, SectionModels, "test/new-model", changes)
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

	changes := []Change{
		{Key: "context-window", Old: 65536, New: 32768},
		{Key: "sampling-parameters.max_tokens", Old: 8192, Del: true},
		{Key: "sampling-parameters.temperature", Old: 1, Del: true},
		{Key: "sampling-parameters.top_k", Old: 20, Del: true},
		{Key: "sampling-parameters.top_p", Old: 0.95, Del: true},
	}

	if _, err := SaveEntry(path, SectionModels, id, changes); err != nil {
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

	backup, err := SaveEntry(path, SectionModels, "test/model", []Change{{Key: "nseq-max", New: 2}})
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

func TestSaveEntryValuesReplacesTheEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model_config.yaml")
	in := "version: 2\n\nmodels:\n  a/b:\n    nseq-max: 2\n    sampling-parameters:\n      temperature: 0.2\n  c/d:\n    nseq-max: 3\n"
	if err := os.WriteFile(path, []byte(in), 0o644); err != nil {
		t.Fatal(err)
	}

	values := map[string]any{
		"context-window":                 8192,
		"sampling-parameters.max_tokens": 1024,
	}

	preview, err := PreviewEntryValues(path, SectionModels, "a/b", values)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := SaveEntryValues(path, SectionModels, "a/b", values); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), preview) {
		t.Fatalf("preview not in the saved file:\n%s\nfile:\n%s", preview, data)
	}

	doc, err := modelconfig.Load(path)
	if err != nil {
		t.Fatal(err)
	}

	mc := doc.Models["a/b"]
	if mc.PtrContextWindow == nil || *mc.PtrContextWindow != 8192 {
		t.Fatalf("context-window = %v", mc.PtrContextWindow)
	}
	if mc.Sampling.MaxTokens != 1024 {
		t.Fatalf("max_tokens = %d", mc.Sampling.MaxTokens)
	}
	if mc.Sampling.Temperature == 0.2 {
		t.Fatal("a value left out of the entry survived: temperature")
	}
	if n := doc.Models["c/d"].PtrNSeqMax; n == nil || *n != 3 {
		t.Fatalf("the other entry changed: nseq-max = %v", n)
	}
}

func TestSaveEntryRejectsInvalid(t *testing.T) {
	path := copyDefault(t)
	before, _ := os.ReadFile(path)

	if _, err := SaveEntry(path, SectionModels, "test/model", []Change{{Key: "flash-attention", New: "sometimes"}}); err == nil {
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

	out, err := editEntry(data, SectionModels, "test/new", []Change{{Key: "context-window", New: 65536}})
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
	out, err = editEntry(data, SectionModels, id, []Change{{Key: "context-window", Old: 65536, New: 32768}})
	if err != nil {
		t.Fatal(err)
	}

	want := strings.Replace(orig, id+":\n    context-window: 65536", id+":\n    context-window: 32768", 1)
	if string(out) != want {
		t.Fatalf("edit changed more than the value:\n%s", out)
	}
}

func TestEditEntryEmptyModels(t *testing.T) {
	out, err := editEntry([]byte("version: 2\n\nkms: {}\n\nmodels: {}\n\nbucky-models: {}\n"), SectionModels, "a/b", []Change{{Key: "nseq-max", New: 1}})
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

func TestMediaSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model_config.yaml")

	if _, err := SaveEntry(path, SectionBucky, "tiny", []Change{{Key: "nseq-max", New: 2}}); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveEntry(path, SectionMalina, "sd-1.5", []Change{{Key: "concurrency", New: 2}}); err != nil {
		t.Fatal(err)
	}

	doc, err := modelconfig.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if n := doc.BuckyModels["tiny"].NSeqMax; n == nil || *n != 2 {
		data, _ := os.ReadFile(path)
		t.Fatalf("bucky entry not written:\n%s", data)
	}
	if n := doc.MalinaModels["sd-1.5"].Concurrency; n == nil || *n != 2 {
		data, _ := os.ReadFile(path)
		t.Fatalf("malina entry not written:\n%s", data)
	}
}

func TestEditEntryNothingToWrite(t *testing.T) {
	for _, data := range []string{newFile, "version: 2\n\nkms: {}\n\nmodels: {}\n\nbucky-models: {}\n"} {
		for _, section := range []string{SectionModels, SectionBucky, SectionMalina} {
			out, err := editEntry([]byte(data), section, "a/b", nil)
			if err != nil {
				t.Fatal(err)
			}
			if string(out) != data {
				t.Errorf("%s with no changes rewrote the file:\n%s\nwant:\n%s", section, out, data)
			}
		}
	}
}
