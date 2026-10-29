package questions

import (
	"strings"
	"testing"
)

func TestProblems(t *testing.T) {
	for name, tt := range map[string]struct {
		flat map[string]any
		want string
	}{
		"quantized V without flash": {map[string]any{"cache-type-v": "q8_0", "flash-attention": "disabled"}, "flash attention"},
		"cache capacity too small":  {map[string]any{"nseq-max": 4, "queue-depth": 2, "imc-session-capacity": 4}, "at least 8"},
		"shares per GPU":            {map[string]any{"devices": []any{"CUDA0", "CUDA1"}, "tensor-split": []any{1.0}}, "one per GPU"},
		"projector twice":           {map[string]any{"proj-device": "CUDA1", "proj-on-cpu": true}, "projector"},
		"keep top N without N":      {map[string]any{"moe.mode": "keep_top_n"}, "number of layers"},
	} {
		got := problemsFor(t, tt.flat)
		if len(got) != 1 || !strings.Contains(got[0], tt.want) {
			t.Errorf("%s: problems = %v, want one about %q", name, got, tt.want)
		}
	}

	ok := map[string]any{
		"cache-type-k": "q8_0", "cache-type-v": "q8_0", "flash-attention": "auto",
		"nseq-max": 2, "queue-depth": 3, "imc-session-capacity": 6,
		"devices": []any{"CUDA0", "CUDA1"}, "tensor-split": []any{0.6, 0.4},
		"moe.mode": "keep_top_n", "moe.keep-experts-top-n": 4,
	}
	if got := problemsFor(t, ok); len(got) != 0 {
		t.Fatalf("valid answers reported: %v", got)
	}
}

func problemsFor(t *testing.T, flat map[string]any) []string {
	t.Helper()
	mc, err := ToModelConfig(flat)
	if err != nil {
		t.Fatalf("decode %v: %v", flat, err)
	}
	return Problems(mc)
}

func TestQueueDepthChoices(t *testing.T) {
	f := sampleFacts()
	qs := Chat()
	a := NewAnswers(qs)

	q := a.ByKey["queue-depth"]
	var values []string
	for _, o := range q.Options(f) {
		values = append(values, o.Value)
	}
	if strings.Join(values, ",") != "1,2,3" || q.Same != "2" {
		t.Fatalf("queue-depth values = %v, same = %q; 2 is Kronk's default", values, q.Same)
	}

	*a.Sel["queue-depth"] = "1"
	if got := changesFor(t, qs, f, a, nil); got["queue-depth"].New != 1 {
		t.Fatalf("queue-depth = %+v", got["queue-depth"])
	}
}

func TestContextOptionsWithinTraining(t *testing.T) {
	f := sampleFacts() // trained on 40960
	var values []string
	for _, o := range NewAnswers(Chat()).ByKey["context-window"].Options(f) {
		values = append(values, o.Value)
	}
	if strings.Join(values, ",") != "8192,32768,40960" {
		t.Fatalf("context options = %v; larger sizes go through Other value and YaRN", values)
	}
}

func TestRemovedQuestions(t *testing.T) {
	a := NewAnswers(Chat())
	for _, key := range []string{
		"sampling-parameters.repeat_last_n", "sampling-parameters.dry_multiplier", "sampling-parameters.xtc_probability",
		"sampling-parameters.seed", "physical-batch-size", "image-batch-token-capacity",
	} {
		if _, ok := a.ByKey[key]; ok {
			t.Errorf("%s is left to requests or AutoTune and must not be asked", key)
		}
	}
}
