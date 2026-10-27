package training

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	optimization "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training/split"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// records reads bounded, confined training input and verifies its provenance.
func (b *Business) records(ctx context.Context, request Request, artifacts Artifacts, runID string) ([]optimization.Record, error) {
	root, err := os.OpenRoot(b.cfg.Imports.Root())
	if err != nil {
		return nil, err
	}
	defer root.Close()
	path := filepath.Join(storageDirectory, runID, "steps", "preparation", "dataset.jsonl")
	var size int64
	var hash string
	if request.Prepare == nil {
		source, err := b.cfg.Imports.GetDataset(ctx, request.DatasetID)
		if err != nil {
			return nil, err
		}
		if source.Format != ".jsonl" {
			return nil, fmt.Errorf("%w: training requires prepared JSONL", tmprl.ErrInvalidArgument)
		}
		path, size, hash = filepath.Join("imports", source.ID, "source.jsonl"), source.SizeBytes, source.SHA256
	} else if artifacts.Preparation == nil || !artifacts.Preparation.TrainingReady || artifacts.Preparation.DatasetID != request.Prepare.DatasetID {
		return nil, fmt.Errorf("%w: preparation is not training-ready", tmprl.ErrInvalidArgument)
	}
	file, err := root.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(tmprl.ContextReader{Context: ctx, Reader: file}, b.cfg.Imports.MaximumImportBytes()+1))
	if err != nil {
		return nil, err
	}
	actual := sha256.Sum256(data)
	if int64(len(data)) > b.cfg.Imports.MaximumImportBytes() || hash != "" && (int64(len(data)) != size || hex.EncodeToString(actual[:]) != hash) {
		return nil, fmt.Errorf("%w: training input size or digest changed", tmprl.ErrInvalidArgument)
	}
	records, err := optimization.ReadRecords(ctx, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if request.Prepare != nil && len(records) != artifacts.Preparation.RecordCount {
		return nil, fmt.Errorf("%w: prepared record count changed", tmprl.ErrInvalidArgument)
	}
	return records, optimization.TrainingReadiness(records)
}

// trainingContext resolves immutable inputs and borrows foundation's workspace.
// It does not own activity identity, heartbeats, retries, or completed-step commits.
func (b *Business) trainingContext(ctx context.Context, input tmprl.StepContext[Request, Artifacts]) (optimization.StepContext, error) {
	r := input.Request
	records, err := b.records(ctx, r.Request, r.Artifacts, r.RunID)
	if err != nil {
		return optimization.StepContext{}, err
	}
	modelPath := b.cfg.Models[r.Request.Model]
	file, err := os.Open(modelPath)
	if err != nil {
		return optimization.StepContext{}, err
	}
	metadata, statErr := file.Stat()
	if statErr != nil || !metadata.Mode().IsRegular() {
		return optimization.StepContext{}, errors.Join(statErr, invalidInput("model must be a regular GGUF file"), file.Close())
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, tmprl.ContextReader{Context: ctx, Reader: file})
	if err := errors.Join(copyErr, file.Close()); err != nil {
		return optimization.StepContext{}, err
	}
	identity, err := json.Marshal(struct {
		Request Request
		Records []optimization.Record
		Model   string
	}{r.Request, records, hex.EncodeToString(hash.Sum(nil))})
	if err != nil {
		return optimization.StepContext{}, err
	}
	fingerprint := sha256.Sum256(identity)
	key := hex.EncodeToString(fingerprint[:])
	b.cfg.Log(ctx, "training inputs resolved", "workflow_id", r.WorkflowID, "run_id", r.RunID, "step", r.Step, "model_id", r.Request.Model, "records", len(records), "fingerprint", key)
	var state split.State
	data, err := input.Workspace.ReadFile("dataset.json")
	if err == nil {
		if err := json.Unmarshal(data, &state); err != nil {
			return optimization.StepContext{}, err
		}
		if state.Fingerprint != key {
			return optimization.StepContext{}, invalidInput("training input changed after splitting")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return optimization.StepContext{}, err
	}
	scratch, err := os.MkdirTemp("", "moxie-training-")
	if err != nil {
		return optimization.StepContext{}, err
	}
	return optimization.StepContext{
		Artifacts: r.Artifacts.Training, Workspace: input.Workspace, Trainer: b.cfg.Trainer,
		Job:     optimization.TrainingJob{Model: modelPath, Training: r.Request.Training, Dataset: state.Dataset, Workdir: scratch},
		Records: records, Fingerprint: key, Publish: input.Publish,
	}, nil
}
