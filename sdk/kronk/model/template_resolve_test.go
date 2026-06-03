package model

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStripQuantSuffix(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// Standard k-quants (dot and hyphen separators).
		{"dot Q8_0", "Qwopus3.5-4B-Coder.Q8_0", "Qwopus3.5-4B-Coder"},
		{"hyphen Q8_0", "Qwen3.5-0.8B-Q8_0", "Qwen3.5-0.8B"},
		{"dot Q4_K_M", "MyModel-7B.Q4_K_M", "MyModel-7B"},
		{"hyphen Q5_K_S", "MyModel-7B-Q5_K_S", "MyModel-7B"},
		{"dot Q6_K", "MyModel-7B.Q6_K", "MyModel-7B"},

		// Block k-quants with extra _N_N tail.
		{"block Q4_0_4_4", "MyModel-7B.Q4_0_4_4", "MyModel-7B"},
		{"block Q4_0_8_8", "MyModel-7B.Q4_0_8_8", "MyModel-7B"},

		// i-quants.
		{"dot IQ4_NL", "MyModel-7B.IQ4_NL", "MyModel-7B"},
		{"dot IQ3_XXS", "MyModel-7B.IQ3_XXS", "MyModel-7B"},
		{"hyphen IQ2_M", "MyModel-7B-IQ2_M", "MyModel-7B"},

		// Unsloth dynamic.
		{"UD-Q8_K_XL", "Qwen3.6-35B-A3B-UD-Q8_K_XL", "Qwen3.6-35B-A3B"},
		{"UD-Q4_K_XL", "MyModel-7B-UD-Q4_K_XL", "MyModel-7B"},

		// Float variants, case-insensitive.
		{"dot f16", "MyModel-7B.f16", "MyModel-7B"},
		{"dot F16", "MyModel-7B.F16", "MyModel-7B"},
		{"hyphen bf16", "MyModel-7B-bf16", "MyModel-7B"},
		{"dot f32", "MyModel-7B.f32", "MyModel-7B"},

		// No quant suffix — left untouched.
		{"no suffix", "MyModel-7B", "MyModel-7B"},
		{"empty", "", ""},
		{"role tag preserved", "Qwen3.6-35B-A3B/AGENT", "Qwen3.6-35B-A3B/AGENT"},

		// Suffix-like substrings that should NOT be stripped (not at end).
		{"quant mid-string", "Q8_0-Variant", "Q8_0-Variant"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripQuantSuffix(tt.input); got != tt.want {
				t.Errorf("stripQuantSuffix(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestAutoDiscoverTemplate(t *testing.T) {
	// Redirect the base path to a per-test temp dir so we don't touch the
	// real ~/.kronk/jinja directory. defaults.JinjaDir reads
	// KRONK_BASE_PATH via defaults.BaseDir.
	tmpBase := t.TempDir()
	jinjaDir := filepath.Join(tmpBase, "jinja")

	if err := os.MkdirAll(jinjaDir, 0o755); err != nil {
		t.Fatalf("mkdir jinja: %v", err)
	}

	t.Setenv("KRONK_BASE_PATH", tmpBase)

	const stub = "{# stub #}"

	writeFile := func(t *testing.T, name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(jinjaDir, name), []byte(stub), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	t.Run("empty model id", func(t *testing.T) {
		if _, ok := autoDiscoverTemplate(""); ok {
			t.Fatal("autoDiscoverTemplate(\"\") = ok, want false")
		}
	})

	t.Run("no match", func(t *testing.T) {
		if _, ok := autoDiscoverTemplate("DoesNotExist.Q8_0"); ok {
			t.Fatal("autoDiscoverTemplate(\"DoesNotExist.Q8_0\") = ok, want false")
		}
	})

	t.Run("exact match wins over stripped", func(t *testing.T) {
		writeFile(t, "MatchExact.Q8_0.jinja")
		t.Cleanup(func() { _ = os.Remove(filepath.Join(jinjaDir, "MatchExact.Q8_0.jinja")) })

		tmpl, ok := autoDiscoverTemplate("MatchExact.Q8_0")
		if !ok {
			t.Fatal("autoDiscoverTemplate: ok=false, want true")
		}
		if want := filepath.Join(jinjaDir, "MatchExact.Q8_0.jinja"); tmpl.FileName != want {
			t.Errorf("FileName = %q, want %q", tmpl.FileName, want)
		}
		if tmpl.Script != stub {
			t.Errorf("Script = %q, want %q", tmpl.Script, stub)
		}
	})

	t.Run("falls back to stripped match", func(t *testing.T) {
		writeFile(t, "MatchStripped.jinja")
		t.Cleanup(func() { _ = os.Remove(filepath.Join(jinjaDir, "MatchStripped.jinja")) })

		tmpl, ok := autoDiscoverTemplate("MatchStripped.Q8_0")
		if !ok {
			t.Fatal("autoDiscoverTemplate: ok=false, want true")
		}
		if want := filepath.Join(jinjaDir, "MatchStripped.jinja"); tmpl.FileName != want {
			t.Errorf("FileName = %q, want %q", tmpl.FileName, want)
		}
	})

	t.Run("exact wins when both present", func(t *testing.T) {
		writeFile(t, "Both.Q8_0.jinja")
		writeFile(t, "Both.jinja")
		t.Cleanup(func() {
			_ = os.Remove(filepath.Join(jinjaDir, "Both.Q8_0.jinja"))
			_ = os.Remove(filepath.Join(jinjaDir, "Both.jinja"))
		})

		tmpl, ok := autoDiscoverTemplate("Both.Q8_0")
		if !ok {
			t.Fatal("autoDiscoverTemplate: ok=false, want true")
		}
		if want := filepath.Join(jinjaDir, "Both.Q8_0.jinja"); tmpl.FileName != want {
			t.Errorf("FileName = %q, want exact match %q", tmpl.FileName, want)
		}
	})
}
