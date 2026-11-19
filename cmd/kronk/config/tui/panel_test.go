package tui

import (
	"testing"

	"github.com/ardanlabs/kronk/cmd/kronk/config/questions"
	"github.com/ardanlabs/kronk/cmd/kronk/config/source"
	"github.com/ardanlabs/kronk/sdk/tools/devices"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func TestAnswerText(t *testing.T) {
	f := sampleFacts()
	f.Analysis.Recommended.NSeqMax = 4
	qs := questions.Chat()
	a := &app{facts: f, qs: qs, ans: questions.NewAnswers(qs)}
	answer := func(key string) (string, bool) { return a.answerText(a.ans.ByKey[key]) }

	if v, def := answer("ngpu-layers"); v != "All layers (default)" || !def {
		t.Fatalf("ngpu-layers default = %q %v", v, def)
	}
	if v, def := answer("nseq-max"); v != "4 (default)" || !def {
		t.Fatalf("nseq-max default = %q %v, want AutoTune's pick without the prefix", v, def)
	}

	*a.ans.Sel["ngpu-layers"] = "-1"
	if v, def := answer("ngpu-layers"); v != "None (CPU only)" || def {
		t.Fatalf("ngpu-layers = %q %v, want the option label", v, def)
	}

	*a.ans.Sel["context-window"] = questions.OtherValue
	if v, _ := answer("context-window"); v != "…" {
		t.Fatalf("other value not typed yet = %q", v)
	}
	*a.ans.Other["context-window"] = "12000"
	if v, _ := answer("context-window"); v != "12000" {
		t.Fatalf("other value = %q", v)
	}

	*a.ans.Sel[questions.KeyTemplateVars] = "a=1\nb=2\n"
	if v, _ := answer(questions.KeyTemplateVars); v != "a=1, b=2" {
		t.Fatalf("vars = %q", v)
	}

	// Every question asked has a row, and every chat question has help.
	if len(a.answerRows()) == 0 {
		t.Fatal("no rows")
	}
	for _, q := range qs {
		if q.About == "" && q.Tip == "" {
			t.Errorf("%s: no help", q.Key)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("abcdef", 4); got != "abc…" {
		t.Fatalf("truncate = %q", got)
	}
	if got := truncate("abc", 4); got != "abc" {
		t.Fatalf("truncate = %q", got)
	}
}

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
