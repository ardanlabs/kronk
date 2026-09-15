package adapter

import (
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/gomlx/compute"
	_ "github.com/gomlx/compute/gobackend"
	"github.com/gomlx/compute/shapes"
	"github.com/gomlx/gomlx/core/graph"
	"github.com/gomlx/gomlx/core/tensors"
	"github.com/gomlx/gomlx/ml/model"
	"github.com/gomlx/gomlx/ml/model/initializer"
)

func TestZeroInitializedLoRAPreservesBaseOutputAtEveryScale(t *testing.T) {
	t.Setenv("GOMLX_BACKEND", "go")

	for _, scale := range []float64{0, 0.5, 1} {
		t.Run(scaleName(scale), func(t *testing.T) {
			backend, err := compute.New()
			if err != nil {
				t.Fatalf("create backend: %v", err)
			}
			defer backend.Finalize()

			store := model.NewStore()
			defer store.Finalize()
			store.SetParam(model.ParamInitialSeed, int64(3407))

			exec, err := model.NewExec1(backend, store, func(scope *model.Scope, input *graph.Node) *graph.Node {
				projection := scope.In("projection")
				weight := projection.
					WithInitializer(initializer.One).
					VariableWithShape("base_weight", shapes.Make(input.DType(), 4, 3)).
					SetTrainable(false).
					NodeValue(input.Graph())
				base := graph.DotGeneral(input, []int{-1}, nil, weight, []int{1}, nil)
				adapted := ApplyScaled(projection, input, base, 2, 4, scale)
				return graph.ReduceAllMax(graph.Abs(graph.Sub(adapted, base)))
			})
			if err != nil {
				t.Fatalf("create executor: %v", err)
			}
			defer exec.Finalize()

			input := tensors.FromValue([][][]float32{{{1, 2, 3}, {4, 5, 6}}})
			defer input.FinalizeAll()
			difference, err := exec.Call(input)
			if err != nil {
				t.Fatalf("execute graph: %v", err)
			}
			defer difference.FinalizeAll()
			if got := difference.Value().(float32); got != 0 {
				t.Fatalf("zero-initialized LoRA changed base output by %f", got)
			}
		})
	}
}

func TestLoRARestrictsGradientsToAdapterVariables(t *testing.T) {
	t.Setenv("GOMLX_BACKEND", "go")

	backend, err := compute.New()
	if err != nil {
		t.Fatalf("create backend: %v", err)
	}
	defer backend.Finalize()
	store := model.NewStore()
	defer store.Finalize()
	store.SetParam(model.ParamInitialSeed, int64(3407))

	exec, err := model.NewExec2(backend, store, func(scope *model.Scope, input *graph.Node) (*graph.Node, *graph.Node) {
		projection := scope.In("projection")
		weight := projection.
			WithInitializer(initializer.One).
			VariableWithShape("base_weight", shapes.Make(input.DType(), 4, 3)).
			SetTrainable(false).
			NodeValue(input.Graph())
		base := graph.DotGeneral(input, []int{-1}, nil, weight, []int{1}, nil)
		loss := graph.ReduceAllSum(Apply(projection, input, base, 2, 4))
		gradients := projection.BuildTrainableVariablesGradientsGraph(loss)
		if len(gradients) != 2 {
			panic("expected gradients for LoRA A and B only")
		}
		return graph.ReduceAllSum(graph.Abs(gradients[0])), graph.ReduceAllSum(graph.Abs(gradients[1]))
	})
	if err != nil {
		t.Fatalf("create executor: %v", err)
	}
	defer exec.Finalize()

	input := tensors.FromValue([][][]float32{{{1, 2, 3}, {4, 5, 6}}})
	defer input.FinalizeAll()
	aGradient, bGradient, err := exec.Call(input)
	if err != nil {
		t.Fatalf("execute graph: %v", err)
	}
	defer aGradient.FinalizeAll()
	defer bGradient.FinalizeAll()
	aMagnitude := aGradient.Value().(float32)
	bMagnitude := bGradient.Value().(float32)
	if math.IsNaN(float64(aMagnitude)) || math.IsInf(float64(aMagnitude), 0) || aMagnitude != 0 {
		t.Fatalf("unexpected initial A gradient %f", aMagnitude)
	}
	if math.IsNaN(float64(bMagnitude)) || math.IsInf(float64(bMagnitude), 0) || bMagnitude <= 0 {
		t.Fatalf("unexpected initial B gradient %f", bMagnitude)
	}

	var trainable []string
	for variable := range store.IterVariables() {
		if variable.Trainable {
			trainable = append(trainable, variable.Path())
		}
	}
	if len(trainable) != 2 {
		t.Fatalf("trainable variables = %v, want LoRA A and B only", trainable)
	}
	for _, path := range trainable {
		if !strings.HasSuffix(path, "/lora/a") && !strings.HasSuffix(path, "/lora/b") {
			t.Fatalf("unexpected trainable variable %q", path)
		}
	}
}

func TestZeroInitializedWeightLoRAPreservesBaseWeightAtEveryScale(t *testing.T) {
	t.Setenv("GOMLX_BACKEND", "go")

	for _, scale := range []float64{0, 0.5, 1} {
		t.Run(scaleName(scale), func(t *testing.T) {
			backend, err := compute.New()
			if err != nil {
				t.Fatalf("create backend: %v", err)
			}
			defer backend.Finalize()
			store := model.NewStore()
			defer store.Finalize()
			store.SetParam(model.ParamInitialSeed, int64(3407))

			exec, err := model.NewExec1(backend, store, func(scope *model.Scope, input *graph.Node) *graph.Node {
				projection := scope.In("projection")
				base := projection.
					WithInitializer(initializer.One).
					VariableWithShape("weight", shapes.Make(input.DType(), 3, 2, 2)).
					SetTrainable(false).
					NodeValue(input.Graph())
				adapted := ApplyWeightScaled(projection, base, 2, 4, scale)
				return graph.ReduceAllMax(graph.Abs(graph.Sub(adapted, base)))
			})
			if err != nil {
				t.Fatalf("create executor: %v", err)
			}
			defer exec.Finalize()

			input := tensors.FromValue([]float32{1})
			defer input.FinalizeAll()
			difference, err := exec.Call(input)
			if err != nil {
				t.Fatalf("execute graph: %v", err)
			}
			defer difference.FinalizeAll()
			if got := difference.Value().(float32); got != 0 {
				t.Fatalf("zero-initialized weight LoRA changed base weight by %f", got)
			}

			wantShapes := map[string][]int{
				"/projection/lora/a": {2, 3},
				"/projection/lora/b": {4, 2},
			}
			for variable := range store.IterVariables() {
				want, isAdapter := wantShapes[variable.Path()]
				if variable.Trainable != isAdapter {
					t.Fatalf("variable %q trainable=%t, want %t", variable.Path(), variable.Trainable, isAdapter)
				}
				if isAdapter && !slices.Equal(variable.Shape().Dimensions, want) {
					t.Fatalf("variable %q shape=%v, want %v", variable.Path(), variable.Shape().Dimensions, want)
				}
			}
		})
	}
}

func scaleName(scale float64) string {
	switch scale {
	case 0:
		return "disabled"
	case 0.5:
		return "half"
	default:
		return "full"
	}
}
