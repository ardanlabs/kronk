package config

import (
	"slices"
	"strconv"
	"testing"

	"github.com/ardanlabs/kronk/sdk/tools/devices"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// sampleFacts turns on every model trait so every question and option is
// visible.
func sampleFacts() facts {
	return facts{
		ID: "test/model",
		Analysis: models.Analysis{
			Model: models.ModelFacts{
				TrainingContext: 40960,
				MoE:             &models.MoEInfo{},
				Attention:       models.AttentionFacts{SlidingWindow: 4096},
			},
		},
		Info:          models.ModelInfo{HasProjection: true, HasMTP: true},
		HardwareKnown: true,
		GPUs:          2,
		Devices: devices.Devices{Devices: []devices.DeviceInfo{
			{Name: "CPU", Type: "cpu"}, {Name: "CUDA0", Type: "gpu_cuda"}, {Name: "CUDA1", Type: "gpu_cuda"},
		}},
		Installed: []string{"test/model", "test/draft"},
		Adapters:  []string{"style"},
		Thinking:  true,
		Reasoning: true,
	}
}

// TestQuestionKeys checks that every key and option value is accepted by
// models.ModelConfig. It fails when a setting is renamed or a value is
// removed in the SDK.
func TestQuestionKeys(t *testing.T) {
	f := sampleFacts()

	for _, q := range questions() {
		var values []string
		if q.options != nil {
			for _, o := range q.options(f) {
				values = append(values, o.value)
			}
		}

		switch q.kind {
		case kindInt:
			values = append(values, strconv.Itoa(int(max(q.min, 1))))
		case kindFloat:
			values = append(values, trimFloat(max(q.min, 0.5)))
		case kindDuration:
			values = append(values, "90s")
		case kindVars:
			values = append(values, "preserve_thinking=true\nlevels=[low, high]")
		case kindText:
			values = append(values, "/srv/template.jinja")
		case kindFloatList:
			values = append(values, "0.6, 0.4")
		case kindAdapters:
			values = append(values, "style=0.8\n/srv/lora/other.gguf")
		}

		if len(values) == 0 {
			t.Fatalf("%s: no sample values", q.key)
		}

		for _, raw := range values {
			v, err := q.parse(raw)
			if err != nil {
				t.Errorf("%s=%q: parse: %v", q.key, raw, err)
				continue
			}

			flat := map[string]any{}
			for _, key := range q.keys() {
				flat[key] = v
			}

			if _, err := toModelConfig(flat); err != nil {
				t.Errorf("%s=%q: %v", q.key, raw, err)
			}
		}
	}
}

func TestQuestionGroups(t *testing.T) {
	seen := map[string]bool{}
	for _, q := range questions() {
		if groupOf(q.group).name != q.group || !hasGroup(q.group) {
			t.Errorf("%s: unknown group %q", q.key, q.group)
		}
		if seen[q.key] {
			t.Errorf("%s: asked twice", q.key)
		}
		seen[q.key] = true
		if q.def == nil || q.show == nil {
			t.Errorf("%s: missing def or show", q.key)
		}
	}
}

func hasGroup(name string) bool {
	for _, g := range groups {
		if g.name == name {
			return true
		}
	}
	return false
}

func TestVisibility(t *testing.T) {
	f := sampleFacts()
	f.Analysis.Model.MoE = nil
	f.Info.HasProjection = false

	qs := questions()
	a := newAnswers(qs)

	shown := func(key string) bool { return a.applies(f, a.byKey[key]) }

	if !shown("context-window") || !shown("nseq-max") {
		t.Fatal("essentials must always be asked")
	}
	if !shown("sampling-parameters.temperature") {
		t.Fatal("sampling questions must always be asked")
	}

	if shown("moe.mode") {
		t.Fatal("moe.mode must be skipped for a dense model")
	}

	if shown("proj-on-cpu") {
		t.Fatal("proj-on-cpu must be skipped without a projector")
	}

	*a.sel["incremental-cache"] = "false"
	if shown("cache-min-tokens") {
		t.Fatal("cache-min-tokens must be skipped when the cache is off")
	}
}

func TestPrefillAndDiff(t *testing.T) {
	f := sampleFacts()
	qs := questions()

	ctx := 32768
	nseq := 2
	existing, err := flattenConfig(models.ModelConfig{
		PtrContextWindow: &ctx,
		PtrNSeqMax:       &nseq,
		Sampling:         models.SamplingConfig{Temperature: 0.7},
	})
	if err != nil {
		t.Fatal(err)
	}

	a := newAnswers(qs)
	a.prefill(f, existing)

	if got := a.value("context-window"); got != "32768" {
		t.Fatalf("context-window prefill = %q", got)
	}

	changes, err := diff(qs, f, a, existing)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("unchanged answers produced changes: %+v", changes)
	}

	*a.sel["context-window"] = otherValue
	*a.other["context-window"] = "36000" // within the training context
	*a.sel["nseq-max"] = ""

	changes, err = diff(qs, f, a, existing)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]change{}
	for _, c := range changes {
		got[c.key] = c
	}
	if c := got["context-window"]; c.new != 36000 {
		t.Fatalf("context-window change = %+v", c)
	}
	if c := got["nseq-max"]; !c.del {
		t.Fatalf("nseq-max must be removed, got %+v", c)
	}
	if len(changes) != 2 {
		t.Fatalf("changes = %+v", changes)
	}
}

func TestSameIsAnOption(t *testing.T) {
	f := sampleFacts()
	for _, q := range questions() {
		if q.same == "" {
			continue
		}
		if q.options == nil || !slices.ContainsFunc(q.options(f), func(o option) bool { return o.value == q.same }) {
			t.Errorf("%s: same %q is not one of its options", q.key, q.same)
		}
	}
}

func TestDefaultValueInFileIsKept(t *testing.T) {
	f := sampleFacts()
	qs := questions()

	existing := map[string]any{"load-mode": "auto"}

	a := newAnswers(qs)
	a.prefill(f, existing)

	if got := a.value("load-mode"); got != "" {
		t.Fatalf("load-mode prefill = %q, want keep the default", got)
	}

	changes, err := diff(qs, f, a, existing)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("a default value in the file produced changes: %+v", changes)
	}
}

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
