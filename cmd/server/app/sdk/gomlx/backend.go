// Package gomlx implements Moxie's local GoMLX fine-tuning backend.
package gomlx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/fineapp/finetune"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/gomlx/internal/gemma"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/gomlx/internal/gguf"
)

const (
	trainingStateFile                   = "training-state.json"
	trainingStateSchema                 = "gomlx-training.v8"
	defaultMaximumEpochs                = 3
	defaultEarlyStoppingPatience        = 3
	defaultMinimumValidationImprovement = 1e-3
	defaultLearningRate                 = 2e-4
	defaultWeightDecay                  = 0.01
	defaultBatchSize                    = 2
	defaultLoRARank                     = 8
	defaultLoRAAlpha                    = 8
	defaultSeed                         = 3407
)

var checkpointProbeEpochs []int

// Recipe describes the fixed local Gemma training settings shown by Moxie.
type Recipe struct {
	Workflow                     string
	MaximumEpochs                int
	EarlyStoppingPatience        int
	MinimumValidationImprovement float64
	LearningRate                 float64
	LoRARank                     int
	TrainingPhases               int
	CandidateCount               int
}

// CurrentRecipe returns the backend-owned settings for a new training run.
func CurrentRecipe() Recipe {
	return Recipe{
		Workflow: trainingStateSchema, MaximumEpochs: defaultMaximumEpochs,
		EarlyStoppingPatience:        defaultEarlyStoppingPatience,
		MinimumValidationImprovement: defaultMinimumValidationImprovement,
		LearningRate:                 defaultLearningRate, LoRARank: defaultLoRARank,
		TrainingPhases: 1, CandidateCount: 1,
	}
}

type trainFunc func(context.Context, finetune.Request, finetune.DatasetContract, finetune.ProgressFunc) (trainingOutput, error)

// Backend connects Moxie's model-neutral run manager to the Gemma GoMLX
// implementation. Training recipes remain internal presets.
type Backend struct {
	preflight func(context.Context, finetune.Request) error
	train     trainFunc
	evaluate  evaluateFunc
}

type trainingOutput struct {
	Candidate          string
	SelectedEpoch      int
	SelectedCheckpoint string
	SelectionMethod    string
	GeneratedScore     float64
	BestValidationLoss float64
	Epochs             []gemma.EpochMetrics
	StoppedEarly       bool
	Evaluation         *finetune.EvaluationReport
	Resumable          bool
}

type trainingState struct {
	Schema     string                         `json:"schema"`
	Model      string                         `json:"model"`
	Dataset    finetune.DatasetContractReport `json:"dataset"`
	Training   gemma.TrainingResult           `json:"training"`
	Selection  *checkpointSelection           `json:"selection,omitempty"`
	Candidate  string                         `json:"candidate,omitempty"`
	Evaluation *finetune.EvaluationReport     `json:"evaluation,omitempty"`
}

type checkpointSelection struct {
	Epoch             int     `json:"epoch"`
	Checkpoint        string  `json:"checkpoint"`
	ValidationLoss    float64 `json:"validation_loss"`
	GeneratedPassRate float64 `json:"generated_pass_rate"`
}

type trainingReport struct {
	Schema                       string                         `json:"schema"`
	Workflow                     string                         `json:"workflow"`
	Model                        string                         `json:"model"`
	Dataset                      finetune.DatasetContractReport `json:"dataset"`
	SelectedEpoch                int                            `json:"selected_epoch"`
	SelectedCheckpoint           string                         `json:"selected_checkpoint"`
	SelectionMethod              string                         `json:"selection_method"`
	GeneratedValidationPassRate  float64                        `json:"generated_validation_pass_rate"`
	BestValidationLoss           float64                        `json:"best_validation_loss"`
	MaximumEpochs                int                            `json:"maximum_epochs"`
	EarlyStopPatience            int                            `json:"early_stopping_patience"`
	MinimumValidationImprovement float64                        `json:"minimum_validation_improvement"`
	StoppedEarly                 bool                           `json:"stopped_early"`
	Epochs                       []epochReport                  `json:"epochs"`
	Evaluation                   *finetune.EvaluationReport     `json:"evaluation,omitempty"`
}

type epochReport struct {
	Epoch          int     `json:"epoch"`
	TrainingLoss   float64 `json:"training_loss"`
	ValidationLoss float64 `json:"validation_loss"`
	Checkpoint     string  `json:"checkpoint"`
}

// New returns the production local backend with Moxie's supported Gemma
// training preset.
func New() *Backend {
	backend := &Backend{}
	backend.preflight = backend.preflightRequest
	backend.train = backend.trainGemma
	backend.evaluate = evaluateProtectedTest
	return backend
}

// Preflight checks local inputs without creating a managed run directory or
// loading the complete model into memory.
func (backend *Backend) Preflight(ctx context.Context, request finetune.Request) error {
	if backend == nil || backend.preflight == nil {
		return errors.New("GoMLX backend is not initialized")
	}
	return backend.preflight(ctx, request)
}

// Run builds the protected dataset contract, trains Gemma, selects the best
// validation checkpoint, and exports its GGUF adapter into the managed run.
func (backend *Backend) Run(ctx context.Context, request finetune.Request, progress finetune.ProgressFunc) (finetune.Result, error) {
	if backend == nil || backend.train == nil || backend.evaluate == nil {
		return finetune.Result{}, errors.New("GoMLX backend is not initialized")
	}
	if err := context.Cause(ctx); err != nil {
		return finetune.Result{Status: finetune.StatusStopped}, err
	}
	if err := requireDirectory(request.Output); err != nil {
		return finetune.Result{}, err
	}

	reportProgress(progress, finetune.Progress{Stage: finetune.StageImporting, Total: 1, Message: "Reading prepared dataset"})
	state, resuming, err := readTrainingState(request.Output)
	if err != nil {
		return finetune.Result{}, err
	}
	var contract finetune.DatasetContract
	if resuming {
		contract, err = finetune.ValidateDatasetContract(request.Dataset)
		if err == nil {
			err = validateTrainingState(request, contract, state)
		}
	} else {
		contract, err = finetune.BuildDatasetContract(request.Dataset, request.Output)
	}
	if err != nil {
		return finetune.Result{}, fmt.Errorf("build training dataset contract: %w", err)
	}
	reportProgress(progress, finetune.Progress{Stage: finetune.StageValidating, Completed: 1, Total: 1, Message: "Dataset contract is ready"})

	trained, evaluation, err := backend.trainAndEvaluate(ctx, request, contract, progress)
	if err != nil {
		failed := finetune.Result{
			Status: finetune.StatusFailed, Resumable: trained.Resumable,
			SelectedEpoch: trained.SelectedEpoch, SelectedCheckpoint: trained.SelectedCheckpoint,
		}
		if errors.Is(err, context.Canceled) {
			failed.Status = finetune.StatusStopped
		}
		return failed, err
	}
	if !evaluation.Decision.Passed && len(evaluation.SubstitutedFactIDs) > 0 {
		recoveryRequest, recoveryContract, available, recoveryErr := buildRecoveryRequest(request, evaluation.SubstitutedFactIDs)
		if recoveryErr != nil {
			return resumableResult(trained), recoveryErr
		}
		if available {
			reportProgress(progress, finetune.Progress{
				Stage:   finetune.StageTraining,
				Message: "Retrying once without facts whose answers were substituted",
			})
			trained, evaluation, err = backend.trainAndEvaluate(ctx, recoveryRequest, recoveryContract, progress)
			if err != nil {
				return resumableResult(trained), err
			}
			contract = recoveryContract
		}
	}
	reportPath := filepath.Join(request.Output, "training-report.json")
	if err := writeTrainingReport(reportPath, request.Model, contract.Report, trained); err != nil {
		return resumableResult(trained), err
	}
	failed := finetune.Result{
		Report: reportPath, Status: finetune.StatusFailed, Resumable: trained.Resumable,
		SelectedEpoch: trained.SelectedEpoch, SelectedCheckpoint: trained.SelectedCheckpoint,
	}
	if !evaluation.Decision.Passed {
		failed.Status = finetune.StatusNeedsImprovement
		failed.Resumable = false
		failed.Error = "the trained candidate needs improvement: " + strings.Join(evaluation.Decision.RejectionReasons, "; ")
		return failed, nil
	}

	artifactDirectory := filepath.Join(request.Output, "artifacts")
	if err := os.MkdirAll(artifactDirectory, 0o700); err != nil {
		return failed, fmt.Errorf("create adapter artifact directory: %w", err)
	}
	adapterPath := filepath.Join(artifactDirectory, "adapter-f32.gguf")
	reportProgress(progress, finetune.Progress{Stage: finetune.StagePublishing, Total: 1, Message: "Packaging verified GGUF adapter"})
	if err := publishCandidate(ctx, trained.Candidate, adapterPath); err != nil {
		return failed, err
	}
	reportProgress(progress, finetune.Progress{Stage: finetune.StagePublishing, Completed: 1, Total: 1, Message: "Verified GGUF adapter is ready"})
	return finetune.Result{
		Adapter: adapterPath, Report: reportPath, Status: finetune.StatusCompleted,
		SelectedEpoch: trained.SelectedEpoch, SelectedCheckpoint: trained.SelectedCheckpoint,
	}, nil
}

func (backend *Backend) trainAndEvaluate(ctx context.Context, request finetune.Request, contract finetune.DatasetContract, progress finetune.ProgressFunc) (trainingOutput, finetune.EvaluationReport, error) {
	trained, err := backend.train(ctx, request, contract, progress)
	if err != nil {
		return trained, finetune.EvaluationReport{}, err
	}
	if trained.Evaluation != nil {
		reportProgress(progress, finetune.Progress{
			Stage: finetune.StageFinalEvaluation, Completed: 1, Total: 1,
			Message: "Using completed protected evaluation",
		})
		return trained, *trained.Evaluation, nil
	}
	evaluation, err := backend.evaluate(ctx, request, contract, trained.Candidate, progress)
	if evaluation.Schema != "" {
		trained.Evaluation = &evaluation
	}
	if err != nil {
		return trained, evaluation, err
	}
	state, found, err := readTrainingState(request.Output)
	if err != nil {
		return trained, evaluation, err
	}
	if found {
		state.Evaluation = trained.Evaluation
		if err := writeTrainingState(request.Output, state); err != nil {
			return trained, evaluation, err
		}
	}
	return trained, evaluation, nil
}

func buildRecoveryRequest(request finetune.Request, failedFactIDs []string) (finetune.Request, finetune.DatasetContract, bool, error) {
	recoveryOutput := filepath.Join(request.Output, "recovery")
	recoveryDataset := filepath.Join(request.Output, "recovery-dataset.jsonl")
	spares := filepath.Join(filepath.Dir(request.Dataset), "dataset-spares.jsonl")
	built, err := finetune.BuildRecoveryDataset(request.Dataset, spares, recoveryDataset, failedFactIDs)
	if err != nil || !built {
		return finetune.Request{}, finetune.DatasetContract{}, false, err
	}
	recoveryRequest := request
	recoveryRequest.Dataset = recoveryDataset
	recoveryRequest.Output = recoveryOutput
	_, resuming, err := readTrainingState(recoveryOutput)
	if err != nil {
		return finetune.Request{}, finetune.DatasetContract{}, false, err
	}
	if !resuming {
		if err := os.RemoveAll(recoveryOutput); err != nil {
			return finetune.Request{}, finetune.DatasetContract{}, false, fmt.Errorf("clear incomplete recovery output: %w", err)
		}
		if err := os.Mkdir(recoveryOutput, 0o700); err != nil {
			return finetune.Request{}, finetune.DatasetContract{}, false, fmt.Errorf("create recovery output: %w", err)
		}
		contract, err := finetune.BuildDatasetContract(recoveryDataset, recoveryOutput)
		return recoveryRequest, contract, true, err
	}
	contract, err := finetune.ValidateDatasetContract(recoveryDataset)
	return recoveryRequest, contract, true, err
}

func (backend *Backend) preflightRequest(ctx context.Context, request finetune.Request) error {
	if err := context.Cause(ctx); err != nil {
		return err
	}
	if err := requireRegularFile("prepared dataset", request.Dataset); err != nil {
		return err
	}
	if err := requireRegularFile("Gemma GGUF", request.Model); err != nil {
		return err
	}
	reader, err := gguf.OpenReader(request.Model)
	if err != nil {
		return fmt.Errorf("inspect Gemma GGUF: %w", err)
	}
	architecture, architectureErr := reader.String("general.architecture")
	closeErr := reader.Close()
	if architectureErr != nil {
		return fmt.Errorf("inspect Gemma GGUF architecture: %w", architectureErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close Gemma GGUF: %w", closeErr)
	}
	if architecture != "gemma3" {
		return fmt.Errorf("model architecture is %q, want gemma3", architecture)
	}
	if request.Runtime.Port < 0 || request.Runtime.Port > 65535 {
		return fmt.Errorf("llama-server port must be between 0 and 65535")
	}
	if request.Runtime.ContextSize < 0 {
		return fmt.Errorf("llama-server context size must not be negative")
	}
	_, err = resolveLlamaServer(request.Runtime.LlamaServer)
	return err
}

func (backend *Backend) trainGemma(ctx context.Context, request finetune.Request, contract finetune.DatasetContract, progress finetune.ProgressFunc) (trainingOutput, error) {
	state, resumed, err := readTrainingState(request.Output)
	if err != nil {
		return trainingOutput{}, err
	}
	if resumed {
		if err := validateTrainingState(request, contract, state); err != nil {
			return trainingOutput{}, err
		}
		if state.Candidate != "" && state.Selection != nil {
			return trainingOutputFromState(state), nil
		}
	}
	runtime, err := gemma.NewLoRARuntime(request.Model, gemma.AdapterConfig{
		Rank: defaultLoRARank, Alpha: defaultLoRAAlpha, AdapterScale: 1, Seed: defaultSeed,
	})
	if err != nil {
		return trainingOutput{}, fmt.Errorf("load Gemma training model: %w", err)
	}
	runtimeOpen := true
	defer func() {
		if runtimeOpen {
			runtime.Close()
		}
	}()

	state.Schema = trainingStateSchema
	state.Model = request.Model
	state.Dataset = contract.Report
	if state.Training.BestCheckpoint == "" {
		client, stopServer, err := startTokenizerServer(ctx, request, &http.Client{Timeout: 30 * time.Second})
		if err != nil {
			return trainingOutputFromState(state), err
		}
		serverRunning := true
		defer func() {
			if serverRunning {
				_ = stopServer()
			}
		}()

		prepared, err := runtime.PrepareDataset(ctx, contract, client, func(update gemma.TokenizationProgress) {
			reportProgress(progress, finetune.Progress{
				Stage: finetune.StageTokenizing, Completed: update.Completed, Total: update.Total,
				Message: fmt.Sprintf("Tokenizing %s split", update.Split),
			})
		})
		if err != nil {
			return trainingOutput{}, err
		}
		if err := stopServer(); err != nil {
			return trainingOutput{}, err
		}
		serverRunning = false
		conversationTraining := len(prepared.Training) - len(contract.Train)
		conversationValidation := len(prepared.Validation) - len(contract.Validation)
		if contract.Report.Task == "generation" && (conversationTraining == 0 || conversationValidation == 0) {
			return trainingOutput{}, errors.New("verify conversation-aware training plan: prepared dataset has no conversation examples")
		}
		stepsPerEpoch := (len(prepared.Training) + defaultBatchSize - 1) / defaultBatchSize
		totalSteps := stepsPerEpoch * defaultMaximumEpochs
		reportProgress(progress, finetune.Progress{
			Stage: finetune.StageTraining, Total: totalSteps,
			Message: fmt.Sprintf("Preparing %d examples · %d conversations", len(prepared.Training), conversationTraining),
		})
		state.Training, err = runtime.TrainDataset(ctx, prepared, gemma.TrainingConfig{
			Epochs: defaultMaximumEpochs, EarlyStoppingPatience: defaultEarlyStoppingPatience,
			MinimumValidationImprovement: defaultMinimumValidationImprovement,
			LearningRate:                 defaultLearningRate, WeightDecay: defaultWeightDecay,
			BatchSize: defaultBatchSize, Seed: defaultSeed,
			AdapterCheckpointDirectory: filepath.Join(request.Output, "checkpoints"),
			RetainedCheckpointEpochs:   checkpointProbeEpochs,
		}, func(update gemma.TrainingProgress) {
			reportProgress(progress, finetune.Progress{
				Stage: finetune.StageTraining, Completed: update.CompletedSteps,
				Total: totalSteps,
				Message: fmt.Sprintf("Epoch %d/%d · %d examples · %d conversations",
					update.Epoch, defaultMaximumEpochs, len(prepared.Training), conversationTraining),
			})
		})
		if err != nil {
			return trainingOutput{}, err
		}
		if err := writeTrainingState(request.Output, state); err != nil {
			return trainingOutput{}, err
		}
		reportProgress(progress, finetune.Progress{
			Stage: finetune.StageTraining, Completed: totalSteps, Total: totalSteps,
			Message:   "Training complete; checking generated answers",
			Resumable: true, SelectedEpoch: state.Training.BestEpoch, SelectedCheckpoint: state.Training.BestCheckpoint,
		})
	}
	if state.Selection == nil {
		probes, err := exportCheckpointProbes(runtime, request.Output, state.Training)
		if err != nil {
			return trainingOutputFromState(state), err
		}
		runtime.Close()
		runtimeOpen = false

		selection, selectedAdapter, err := selectGeneratedCheckpoint(ctx, request, contract, probes, progress)
		if err != nil {
			return trainingOutputFromState(state), err
		}
		candidateDirectory := filepath.Join(request.Output, "candidates")
		if err := os.MkdirAll(candidateDirectory, 0o700); err != nil {
			return trainingOutput{}, fmt.Errorf("create adapter candidate directory: %w", err)
		}
		state.Candidate = filepath.Join(candidateDirectory, "adapter-f32.gguf")
		if err := publishCandidate(ctx, selectedAdapter, state.Candidate); err != nil {
			return trainingOutputFromState(state), err
		}
		state.Selection = &selection
		if err := removeUnselectedCheckpoints(&state.Training, selection.Checkpoint); err != nil {
			return trainingOutputFromState(state), err
		}
		if err := os.RemoveAll(filepath.Join(request.Output, "checkpoint-probes")); err != nil {
			return trainingOutputFromState(state), fmt.Errorf("remove temporary checkpoint adapters: %w", err)
		}
		if err := writeTrainingState(request.Output, state); err != nil {
			return trainingOutputFromState(state), err
		}
	}
	return trainingOutputFromState(state), nil
}

type checkpointProbe struct {
	Epoch          int
	Checkpoint     string
	Adapter        string
	ValidationLoss float64
}

func exportCheckpointProbes(runtime *gemma.Runtime, output string, result gemma.TrainingResult) ([]checkpointProbe, error) {
	directory := filepath.Join(output, "checkpoint-probes")
	if err := os.RemoveAll(directory); err != nil {
		return nil, fmt.Errorf("clear temporary checkpoint adapters: %w", err)
	}
	if err := os.Mkdir(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create temporary checkpoint adapters: %w", err)
	}
	probes := make([]checkpointProbe, 0, len(checkpointProbeEpochs)+1)
	for _, epoch := range result.Epochs {
		if epoch.Checkpoint == "" {
			continue
		}
		adapter := filepath.Join(directory, fmt.Sprintf("epoch-%02d.gguf", epoch.Epoch))
		if err := runtime.ExportAdapterCheckpoint(epoch.Checkpoint, adapter); err != nil {
			return nil, fmt.Errorf("export epoch %d checkpoint probe: %w", epoch.Epoch, err)
		}
		probes = append(probes, checkpointProbe{
			Epoch: epoch.Epoch, Checkpoint: epoch.Checkpoint, Adapter: adapter, ValidationLoss: epoch.ValidationLoss,
		})
	}
	if len(probes) == 0 {
		return nil, errors.New("training retained no checkpoints for generated-answer selection")
	}
	return probes, nil
}

func removeUnselectedCheckpoints(result *gemma.TrainingResult, selected string) error {
	for index := range result.Epochs {
		checkpoint := result.Epochs[index].Checkpoint
		if checkpoint == "" || checkpoint == selected || checkpoint == result.BestCheckpoint {
			continue
		}
		if err := os.RemoveAll(checkpoint); err != nil {
			return fmt.Errorf("remove unselected epoch %d checkpoint: %w", result.Epochs[index].Epoch, err)
		}
		result.Epochs[index].Checkpoint = ""
	}
	return nil
}

func selectGeneratedCheckpoint(ctx context.Context, request finetune.Request, contract finetune.DatasetContract, probes []checkpointProbe, progress finetune.ProgressFunc) (checkpointSelection, string, error) {
	var selected checkpointSelection
	selectedAdapter := ""
	for index, probe := range probes {
		reportProgress(progress, finetune.Progress{
			Stage: finetune.StageTraining, Completed: index, Total: len(probes),
			Message: fmt.Sprintf("Testing generated answers from epoch %d", probe.Epoch),
		})
		results, err := evaluateAdapterRecords(ctx, request, contract.Validation, probe.Adapter)
		if err != nil {
			return checkpointSelection{}, "", fmt.Errorf("evaluate epoch %d generated answers: %w", probe.Epoch, err)
		}
		passingRate, err := finetune.EvaluationPassingRate(contract.Report.Task, results)
		if err != nil {
			return checkpointSelection{}, "", fmt.Errorf("score epoch %d generated answers: %w", probe.Epoch, err)
		}
		candidate := checkpointSelection{
			Epoch: probe.Epoch, Checkpoint: probe.Checkpoint,
			ValidationLoss: probe.ValidationLoss, GeneratedPassRate: passingRate,
		}
		if selectedAdapter == "" || generatedCheckpointIsBetter(candidate, selected) {
			selected = candidate
			selectedAdapter = probe.Adapter
		}
	}
	reportProgress(progress, finetune.Progress{
		Stage: finetune.StageTraining, Completed: len(probes), Total: len(probes),
		Message:   fmt.Sprintf("Selected epoch %d from generated validation answers", selected.Epoch),
		Resumable: true, SelectedEpoch: selected.Epoch, SelectedCheckpoint: selected.Checkpoint,
	})
	return selected, selectedAdapter, nil
}

func generatedCheckpointIsBetter(candidate, selected checkpointSelection) bool {
	return candidate.GeneratedPassRate > selected.GeneratedPassRate ||
		candidate.GeneratedPassRate == selected.GeneratedPassRate && candidate.ValidationLoss < selected.ValidationLoss
}

func trainingOutputFromState(state trainingState) trainingOutput {
	evaluation := state.Evaluation
	if evaluation != nil && evaluation.Schema != finetune.EvaluationReportSchema {
		evaluation = nil
	}
	selectedEpoch := state.Training.BestEpoch
	selectedCheckpoint := state.Training.BestCheckpoint
	selectionMethod := "teacher-forced validation loss"
	generatedScore := float64(0)
	if state.Selection != nil {
		selectedEpoch = state.Selection.Epoch
		selectedCheckpoint = state.Selection.Checkpoint
		selectionMethod = "generated validation answers"
		generatedScore = state.Selection.GeneratedPassRate
	}
	output := trainingOutput{
		Candidate: state.Candidate, SelectedEpoch: selectedEpoch,
		SelectedCheckpoint: selectedCheckpoint, SelectionMethod: selectionMethod, GeneratedScore: generatedScore,
		BestValidationLoss: state.Training.BestValidationLoss,
		Epochs:             state.Training.Epochs, StoppedEarly: state.Training.StoppedEarly,
		Evaluation: evaluation, Resumable: selectedCheckpoint != "",
	}
	return output
}

func resumableResult(trained trainingOutput) finetune.Result {
	return finetune.Result{
		Status: finetune.StatusFailed, Resumable: trained.Resumable,
		SelectedEpoch: trained.SelectedEpoch, SelectedCheckpoint: trained.SelectedCheckpoint,
	}
}

func readTrainingState(output string) (trainingState, bool, error) {
	path := filepath.Join(output, trainingStateFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return trainingState{}, false, nil
	}
	if err != nil {
		return trainingState{}, false, fmt.Errorf("read training phase state: %w", err)
	}
	var state trainingState
	if err := json.Unmarshal(data, &state); err != nil {
		return trainingState{}, false, fmt.Errorf("decode training phase state: %w", err)
	}
	return state, true, nil
}

func validateTrainingState(request finetune.Request, contract finetune.DatasetContract, state trainingState) error {
	if state.Schema != trainingStateSchema {
		return fmt.Errorf("training phase state uses unsupported schema %q", state.Schema)
	}
	if state.Model != request.Model {
		return errors.New("training phase state belongs to a different base model")
	}
	if state.Dataset != contract.Report {
		return errors.New("training phase state belongs to a different dataset contract")
	}
	if err := validateTrainingResult(request.Output, "training", state.Training); err != nil {
		return err
	}
	if state.Selection != nil {
		matched := false
		for _, epoch := range state.Training.Epochs {
			if epoch.Epoch == state.Selection.Epoch && epoch.Checkpoint == state.Selection.Checkpoint && epoch.ValidationLoss == state.Selection.ValidationLoss {
				matched = true
				break
			}
		}
		if !matched {
			return errors.New("generated-answer checkpoint selection does not match the training metrics")
		}
		if state.Selection.GeneratedPassRate < 0 || state.Selection.GeneratedPassRate > 1 {
			return errors.New("generated-answer checkpoint selection has an invalid passing rate")
		}
		if err := validateManagedTrainingPath(request.Output, "generated-answer checkpoint", state.Selection.Checkpoint, true); err != nil {
			return err
		}
		if state.Candidate == "" {
			return errors.New("generated-answer checkpoint selection has no exported candidate")
		}
	}
	if state.Candidate != "" {
		if state.Selection == nil {
			return errors.New("adapter candidate has no generated-answer checkpoint selection")
		}
		if err := validateManagedTrainingPath(request.Output, "adapter candidate", state.Candidate, false); err != nil {
			return err
		}
	}
	return nil
}

func validateTrainingResult(output, phase string, result gemma.TrainingResult) error {
	if result.BestEpoch <= 0 || result.BestCheckpoint == "" || len(result.Epochs) == 0 {
		return fmt.Errorf("%s training phase has no validation-selected checkpoint", phase)
	}
	selected := false
	for _, epoch := range result.Epochs {
		if epoch.Epoch == result.BestEpoch && epoch.Checkpoint == result.BestCheckpoint && epoch.ValidationLoss == result.BestValidationLoss {
			selected = true
			break
		}
	}
	if !selected {
		return fmt.Errorf("%s training phase selected checkpoint does not match its validation metrics", phase)
	}
	return validateManagedTrainingPath(output, phase+" checkpoint", result.BestCheckpoint, true)
}

func validateManagedTrainingPath(output, name, path string, directory bool) error {
	relative, err := filepath.Rel(output, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return fmt.Errorf("%s is outside the managed run", name)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", name, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || info.IsDir() != directory || (!directory && !info.Mode().IsRegular()) {
		kind := "file"
		if directory {
			kind = "directory"
		}
		return fmt.Errorf("%s is not a valid managed %s", name, kind)
	}
	return nil
}

func writeTrainingState(output string, state trainingState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode training phase state: %w", err)
	}
	temporary, err := os.CreateTemp(output, ".training-state-*.json")
	if err != nil {
		return fmt.Errorf("create training phase state: %w", err)
	}
	temporaryPath := temporary.Name()
	published := false
	defer func() {
		_ = temporary.Close()
		if !published {
			_ = os.Remove(temporaryPath)
		}
	}()
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write training phase state: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync training phase state: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close training phase state: %w", err)
	}
	if err := os.Chmod(temporaryPath, 0o600); err != nil {
		return fmt.Errorf("secure training phase state: %w", err)
	}
	if err := os.Rename(temporaryPath, filepath.Join(output, trainingStateFile)); err != nil {
		return fmt.Errorf("publish training phase state: %w", err)
	}
	published = true
	return nil
}

func writeTrainingReport(path, modelPath string, dataset finetune.DatasetContractReport, trained trainingOutput) error {
	report := trainingReport{
		Schema: "finetune-run.v1", Workflow: trainingStateSchema, Model: modelPath, Dataset: dataset,
		SelectedEpoch: trained.SelectedEpoch, SelectedCheckpoint: trained.SelectedCheckpoint,
		SelectionMethod: trained.SelectionMethod, GeneratedValidationPassRate: trained.GeneratedScore,
		BestValidationLoss: trained.BestValidationLoss, MaximumEpochs: defaultMaximumEpochs,
		EarlyStopPatience:            defaultEarlyStoppingPatience,
		MinimumValidationImprovement: defaultMinimumValidationImprovement,
		StoppedEarly:                 trained.StoppedEarly,
		Epochs:                       make([]epochReport, len(trained.Epochs)), Evaluation: trained.Evaluation,
	}
	for index, epoch := range trained.Epochs {
		report.Epochs[index] = epochReport{
			Epoch: epoch.Epoch, TrainingLoss: epoch.TrainingLoss,
			ValidationLoss: epoch.ValidationLoss, Checkpoint: epoch.Checkpoint,
		}
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode training report: %w", err)
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".training-report-*.json")
	if err != nil {
		return fmt.Errorf("create training report %q: %w", path, err)
	}
	temporaryPath := file.Name()
	written := false
	defer func() {
		_ = file.Close()
		if !written {
			_ = os.Remove(temporaryPath)
		}
	}()
	if _, err := file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write training report %q: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close training report %q: %w", path, err)
	}
	if err := os.Chmod(temporaryPath, 0o600); err != nil {
		return fmt.Errorf("secure training report %q: %w", path, err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish training report %q: %w", path, err)
	}
	written = true
	return nil
}

func publishCandidate(ctx context.Context, candidate, artifact string) error {
	if err := context.Cause(ctx); err != nil {
		return err
	}
	if err := requireRegularFile("evaluated adapter candidate", candidate); err != nil {
		return err
	}
	input, err := os.Open(candidate)
	if err != nil {
		return fmt.Errorf("open evaluated adapter candidate: %w", err)
	}
	defer input.Close()
	output, err := os.CreateTemp(filepath.Dir(artifact), ".adapter-*.gguf")
	if err != nil {
		return fmt.Errorf("create verified adapter: %w", err)
	}
	temporaryPath := output.Name()
	published := false
	defer func() {
		_ = output.Close()
		if !published {
			_ = os.Remove(temporaryPath)
		}
	}()
	if _, err := io.Copy(output, input); err != nil {
		return fmt.Errorf("copy verified adapter: %w", err)
	}
	if err := context.Cause(ctx); err != nil {
		return err
	}
	if err := output.Sync(); err != nil {
		return fmt.Errorf("sync verified adapter: %w", err)
	}
	if err := output.Close(); err != nil {
		return fmt.Errorf("close verified adapter: %w", err)
	}
	if err := os.Chmod(temporaryPath, 0o600); err != nil {
		return fmt.Errorf("secure verified adapter: %w", err)
	}
	if err := os.Rename(temporaryPath, artifact); err != nil {
		return fmt.Errorf("publish verified adapter: %w", err)
	}
	published = true
	return nil
}

func requireRegularFile(name, path string) error {
	if path == "" {
		return fmt.Errorf("%s path is required", name)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect %s %q: %w", name, path, err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s %q is not a regular file", name, path)
	}
	return nil
}

func requireDirectory(path string) error {
	if path == "" {
		return fmt.Errorf("managed run output is required")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect managed run output %q: %w", path, err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("managed run output %q is not a directory", path)
	}
	return nil
}

func reportProgress(progress finetune.ProgressFunc, update finetune.Progress) {
	if progress != nil {
		progress(update)
	}
}

var _ finetune.Backend = (*Backend)(nil)
