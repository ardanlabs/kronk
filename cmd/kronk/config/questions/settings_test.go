package questions

import (
	"testing"

	"github.com/ardanlabs/kronk/cmd/kronk/config/source"
	kronkserver "github.com/ardanlabs/kronk/cmd/server/api/services/kronk"
)

func TestExistingSettingsNoOp(t *testing.T) {
	f := sampleFacts()
	f.Adapters = nil
	qs := Chat()
	existing := map[string]any{
		"context-window":             131072,
		"rope-scaling-type":          "yarn",
		"rope-freq-scale":            0.4,
		"yarn-orig-ctx":              32768,
		"cache-type-k":               "f8_e4m3",
		"cache-type-v":               "q8_0",
		"ngpu-layers":                12,
		"nthreads":                   3,
		"nthreads-batch":             7,
		"insecure-logging":           true,
		"image-batch-token-capacity": 1536,
		"image-token-budget":         768,
		"speculation":                "classic",
		"nseq-max":                   1,
		"draft-model.model-id":       "test/draft",
		"draft-model.devices":        []any{"CUDA1", "CUDA0"},
		"draft-model.main-gpu":       1,
		"draft-model.ngpu-layers":    5,
		"draft-model.tensor-split":   []any{0.25, 0.75},
		"draft-model.ndraft":         6,
		"adapters":                   []any{map[string]any{"path": "/srv/lora/style.gguf", "scale": 0.6}},
	}
	a := NewAnswers(qs)
	a.Prefill(f, existing)
	for key, want := range map[string]string{
		"nthreads": "3", "nthreads-batch": "7",
		"cache-type-k": "f8_e4m3", "cache-type-v": "q8_0",
		"insecure-logging": "true", "image-batch-token-capacity": "1536",
		"draft-model.devices": "CUDA1,CUDA0", "draft-model.main-gpu": "1",
		"draft-model.ngpu-layers": "5", "draft-model.tensor-split": "0.25, 0.75",
		"adapters": "/srv/lora/style.gguf=0.6",
	} {
		q, ok := a.ByKey[key]
		if !ok || !a.Applies(f, q) {
			t.Errorf("%s: setting is not editable", key)
		}
		if got := a.Value(key); got != want {
			t.Errorf("%s prefill: got %q, want %q", key, got, want)
		}
	}
	if got := changesFor(t, qs, f, a, existing); len(got) != 0 {
		t.Fatalf("unchanged settings produced changes: %+v", got)
	}

	*a.Sel["nthreads"] = "5"
	got := changesFor(t, qs, f, a, existing)
	if len(got) != 1 || got["nthreads"].New != 5 {
		t.Fatalf("generation thread edit: got %+v, want only nthreads=5", got)
	}
	*a.Sel["nthreads"] = "3"
	*a.Sel["nthreads-batch"] = "11"
	got = changesFor(t, qs, f, a, existing)
	if len(got) != 1 || got["nthreads-batch"].New != 11 {
		t.Fatalf("batch thread edit: got %+v, want only nthreads-batch=11", got)
	}
}

func TestCachePrecisionNoOp(t *testing.T) {
	for name, existing := range map[string]map[string]any{
		"keys only":   {"cache-type-k": "q8_0"},
		"different":   {"cache-type-k": "q8_0", "cache-type-v": "f8_e4m3"},
		"values only": {"cache-type-v": "f8_e4m3"},
	} {
		t.Run(name, func(t *testing.T) {
			qs, f := Chat(), sampleFacts()
			a := NewAnswers(qs)
			a.Prefill(f, existing)
			if got := changesFor(t, qs, f, a, existing); len(got) != 0 {
				t.Fatalf("unchanged precision produced changes: %+v", got)
			}
		})
	}
}

func TestAdapterWithoutInventory(t *testing.T) {
	f := sampleFacts()
	f.Adapters = nil
	qs := Chat()
	a := NewAnswers(qs)
	if !a.Applies(f, a.ByKey["adapters"]) {
		t.Fatal("adapter input must allow absolute paths without a local inventory")
	}
	*a.Sel["adapters"] = "/srv/lora/style.gguf=0.6"
	got := changesFor(t, qs, f, a, nil)
	if len(got) != 1 || got["adapters"].New == nil {
		t.Fatalf("adapter input: got %+v, want one adapter change", got)
	}
}

func TestValuePrecisionResetToKeys(t *testing.T) {
	qs, f := Chat(), sampleFacts()
	existing := map[string]any{"cache-type-k": "q8_0", "cache-type-v": "f8_e4m3"}
	a := NewAnswers(qs)
	a.Prefill(f, existing)
	*a.Sel["cache-type-v"] = ""
	got := changesFor(t, qs, f, a, existing)
	if len(got) != 1 || got["cache-type-v"].New != "q8_0" {
		t.Fatalf("same as keys: got %+v, want only cache-type-v=q8_0", got)
	}
}

func TestSecretClear(t *testing.T) {
	for _, key := range []string{"hf-token", keyAdminPassword} {
		t.Run(key, func(t *testing.T) {
			existing := map[string]any{key: "existing-secret"}
			picked := []string{"runtime", "web"}
			qs := Server(kronkserver.Settings(), &picked, existing)
			a := NewAnswers(qs)
			a.Prefill(source.Facts{}, existing)
			if got := a.Value(key); got != "" {
				t.Fatalf("secret prefill: got %q, want empty", got)
			}
			if got := changesFor(t, qs, source.Facts{}, a, existing); len(got) != 0 {
				t.Fatalf("empty secret changed the file: %+v", got)
			}
			*a.Sel[key] = ClearSecretValue
			if err := a.ByKey[key].Validate(ClearSecretValue); err != nil {
				t.Fatal(err)
			}
			got := changesFor(t, qs, source.Facts{}, a, existing)
			if len(got) != 1 || !got[key].Del || got[key].New != nil {
				t.Fatalf("clear secret: got %+v, want deletion without a replacement", got)
			}
			if got := changesFor(t, qs, source.Facts{}, a, nil); len(got) != 0 {
				t.Fatalf("clearing absent secret produced changes: %+v", got)
			}
		})
	}
}
