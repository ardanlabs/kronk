package questions

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/kronk/config/configfile"
	"github.com/ardanlabs/kronk/cmd/kronk/config/source"
	"github.com/ardanlabs/kronk/sdk/tools/devices"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// sampleFacts turns on every model trait so every question and option is
// visible.
func sampleFacts() source.Facts {
	return source.Facts{
		ID:       "test/model",
		Endpoint: source.EndpointChat,
		Analysis: models.Analysis{
			Model: models.ModelFacts{
				TrainingContext: 40960,
				MoE:             &models.MoEInfo{},
				Attention:       models.AttentionFacts{SlidingWindow: 4096},
			},
		},
		Info:          models.ModelInfo{HasProjection: true, HasMTP: true},
		HardwareKnown: true,
		Devices: devices.Devices{Devices: []devices.DeviceInfo{
			{Name: "CPU", Type: "cpu"}, {Name: "CUDA0", Type: "gpu_cuda"}, {Name: "CUDA1", Type: "gpu_cuda"},
		}},
		Drafts:   []string{"test/draft"},
		Adapters: []string{"style"},
	}
}

// TestQuestionKeys checks that every key and option value is accepted by
// models.ModelConfig. It fails when a setting is renamed or a value is
// removed in the SDK.
func TestQuestionKeys(t *testing.T) {
	f := sampleFacts()

	for _, q := range Chat() {
		var values []string
		if q.Options != nil {
			for _, o := range q.Options(f) {
				values = append(values, o.Value)
			}
		}

		switch q.Kind {
		case kindInt:
			values = append(values, strconv.Itoa(int(max(q.min, 1))))
		case kindFloat:
			values = append(values, trimFloat(max(q.min, 0.5)))
		case kindDuration:
			values = append(values, "90s")
		case KindVars:
			values = append(values, "preserve_thinking=true\nlevels=[low, high]")
		case kindText:
			values = append(values, "/srv/template.jinja")
		case kindFloatList:
			values = append(values, "0.6, 0.4")
		case KindAdapters:
			values = append(values, "style=0.8\n/srv/lora/other.gguf")
		}

		if len(values) == 0 {
			t.Fatalf("%s: no sample values", q.Key)
		}

		for _, raw := range values {
			v, err := q.parse(raw)
			if err != nil {
				t.Errorf("%s=%q: parse: %v", q.Key, raw, err)
				continue
			}

			flat := map[string]any{}
			for _, key := range q.Keys() {
				flat[key] = v
			}

			if _, err := ToModelConfig(flat); err != nil {
				t.Errorf("%s=%q: %v", q.Key, raw, err)
			}
		}
	}
}

func TestQuestionGroups(t *testing.T) {
	seen := map[string]bool{}
	for _, q := range Chat() {
		if !slices.Contains(groups, q.Group) {
			t.Errorf("%s: unknown group %q", q.Key, q.Group)
		}
		if seen[q.Key] {
			t.Errorf("%s: asked twice", q.Key)
		}
		seen[q.Key] = true
		if q.Def == nil || q.show == nil {
			t.Errorf("%s: missing def or show", q.Key)
		}
	}
}

// groups are the names a chat model question can be grouped under.
var groups = []string{gEssentials, gBehavior, gSampling, gMemory, gGPUs, gThroughput, gFeatures}

func TestVisibility(t *testing.T) {
	f := sampleFacts()
	f.Analysis.Model.MoE = nil
	f.Info.HasProjection = false

	qs := Chat()
	a := NewAnswers(qs)

	shown := func(key string) bool { return a.Applies(f, a.ByKey[key]) }

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

	*a.Sel["incremental-cache"] = "false"
	if shown("cache-min-tokens") {
		t.Fatal("cache-min-tokens must be skipped when the cache is off")
	}
}

func TestPrefillAndDiff(t *testing.T) {
	f := sampleFacts()
	qs := Chat()

	ctx := 32768
	nseq := 2
	existing, err := configfile.FlattenYAML(models.ModelConfig{
		PtrContextWindow: &ctx,
		PtrNSeqMax:       &nseq,
		Sampling:         models.SamplingConfig{Temperature: 0.7},
	})
	if err != nil {
		t.Fatal(err)
	}

	a := NewAnswers(qs)
	a.Prefill(f, existing)

	if got := a.Value("context-window"); got != "32768" {
		t.Fatalf("context-window prefill = %q", got)
	}

	changes, err := Diff(qs, f, a, existing)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("unchanged answers produced changes: %+v", changes)
	}

	*a.Sel["context-window"] = OtherValue
	*a.Other["context-window"] = "36000" // within the training context
	*a.Sel["nseq-max"] = ""

	changes, err = Diff(qs, f, a, existing)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]configfile.Change{}
	for _, c := range changes {
		got[c.Key] = c
	}
	if c := got["context-window"]; c.New != 36000 {
		t.Fatalf("context-window change = %+v", c)
	}
	if c := got["nseq-max"]; !c.Del {
		t.Fatalf("nseq-max must be removed, got %+v", c)
	}
	if len(changes) != 2 {
		t.Fatalf("changes = %+v", changes)
	}
}

func TestSameIsAnOption(t *testing.T) {
	f := sampleFacts()
	for _, q := range Chat() {
		if q.Same == "" {
			continue
		}
		if q.Options == nil || !slices.ContainsFunc(q.Options(f), func(o option) bool { return o.Value == q.Same }) {
			t.Errorf("%s: same %q is not one of its options", q.Key, q.Same)
		}
	}
}

func TestDefaultValueInFileIsKept(t *testing.T) {
	f := sampleFacts()
	qs := Chat()

	existing := map[string]any{"load-mode": "auto"}

	a := NewAnswers(qs)
	a.Prefill(f, existing)

	if got := a.Value("load-mode"); got != "" {
		t.Fatalf("load-mode prefill = %q, want keep the default", got)
	}

	changes, err := Diff(qs, f, a, existing)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("a default value in the file produced changes: %+v", changes)
	}
}

func TestShowWithoutDevicesQuestion(t *testing.T) {
	f := sampleFacts() // 2 GPUs

	var qs []Question
	for _, q := range Chat() {
		if q.Key != "devices" {
			qs = append(qs, q)
		}
	}
	a := NewAnswers(qs)

	// split-mode and tensor-split read the devices answer; without the
	// question they must treat it as none picked, not panic.
	for _, q := range qs {
		a.Applies(f, q)
	}
}

func TestSplitList(t *testing.T) {
	got := SplitList(" a, ,b ,\n", ",")
	if strings.Join(got, "|") != "a|b" {
		t.Fatalf("splitList = %q", got)
	}
	if got := SplitList("x=1\n\n  y=2  \n", "\n"); strings.Join(got, "|") != "x=1|y=2" {
		t.Fatalf("splitList lines = %q", got)
	}
}
