package modeltraining

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

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/dataset"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/model"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/training/split"

	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
)

// StartTraining submits the YAML-defined job using already-prepared data.
// Use StartJob to include document preparation in the same worker reservation.
func (b *Business) StartTraining(ctx context.Context, input TrainingRequest) (tmprl.Run[model.Request, model.Artifacts], error) {
	return b.StartJob(ctx, model.JobRequest{ID: input.ID, Model: input.Model, Training: input.Training, DatasetID: input.DatasetID, Preparation: input.Preparation})
}

func (b *Business) validateTraining(input Request) error {
	if err := validate(input); err != nil {
		return err
	}
	sources := 0
	if input.DatasetID != "" {
		sources++
	}
	if input.Preparation != (tmprl.Execution{}) {
		sources++
	}
	if input.Prepare != nil {
		sources++
	}
	if sources != 1 {
		return fmt.Errorf("%w: select exactly one document preparation, prepared dataset or preparation execution", ErrInvalidArgument)
	}
	if input.Preparation != (tmprl.Execution{}) {
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
	if run.Status != tmprl.StatusCompleted || run.Kind != PreparationFull || run.Result == nil || !run.Result.TrainingReady {
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

// trainingLegacyStep preserves the original activity result for older histories.
func (b *Business) trainingLegacyStep(ctx context.Context, input LegacyStepRequest, publish func(tmprl.Progress)) (Artifacts, error) {
	info := activity.GetInfo(ctx)
	if input.Execution != (tmprl.Execution{WorkflowID: info.WorkflowExecution.ID, RunID: info.WorkflowExecution.RunID}) || input.Key != input.RunID+"/"+string(input.Stage) {
		return Artifacts{}, model.InvalidOutput(string(input.Stage))
	}
	if _, exists := b.legacySteps[input.Stage]; !exists {
		return Artifacts{}, model.InvalidOutput(string(input.Stage))
	}
	result, err := b.managedLegacyStep(ctx, input, publish)
	return result.Artifacts, err
}

// managedLegacyStep resolves a flat-pipeline step by its historical identity.
// managedStep owns resource lifetime and caching; handlers own computation.
func (b *Business) managedLegacyStep(ctx context.Context, input LegacyStepRequest, publish func(tmprl.Progress)) (tmprl.StepResult[model.Artifacts], error) {
	return b.managedStep(ctx, input, b.legacySteps[input.Stage], publish)
}

func (b *Business) managedStep(ctx context.Context, input LegacyStepRequest, implementation model.StepHandler, publish func(tmprl.Progress)) (tmprl.StepResult[model.Artifacts], error) {
	log := activity.GetLogger(ctx)
	if err := b.validateTraining(input.Request); err != nil {
		return tmprl.StepResult[model.Artifacts]{}, temporal.NewNonRetryableApplicationError(err.Error(), "invalid-training", err)
	}
	if b.cfg.Trainer == nil {
		return tmprl.StepResult[model.Artifacts]{}, temporal.NewNonRetryableApplicationError("trainer is not configured", "invalid-training", nil)
	}
	var records []Record
	var err error
	if input.Request.Prepare != nil {
		prepared := input.Artifacts.Preparation
		if prepared == nil || !prepared.TrainingReady {
			return tmprl.StepResult[model.Artifacts]{}, temporal.NewNonRetryableApplicationError("preparation is not training-ready", "invalid-training", nil)
		}
		reader, openErr := b.cfg.Preparation.OpenRecords(model.PreparationRun{Status: tmprl.StatusCompleted, Result: prepared})
		if openErr != nil {
			return tmprl.StepResult[model.Artifacts]{}, openErr
		}
		records, err = dataset.ReadRecords(ctx, io.LimitReader(dataset.ContextReader{Context: ctx, Reader: reader}, b.cfg.Datasets.MaximumImportBytes()+1))
		closeErr := reader.Close()
		if err == nil {
			err = closeErr
		}
		if err == nil && len(records) != prepared.RecordCount {
			err = fmt.Errorf("%w: prepared record count changed", ErrInvalidArgument)
		}
		if err == nil {
			err = dataset.TrainingReadiness(records)
		}
	} else {
		records, err = b.trainingRecords(ctx, input.Request)
	}
	if err != nil {
		if errors.Is(err, ErrInvalidArgument) || errors.Is(err, ErrNotFound) {
			return tmprl.StepResult[model.Artifacts]{}, temporal.NewNonRetryableApplicationError(err.Error(), "invalid-training", err)
		}
		return tmprl.StepResult[model.Artifacts]{}, err
	}
	log.Info("training input resolved", "step", input.Stage, "records", len(records), "model_id", input.Request.Model)
	modelPath := b.cfg.Models[input.Request.Model]
	metadata, err := os.Stat(modelPath)
	if err != nil {
		return tmprl.StepResult[model.Artifacts]{}, err
	}
	if !metadata.Mode().IsRegular() {
		return tmprl.StepResult[model.Artifacts]{}, temporal.NewNonRetryableApplicationError("model must be a regular GGUF file", "invalid-training", nil)
	}
	modelFile, err := os.Open(modelPath)
	if err != nil {
		return tmprl.StepResult[model.Artifacts]{}, err
	}
	modelHash := sha256.New()
	_, copyErr := io.Copy(modelHash, dataset.ContextReader{Context: ctx, Reader: modelFile})
	if err := errors.Join(copyErr, modelFile.Close()); err != nil {
		return tmprl.StepResult[model.Artifacts]{}, err
	}
	identity, err := json.Marshal(struct {
		Request Request
		Records []Record
		Model   string
	}{input.Request, records, hex.EncodeToString(modelHash.Sum(nil))})
	if err != nil {
		return tmprl.StepResult[model.Artifacts]{}, err
	}
	fingerprint := sha256.Sum256(identity)
	root, err := os.OpenRoot(b.cfg.Datasets.Root())
	if err != nil {
		return tmprl.StepResult[model.Artifacts]{}, err
	}
	defer root.Close()
	path := filepath.Join("training", input.RunID)
	if err := root.MkdirAll(path, 0o700); err != nil {
		return tmprl.StepResult[model.Artifacts]{}, err
	}
	workspace, err := root.OpenRoot(path)
	if err != nil {
		return tmprl.StepResult[model.Artifacts]{}, err
	}
	defer workspace.Close()
	var state split.State
	if data, err := workspace.ReadFile("dataset.json"); err == nil {
		if err := json.Unmarshal(data, &state); err != nil {
			return tmprl.StepResult[model.Artifacts]{}, err
		}
		if state.Fingerprint != hex.EncodeToString(fingerprint[:]) {
			return tmprl.StepResult[model.Artifacts]{}, temporal.NewNonRetryableApplicationError("training input changed", "invalid-training", nil)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return tmprl.StepResult[model.Artifacts]{}, err
	}
	if err := workspace.MkdirAll("stages", 0o700); err != nil {
		return tmprl.StepResult[model.Artifacts]{}, err
	}
	// Older activities checkpointed only their owned artifact at the root.
	// Reuse those commits too if an old activity is retried after deployment.
	if activity.GetInfo(ctx).ActivityType.Name == ActivityName {
		if data, err := workspace.ReadFile(string(input.Stage) + ".json"); err == nil {
			var output Artifacts
			if err := json.Unmarshal(data, &output); err != nil {
				return tmprl.StepResult[model.Artifacts]{}, err
			}
			log.Info("legacy training checkpoint reused", "step", input.Stage)
			return legacyStepResult(input, output)
		} else if !errors.Is(err, os.ErrNotExist) {
			return tmprl.StepResult[model.Artifacts]{}, err
		}
	}
	cache := filepath.Join("stages", string(input.Stage)+".json")
	if data, err := workspace.ReadFile(cache); err == nil {
		var output tmprl.StepResult[model.Artifacts]
		if err := json.Unmarshal(data, &output); err != nil {
			return tmprl.StepResult[model.Artifacts]{}, err
		}
		log.Info("training step checkpoint reused", "step", input.Stage, "fingerprint", hex.EncodeToString(fingerprint[:]))
		return output, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return tmprl.StepResult[model.Artifacts]{}, err
	}
	scratch, err := os.MkdirTemp("", "moxie-training-")
	if err != nil {
		return tmprl.StepResult[model.Artifacts]{}, err
	}
	defer os.RemoveAll(scratch)
	job := TrainingJob{Model: modelPath, Training: input.Request.Training, Dataset: state.Dataset, Workdir: scratch}
	output, err := implementation.Execute(ctx, model.StepContext{
		Request: input, Workspace: workspace, Trainer: b.cfg.Trainer, Job: job,
		Records: records, Fingerprint: hex.EncodeToString(fingerprint[:]), Publish: publish,
	})
	if err != nil {
		return tmprl.StepResult[model.Artifacts]{}, err
	}
	if err := ctx.Err(); err != nil {
		return tmprl.StepResult[model.Artifacts]{}, err
	}
	if err := dataset.WriteJSON(workspace, cache, output); err != nil {
		return tmprl.StepResult[model.Artifacts]{}, err
	}
	log.Info("training step checkpoint committed", "step", input.Stage, "fingerprint", hex.EncodeToString(fingerprint[:]), "outcome", output.Status)
	return output, nil
}

// GetTrainingEvaluation reads the SDK-owned report, including rejected runs.
// A head never needs to interpret worker storage paths.
func (b *Business) GetTrainingEvaluation(ctx context.Context, e tmprl.Execution) (EvaluationReport, error) {
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
func (b *Business) OpenTrainingAdapter(ctx context.Context, e tmprl.Execution) (io.ReadCloser, error) {
	return b.openTrainingArtifact(ctx, e, "adapter.gguf", true)
}

func (b *Business) openTrainingArtifact(ctx context.Context, e tmprl.Execution, name string, accepted bool) (io.ReadCloser, error) {
	run, err := b.Get(ctx, e)
	if err != nil {
		return nil, err
	}
	if (run.Request.DatasetID == "" && run.Request.Preparation == (tmprl.Execution{})) || run.Artifacts.Report == "" || (accepted && (run.Status != tmprl.StatusCompleted || !run.Artifacts.Accepted)) {
		return nil, fmt.Errorf("%w: training artifact is not available", ErrInvalidArgument)
	}
	root, err := os.OpenRoot(b.cfg.Datasets.Root())
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.Open(filepath.Join("training", e.RunID, name))
}
