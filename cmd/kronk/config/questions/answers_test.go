package questions

import (
	"testing"

	"github.com/ardanlabs/kronk/cmd/kronk/config/configfile"
	"github.com/ardanlabs/kronk/cmd/kronk/config/source"
)

func changesFor(t *testing.T, qs []Question, f source.Facts, a *Answers, existing map[string]any) map[string]configfile.Change {
	t.Helper()

	changes, err := Diff(qs, f, a, existing)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]configfile.Change{}
	for _, c := range changes {
		if _, dup := out[c.Key]; dup {
			t.Fatalf("%s changed twice: %+v", c.Key, changes)
		}
		out[c.Key] = c
	}
	return out
}

func TestMediaQuestionKeys(t *testing.T) {
	for kind, qs := range map[source.ModelKind][]Question{source.KindAudioModel: Audio(), source.KindImageModel: Image()} {
		flat := map[string]any{}
		for _, q := range qs {
			raw := "3"
			if q.Kind == kindDuration {
				raw = "90s"
			}
			v, err := q.parse(raw)
			if err != nil {
				t.Fatalf("%s %s: %v", kind, q.Key, err)
			}
			flat[q.Key] = v
		}
		if err := CheckMedia(kind, flat); err != nil {
			t.Errorf("%s: %v", kind, err)
		}
	}

	if err := CheckMedia(source.KindAudioModel, map[string]any{"concurrency": 1}); err == nil {
		t.Error("an image key must be rejected for an audio model")
	}
}

func TestValuePrecision(t *testing.T) {
	f := sampleFacts()
	qs := Chat()

	// Same precision in the file: the values question starts empty and
	// nothing changes.
	existing := map[string]any{"cache-type-k": "q8_0", "cache-type-v": "q8_0"}
	a := NewAnswers(qs)
	a.Prefill(f, existing)
	if v := a.Value("cache-type-v"); v != "" {
		t.Fatalf("cache-type-v prefill = %q, want same as keys", v)
	}
	if got := changesFor(t, qs, f, a, existing); len(got) != 0 {
		t.Fatalf("unchanged precision produced changes: %+v", got)
	}

	// A different value precision wins over the keys question.
	*a.Sel["cache-type-v"] = "f16"
	got := changesFor(t, qs, f, a, existing)
	if got["cache-type-v"].New != "f16" {
		t.Fatalf("cache-type-v = %+v", got["cache-type-v"])
	}

	// Values follow keys when left empty.
	a = NewAnswers(qs)
	*a.Sel["cache-type-k"] = "q4_0"
	got = changesFor(t, qs, f, a, nil)
	if got["cache-type-k"].New != "q4_0" || got["cache-type-v"].New != "q4_0" {
		t.Fatalf("changes = %+v", got)
	}
}

func TestClassicSpeculation(t *testing.T) {
	f := sampleFacts()
	qs := Chat()
	a := NewAnswers(qs)
	*a.Sel["nseq-max"] = "4"
	*a.Sel["speculation"] = "classic"
	*a.Sel["draft-model.model-id"] = "test/draft"

	got := changesFor(t, qs, f, a, nil)
	if got["nseq-max"].New != 1 {
		t.Fatalf("classic must set nseq-max to 1, got %+v", got["nseq-max"])
	}
	if got["draft-model.model-id"].New != "test/draft" {
		t.Fatalf("draft = %+v", got["draft-model.model-id"])
	}

	cfg, err := ToModelConfig(Apply(nil, func() []configfile.Change {
		var out []configfile.Change
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
	qs := Chat()
	existing := map[string]any{
		"devices":      []any{"CUDA0", "CUDA1"},
		"tensor-split": []any{0.6, 0.4},
		"adapters":     []any{map[string]any{"id": "style", "scale": 0.8}, map[string]any{"path": "/srv/lora/x.gguf"}},
	}

	a := NewAnswers(qs)
	a.Prefill(f, existing)

	if v := a.Value("adapters"); v != "style=0.8\n/srv/lora/x.gguf" {
		t.Fatalf("adapters prefill = %q", v)
	}
	if got := changesFor(t, qs, f, a, existing); len(got) != 0 {
		t.Fatalf("unchanged lists produced changes: %+v", got)
	}

	*a.Multi["devices"] = []string{"CUDA1"}
	got := changesFor(t, qs, f, a, existing)
	if configfile.InlineYAML(got["devices"].New) != "[CUDA1]" {
		t.Fatalf("devices = %+v", got["devices"])
	}
}

func TestParseAdapters(t *testing.T) {
	list, err := parseAdapters("style=0.8\n/srv/lora/x.gguf\nnested/one.gguf")
	if err != nil {
		t.Fatal(err)
	}
	if configfile.InlineYAML(list) != "[{id: style, scale: 0.8}, {path: /srv/lora/x.gguf}, {id: nested/one}]" {
		t.Fatalf("adapters = %s", configfile.InlineYAML(list))
	}

	for _, bad := range []string{"style=strong", "./rel.gguf", "~/x.gguf", "/srv/lora/x.bin", "style=-1", "style\nstyle=0.5"} {
		if _, err := parseAdapters(bad); err == nil {
			t.Errorf("%q: no error", bad)
		}
	}
}

func TestVisibility2(t *testing.T) {
	f := sampleFacts() // 2 GPUs, MoE
	a := NewAnswers(Chat())
	shown := func(key string) bool { return a.Applies(f, a.ByKey[key]) }

	if !shown("devices") || !shown("split-mode") || shown("main-gpu") {
		t.Fatal("two GPUs: devices and split-mode asked, main-gpu only for split none")
	}
	*a.Sel["split-mode"] = "none"
	if !shown("main-gpu") || shown("tensor-split") {
		t.Fatal("split none: main-gpu asked, tensor-split skipped")
	}

	if shown("numa") || shown("nthreads") {
		t.Fatal("all layers on the GPU: no NUMA or thread questions")
	}
	*a.Sel["moe.mode"] = "experts_cpu"
	if !shown("numa") || !shown("nthreads") {
		t.Fatal("experts on the CPU: NUMA and threads asked")
	}

	f.Devices.Devices = f.Devices.Devices[:2] // CPU and one GPU
	if shown("devices") {
		t.Fatal("one GPU: no multi-GPU questions")
	}

}
