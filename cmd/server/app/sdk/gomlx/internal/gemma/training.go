package gemma

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/gomlx/internal/checkpoint"
)

// TrainingConfig controls a deterministic Gemma LoRA training run.
type TrainingConfig struct {
	Epochs                       int
	EarlyStoppingPatience        int
	MinimumValidationImprovement float64
	LearningRate                 float64
	WeightDecay                  float64
	BatchSize                    int
	Seed                         uint64
	AdapterCheckpointDirectory   string
	RetainedCheckpointEpochs     []int
}

// EpochMetrics reports the measured losses after one complete epoch.
type EpochMetrics struct {
	Epoch          int
	TrainingLoss   float64
	ValidationLoss float64
	Checkpoint     string
}

// TrainingProgress reports completed optimizer updates for the entire run.
type TrainingProgress struct {
	Epoch          int
	CompletedSteps int
	TotalSteps     int
}

// TrainingResult contains the metrics needed for later checkpoint selection.
type TrainingResult struct {
	Epochs             []EpochMetrics
	BestEpoch          int
	BestValidationLoss float64
	BestCheckpoint     string
	StoppedEarly       bool
}

type earlyStopper struct {
	patience           int
	minimumImprovement float64
	best               float64
	misses             int
	initialized        bool
}

func (stopper *earlyStopper) observe(validationLoss float64) bool {
	improvement := stopper.best - validationLoss
	if !stopper.initialized || improvement > 0 && improvement >= stopper.minimumImprovement {
		stopper.best = validationLoss
		stopper.misses = 0
		stopper.initialized = true
		return false
	}
	stopper.misses++
	return stopper.patience > 0 && stopper.misses >= stopper.patience
}

// TrainDataset trains on every training record and measures validation loss
// after each epoch. It never reads or evaluates the protected test split.
func (runtime *Runtime) TrainDataset(ctx context.Context, dataset PreparedDataset, config TrainingConfig, progress func(TrainingProgress)) (TrainingResult, error) {
	if runtime == nil || runtime.base == nil {
		return TrainingResult{}, fmt.Errorf("gemma runtime is closed")
	}
	if runtime.adapterConfig == nil {
		return TrainingResult{}, fmt.Errorf("gemma runtime has no LoRA adapter")
	}
	if runtime.adapterConfig.AdapterScale != 1 {
		return TrainingResult{}, fmt.Errorf("lora adapter scale must equal one for exportable training")
	}
	if config.Epochs <= 0 {
		return TrainingResult{}, fmt.Errorf("epochs must be greater than zero")
	}
	if config.EarlyStoppingPatience < 0 {
		return TrainingResult{}, fmt.Errorf("early stopping patience must not be negative")
	}
	if config.MinimumValidationImprovement < 0 || math.IsNaN(config.MinimumValidationImprovement) || math.IsInf(config.MinimumValidationImprovement, 0) {
		return TrainingResult{}, fmt.Errorf("minimum validation improvement must be finite and not negative")
	}
	if config.LearningRate <= 0 || math.IsNaN(config.LearningRate) || math.IsInf(config.LearningRate, 0) {
		return TrainingResult{}, fmt.Errorf("learning rate must be finite and greater than zero")
	}
	if config.WeightDecay < 0 || math.IsNaN(config.WeightDecay) || math.IsInf(config.WeightDecay, 0) {
		return TrainingResult{}, fmt.Errorf("weight decay must be finite and not negative")
	}
	if config.BatchSize <= 0 {
		config.BatchSize = 1
	}
	if len(dataset.Training) == 0 {
		return TrainingResult{}, fmt.Errorf("training split is empty")
	}
	if len(dataset.Validation) == 0 {
		return TrainingResult{}, fmt.Errorf("validation split is empty")
	}
	if config.AdapterCheckpointDirectory == "" {
		return TrainingResult{}, fmt.Errorf("adapter checkpoint directory is required")
	}
	retainedEpochs := make(map[int]bool, len(config.RetainedCheckpointEpochs))
	for _, epoch := range config.RetainedCheckpointEpochs {
		if epoch <= 0 || epoch > config.Epochs {
			return TrainingResult{}, fmt.Errorf("retained checkpoint epoch %d is outside [1,%d]", epoch, config.Epochs)
		}
		retainedEpochs[epoch] = true
	}
	for epoch := 1; epoch <= config.Epochs; epoch++ {
		path := filepath.Join(config.AdapterCheckpointDirectory, fmt.Sprintf("epoch-%02d", epoch))
		if _, err := os.Lstat(path); err == nil {
			return TrainingResult{}, fmt.Errorf("adapter checkpoint path %q already exists", path)
		} else if !os.IsNotExist(err) {
			return TrainingResult{}, fmt.Errorf("inspect adapter checkpoint path %q: %w", path, err)
		}
	}
	stepsPerEpoch := (len(dataset.Training) + config.BatchSize - 1) / config.BatchSize
	if stepsPerEpoch > math.MaxInt/config.Epochs {
		return TrainingResult{}, fmt.Errorf("total training step count overflows int")
	}
	for _, record := range dataset.Training {
		if err := runtime.validateTrainingRecord(record); err != nil {
			return TrainingResult{}, fmt.Errorf("validate training split: %w", err)
		}
	}
	for _, record := range dataset.Validation {
		if err := runtime.validateTrainingRecord(record); err != nil {
			return TrainingResult{}, fmt.Errorf("validate validation split: %w", err)
		}
	}
	if err := runtime.beginTrainingPhase(config.LearningRate, config.WeightDecay); err != nil {
		return TrainingResult{}, err
	}

	result := TrainingResult{Epochs: make([]EpochMetrics, 0, config.Epochs), BestValidationLoss: math.Inf(1)}
	stopper := earlyStopper{
		patience: config.EarlyStoppingPatience, minimumImprovement: config.MinimumValidationImprovement,
	}
	totalSteps := config.Epochs * stepsPerEpoch
	completedSteps := 0
	for epoch := 1; epoch <= config.Epochs; epoch++ {
		order := deterministicOrder(len(dataset.Training), config.Seed, epoch)
		var weightedLoss float64
		var answerTokens int
		for start := 0; start < len(order); start += config.BatchSize {
			if err := context.Cause(ctx); err != nil {
				return TrainingResult{}, err
			}
			end := min(start+config.BatchSize, len(order))
			batch := make([]TrainingRecord, end-start)
			activeTokens := 0
			for index, recordIndex := range order[start:end] {
				batch[index] = dataset.Training[recordIndex]
				activeTokens += activeLossPositions(batch[index].LossMask)
			}
			trainingLoss, err := runtime.trainBatch(batch, config.LearningRate, config.WeightDecay)
			if err != nil {
				return TrainingResult{}, fmt.Errorf("train epoch %d batch %d: %w", epoch, start/config.BatchSize+1, err)
			}
			weightedLoss += trainingLoss * float64(activeTokens)
			answerTokens += activeTokens
			completedSteps++
			if progress != nil {
				progress(TrainingProgress{Epoch: epoch, CompletedSteps: completedSteps, TotalSteps: totalSteps})
			}
		}
		if err := context.Cause(ctx); err != nil {
			return TrainingResult{}, err
		}
		validationLoss, err := runtime.evaluate(dataset.Validation)
		if err != nil {
			return TrainingResult{}, fmt.Errorf("validate epoch %d: %w", epoch, err)
		}
		if err := context.Cause(ctx); err != nil {
			return TrainingResult{}, err
		}
		improved := result.BestEpoch == 0 || validationLoss < result.BestValidationLoss
		metrics := EpochMetrics{
			Epoch:          epoch,
			TrainingLoss:   weightedLoss / float64(answerTokens),
			ValidationLoss: validationLoss,
		}
		if improved || retainedEpochs[epoch] {
			checkpointPath := filepath.Join(config.AdapterCheckpointDirectory, fmt.Sprintf("epoch-%02d", epoch))
			if err := checkpoint.SaveAdapter(checkpointPath, runtime.base.store); err != nil {
				return TrainingResult{}, fmt.Errorf("save epoch %d adapter checkpoint: %w", epoch, err)
			}
			metrics.Checkpoint = checkpointPath
		}
		if improved {
			if result.BestCheckpoint != "" && !retainedEpochs[result.BestEpoch] {
				if err := os.RemoveAll(result.BestCheckpoint); err != nil {
					return TrainingResult{}, fmt.Errorf("remove superseded epoch %d adapter checkpoint: %w", result.BestEpoch, err)
				}
				for index := range result.Epochs {
					if result.Epochs[index].Epoch == result.BestEpoch {
						result.Epochs[index].Checkpoint = ""
						break
					}
				}
			}
			result.BestEpoch = epoch
			result.BestValidationLoss = validationLoss
			result.BestCheckpoint = metrics.Checkpoint
		}
		result.Epochs = append(result.Epochs, metrics)
		if stopper.observe(validationLoss) {
			result.StoppedEarly = true
			break
		}
	}
	return result, nil
}

func deterministicOrder(count int, seed uint64, epoch int) []int {
	order := make([]int, count)
	for index := range order {
		order[index] = index
	}
	random := rand.New(rand.NewPCG(seed, uint64(epoch)))
	random.Shuffle(len(order), func(left, right int) {
		order[left], order[right] = order[right], order[left]
	})
	return order
}

func activeLossPositions(mask []bool) int {
	count := 0
	for _, active := range mask {
		if active {
			count++
		}
	}
	return count
}
