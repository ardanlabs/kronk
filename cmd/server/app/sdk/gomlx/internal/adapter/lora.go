package adapter

import (
	"fmt"
	"math"

	"github.com/gomlx/compute/shapes"
	"github.com/gomlx/gomlx/core/graph"
	"github.com/gomlx/gomlx/ml/model"
	"github.com/gomlx/gomlx/ml/model/initializer"
)

// Apply adds a PEFT-compatible low-rank update to an existing projection.
func Apply(scope *model.Scope, input, baseOutput *graph.Node, rank int, alpha float64) *graph.Node {
	return ApplyScaled(scope, input, baseOutput, rank, alpha, 1)
}

// ApplyScaled adds a low-rank update multiplied by adapterScale. A scale of
// zero creates the adapter variables without changing the base projection.
func ApplyScaled(scope *model.Scope, input, baseOutput *graph.Node, rank int, alpha, adapterScale float64) *graph.Node {
	if scope == nil {
		panic("LoRA scope must not be nil")
	}
	if input == nil {
		panic("LoRA input must not be nil")
	}
	if baseOutput == nil {
		panic("LoRA base output must not be nil")
	}
	if rank <= 0 {
		panic(fmt.Sprintf("LoRA rank must be greater than 0, got %d", rank))
	}
	if alpha <= 0 || math.IsNaN(alpha) || math.IsInf(alpha, 0) {
		panic(fmt.Sprintf("LoRA alpha must be finite and greater than 0, got %f", alpha))
	}
	if adapterScale < 0 || math.IsNaN(adapterScale) || math.IsInf(adapterScale, 0) {
		panic(fmt.Sprintf("LoRA adapter scale must be finite and non-negative, got %f", adapterScale))
	}

	inputDimension := input.Shape().Dim(-1)
	outputDimension := baseOutput.Shape().Dim(-1)
	if rank > inputDimension {
		panic(fmt.Sprintf("LoRA rank %d exceeds input dimension %d", rank, inputDimension))
	}

	loraScope := scope.In("lora")
	a := loraScope.
		WithInitializer(initializer.RandomNormalFn(loraScope, 0.01)).
		VariableWithShape("a", shapes.Make(input.DType(), rank, inputDimension)).
		SetTrainable(true).
		NodeValue(input.Graph())
	b := loraScope.
		WithInitializer(initializer.Zero).
		VariableWithShape("b", shapes.Make(input.DType(), outputDimension, rank)).
		SetTrainable(true).
		NodeValue(input.Graph())

	// PEFT stores A as [rank, input] and B as [output, rank].
	update := graph.DotGeneral(input, []int{-1}, nil, a, []int{1}, nil)
	update = graph.DotGeneral(update, []int{-1}, nil, b, []int{1}, nil)
	update = graph.MulScalar(update, alpha/float64(rank)*adapterScale)

	return graph.Add(baseOutput, update)
}

// ApplyWeight adds a PEFT-compatible low-rank update directly to a projection
// weight. The first weight dimension is the input dimension; all remaining
// dimensions form the flattened output dimension.
func ApplyWeight(scope *model.Scope, baseWeight *graph.Node, rank int, alpha float64) *graph.Node {
	return ApplyWeightScaled(scope, baseWeight, rank, alpha, 1)
}

// ApplyWeightScaled adds a low-rank weight update multiplied by adapterScale.
func ApplyWeightScaled(scope *model.Scope, baseWeight *graph.Node, rank int, alpha, adapterScale float64) *graph.Node {
	if scope == nil {
		panic("LoRA scope must not be nil")
	}
	if baseWeight == nil {
		panic("LoRA base weight must not be nil")
	}
	if baseWeight.Shape().Rank() < 2 {
		panic(fmt.Sprintf("LoRA base weight rank must be at least 2, got %d", baseWeight.Shape().Rank()))
	}
	if rank <= 0 {
		panic(fmt.Sprintf("LoRA rank must be greater than 0, got %d", rank))
	}
	if alpha <= 0 || math.IsNaN(alpha) || math.IsInf(alpha, 0) {
		panic(fmt.Sprintf("LoRA alpha must be finite and greater than 0, got %f", alpha))
	}
	if adapterScale < 0 || math.IsNaN(adapterScale) || math.IsInf(adapterScale, 0) {
		panic(fmt.Sprintf("LoRA adapter scale must be finite and non-negative, got %f", adapterScale))
	}

	inputDimension := baseWeight.Shape().Dim(0)
	if rank > inputDimension {
		panic(fmt.Sprintf("LoRA rank %d exceeds input dimension %d", rank, inputDimension))
	}
	outputDimension := baseWeight.Shape().Size() / inputDimension
	loraScope := scope.In("lora")
	a := loraScope.
		WithInitializer(initializer.RandomNormalFn(loraScope, 0.01)).
		VariableWithShape("a", shapes.Make(baseWeight.DType(), rank, inputDimension)).
		SetTrainable(true).
		NodeValue(baseWeight.Graph())
	b := loraScope.
		WithInitializer(initializer.Zero).
		VariableWithShape("b", shapes.Make(baseWeight.DType(), outputDimension, rank)).
		SetTrainable(true).
		NodeValue(baseWeight.Graph())

	update := graph.DotGeneral(a, []int{0}, nil, b, []int{1}, nil)
	update = graph.Reshape(update, baseWeight.Shape().Dimensions...)
	update = graph.MulScalar(update, alpha/float64(rank)*adapterScale)
	return graph.Add(baseWeight, update)
}
