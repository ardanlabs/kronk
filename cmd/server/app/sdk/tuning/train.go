package tuning

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
)

// StartTraining starts the existing durable pipeline with SDK-managed input.
// Stop/Pause/Resume/Get/Wait/History operate on the returned execution.
func (b *Business) StartTraining(ctx context.Context, input TrainingRequest) (Run, error) {
	input.ID = strings.TrimSpace(input.ID)
	input.Model = strings.TrimSpace(input.Model)
	input.DatasetID = strings.TrimSpace(input.DatasetID)
	if input.Training == (Training{}) {
		input.Training = Training{MaximumEpochs: 3, BatchSize: 2, LearningRate: 0.0002, Seed: 3407}
	}
	if input.Training.LoRARank == 0 {
		input.Training.LoRARank = 8
	}
	if input.Training.LoRAAlpha == 0 {
		input.Training.LoRAAlpha = 8
	}
	request := Request{ID: input.ID, Model: input.Model, Training: input.Training, DatasetID: input.DatasetID, Preparation: input.Preparation}
	if err := b.validateTraining(request); err != nil {
		return Run{}, err
	}
	if _, err := b.trainingRecords(ctx, request); err != nil {
		return Run{}, err
	}
	return b.Start(ctx, request)
}

func (b *Business) validateTraining(input Request) error {
	if err := validate(input); err != nil {
		return err
	}
	if (input.DatasetID != "") == (input.Preparation != (Execution{})) {
		return fmt.Errorf("%w: select exactly one prepared dataset or preparation execution", ErrInvalidArgument)
	}
	if input.Preparation != (Execution{}) {
		if err := validateExecution(input.Preparation); err != nil {
			return err
		}
	}
	if _, ok := b.cfg.Models[input.Model]; !ok {
		return fmt.Errorf("%w: model is not approved for training", ErrInvalidArgument)
	}
	if input.Training.LoRARank < 1 || input.Training.LoRAAlpha <= 0 || math.IsNaN(input.Training.LoRAAlpha) || math.IsInf(input.Training.LoRAAlpha, 0) {
		return fmt.Errorf("%w: invalid LoRA rank or alpha", ErrInvalidArgument)
	}
	return nil
}

func (b *Business) trainingRecords(ctx context.Context, input Request) ([]Record, error) {
	var reader io.ReadCloser
	var err error
	if input.DatasetID != "" {
		dataset, lookupErr := b.GetDataset(ctx, input.DatasetID)
		if lookupErr != nil {
			return nil, lookupErr
		}
		if dataset.Format != ".jsonl" {
			return nil, fmt.Errorf("%w: training requires prepared JSONL", ErrInvalidArgument)
		}
		root, openErr := os.OpenRoot(b.cfg.DatasetRoot)
		if openErr != nil {
			return nil, openErr
		}
		defer root.Close()
		reader, err = root.Open(filepath.Join("imports", dataset.ID, "source.jsonl"))
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		data, readErr := io.ReadAll(io.LimitReader(contextReader{Context: ctx, Reader: reader}, b.cfg.MaximumImportBytes+1))
		if readErr != nil {
			return nil, readErr
		}
		hash := sha256.Sum256(data)
		if int64(len(data)) != dataset.SizeBytes || hex.EncodeToString(hash[:]) != dataset.SHA256 {
			return nil, fmt.Errorf("%w: training source changed", ErrInvalidArgument)
		}
		records, err := readDatasetRecords(ctx, strings.NewReader(string(data)))
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrInvalidArgument, err)
		}
		return records, TrainingReadiness(records)
	}
	run, err := b.GetPreparation(ctx, input.Preparation)
	if err != nil {
		return nil, err
	}
	if run.Status != StatusCompleted || run.Kind != PreparationFull || run.Result == nil || !run.Result.TrainingReady {
		return nil, fmt.Errorf("%w: preparation is not training-ready", ErrInvalidArgument)
	}
	reader, err = b.openPreparationRecords(run)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	records, err := readDatasetRecords(ctx, io.LimitReader(reader, b.cfg.MaximumImportBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidArgument, err)
	}
	if len(records) != run.Result.RecordCount {
		return nil, fmt.Errorf("%w: prepared record count changed", ErrInvalidArgument)
	}
	if err := TrainingReadiness(records); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidArgument, err)
	}
	return records, nil
}

func (b *Business) trainingStage(ctx context.Context, input StageRequest, publish func(Progress)) (Artifacts, error) {
	info := activity.GetInfo(ctx)
	if input.Execution != (Execution{WorkflowID: info.WorkflowExecution.ID, RunID: info.WorkflowExecution.RunID}) || input.Key != input.RunID+"/"+string(input.Stage) {
		return Artifacts{}, invalidOutput(input.Stage)
	}
	if err := b.validateTraining(input.Request); err != nil {
		return Artifacts{}, temporal.NewNonRetryableApplicationError(err.Error(), "invalid-training", err)
	}
	if b.cfg.Trainer == nil {
		return Artifacts{}, temporal.NewNonRetryableApplicationError("trainer is not configured", "invalid-training", nil)
	}
	records, err := b.trainingRecords(ctx, input.Request)
	if err != nil {
		if errors.Is(err, ErrInvalidArgument) || errors.Is(err, ErrNotFound) {
			return Artifacts{}, temporal.NewNonRetryableApplicationError(err.Error(), "invalid-training", err)
		}
		return Artifacts{}, err
	}
	model := b.cfg.Models[input.Request.Model]
	metadata, err := os.Stat(model)
	if err != nil {
		return Artifacts{}, err
	}
	if !metadata.Mode().IsRegular() {
		return Artifacts{}, temporal.NewNonRetryableApplicationError("model must be a regular GGUF file", "invalid-training", nil)
	}
	modelFile, err := os.Open(model)
	if err != nil {
		return Artifacts{}, err
	}
	modelHash := sha256.New()
	_, copyErr := io.Copy(modelHash, contextReader{Context: ctx, Reader: modelFile})
	if err := errors.Join(copyErr, modelFile.Close()); err != nil {
		return Artifacts{}, err
	}
	identity, err := json.Marshal(struct {
		Request Request
		Records []Record
		Model   string
	}{input.Request, records, hex.EncodeToString(modelHash.Sum(nil))})
	if err != nil {
		return Artifacts{}, err
	}
	fingerprint := sha256.Sum256(identity)
	root, err := os.OpenRoot(b.cfg.DatasetRoot)
	if err != nil {
		return Artifacts{}, err
	}
	defer root.Close()
	path := filepath.Join("training", input.RunID)
	if err := root.MkdirAll(path, 0o700); err != nil {
		return Artifacts{}, err
	}
	workspace, err := root.OpenRoot(path)
	if err != nil {
		return Artifacts{}, err
	}
	defer workspace.Close()
	var state struct {
		Fingerprint string          `json:"fingerprint"`
		Dataset     TrainingDataset `json:"dataset"`
	}
	statePath := "dataset.json"
	if input.Stage == StageSplit {
		splits, err := splitTrainingDataset(records)
		if err != nil {
			return Artifacts{}, temporal.NewNonRetryableApplicationError(err.Error(), "invalid-training", err)
		}
		if trainer, ok := b.cfg.Trainer.(goMLXTrainer); ok {
			endpoint, stop, err := trainer.server(ctx, TrainingJob{Model: model}, "")
			if err != nil {
				return Artifacts{}, err
			}
			err = trainer.validateContext(ctx, endpoint, splits)
			stop()
			if errors.Is(err, ErrInvalidArgument) {
				return Artifacts{}, temporal.NewNonRetryableApplicationError(err.Error(), "invalid-training", err)
			}
			if err != nil {
				return Artifacts{}, err
			}
		}
		state.Fingerprint, state.Dataset = hex.EncodeToString(fingerprint[:]), splits
		if data, err := workspace.ReadFile(statePath); err == nil {
			var previous struct {
				Fingerprint string `json:"fingerprint"`
			}
			if err := json.Unmarshal(data, &previous); err != nil {
				return Artifacts{}, err
			}
			if previous.Fingerprint != state.Fingerprint {
				return Artifacts{}, fmt.Errorf("training input changed")
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return Artifacts{}, err
		}
		if err := writeJSON(workspace, statePath, state); err != nil {
			return Artifacts{}, err
		}
		return Artifacts{Dataset: filepath.Join(workspace.Name(), statePath)}, nil
	}
	data, err := workspace.ReadFile(statePath)
	if err != nil {
		return Artifacts{}, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return Artifacts{}, err
	}
	if state.Fingerprint != hex.EncodeToString(fingerprint[:]) {
		return Artifacts{}, temporal.NewNonRetryableApplicationError("training input changed", "invalid-training", nil)
	}
	cache := string(input.Stage) + ".json"
	if data, err := workspace.ReadFile(cache); err == nil {
		var output Artifacts
		if err := json.Unmarshal(data, &output); err != nil {
			return Artifacts{}, err
		}
		return output, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return Artifacts{}, err
	}
	scratch, err := os.MkdirTemp("", "moxie-training-")
	if err != nil {
		return Artifacts{}, err
	}
	defer os.RemoveAll(scratch)
	job := TrainingJob{Model: model, Training: input.Request.Training, Dataset: state.Dataset, Workdir: scratch}
	output := Artifacts{}
	switch input.Stage {
	case StageTrain:
		// Do not expose protected test prompts or targets to the trainer.
		job.Dataset.Test = nil
		candidate, err := b.cfg.Trainer.Train(ctx, job, publish)
		if err != nil {
			return Artifacts{}, err
		}
		if candidate.BestEpoch < 1 || candidate.BestEpoch > job.Training.MaximumEpochs || math.IsNaN(candidate.ValidationLoss) || math.IsInf(candidate.ValidationLoss, 0) {
			return Artifacts{}, invalidOutput(StageTrain)
		}
		relative, err := filepath.Rel(scratch, candidate.Path)
		if err != nil {
			return Artifacts{}, err
		}
		private, err := os.OpenRoot(scratch)
		if err != nil {
			return Artifacts{}, err
		}
		defer private.Close()
		file, err := private.Open(relative)
		if err != nil {
			return Artifacts{}, err
		}
		defer file.Close()
		if err := copyTrainingArtifact(ctx, workspace, "candidate.gguf", file); err != nil {
			return Artifacts{}, err
		}
		candidate.Path = filepath.Join(workspace.Name(), "candidate.gguf")
		if err := writeJSON(workspace, "candidate.json", candidate); err != nil {
			return Artifacts{}, err
		}
		output.Checkpoint = candidate.Path
	case StageEvaluate, StagePublish:
		file, err := workspace.Open("candidate.gguf")
		if err != nil {
			return Artifacts{}, err
		}
		defer file.Close()
		if input.Stage == StagePublish {
			data, err := workspace.ReadFile("evaluation.json")
			if err != nil {
				return Artifacts{}, err
			}
			var report EvaluationReport
			if err := json.Unmarshal(data, &report); err != nil {
				return Artifacts{}, err
			}
			if !report.Accepted || !input.Artifacts.Accepted {
				return Artifacts{}, invalidOutput(StagePublish)
			}
			if err := copyTrainingArtifact(ctx, workspace, "adapter.gguf", file); err != nil {
				return Artifacts{}, err
			}
			output.Adapter = filepath.Join(workspace.Name(), "adapter.gguf")
			break
		}
		private, err := os.OpenRoot(scratch)
		if err != nil {
			return Artifacts{}, err
		}
		defer private.Close()
		if err := copyTrainingArtifact(ctx, private, "candidate.gguf", file); err != nil {
			return Artifacts{}, err
		}
		base, err := b.cfg.Trainer.Predict(ctx, job, "", state.Dataset.Test)
		if err != nil {
			return Artifacts{}, err
		}
		adapted, err := b.cfg.Trainer.Predict(ctx, job, filepath.Join(scratch, "candidate.gguf"), state.Dataset.Test)
		if err != nil {
			return Artifacts{}, err
		}
		if len(base) != len(state.Dataset.Test) || len(adapted) != len(base) {
			return Artifacts{}, invalidOutput(StageEvaluate)
		}
		report := EvaluationReport{Total: len(base)}
		for i, record := range state.Dataset.Test {
			report.Answers = append(report.Answers, EvaluationAnswer{Prompt: record.Prompt, Expected: record.Target, BaseAnswer: base[i], AdapterAnswer: adapted[i]})
			if recordKey(base[i]) == recordKey(record.Target) {
				report.BaseCorrect++
			}
			if recordKey(adapted[i]) == recordKey(record.Target) {
				report.AdapterCorrect++
			}
		}
		report.Accepted = report.Total >= 20 && report.AdapterCorrect*10 >= report.Total*9 && report.AdapterCorrect >= report.BaseCorrect
		if err := writeJSON(workspace, "evaluation.json", report); err != nil {
			return Artifacts{}, err
		}
		output.Report, output.Accepted = filepath.Join(workspace.Name(), "evaluation.json"), report.Accepted
	default:
		return Artifacts{}, invalidOutput(input.Stage)
	}
	if err := ctx.Err(); err != nil {
		return Artifacts{}, err
	}
	if err := writeJSON(workspace, cache, output); err != nil {
		return Artifacts{}, err
	}
	return output, nil
}

func copyTrainingArtifact(ctx context.Context, root *os.Root, path string, reader io.Reader) error {
	temporary := ".artifact-" + rand.Text()
	file, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer root.Remove(temporary)
	size, copyErr := io.Copy(file, io.LimitReader(contextReader{Context: ctx, Reader: reader}, (1<<30)+1))
	if err := errors.Join(copyErr, file.Close()); err != nil {
		return err
	}
	if size == 0 || size > 1<<30 {
		return fmt.Errorf("invalid training artifact size")
	}
	return root.Rename(temporary, path)
}

// GetTrainingEvaluation reads the SDK-owned report, including rejected runs.
// A head never needs to interpret worker storage paths.
func (b *Business) GetTrainingEvaluation(ctx context.Context, e Execution) (EvaluationReport, error) {
	reader, err := b.openTrainingArtifact(ctx, e, "evaluation.json", false)
	if err != nil {
		return EvaluationReport{}, err
	}
	defer reader.Close()
	var report EvaluationReport
	err = json.NewDecoder(contextReader{Context: ctx, Reader: reader}).Decode(&report)
	return report, err
}

// OpenTrainingAdapter returns only a published, accepted adapter. The caller
// closes the stream; unaccepted candidates are never exposed as deliverables.
func (b *Business) OpenTrainingAdapter(ctx context.Context, e Execution) (io.ReadCloser, error) {
	return b.openTrainingArtifact(ctx, e, "adapter.gguf", true)
}

func (b *Business) openTrainingArtifact(ctx context.Context, e Execution, name string, accepted bool) (io.ReadCloser, error) {
	run, err := b.Get(ctx, e)
	if err != nil {
		return nil, err
	}
	if (run.Request.DatasetID == "" && run.Request.Preparation == (Execution{})) || run.Artifacts.Report == "" || (accepted && (run.Status != StatusCompleted || !run.Artifacts.Accepted)) {
		return nil, fmt.Errorf("%w: training artifact is not available", ErrInvalidArgument)
	}
	root, err := os.OpenRoot(b.cfg.DatasetRoot)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.Open(filepath.Join("training", e.RunID, name))
}
