package providers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/gomlx/engine"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/pool"
)

type inferenceModel interface {
	Chat(context.Context, model.D) (model.ChatResponse, error)
	Tokenize(context.Context, model.D) (model.TokenizeResponse, error)
	TokenizeIDs(context.Context, string) ([]int32, error)
	ModelConfig() model.Config
}

// Record contains the prompt and target used for computation.
type Record struct {
	Prompt string `json:"prompt"`
	Target string `json:"target"`
}

// Training configures model optimization.
type Training struct {
	MaximumEpochs int     `json:"maximum_epochs"`
	BatchSize     int     `json:"batch_size"`
	LearningRate  float64 `json:"learning_rate"`
	Seed          int64   `json:"seed"`
	LoRARank      int     `json:"lora_rank,omitzero"`
	LoRAAlpha     float64 `json:"lora_alpha,omitzero"`
}

// TrainingDataset contains locked splits. Test never enters optimization.
type TrainingDataset struct {
	Train      []Record `json:"train"`
	Validation []Record `json:"validation"`
	Test       []Record `json:"test"`
}

// TrainingJob supplies worker-local computation inputs and disposable storage.
type TrainingJob struct {
	ModelID          string                      `json:"model_id"`
	Model            string                      `json:"model"`
	Training         Training                    `json:"training"`
	Dataset          TrainingDataset             `json:"dataset"`
	Workdir          string                      `json:"workdir"`
	PredictionTokens int                         `json:"prediction_tokens"`
	PausePoint       func(context.Context) error `json:"-"`
	// SafePoint receives a committed file relative to Workdir and may suspend.
	SafePoint func(context.Context, string) error `json:"-"`
}

// TrainingCandidate identifies a trained checkpoint and its selection metrics.
type TrainingCandidate struct {
	Path           string  `json:"path"`
	BestEpoch      int     `json:"best_epoch"`
	TrainingLoss   float64 `json:"training_loss"`
	ValidationLoss float64 `json:"validation_loss"`
}

// GoMLX performs training and inference using isolated KMS models.
type GoMLX struct {
	acquire func(context.Context, TrainingJob, string) (inferenceModel, func() error, error)
}

// NewGoMLX constructs a trainer using the host's KMS pool for exact tokenization
// and inference. Job-specific models never change adapters on serving models.
func NewGoMLX(modelPool *pool.Pool) (*GoMLX, error) {
	if modelPool == nil {
		return nil, fmt.Errorf("%w: KMS model pool is required", tmprl.ErrInvalidArgument)
	}
	t := GoMLX{acquire: func(ctx context.Context, job TrainingJob, adapter string) (inferenceModel, func() error, error) {
		cfg, err := modelPool.ResolvedKronkConfig(job.ModelID)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve training inference model: %w", err)
		}
		if len(cfg.ModelFiles) != 1 {
			return nil, nil, fmt.Errorf("%w: training requires one GGUF model file", tmprl.ErrInvalidArgument)
		}
		if err := verifyModel(job.Model, cfg.ModelFiles[0]); err != nil {
			return nil, nil, err
		}
		cfg.Adapters = nil
		if adapter != "" {
			cfg.Adapters = []model.AdapterConfig{{Path: adapter, Scale: 1}}
		}
		// Tokenization must leave GPU memory available for the GoMLX runtime.
		// These private CPU instances are still admitted and budgeted by KMS.
		cfg.PtrNGpuLayers = new(-1)
		cfg.PtrIncrementalCache = new(false)
		cfg.PtrNSeqMax = new(1)
		cfg.PtrInsecureLogging = new(false)
		key := job.ModelID + "/training/" + rand.Text()
		krn, err := modelPool.AquireCustom(ctx, key, cfg)
		if err != nil {
			return nil, nil, fmt.Errorf("acquire training inference model: %w", err)
		}
		release := func() error {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
			defer cancel()
			return modelPool.InvalidateSync(cleanup, key)
		}
		return krn, release, nil
	}}
	return &t, nil
}

// Validate checks all splits before training without generating protected answers.
func (t *GoMLX) Validate(ctx context.Context, job TrainingJob) (err error) {
	krn, release, err := t.acquire(ctx, job, "")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, release()) }()
	return validateContext(ctx, krn, job.Dataset, job.PredictionTokens)
}

// Train optimizes and exports the best candidate checkpoint.
func (t *GoMLX) Train(ctx context.Context, job TrainingJob, publish func(tmprl.Progress)) (TrainingCandidate, error) {
	krn, release, err := t.acquire(ctx, job, "")
	if err != nil {
		return TrainingCandidate{}, err
	}
	runtime, err := engine.NewLoRARuntime(job.Model, engine.AdapterConfig{Rank: job.Training.LoRARank, Alpha: job.Training.LoRAAlpha, AdapterScale: 1, Seed: job.Training.Seed})
	if err != nil {
		return TrainingCandidate{}, errors.Join(err, release())
	}
	defer runtime.Close()
	dataset := engine.Dataset{}
	for _, r := range job.Dataset.Train {
		dataset.Train = append(dataset.Train, engine.Record{Prompt: r.Prompt, Target: r.Target})
	}
	for _, r := range job.Dataset.Validation {
		dataset.Validation = append(dataset.Validation, engine.Record{Prompt: r.Prompt, Target: r.Target})
	}
	prepared, prepareErr := runtime.PrepareDataset(ctx, dataset, kmsTokenizer{model: krn, pausePoint: job.PausePoint}, func(p engine.TokenizationProgress) {
		publish(tmprl.Progress{Stage: "train", Completed: p.Completed, Total: p.Total, Message: "Tokenizing " + p.Split})
	})
	if err := errors.Join(prepareErr, release()); err != nil {
		return TrainingCandidate{}, err
	}
	resumeDirectory := ""
	pausePoint := job.PausePoint
	if job.SafePoint != nil {
		resumeDirectory = job.Workdir
		// The engine must save optimizer state before it can acknowledge Stop.
		pausePoint = nil
	}
	// Release the KMS tokenizer before allocating training graphs.
	result, err := runtime.TrainDataset(ctx, prepared, engine.TrainingConfig{
		Epochs: job.Training.MaximumEpochs, BatchSize: job.Training.BatchSize,
		LearningRate: job.Training.LearningRate, Seed: uint64(job.Training.Seed),
		EarlyStoppingPatience: 3, MinimumValidationImprovement: 0.0001,
		AdapterCheckpointDirectory: filepath.Join(job.Workdir, "epochs"),
		PausePoint:                 pausePoint,
		ResumeDirectory:            resumeDirectory,
		SafePoint:                  job.SafePoint,
	}, func(p engine.TrainingProgress) {
		publish(tmprl.Progress{Stage: "train", Completed: p.CompletedSteps, Total: p.TotalSteps, Message: fmt.Sprintf("Training epoch %d", p.Epoch)})
	})
	if err != nil {
		return TrainingCandidate{}, err
	}
	if err := ctx.Err(); err != nil {
		return TrainingCandidate{}, err
	}
	if job.SafePoint == nil && job.PausePoint != nil {
		if err := job.PausePoint(ctx); err != nil {
			return TrainingCandidate{}, err
		}
	}
	path := filepath.Join(job.Workdir, "candidate.gguf")
	exportPath := path
	if job.SafePoint != nil {
		exportPath = path + "." + rand.Text()
		defer os.Remove(exportPath)
	}
	if err := runtime.ExportBestAdapter(result, exportPath); err != nil {
		return TrainingCandidate{}, err
	}
	if exportPath != path {
		if err := os.Rename(exportPath, path); err != nil {
			return TrainingCandidate{}, err
		}
	}
	candidate := TrainingCandidate{Path: path, BestEpoch: result.BestEpoch, ValidationLoss: result.BestValidationLoss}
	for _, metrics := range result.Epochs {
		if metrics.Epoch == result.BestEpoch {
			candidate.TrainingLoss = metrics.TrainingLoss
		}
	}
	return candidate, nil
}

// Predict answers independent prompts using the base model or supplied adapter.
// After each answer, answered, when not nil, receives the number answered so far.
func (t *GoMLX) Predict(ctx context.Context, job TrainingJob, adapter string, records []Record, answered func(int)) (answers []string, err error) {
	if job.PredictionTokens < 1 {
		return nil, fmt.Errorf("%w: prediction token budget must be positive", tmprl.ErrInvalidArgument)
	}
	var saved struct {
		Identity string
		Answers  []string
	}
	var root *os.Root
	checkpoint := "base-answers.json"
	if job.SafePoint != nil {
		root, err = os.OpenRoot(job.Workdir)
		if err != nil {
			return nil, err
		}
		defer root.Close()
		var modelHashes [2][sha256.Size]byte
		if adapter != "" {
			checkpoint = "adapter-answers.json"
		}
		for i, path := range []string{job.Model, adapter} {
			if i == 1 && path == "" {
				continue
			}
			file, err := os.Open(path)
			if err != nil {
				return nil, err
			}
			hash := sha256.New()
			_, readErr := io.Copy(hash, tmprl.ContextReader{Context: ctx, Reader: file})
			if err := errors.Join(readErr, file.Close()); err != nil {
				return nil, err
			}
			modelHashes[i] = [sha256.Size]byte(hash.Sum(nil))
		}
		identity, err := json.Marshal([]any{job.ModelID, job.Model, job.Training, job.PredictionTokens, records, modelHashes})
		if err != nil {
			return nil, err
		}
		digest := fmt.Sprintf("%x", sha256.Sum256(identity))
		data, readErr := root.ReadFile(checkpoint)
		switch {
		case readErr == nil:
			if err := json.Unmarshal(data, &saved); err != nil {
				return nil, err
			}
			if saved.Identity != digest || len(saved.Answers) > len(records) {
				return nil, fmt.Errorf("saved evaluation does not match this run")
			}
		case errors.Is(readErr, os.ErrNotExist):
			saved.Identity = digest
		default:
			return nil, readErr
		}
		answers = saved.Answers
		if answered != nil {
			answered(len(answers))
		}
		reference := ""
		if readErr == nil {
			reference = checkpoint
		}
		if err := job.SafePoint(ctx, reference); err != nil {
			return nil, err
		}
		if len(answers) == len(records) {
			return answers, nil
		}
	}
	krn, release, err := t.acquire(ctx, job, adapter)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, release()) }()
	if err := validateContext(ctx, krn, TrainingDataset{Test: records}, job.PredictionTokens); err != nil {
		return nil, err
	}
	for _, record := range records[len(answers):] {
		if job.SafePoint == nil && job.PausePoint != nil {
			if err := job.PausePoint(ctx); err != nil {
				return nil, err
			}
		}
		// Targets never enter inference; every question is independent.
		response, err := krn.Chat(ctx, model.D{
			"messages":    []model.D{{"role": "user", "content": record.Prompt}},
			"temperature": 0, "seed": uint32(job.Training.Seed), "max_completion_tokens": job.PredictionTokens,
			"enable_thinking": false, "stream": false, "repeat_penalty": 1.0, "dry_multiplier": 0.0,
		})
		if err != nil {
			return nil, err
		}
		if len(response.Choices) != 1 || response.Choices[0].Message == nil {
			return nil, fmt.Errorf("protected prediction is missing")
		}
		// An answer cut off at the token budget or left empty is kept as given:
		// it cannot match its target, so it counts as wrong instead of failing.
		answers = append(answers, response.Choices[0].Message.Content)
		if root != nil {
			saved.Answers = answers
			if err := tmprl.WriteJSON(root, checkpoint, saved); err != nil {
				return nil, err
			}
			if err := job.SafePoint(ctx, checkpoint); err != nil {
				return nil, err
			}
		}
		if answered != nil {
			answered(len(answers))
		}
	}
	return answers, nil
}

func validateContext(ctx context.Context, krn inferenceModel, dataset TrainingDataset, predictionTokens int) error {
	if len(dataset.Test) > 0 && predictionTokens < 1 {
		return fmt.Errorf("%w: prediction token budget must be positive", tmprl.ErrInvalidArgument)
	}
	window := krn.ModelConfig().ContextWindow()
	if window < 1 {
		return fmt.Errorf("inference model returned invalid context window")
	}
	for _, split := range []struct {
		name      string
		rows      []Record
		protected bool
	}{{"training", dataset.Train, false}, {"validation", dataset.Validation, false}, {"protected test", dataset.Test, true}} {
		for i, record := range split.rows {
			var count int
			if split.protected {
				response, err := krn.Tokenize(ctx, model.D{"input": record.Prompt, "apply_template": true})
				if err != nil {
					return err
				}
				count = response.Tokens + predictionTokens
			} else {
				// Match the complete Gemma training turn, including its explicit BOS.
				text := "<bos><start_of_turn>user\n" + strings.TrimSpace(record.Prompt) + "<end_of_turn>\n<start_of_turn>model\n" + strings.TrimSpace(record.Target) + "<end_of_turn>"
				tokens, err := krn.TokenizeIDs(ctx, text)
				if err != nil {
					return err
				}
				count = len(tokens) - 1
			}
			if count > window {
				return fmt.Errorf("%w: %s record %d needs %d tokens including answer space; model context is %d", tmprl.ErrInvalidArgument, split.name, i+1, count, window)
			}
		}
	}
	return nil
}

type kmsTokenizer struct {
	model      inferenceModel
	pausePoint func(context.Context) error
}

func (t kmsTokenizer) Tokenize(ctx context.Context, text string) ([]int32, error) {
	if t.pausePoint != nil {
		if err := t.pausePoint(ctx); err != nil {
			return nil, err
		}
	}
	return t.model.TokenizeIDs(ctx, text)
}

func (t kmsTokenizer) VerifyModel(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	files := t.model.ModelConfig().ModelFiles
	if len(files) != 1 {
		return fmt.Errorf("%w: tokenizer requires one GGUF model file", tmprl.ErrInvalidArgument)
	}
	return verifyModel(path, files[0])
}

func verifyModel(wanted, served string) error {
	left, err := os.Stat(wanted)
	if err != nil {
		return fmt.Errorf("inspect training model: %w", err)
	}
	right, err := os.Stat(served)
	if err != nil {
		return fmt.Errorf("inspect KMS model: %w", err)
	}
	if !os.SameFile(left, right) {
		return fmt.Errorf("%w: KMS tokenizer model does not match training model", tmprl.ErrInvalidArgument)
	}
	return nil
}
