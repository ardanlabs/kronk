package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ardanlabs/kronk/sdk/tools/modelconfig"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func changesFor(t *testing.T, qs []question, f facts, a *answers, existing map[string]any) map[string]change {
	t.Helper()

	changes, err := diff(qs, f, a, existing)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]change{}
	for _, c := range changes {
		if _, dup := out[c.key]; dup {
			t.Fatalf("%s changed twice: %+v", c.key, changes)
		}
		out[c.key] = c
	}
	return out
}

func TestMediaQuestionKeys(t *testing.T) {
	for kind, qs := range map[string][]question{kindAudioModel: audioQuestions(), kindImageModel: imageQuestions()} {
		flat := map[string]any{}
		for _, q := range qs {
			raw := "3"
			if q.kind == kindDuration {
				raw = "90s"
			}
			v, err := q.parse(raw)
			if err != nil {
				t.Fatalf("%s %s: %v", kind, q.key, err)
			}
			flat[q.key] = v
		}
		if err := checkMedia(kind, flat); err != nil {
			t.Errorf("%s: %v", kind, err)
		}
	}

	if err := checkMedia(kindAudioModel, map[string]any{"concurrency": 1}); err == nil {
		t.Error("an image key must be rejected for an audio model")
	}
}

func TestMediaSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model_config.yaml")

	if _, err := saveSectionEntry(path, sectionBucky, "tiny", []change{{key: "nseq-max", new: 2}}); err != nil {
		t.Fatal(err)
	}
	if _, err := saveSectionEntry(path, sectionMalina, "sd-1.5", []change{{key: "concurrency", new: 2}}); err != nil {
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

func TestValuePrecision(t *testing.T) {
	f := sampleFacts()
	qs := questions()

	// Same precision in the file: the values question starts empty and
	// nothing changes.
	existing := map[string]any{"cache-type-k": "q8_0", "cache-type-v": "q8_0"}
	a := newAnswers(qs)
	a.prefill(f, existing)
	if v := a.value("cache-type-v"); v != "" {
		t.Fatalf("cache-type-v prefill = %q, want same as keys", v)
	}
	if got := changesFor(t, qs, f, a, existing); len(got) != 0 {
		t.Fatalf("unchanged precision produced changes: %+v", got)
	}

	// A different value precision wins over the keys question.
	*a.sel["cache-type-v"] = "f16"
	got := changesFor(t, qs, f, a, existing)
	if got["cache-type-v"].new != "f16" {
		t.Fatalf("cache-type-v = %+v", got["cache-type-v"])
	}

	// Values follow keys when left empty.
	a = newAnswers(qs)
	*a.sel["cache-type-k"] = "q4_0"
	got = changesFor(t, qs, f, a, nil)
	if got["cache-type-k"].new != "q4_0" || got["cache-type-v"].new != "q4_0" {
		t.Fatalf("changes = %+v", got)
	}
}

func TestClassicSpeculation(t *testing.T) {
	f := sampleFacts()
	qs := questions()
	a := newAnswers(qs)
	*a.sel["nseq-max"] = "4"
	*a.sel["speculation"] = "classic"
	*a.sel["draft-model.model-id"] = "test/draft"

	got := changesFor(t, qs, f, a, nil)
	if got["nseq-max"].new != 1 {
		t.Fatalf("classic must set nseq-max to 1, got %+v", got["nseq-max"])
	}
	if got["draft-model.model-id"].new != "test/draft" {
		t.Fatalf("draft = %+v", got["draft-model.model-id"])
	}

	cfg, err := toModelConfig(apply(nil, func() []change {
		var out []change
		for _, c := range got {
			out = append(out, c)
		}
		return out
	}()))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DraftModel == nil || cfg.DraftModel.ModelID != "test/draft" {
		t.Fatalf("draft model = %+v", cfg.DraftModel)
	}
}

func TestListRoundTrips(t *testing.T) {
	f := sampleFacts()
	qs := questions()
	existing := map[string]any{
		"devices":      []any{"CUDA0", "CUDA1"},
		"tensor-split": []any{0.6, 0.4},
		"adapters":     []any{map[string]any{"id": "style", "scale": 0.8}, map[string]any{"path": "/srv/lora/x.gguf"}},
	}

	a := newAnswers(qs)
	a.prefill(f, existing)

	if v := a.value("adapters"); v != "style=0.8\n/srv/lora/x.gguf" {
		t.Fatalf("adapters prefill = %q", v)
	}
	if got := changesFor(t, qs, f, a, existing); len(got) != 0 {
		t.Fatalf("unchanged lists produced changes: %+v", got)
	}

	*a.multi["devices"] = []string{"CUDA1"}
	got := changesFor(t, qs, f, a, existing)
	if inlineYAML(got["devices"].new) != "[CUDA1]" {
		t.Fatalf("devices = %+v", got["devices"])
	}
}

func TestParseAdapters(t *testing.T) {
	list, err := parseAdapters("style=0.8\n/srv/lora/x.gguf\nnested/one.gguf")
	if err != nil {
		t.Fatal(err)
	}
	if inlineYAML(list) != "[{id: style, scale: 0.8}, {path: /srv/lora/x.gguf}, {id: nested/one}]" {
		t.Fatalf("adapters = %s", inlineYAML(list))
	}

	for _, bad := range []string{"style=strong", "./rel.gguf", "~/x.gguf", "/srv/lora/x.bin", "style=-1", "style\nstyle=0.5"} {
		if _, err := parseAdapters(bad); err == nil {
			t.Errorf("%q: no error", bad)
		}
	}
}

func TestVisibility2(t *testing.T) {
	f := sampleFacts() // 2 GPUs, MoE
	a := newAnswers(questions())
	shown := func(key string) bool { return a.applies(f, a.byKey[key]) }

	if !shown("devices") || !shown("split-mode") || shown("main-gpu") {
		t.Fatal("two GPUs: devices and split-mode asked, main-gpu only for split none")
	}
	*a.sel["split-mode"] = "none"
	if !shown("main-gpu") || shown("tensor-split") {
		t.Fatal("split none: main-gpu asked, tensor-split skipped")
	}

	if shown("numa") || shown("nthreads") {
		t.Fatal("all layers on the GPU: no NUMA or thread questions")
	}
	*a.sel["moe.mode"] = "experts_cpu"
	if !shown("numa") || !shown("nthreads") {
		t.Fatal("experts on the CPU: NUMA and threads asked")
	}

	f.GPUs = 1
	if shown("devices") {
		t.Fatal("one GPU: no multi-GPU questions")
	}

}

func TestEntriesFor(t *testing.T) {
	doc := modelconfig.Document{Models: map[string]models.ModelConfig{
		"a/b":       {},
		"a/b/LONG":  {},
		"a/b/AGENT": {},
		"a/bc":      {},
		"a/b/x/y":   {},
	}}
	got := entriesFor(doc, "a/b")
	if len(got) != 3 || got[0] != "a/b" || got[1] != "a/b/AGENT" || got[2] != "a/b/LONG" {
		t.Fatalf("entries = %v", got)
	}
}
