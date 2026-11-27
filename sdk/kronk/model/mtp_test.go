package model

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

func TestQwen4ExpCompanionMTPResourceEstimates(t *testing.T) {
	file := filepath.Join(t.TempDir(), "qwen4exp-mtp.gguf")
	writeTestGGUF(t, file, map[string]any{
		"general.architecture":          "qwen4exp",
		"qwen4exp.nextn_predict_layers": uint32(1),
	})
	target := filepath.Join(t.TempDir(), "target.gguf")
	writeTestGGUF(t, target, map[string]any{"general.architecture": "llama"})
	cfg := Config{ModelFiles: []string{target}, DrafterFile: file}

	plan, err := resolveSpeculationPlan(t.Context(), nil, cfg)
	if err != nil || plan.Source != speculationSourceMTP || plan.MTPArtifact != mtpArtifactCompanion {
		t.Fatalf("auto companion plan = %+v, %v, want companion MTP", plan, err)
	}

	if !mtpFilesEnabled([]string{file}) {
		t.Fatal("mtpFilesEnabled = false, want Qwen4Exp MTP enabled")
	}
	if got := SpeculativeContextCount(cfg); got != 2 {
		t.Errorf("SpeculativeContextCount = %d, want 2 for Qwen4Exp companion MTP", got)
	}
}

func TestUnsupportedMTPResourceEstimates(t *testing.T) {
	for _, arch := range []string{"glm4moe", "glm5-next"} {
		t.Run(arch, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "model.gguf")
			writeTestGGUF(t, file, map[string]any{
				"general.architecture":         arch,
				arch + ".nextn_predict_layers": uint32(1),
			})
			if got := SpeculativeContextCount(Config{DrafterFile: file}); got != 1 {
				t.Errorf("SpeculativeContextCount: got %d, want 1", got)
			}
			if got := RecurrentStateCopies(Config{ModelFiles: []string{file}}, true); got != 1 {
				t.Errorf("RecurrentStateCopies: got %d, want 1", got)
			}
		})
	}
}

func TestResolveSpeculationPlanGLM5Next(t *testing.T) {
	file := filepath.Join(t.TempDir(), "glm5-next.gguf")
	writeTestGGUF(t, file, map[string]any{
		"general.architecture":           "glm5-next",
		"glm5-next.nextn_predict_layers": uint32(1),
	})

	plan, err := resolveSpeculationPlan(context.Background(), nil, Config{ModelFiles: []string{file}})
	if err != nil {
		t.Fatalf("resolveSpeculationPlan(auto) error = %v", err)
	}
	if plan.Source != speculationSourceNone || plan.LoadMTP {
		t.Fatalf("resolveSpeculationPlan(auto) = %+v, want target-only plan", plan)
	}
	if mtpFilesEnabled([]string{file}) {
		t.Fatal("mtpFilesEnabled = true, want false for GLM5-Next")
	}

	_, err = resolveSpeculationPlan(context.Background(), nil, Config{
		ModelFiles:  []string{file},
		Speculation: SpeculationMTP,
	})
	if err == nil || !strings.Contains(err.Error(), "MTP architecture glm5-next is unsupported") {
		t.Fatalf("resolveSpeculationPlan(mtp) error = %v, want unsupported glm5-next error", err)
	}
}

func TestModelFilesLoadMTPUsesFirstShard(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "model-00001-of-00002.gguf")
	second := filepath.Join(dir, "model-00002-of-00002.gguf")

	writeTestGGUF(t, first, map[string]any{"general.architecture": "qwen35moe", "qwen35moe.block_count": uint32(41), "qwen35moe.nextn_predict_layers": uint32(1)})
	writeTestGGUF(t, second, nil, "blk.40.nextn.eh_proj.weight")

	got, err := modelFilesMTPArchitecture([]string{first, second})
	if err != nil {
		t.Fatalf("modelFilesMTPArchitecture() error = %v", err)
	}
	if got != modelprofile.MTPArchitectureQwen35OwnKV {
		t.Errorf("modelFilesMTPArchitecture() = %q, want %q", got, modelprofile.MTPArchitectureQwen35OwnKV)
	}

	writeTestGGUF(t, first, nil)
	writeTestGGUF(t, second, map[string]any{"general.architecture": "qwen35moe", "qwen35moe.nextn_predict_layers": uint32(1)})

	got, err = modelFilesMTPArchitecture([]string{first, second})
	if err != nil {
		t.Fatalf("modelFilesMTPArchitecture() error = %v", err)
	}
	if got != modelprofile.MTPArchitectureNone {
		t.Errorf("modelFilesMTPArchitecture() = %q, want none", got)
	}
}

func TestSpeculationRejectsStrippedQwenMTP(t *testing.T) {
	for _, arch := range []string{"qwen35", "qwen35moe"} {
		t.Run(arch, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "model.gguf")
			writeTestGGUF(t, file, map[string]any{
				"general.architecture":         arch,
				arch + ".block_count":          uint32(41),
				arch + ".nextn_predict_layers": uint32(1),
			}, "blk.39.nextn.eh_proj.weight")
			plan, err := resolveSpeculationPlan(context.Background(), nil, Config{ModelFiles: []string{file}})
			if err != nil {
				t.Fatalf("automatic speculation: %v", err)
			}
			if plan.Source != speculationSourceNone || plan.LoadMTP {
				t.Fatalf("plan = %+v, want target only for missing MTP block 40", plan)
			}
			if mtpFilesEnabled([]string{file}) {
				t.Fatal("missing MTP weights reserve draft resources")
			}
			if _, err := resolveSpeculationPlan(context.Background(), nil, Config{ModelFiles: []string{file}, Speculation: SpeculationMTP}); err == nil {
				t.Fatal("explicit MTP accepted an artifact without its MTP head")
			}

			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(file, data[:len(data)-1], 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := resolveSpeculationPlan(context.Background(), nil, Config{ModelFiles: []string{file}}); err == nil {
				t.Fatal("truncated tensor directory treated as confirmed absent MTP weights")
			}
		})
	}
}

func writeTestGGUF(t *testing.T, file string, metadata map[string]any, tensorNames ...string) {
	t.Helper()

	var data bytes.Buffer
	values := []any{gguf.Magic, uint32(3), uint64(len(tensorNames)), uint64(len(metadata))}
	for _, value := range values {
		if err := binary.Write(&data, binary.LittleEndian, value); err != nil {
			t.Fatalf("binary.Write() error = %v", err)
		}
	}

	for key, value := range metadata {
		if err := binary.Write(&data, binary.LittleEndian, uint64(len(key))); err != nil {
			t.Fatalf("binary.Write() key length error = %v", err)
		}
		if _, err := data.WriteString(key); err != nil {
			t.Fatalf("WriteString() error = %v", err)
		}
		switch value := value.(type) {
		case uint32:
			if err := binary.Write(&data, binary.LittleEndian, gguf.MetadataValueTypeUInt32); err != nil {
				t.Fatalf("binary.Write() value type error = %v", err)
			}
			if err := binary.Write(&data, binary.LittleEndian, value); err != nil {
				t.Fatalf("binary.Write() value error = %v", err)
			}
		case string:
			if err := binary.Write(&data, binary.LittleEndian, gguf.MetadataValueTypeString); err != nil {
				t.Fatalf("binary.Write() value type error = %v", err)
			}
			if err := binary.Write(&data, binary.LittleEndian, uint64(len(value))); err != nil {
				t.Fatalf("binary.Write() string length error = %v", err)
			}
			if _, err := data.WriteString(value); err != nil {
				t.Fatalf("WriteString() value error = %v", err)
			}
		default:
			t.Fatalf("unsupported metadata value type %T", value)
		}
	}

	for _, name := range tensorNames {
		for _, value := range []any{uint64(len(name)), []byte(name), uint32(1), uint64(1), uint32(0), uint64(0)} {
			if err := binary.Write(&data, binary.LittleEndian, value); err != nil {
				t.Fatalf("write tensor descriptor: %v", err)
			}
		}
	}

	if err := os.WriteFile(file, data.Bytes(), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
}
