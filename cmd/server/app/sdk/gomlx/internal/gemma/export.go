package gemma

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/gomlx/internal/gguf"
	"github.com/gomlx/compute/dtypes"
	"github.com/gomlx/gomlx/core/tensors"
	"github.com/gomlx/gomlx/ml/model"
	gomlxcheckpoint "github.com/gomlx/gomlx/ml/model/checkpoint"
)

var loraVariablePath = regexp.MustCompile(`^/layer_([0-9]+)/(self_attn/MultiHeadAttention/(query|key|value|output)|mlp/(gate_proj|up_proj|down_proj))/lora/(a|b)$`)

// ExportBestAdapter exports the validation-selected checkpoint as a
// llama.cpp-compatible F32 LoRA GGUF without modifying the base model.
func (runtime *Runtime) ExportBestAdapter(result TrainingResult, output string) error {
	if runtime == nil || runtime.base == nil || runtime.adapterConfig == nil {
		return fmt.Errorf("gemma LoRA runtime is closed")
	}
	if err := validateSelectedCheckpoint(result); err != nil {
		return err
	}
	return exportAdapterGGUF(result.BestCheckpoint, output, runtime.base.config, *runtime.adapterConfig)
}

// ExportAdapterCheckpoint exports one retained checkpoint for generated-answer
// validation. The caller remains responsible for selecting and publishing it.
func (runtime *Runtime) ExportAdapterCheckpoint(checkpoint, output string) error {
	if runtime == nil || runtime.base == nil || runtime.adapterConfig == nil {
		return fmt.Errorf("gemma LoRA runtime is closed")
	}
	return exportAdapterGGUF(checkpoint, output, runtime.base.config, *runtime.adapterConfig)
}

func validateSelectedCheckpoint(result TrainingResult) error {
	if result.BestEpoch <= 0 || result.BestCheckpoint == "" {
		return fmt.Errorf("training result contains no selected checkpoint")
	}
	selected := false
	for _, epoch := range result.Epochs {
		if epoch.Epoch == result.BestEpoch && epoch.Checkpoint == result.BestCheckpoint && epoch.ValidationLoss == result.BestValidationLoss {
			selected = true
			break
		}
	}
	if !selected {
		return fmt.Errorf("selected checkpoint does not match an evaluated epoch")
	}
	return nil
}

func exportAdapterGGUF(checkpointDirectory, output string, config Config, adapterConfig AdapterConfig) error {
	if checkpointDirectory == "" {
		return fmt.Errorf("adapter checkpoint directory is required")
	}
	if output == "" {
		return fmt.Errorf("adapter GGUF output path is required")
	}
	if err := validateAdapterConfig(adapterConfig); err != nil {
		return err
	}
	if adapterConfig.AdapterScale != 1 {
		return fmt.Errorf("lora adapter scale must equal one for GGUF export")
	}
	if adapterConfig.Alpha > math.MaxFloat32 {
		return fmt.Errorf("lora alpha is too large for GGUF F32 metadata")
	}

	store := model.NewStore()
	defer store.Finalize()
	if _, err := gomlxcheckpoint.Load(store).Dir(checkpointDirectory).Immediate().Done(); err != nil {
		return fmt.Errorf("load adapter checkpoint %q: %w", checkpointDirectory, err)
	}

	expected := config.LayerCount * 14
	exported := make([]gguf.Tensor, 0, expected)
	seen := make(map[string]bool, expected)
	for variable := range store.IterVariables() {
		if variable.Path() == "/global_step" {
			continue
		}
		match := loraVariablePath.FindStringSubmatch(variable.Path())
		if match == nil {
			return fmt.Errorf("adapter checkpoint contains non-LoRA tensor %q", variable.Path())
		}
		layer, err := strconv.Atoi(match[1])
		if err != nil || layer < 0 || layer >= config.LayerCount {
			return fmt.Errorf("adapter checkpoint contains invalid LoRA layer in %q", variable.Path())
		}
		if seen[variable.Path()] {
			return fmt.Errorf("adapter checkpoint contains duplicate LoRA tensor %q", variable.Path())
		}
		seen[variable.Path()] = true

		projection, side := match[2], match[5]
		inputDimension := config.EmbeddingDimension
		outputDimension := config.AttentionHeads * config.HeadDimension
		projectionName := "attn_q"
		switch projection {
		case "self_attn/MultiHeadAttention/key":
			outputDimension = config.KeyValueHeads * config.HeadDimension
			projectionName = "attn_k"
		case "self_attn/MultiHeadAttention/value":
			outputDimension = config.KeyValueHeads * config.HeadDimension
			projectionName = "attn_v"
		case "self_attn/MultiHeadAttention/output":
			inputDimension = config.AttentionHeads * config.HeadDimension
			outputDimension = config.EmbeddingDimension
			projectionName = "attn_output"
		case "mlp/gate_proj":
			outputDimension = config.FeedForwardDimension
			projectionName = "ffn_gate"
		case "mlp/up_proj":
			outputDimension = config.FeedForwardDimension
			projectionName = "ffn_up"
		case "mlp/down_proj":
			inputDimension = config.FeedForwardDimension
			outputDimension = config.EmbeddingDimension
			projectionName = "ffn_down"
		}
		wantShape := []int{adapterConfig.Rank, inputDimension}
		if side == "b" {
			wantShape = []int{outputDimension, adapterConfig.Rank}
		}
		shape := variable.Shape()
		if shape.DType != dtypes.Float32 || !slices.Equal(shape.Dimensions, wantShape) {
			return fmt.Errorf("LoRA tensor %q is %s, want F32 %v", variable.Path(), shape, wantShape)
		}
		value, err := variable.Value()
		if err != nil {
			return fmt.Errorf("read LoRA tensor %q: %w", variable.Path(), err)
		}
		var data []float32
		if err := tensors.ConstFlatData[float32](value, func(values []float32) {
			data = slices.Clone(values)
		}); err != nil {
			return fmt.Errorf("read LoRA tensor %q data: %w", variable.Path(), err)
		}
		exported = append(exported, gguf.Tensor{
			Name:       fmt.Sprintf("blk.%d.%s.weight.lora_%s", layer, projectionName, side),
			Dimensions: []uint64{uint64(wantShape[1]), uint64(wantShape[0])},
			Data:       data,
		})
	}
	if len(exported) != expected {
		return fmt.Errorf("adapter checkpoint contains %d LoRA tensors, want exactly %d", len(exported), expected)
	}
	slices.SortFunc(exported, func(left, right gguf.Tensor) int {
		return strings.Compare(left.Name, right.Name)
	})

	var writer gguf.Writer
	writer.String("general.type", "adapter")
	writer.String("general.architecture", "gemma3")
	writer.String("adapter.type", "lora")
	writer.Float32("adapter.lora.alpha", float32(adapterConfig.Alpha))
	writer.Uint32("general.alignment", gguf.Alignment)
	for _, tensor := range exported {
		writer.AddTensor(tensor)
	}
	if err := writer.WriteFile(output); err != nil {
		return fmt.Errorf("export Gemma adapter: %w", err)
	}
	return nil
}
