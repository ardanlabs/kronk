package training

import (
	"context"
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

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/dataset"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/model"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/training/evaluate"
	publishstage "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/training/publish"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/training/split"
	trainstage "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/training/train"

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
		if err := model.ValidateExecution(input.Preparation); err != nil {
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
		source, lookupErr := b.cfg.Datasets.GetDataset(ctx, input.DatasetID)
		if lookupErr != nil {
			return nil, lookupErr
		}
		if source.Format != ".jsonl" {
			return nil, fmt.Errorf("%w: training requires prepared JSONL", ErrInvalidArgument)
		}
		root, openErr := os.OpenRoot(b.cfg.Datasets.Root())
		if openErr != nil {
			return nil, openErr
		}
		defer root.Close()
		reader, err = root.Open(filepath.Join("imports", source.ID, "source.jsonl"))
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		data, readErr := io.ReadAll(io.LimitReader(dataset.ContextReader{Context: ctx, Reader: reader}, b.cfg.Datasets.MaximumImportBytes()+1))
		if readErr != nil {
			return nil, readErr
		}
		hash := sha256.Sum256(data)
		if int64(len(data)) != source.SizeBytes || hex.EncodeToString(hash[:]) != source.SHA256 {
			return nil, fmt.Errorf("%w: training source changed", ErrInvalidArgument)
		}
		records, err := dataset.ReadRecords(ctx, strings.NewReader(string(data)))
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrInvalidArgument, err)
		}
		return records, dataset.TrainingReadiness(records)
	}
	run, err := b.cfg.Preparation.GetPreparation(ctx, input.Preparation)
	if err != nil {
		return nil, err
	}
	if run.Status != StatusCompleted || run.Kind != PreparationFull || run.Result == nil || !run.Result.TrainingReady {
		return nil, fmt.Errorf("%w: preparation is not training-ready", ErrInvalidArgument)
	}
	reader, err = b.cfg.Preparation.OpenRecords(run)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	records, err := dataset.ReadRecords(ctx, io.LimitReader(dataset.ContextReader{Context: ctx, Reader: reader}, b.cfg.Datasets.MaximumImportBytes()+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidArgument, err)
	}
	if len(records) != run.Result.RecordCount {
		return nil, fmt.Errorf("%w: prepared record count changed", ErrInvalidArgument)
	}
	if err := dataset.TrainingReadiness(records); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidArgument, err)
	}
	return records, nil
}

func (b *Business) trainingStage(ctx context.Context, input StageRequest, publish func(Progress)) (Artifacts, error) {
	info := activity.GetInfo(ctx)
	if input.Execution != (Execution{WorkflowID: info.WorkflowExecution.ID, RunID: info.WorkflowExecution.RunID}) || input.Key != input.RunID+"/"+string(input.Stage) {
		return Artifacts{}, model.InvalidOutput(input.Stage)
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
	modelPath := b.cfg.Models[input.Request.Model]
	metadata, err := os.Stat(modelPath)
	if err != nil {
		return Artifacts{}, err
	}
	if !metadata.Mode().IsRegular() {
		return Artifacts{}, temporal.NewNonRetryableApplicationError("model must be a regular GGUF file", "invalid-training", nil)
	}
	modelFile, err := os.Open(modelPath)
	if err != nil {
		return Artifacts{}, err
	}
	modelHash := sha256.New()
	_, copyErr := io.Copy(modelHash, dataset.ContextReader{Context: ctx, Reader: modelFile})
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
	root, err := os.OpenRoot(b.cfg.Datasets.Root())
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
	var state split.State
	statePath := "dataset.json"
	if input.Stage == StageSplit {
		return split.Run(ctx, workspace, b.cfg.Trainer, TrainingJob{Model: modelPath}, records, hex.EncodeToString(fingerprint[:]))
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
	job := TrainingJob{Model: modelPath, Training: input.Request.Training, Dataset: state.Dataset, Workdir: scratch}
	output := Artifacts{}
	switch input.Stage {
	case StageTrain:
		output, err = trainstage.Run(ctx, workspace, b.cfg.Trainer, job, publish)
	case StageEvaluate:
		output, err = evaluate.Run(ctx, workspace, b.cfg.Trainer, job)
	case StagePublish:
		output, err = publishstage.Run(ctx, workspace, input.Artifacts)
	default:
		return Artifacts{}, model.InvalidOutput(input.Stage)
	}
	if err != nil {
		return Artifacts{}, err
	}
	if err := ctx.Err(); err != nil {
		return Artifacts{}, err
	}
	if err := dataset.WriteJSON(workspace, cache, output); err != nil {
		return Artifacts{}, err
	}
	return output, nil
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
	err = json.NewDecoder(dataset.ContextReader{Context: ctx, Reader: reader}).Decode(&report)
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
	root, err := os.OpenRoot(b.cfg.Datasets.Root())
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.Open(filepath.Join("training", e.RunID, name))
}
