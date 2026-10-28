package kronk

import (
	"testing"
)

func TestSettings(t *testing.T) {
	settings := Settings()

	byKey := map[string]Setting{}
	for _, s := range settings {
		if s.Help == "" {
			t.Errorf("%s: no help tag", s.Key)
		}
		if s.Kind == "" {
			t.Errorf("%s: unknown kind", s.Key)
		}
		byKey[s.Key] = s
	}

	for key, want := range map[string]Setting{
		"web.api-host":        {Kind: KindString, Default: "127.0.0.1:11435"},
		"web.write-timeout":   {Kind: KindDuration, Default: "1h1m"},
		"web.admin.enabled":   {Kind: KindBool, Default: "true"},
		"pool.budget-percent": {Kind: KindInt, Default: "90"},
		"tempo.probability":   {Kind: KindFloat, Default: "0.25"},
		"authorization.mode":  {Kind: KindChoice, Default: "open"},
	} {
		got, ok := byKey[key]
		if !ok {
			t.Errorf("%s: missing", key)
			continue
		}
		if got.Kind != want.Kind || got.Default != want.Default {
			t.Errorf("%s = %s %q, want %s %q", key, got.Kind, got.Default, want.Kind, want.Default)
		}
	}

	if _, ok := byKey["pool.model-config-file"]; ok {
		t.Error("yaml:\"-\" fields must be skipped")
	}
	if !byKey["hf-token"].Secret {
		t.Error("hf-token must be secret")
	}
}

func TestCheckKMS(t *testing.T) {
	ok := map[string]any{"web": map[string]any{"api-host": "0.0.0.0:11435"}, "authorization": map[string]any{"mode": "management"}}
	if err := CheckKMS(ok); err != nil {
		t.Fatalf("valid kms: %v", err)
	}

	for name, kms := range map[string]map[string]any{
		"unknown key":  {"web": map[string]any{"api-hots": "x"}},
		"bad mode":     {"authorization": map[string]any{"mode": "nope"}},
		"bad duration": {"web": map[string]any{"read-timeout": "soon"}},
		"write <= inf": {"web": map[string]any{"write-timeout": "1m"}},
	} {
		if err := CheckKMS(kms); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}
