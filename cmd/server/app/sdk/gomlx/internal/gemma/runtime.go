package gemma

import (
	"fmt"
	"math"
	"slices"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/gomlx/internal/adapter"
	"github.com/gomlx/compute"
	"github.com/gomlx/compute/dtypes"
	_ "github.com/gomlx/compute/gobackend"
	"github.com/gomlx/compute/shapes"
	"github.com/gomlx/gomlx/core/graph"
	"github.com/gomlx/gomlx/core/tensors"
	"github.com/gomlx/gomlx/ml/layers"
	"github.com/gomlx/gomlx/ml/layers/activation"
	"github.com/gomlx/gomlx/ml/layers/attention/pos"
	"github.com/gomlx/gomlx/ml/model"
	"github.com/gomlx/gomlx/ml/train"
	"github.com/gomlx/gomlx/ml/train/loss"
	"github.com/gomlx/gomlx/ml/train/optimizer"
	"github.com/gomlx/gomlx/ml/zoo/transformer"
)

// Result describes one Gemma forward pass.
type Result struct {
	Shape  []int
	Finite bool
}

// AdapterConfig controls the internal Gemma LoRA recipe.
type AdapterConfig struct {
	Rank         int
	Alpha        float64
	AdapterScale float64
	Seed         int64
}

// Runtime owns a loaded base model, GoMLX backend, and compiled forward graph.
type Runtime struct {
	base          *BaseModel
	backend       compute.Backend
	exec          *model.ExecOneOutput
	trainer       *train.Trainer
	adapterConfig *AdapterConfig
	learningRate  float64
	weightDecay   float64
}

// NewRuntime loads a Gemma 3 GGUF and prepares it for forward execution.
func NewRuntime(modelPath string) (*Runtime, error) {
	return newRuntime(modelPath, nil)
}

// NewLoRARuntime loads a Gemma 3 GGUF and attaches trainable, zero-initialized
// LoRA weights to the attention and feed-forward projections.
func NewLoRARuntime(modelPath string, config AdapterConfig) (*Runtime, error) {
	if err := validateAdapterConfig(config); err != nil {
		return nil, err
	}
	return newRuntime(modelPath, &config)
}

func newRuntime(modelPath string, adapterConfig *AdapterConfig) (*Runtime, error) {
	base, err := LoadBaseModel(modelPath)
	if err != nil {
		return nil, err
	}
	if adapterConfig != nil && adapterConfig.Rank > base.config.EmbeddingDimension {
		base.Close()
		return nil, fmt.Errorf("lora rank %d exceeds Gemma embedding dimension %d", adapterConfig.Rank, base.config.EmbeddingDimension)
	}
	if adapterConfig != nil {
		base.store.SetParam(model.ParamInitialSeed, adapterConfig.Seed)
	}

	backend, err := compute.New()
	if err != nil {
		base.Close()
		return nil, fmt.Errorf("create GoMLX backend: %w", err)
	}

	exec, err := model.NewExec1(
		backend,
		base.store,
		func(scope *model.Scope, tokens *graph.Node) *graph.Node {
			if adapterConfig != nil {
				restore := installLoRAWeights(scope, tokens.Graph(), base.config, *adapterConfig)
				defer restore()
			}
			return newTransformer(scope, base.config).Logits(tokens, transformer.CallOptions{})
		},
	)
	if err != nil {
		base.Close()
		backend.Finalize()
		return nil, fmt.Errorf("create Gemma forward executor: %w", err)
	}

	return &Runtime{base: base, backend: backend, exec: exec, adapterConfig: adapterConfig}, nil
}

// Run executes one batch containing the supplied token sequence.
func (runtime *Runtime) Run(tokens []int32) (Result, error) {
	return runtime.run(tokens, nil)
}

func (runtime *Runtime) run(tokens []int32, inspect func([]float32)) (Result, error) {
	if runtime == nil || runtime.base == nil || runtime.exec == nil {
		return Result{}, fmt.Errorf("gemma runtime is closed")
	}
	if err := validateTokens("token", tokens, runtime.base.config); err != nil {
		return Result{}, err
	}
	config := runtime.base.config

	input := tensors.FromValue([][]int32{tokens})
	defer input.FinalizeAll()
	output, err := runtime.exec.Call(input)
	if err != nil {
		return Result{}, fmt.Errorf("execute Gemma forward graph: %w", err)
	}
	defer output.FinalizeAll()

	shape := slices.Clone(output.Shape().Dimensions)
	expected := []int{1, len(tokens), config.VocabSize}
	if !slices.Equal(shape, expected) {
		return Result{}, fmt.Errorf("gemma logits shape is %v, want %v", shape, expected)
	}

	result := Result{Shape: shape, Finite: true}
	if err := tensors.ConstFlatData[float32](output, func(values []float32) {
		if inspect != nil {
			inspect(values)
		}
		for _, value := range values {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				result.Finite = false
				return
			}
		}
	}); err != nil {
		return Result{}, fmt.Errorf("read Gemma logits: %w", err)
	}
	if !result.Finite {
		return Result{}, fmt.Errorf("gemma forward graph produced non-finite logits")
	}
	return result, nil
}

// TrainStep applies one answer-masked optimizer update to a LoRA runtime and
// returns the batch loss. The learning rate is fixed by the first attempted
// step because the compiled trainer and its optimizer state are reused.
func (runtime *Runtime) TrainStep(tokens, targets []int32, lossMask []bool, learningRate float64) (float64, error) {
	return runtime.trainBatch([]TrainingRecord{{Tokens: tokens, Targets: targets, LossMask: lossMask}}, learningRate, 0)
}

func (runtime *Runtime) trainBatch(records []TrainingRecord, learningRate, weightDecay float64) (float64, error) {
	if runtime == nil || runtime.base == nil || runtime.exec == nil {
		return 0, fmt.Errorf("gemma runtime is closed")
	}
	if runtime.adapterConfig == nil {
		return 0, fmt.Errorf("gemma runtime has no LoRA adapter")
	}
	if runtime.adapterConfig.AdapterScale == 0 {
		return 0, fmt.Errorf("lora adapter scale must be greater than zero for training")
	}
	if len(records) == 0 {
		return 0, fmt.Errorf("training batch is empty")
	}
	for _, record := range records {
		if err := runtime.validateTrainingRecord(record); err != nil {
			return 0, err
		}
	}
	if learningRate <= 0 || math.IsNaN(learningRate) || math.IsInf(learningRate, 0) {
		return 0, fmt.Errorf("learning rate must be finite and greater than zero")
	}
	if weightDecay < 0 || math.IsNaN(weightDecay) || math.IsInf(weightDecay, 0) {
		return 0, fmt.Errorf("weight decay must be finite and not negative")
	}
	if runtime.trainer != nil && (learningRate != runtime.learningRate || weightDecay != runtime.weightDecay) {
		return 0, fmt.Errorf("optimizer settings differ from the initialized runtime")
	}
	if runtime.trainer == nil {
		config := runtime.base.config
		adapterConfig := *runtime.adapterConfig
		modelFn := func(scope *model.Scope, _ any, inputs []*graph.Node) []*graph.Node {
			restore := installLoRAWeights(scope, inputs[0].Graph(), config, adapterConfig)
			defer restore()
			return []*graph.Node{newTransformer(scope, config).Logits(inputs[0], transformer.CallOptions{})}
		}
		runtime.trainer = train.NewTrainer(
			runtime.backend,
			runtime.base.store,
			modelFn,
			loss.SparseCategoricalCrossEntropyLogits,
			optimizer.Adam().LearningRate(learningRate).WeightDecay(weightDecay).Done(),
			nil,
			nil,
		)
		runtime.learningRate = learningRate
		runtime.weightDecay = weightDecay
	}

	metrics, err := runtime.trainer.TrainStep(trainingBatch(records, runtime.base.config.MaxSequenceLength))
	if err != nil {
		return 0, fmt.Errorf("train Gemma LoRA step: %w", err)
	}
	return metricLoss(metrics, "train Gemma LoRA step")
}

func (runtime *Runtime) beginTrainingPhase(learningRate, weightDecay float64) error {
	if runtime.trainer == nil || runtime.learningRate == learningRate && runtime.weightDecay == weightDecay {
		return nil
	}

	runtime.trainer.ResetComputationGraphs()
	if err := optimizer.Adam().Done().Clear(runtime.base.store.RootScope()); err != nil {
		return fmt.Errorf("reset Gemma optimizer for learning rate %g: %w", learningRate, err)
	}
	runtime.trainer = nil
	runtime.learningRate = 0
	runtime.weightDecay = 0
	return nil
}

func (runtime *Runtime) evaluate(records []TrainingRecord) (float64, error) {
	if runtime == nil || runtime.base == nil || runtime.trainer == nil {
		return 0, fmt.Errorf("gemma trainer is not initialized")
	}
	for _, record := range records {
		if err := runtime.validateTrainingRecord(record); err != nil {
			return 0, err
		}
	}
	metrics, err := runtime.trainer.EvalStep(trainingBatch(records, runtime.base.config.MaxSequenceLength))
	if err != nil {
		return 0, fmt.Errorf("evaluate Gemma LoRA: %w", err)
	}
	return metricLoss(metrics, "evaluate Gemma LoRA")
}

func (runtime *Runtime) validateTrainingRecord(record TrainingRecord) error {
	if err := validateTokens("token", record.Tokens, runtime.base.config); err != nil {
		return err
	}
	if err := validateTokens("target", record.Targets, runtime.base.config); err != nil {
		return err
	}
	if len(record.Targets) != len(record.Tokens) {
		return fmt.Errorf("target count %d differs from token count %d", len(record.Targets), len(record.Tokens))
	}
	if len(record.LossMask) != len(record.Tokens) {
		return fmt.Errorf("loss mask count %d differs from token count %d", len(record.LossMask), len(record.Tokens))
	}
	if !slices.Contains(record.LossMask, true) {
		return fmt.Errorf("loss mask must select at least one answer token")
	}
	return nil
}

func trainingBatch(records []TrainingRecord, maxSequenceLength int) train.Batch {
	sequenceLength := 0
	for _, record := range records {
		sequenceLength = max(sequenceLength, len(record.Tokens))
	}
	sequenceLength = paddedSequenceLength(sequenceLength, maxSequenceLength)
	tokens := make([][]int32, len(records))
	targets := make([][][]int32, len(records))
	masks := make([][]bool, len(records))
	for recordIndex, record := range records {
		tokens[recordIndex] = make([]int32, sequenceLength)
		copy(tokens[recordIndex], record.Tokens)
		targets[recordIndex] = make([][]int32, sequenceLength)
		for tokenIndex := range sequenceLength {
			targets[recordIndex][tokenIndex] = []int32{0}
		}
		for tokenIndex, target := range record.Targets {
			targets[recordIndex][tokenIndex][0] = target
		}
		masks[recordIndex] = make([]bool, sequenceLength)
		copy(masks[recordIndex], record.LossMask)
	}
	return train.Batch{
		Inputs: []*tensors.Tensor{tensors.FromValue(tokens)},
		Labels: []*tensors.Tensor{tensors.FromValue(targets), tensors.FromValue(masks)},
	}
}

// paddedSequenceLength limits graph compilation to power-of-two sequence
// buckets. Padding is appended after each causal sequence and remains excluded
// from the loss mask, so it cannot change the loss for real answer tokens.
func paddedSequenceLength(length, maximum int) int {
	padded := 1
	for padded < length && padded < maximum {
		padded *= 2
	}
	return min(padded, maximum)
}

func metricLoss(metrics []*tensors.Tensor, operation string) (float64, error) {
	defer func() {
		for _, metric := range metrics {
			metric.FinalizeAll()
		}
	}()
	if len(metrics) == 0 {
		return 0, fmt.Errorf("%s returned no loss", operation)
	}
	value, ok := metrics[0].Value().(float32)
	if !ok || math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
		return 0, fmt.Errorf("%s produced non-finite loss", operation)
	}
	return float64(value), nil
}

func validateTokens(name string, tokens []int32, config Config) error {
	if len(tokens) == 0 {
		return fmt.Errorf("%ss are required", name)
	}
	if len(tokens) > config.MaxSequenceLength {
		return fmt.Errorf("%s count %d exceeds Gemma context length %d", name, len(tokens), config.MaxSequenceLength)
	}
	for index, token := range tokens {
		if token < 0 || int(token) >= config.VocabSize {
			return fmt.Errorf("%s[%d]=%d is outside vocabulary [0,%d)", name, index, token, config.VocabSize)
		}
	}
	return nil
}

func validateAdapterConfig(config AdapterConfig) error {
	if config.Rank <= 0 {
		return fmt.Errorf("lora rank must be greater than zero")
	}
	if config.Alpha <= 0 || math.IsNaN(config.Alpha) || math.IsInf(config.Alpha, 0) {
		return fmt.Errorf("lora alpha must be finite and greater than zero")
	}
	if config.AdapterScale < 0 || math.IsNaN(config.AdapterScale) || math.IsInf(config.AdapterScale, 0) {
		return fmt.Errorf("lora adapter scale must be finite and non-negative")
	}
	return nil
}

// installLoRAWeights temporarily substitutes adjusted projection weights while the
// transformer consumes them. Restoring the graph nodes prevents an effective
// weight from replacing its frozen base variable.
func installLoRAWeights(scope *model.Scope, g *graph.Graph, config Config, adapterConfig AdapterConfig) func() {
	type installedWeight struct {
		variable *model.Variable
		base     *graph.Node
	}

	type projection struct {
		scope *model.Scope
		shape []int
	}

	installed := make([]installedWeight, 0, config.LayerCount*7)
	for layer := range config.LayerCount {
		attentionScope := scope.At("layer_%d", layer).At("self_attn").At("MultiHeadAttention")
		mlpScope := scope.At("layer_%d", layer).At("mlp")
		projections := []projection{
			{attentionScope.At("query"), []int{config.EmbeddingDimension, config.AttentionHeads * config.HeadDimension}},
			{attentionScope.At("key"), []int{config.EmbeddingDimension, config.KeyValueHeads * config.HeadDimension}},
			{attentionScope.At("value"), []int{config.EmbeddingDimension, config.KeyValueHeads * config.HeadDimension}},
			{attentionScope.At("output"), []int{config.AttentionHeads * config.HeadDimension, config.EmbeddingDimension}},
			{mlpScope.At("gate_proj"), []int{config.EmbeddingDimension, config.FeedForwardDimension}},
			{mlpScope.At("up_proj"), []int{config.EmbeddingDimension, config.FeedForwardDimension}},
			{mlpScope.At("down_proj"), []int{config.FeedForwardDimension, config.EmbeddingDimension}},
		}
		for _, projection := range projections {
			weight := projection.scope.At("dense").
				VariableWithShape(
					"weights",
					shapes.Make(dtypes.Float32, projection.shape...),
				).
				SetTrainable(false)
			base := weight.NodeValue(g)
			adjusted := adapter.ApplyWeightScaled(
				projection.scope,
				base,
				adapterConfig.Rank,
				adapterConfig.Alpha,
				adapterConfig.AdapterScale,
			)
			weight.SetNodeValue(adjusted)
			installed = append(installed, installedWeight{variable: weight, base: base})
		}
	}
	return func() {
		for _, weight := range installed {
			weight.variable.SetNodeValue(weight.base)
		}
	}
}

// Backend returns the selected GoMLX backend name.
func (runtime *Runtime) Backend() string {
	if runtime == nil || runtime.backend == nil {
		return ""
	}
	return runtime.backend.Name()
}

// Close releases the graph, loaded model, and backend. It is safe to call more
// than once.
func (runtime *Runtime) Close() {
	if runtime == nil {
		return
	}
	if runtime.trainer != nil {
		runtime.trainer.ResetComputationGraphs()
		runtime.trainer = nil
	}
	if runtime.exec != nil {
		runtime.exec.Finalize()
		runtime.exec = nil
	}
	if runtime.base != nil {
		runtime.base.Close()
		runtime.base = nil
	}
	if runtime.backend != nil {
		if !runtime.backend.IsFinalized() {
			runtime.backend.Finalize()
		}
		runtime.backend = nil
	}
}

func newTransformer(scope *model.Scope, config Config) *transformer.Model {
	layerTypes := make([]transformer.LayerType, config.LayerCount)
	for layer := range layerTypes {
		layerTypes[layer] = transformer.LocalLayer
		if (layer+1)%config.SlidingWindowPattern == 0 {
			layerTypes[layer] = transformer.GlobalLayer
		}
	}

	model := transformer.New(scope).
		WithArchitecture(transformer.ArchitectureGemma3).
		WithVocabSize(config.VocabSize).
		WithEmbedDim(config.EmbeddingDimension).
		WithNumLayers(config.LayerCount).
		WithNumHeads(config.AttentionHeads).
		WithNumKVHeads(config.KeyValueHeads).
		WithHeadDim(config.HeadDimension).
		WithFFNDim(config.FeedForwardDimension).
		WithMaxPosEmbed(config.MaxSequenceLength).
		WithDType(dtypes.Float32).
		WithDropout(0).
		WithPositionalEncoder(pos.NewRoPE(config.LocalRoPEBase)).
		WithLayerTypes(layerTypes).
		WithSlidingWindow(config.SlidingWindow).
		WithNormalization(layers.NormalizationRMSNorm).
		WithFinalNormalization(layers.NormalizationRMSNorm).
		WithNormEpsilon(1e-6).
		WithRMSNormOffset(1).
		WithActivation(activation.TypeGeluApprox).
		WithBias(false).
		WithCausalMask(true).
		WithQueryKeyScale(1 / math.Sqrt(float64(config.HeadDimension)))

	for layer, layerType := range layerTypes {
		if layerType == transformer.GlobalLayer {
			model.WithLayerPositionalEncoder(layer, pos.NewRoPE(config.GlobalRoPEBase))
		}
	}
	return model
}
