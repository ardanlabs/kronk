package gomlx

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/fineapp/finetune"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/gomlx/internal/gemma"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/gomlx/internal/gguf"
)

func TestBackendConnectsManagedRunToDatasetAndTrainingOutputs(t *testing.T) {
	dataset := writePreparedDataset(t, 120)
	var trainedContract finetune.DatasetContract
	backend := &Backend{
		preflight: func(context.Context, finetune.Request) error { return nil },
		train: func(_ context.Context, request finetune.Request, contract finetune.DatasetContract, progress finetune.ProgressFunc) (trainingOutput, error) {
			trainedContract = contract
			progress(finetune.Progress{Stage: finetune.StageTraining, Completed: 1, Total: 1, Message: "Training test adapter"})
			checkpoint := filepath.Join(request.Output, "checkpoints", "epoch-01")
			if err := os.MkdirAll(checkpoint, 0o700); err != nil {
				return trainingOutput{}, err
			}
			candidate := filepath.Join(request.Output, "candidates", "final-test-adapter-f32.gguf")
			if err := os.MkdirAll(filepath.Dir(candidate), 0o700); err != nil {
				return trainingOutput{}, err
			}
			if err := os.WriteFile(candidate, []byte("adapter"), 0o600); err != nil {
				return trainingOutput{}, err
			}
			return trainingOutput{
				Candidate: candidate, SelectedEpoch: 1, SelectedCheckpoint: checkpoint,
				BestValidationLoss: 0.25,
				Epochs:             []gemma.EpochMetrics{{Epoch: 1, TrainingLoss: 0.5, ValidationLoss: 0.25, Checkpoint: checkpoint}},
			}, nil
		},
		evaluate: func(_ context.Context, _ finetune.Request, contract finetune.DatasetContract, candidate string, progress finetune.ProgressFunc) (finetune.EvaluationReport, error) {
			if len(contract.Test) == 0 || candidate == "" {
				t.Fatal("protected evaluation did not receive the selected candidate and test split")
			}
			progress(finetune.Progress{Stage: finetune.StageFinalEvaluation, Completed: 1, Total: 1, Message: "Protected evaluation passed"})
			return finetune.EvaluationReport{
				Schema:   finetune.EvaluationReportSchema,
				Decision: finetune.ReliabilityDecision{Passed: true},
			}, nil
		},
	}
	manager, err := finetune.NewManager(t.TempDir(), backend)
	if err != nil {
		t.Fatal(err)
	}
	updates := make(chan finetune.RunInfo, 16)
	initial, err := manager.Start(t.Context(), finetune.Request{Dataset: dataset, Model: "gemma.gguf"}, func(info finetune.RunInfo) {
		updates <- info
	})
	if err != nil {
		t.Fatal(err)
	}
	completed := waitForTerminalRun(t, updates)
	if completed.Status != finetune.StatusCompleted || completed.SelectedEpoch != 1 {
		t.Fatalf("completed run = %+v", completed)
	}
	if len(trainedContract.Train) < finetune.MinimumTrainRecords || len(trainedContract.Validation) < finetune.MinimumValidationRecords || len(trainedContract.Test) < finetune.MinimumTestRecords {
		t.Fatalf("dataset split sizes = %d/%d/%d", len(trainedContract.Train), len(trainedContract.Validation), len(trainedContract.Test))
	}
	for _, name := range []string{"train.jsonl", "validation.jsonl", "test.jsonl", "contract.json", "training-report.json"} {
		if _, err := os.Stat(filepath.Join(initial.Output, name)); err != nil {
			t.Fatalf("managed artifact %q: %v", name, err)
		}
	}
	var report trainingReport
	readJSONFile(t, completed.Report, &report)
	if report.Schema != "finetune-run.v1" || report.Workflow != trainingStateSchema || report.Dataset.TestRecords != len(trainedContract.Test) || report.BestValidationLoss != 0.25 || report.MaximumEpochs != defaultMaximumEpochs || report.EarlyStopPatience != defaultEarlyStoppingPatience || report.MinimumValidationImprovement != defaultMinimumValidationImprovement || report.StoppedEarly || report.Evaluation == nil || !report.Evaluation.Decision.Passed {
		t.Fatalf("training report = %+v", report)
	}
}

func TestCurrentRecipeMatchesTrainingBackend(t *testing.T) {
	recipe := CurrentRecipe()
	if recipe.Workflow != trainingStateSchema || recipe.Method != defaultMethod || recipe.BaseWeights != defaultBaseWeights || recipe.Optimizer != defaultOptimizer || recipe.MaximumEpochs != defaultMaximumEpochs || recipe.EarlyStoppingPatience != defaultEarlyStoppingPatience || recipe.MinimumValidationImprovement != defaultMinimumValidationImprovement || recipe.LearningRate != defaultLearningRate || recipe.WeightDecay != defaultWeightDecay || recipe.BatchSize != defaultBatchSize || recipe.Seed != defaultSeed || recipe.LoRA.Rank != defaultLoRARank || recipe.LoRA.Alpha != defaultLoRAAlpha || recipe.LoRA.Scale != 1 || recipe.Loss != defaultLoss || recipe.CheckpointSelection != defaultCheckpointSelection || recipe.Output.Format != defaultOutputFormat || recipe.Output.Precision != defaultOutputPrecision || recipe.Output.Filename != defaultOutputFilename || recipe.TrainingPhases != 1 || recipe.CandidateCount != 1 {
		t.Fatalf("recipe = %+v", recipe)
	}
	wantModules := []string{"query", "key", "value", "output", "gate", "up", "down"}
	if recipe.MaximumEpochs != 3 || recipe.LoRA.Rank != 8 || !slices.Equal(recipe.LoRA.TargetModules, wantModules) || recipe.Sequence.MaximumTokens != 0 || recipe.Sequence.Source != defaultSequenceSource || recipe.Sequence.Overflow != defaultSequenceOverflow || recipe.Sequence.Padding != defaultSequencePadding || len(checkpointProbeEpochs) != 0 {
		t.Fatalf("training schedule = %+v with probes %v", recipe, checkpointProbeEpochs)
	}
}

func TestRecipeReadsModelContextFromGGUF(t *testing.T) {
	modelPath := filepath.Join(t.TempDir(), "gemma.gguf")
	writer := gguf.Writer{}
	writer.String("general.architecture", "gemma3")
	writer.Uint32("gemma3.context_length", 32768)
	if err := writer.WriteFile(modelPath); err != nil {
		t.Fatal(err)
	}

	recipe, err := New().Recipe(modelPath)
	if err != nil {
		t.Fatal(err)
	}
	if recipe.Sequence.MaximumTokens != 32768 {
		t.Fatalf("maximum tokens: got %d, want 32768", recipe.Sequence.MaximumTokens)
	}
}

func TestBackendDoesNotPublishRejectedAdapter(t *testing.T) {
	dataset := writePreparedDataset(t, 120)
	backend := &Backend{
		preflight: func(context.Context, finetune.Request) error { return nil },
		train: func(_ context.Context, request finetune.Request, _ finetune.DatasetContract, _ finetune.ProgressFunc) (trainingOutput, error) {
			checkpoint := filepath.Join(request.Output, "checkpoints", "epoch-01")
			if err := os.MkdirAll(checkpoint, 0o700); err != nil {
				return trainingOutput{}, err
			}
			candidate := filepath.Join(request.Output, "candidates", "final-test-adapter-f32.gguf")
			if err := os.MkdirAll(filepath.Dir(candidate), 0o700); err != nil {
				return trainingOutput{}, err
			}
			if err := os.WriteFile(candidate, []byte("candidate"), 0o600); err != nil {
				return trainingOutput{}, err
			}
			return trainingOutput{Candidate: candidate, SelectedEpoch: 1, SelectedCheckpoint: checkpoint}, nil
		},
		evaluate: func(context.Context, finetune.Request, finetune.DatasetContract, string, finetune.ProgressFunc) (finetune.EvaluationReport, error) {
			return finetune.EvaluationReport{
				Schema: finetune.EvaluationReportSchema,
				Decision: finetune.ReliabilityDecision{
					RejectionReasons: []string{"exact accuracy is below the publication threshold"},
				},
			}, nil
		},
	}
	manager, err := finetune.NewManager(t.TempDir(), backend)
	if err != nil {
		t.Fatal(err)
	}
	updates := make(chan finetune.RunInfo, 16)
	initial, err := manager.Start(t.Context(), finetune.Request{Dataset: dataset, Model: "gemma.gguf"}, func(info finetune.RunInfo) {
		updates <- info
	})
	if err != nil {
		t.Fatal(err)
	}
	result := waitForTerminalRun(t, updates)
	if result.Status != finetune.StatusNeedsImprovement || result.Adapter != "" || result.Report == "" || !strings.Contains(result.Error, "needs improvement") {
		t.Fatalf("run needing improvement = %+v", result)
	}
	if _, err := os.Stat(filepath.Join(initial.Output, "artifacts", "adapter-f32.gguf")); !os.IsNotExist(err) {
		t.Fatalf("rejected adapter was published: %v", err)
	}
	if _, err := os.Stat(filepath.Join(initial.Output, "candidates", "final-test-adapter-f32.gguf")); err != nil {
		t.Fatalf("rejected candidate evidence is missing: %v", err)
	}
}

func TestBackendRetriesOnceWithPreparedSparesAfterTargetSubstitution(t *testing.T) {
	dataset := writePreparedDataset(t, 120)
	writePreparedFactRange(t, filepath.Join(filepath.Dir(dataset), "dataset-spares.jsonl"), 30, 3)
	output := t.TempDir()
	trainCalls := 0
	evaluationCalls := 0
	backend := &Backend{
		train: func(_ context.Context, request finetune.Request, contract finetune.DatasetContract, _ finetune.ProgressFunc) (trainingOutput, error) {
			trainCalls++
			candidate := filepath.Join(request.Output, "candidates", "adapter-f32.gguf")
			if err := os.MkdirAll(filepath.Dir(candidate), 0o700); err != nil {
				return trainingOutput{}, err
			}
			if err := os.WriteFile(candidate, []byte(fmt.Sprintf("candidate-%d", trainCalls)), 0o600); err != nil {
				return trainingOutput{}, err
			}
			return trainingOutput{Candidate: candidate, SelectedEpoch: trainCalls}, nil
		},
		evaluate: func(_ context.Context, _ finetune.Request, contract finetune.DatasetContract, _ string, _ finetune.ProgressFunc) (finetune.EvaluationReport, error) {
			evaluationCalls++
			if evaluationCalls == 1 {
				return finetune.EvaluationReport{
					Schema: finetune.EvaluationReportSchema,
					Decision: finetune.ReliabilityDecision{
						RejectionReasons: []string{"protected answers substituted another trained target"},
					},
					SubstitutedFactIDs: []string{"fact-000", "fact-001", "fact-002"},
				}, nil
			}
			facts := make(map[string]bool)
			for _, record := range contract.Train {
				facts[record.FactID] = true
			}
			for _, removed := range []string{"fact-000", "fact-001", "fact-002"} {
				if facts[removed] {
					t.Fatalf("failed fact %q reached recovery training", removed)
				}
			}
			return finetune.EvaluationReport{
				Schema: finetune.EvaluationReportSchema, Decision: finetune.ReliabilityDecision{Passed: true},
			}, nil
		},
	}
	result, err := backend.Run(t.Context(), finetune.Request{Dataset: dataset, Model: "gemma.gguf", Output: output}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != finetune.StatusCompleted || trainCalls != 2 || evaluationCalls != 2 {
		t.Fatalf("recovery result = %+v, train/evaluation calls = %d/%d", result, trainCalls, evaluationCalls)
	}
	if stringData, err := os.ReadFile(result.Adapter); err != nil || string(stringData) != "candidate-2" {
		t.Fatalf("published recovery adapter = %q, error %v", string(stringData), err)
	}
	contract, err := finetune.ValidateDatasetContract(filepath.Join(output, "recovery-dataset.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if contract.Report.SourceRecords != 180 {
		t.Fatalf("recovery source records = %d, want 180", contract.Report.SourceRecords)
	}
}

func TestTrainingStateOnlyResumesTheSameValidatedRun(t *testing.T) {
	dataset := writePreparedDataset(t, 120)
	contract, err := finetune.ValidateDatasetContract(dataset)
	if err != nil {
		t.Fatal(err)
	}
	output := t.TempDir()
	mainCheckpoint := filepath.Join(output, "checkpoints", "epoch-02")
	if err := os.MkdirAll(mainCheckpoint, 0o700); err != nil {
		t.Fatal(err)
	}
	candidate := filepath.Join(output, "candidates", "final-test-adapter-f32.gguf")
	if err := os.MkdirAll(filepath.Dir(candidate), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(candidate, []byte("candidate"), 0o600); err != nil {
		t.Fatal(err)
	}
	state := trainingState{
		Schema: trainingStateSchema, Model: "gemma.gguf", Dataset: contract.Report,
		Training: gemma.TrainingResult{
			BestEpoch: 2, BestValidationLoss: 0.25, BestCheckpoint: mainCheckpoint,
			Epochs: []gemma.EpochMetrics{{Epoch: 2, ValidationLoss: 0.25, Checkpoint: mainCheckpoint}},
		},
		Selection: &checkpointSelection{
			Epoch: 2, Checkpoint: mainCheckpoint, ValidationLoss: 0.25, GeneratedPassRate: 0.8,
		},
		Candidate: candidate,
		Evaluation: &finetune.EvaluationReport{
			Schema: "finetune-evaluation.v1",
		},
	}
	if err := writeTrainingState(output, state); err != nil {
		t.Fatal(err)
	}
	loaded, found, err := readTrainingState(output)
	if err != nil || !found {
		t.Fatalf("readTrainingState() = found %v, error %v", found, err)
	}
	request := finetune.Request{Dataset: dataset, Model: "gemma.gguf", Output: output}
	if err := validateTrainingState(request, contract, loaded); err != nil {
		t.Fatalf("validateTrainingState() error = %v", err)
	}
	trained := trainingOutputFromState(loaded)
	if trained.SelectedEpoch != state.Training.BestEpoch || trained.SelectedCheckpoint != mainCheckpoint || trained.Candidate != candidate {
		t.Fatalf("resumed training output = %+v", trained)
	}
	if trained.Evaluation != nil {
		t.Fatal("stale protected evaluation was reused")
	}
	loaded.Evaluation.Schema = finetune.EvaluationReportSchema
	if current := trainingOutputFromState(loaded); current.Evaluation == nil {
		t.Fatal("current protected evaluation was not reused")
	}
	request.Model = "different.gguf"
	if err := validateTrainingState(request, contract, loaded); err == nil || !strings.Contains(err.Error(), "different base model") {
		t.Fatalf("mismatched model error = %v", err)
	}
	loaded.Training.BestValidationLoss = 0.1
	request.Model = "gemma.gguf"
	if err := validateTrainingState(request, contract, loaded); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("mismatched validation selection error = %v", err)
	}
}

func TestTrainingReportCanBeSafelyUpdatedAfterResume(t *testing.T) {
	path := filepath.Join(t.TempDir(), "training-report.json")
	first := trainingOutput{SelectedEpoch: 1, BestValidationLoss: 0.5}
	if err := writeTrainingReport(path, "gemma.gguf", finetune.DatasetContractReport{}, first); err != nil {
		t.Fatal(err)
	}
	second := trainingOutput{SelectedEpoch: 2, BestValidationLoss: 0.25}
	if err := writeTrainingReport(path, "gemma.gguf", finetune.DatasetContractReport{}, second); err != nil {
		t.Fatal(err)
	}
	var report trainingReport
	readJSONFile(t, path, &report)
	if report.SelectedEpoch != 2 || report.BestValidationLoss != 0.25 {
		t.Fatalf("updated training report = %+v", report)
	}
}

func TestGeneratedCheckpointSelectionPrioritizesGeneratedAnswers(t *testing.T) {
	selected := checkpointSelection{Epoch: 20, ValidationLoss: 0.7, GeneratedPassRate: 0.6}
	betterAnswers := checkpointSelection{Epoch: 10, ValidationLoss: 1.2, GeneratedPassRate: 0.8}
	if !generatedCheckpointIsBetter(betterAnswers, selected) {
		t.Fatal("higher generated-answer passing rate did not win")
	}
	betterLoss := checkpointSelection{Epoch: 15, ValidationLoss: 0.8, GeneratedPassRate: 0.6}
	if !generatedCheckpointIsBetter(betterLoss, checkpointSelection{ValidationLoss: 1.0, GeneratedPassRate: 0.6}) {
		t.Fatal("validation loss did not break an equal generated-answer score")
	}
	if generatedCheckpointIsBetter(checkpointSelection{ValidationLoss: 0.5, GeneratedPassRate: 0.4}, selected) {
		t.Fatal("lower generated-answer passing rate won because of validation loss")
	}
}

func TestBackendPreflightChecksModelArchitectureAndInputs(t *testing.T) {
	dataset := writePreparedDataset(t, 120)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	writeModel := func(t *testing.T, architecture string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "model.gguf")
		var writer gguf.Writer
		writer.String("general.architecture", architecture)
		if err := writer.WriteFile(path); err != nil {
			t.Fatal(err)
		}
		return path
	}
	tests := []struct {
		name    string
		request finetune.Request
		want    string
	}{
		{"valid Gemma", finetune.Request{Dataset: dataset, Model: writeModel(t, "gemma3"), Runtime: finetune.Runtime{LlamaServer: executable}}, ""},
		{"wrong architecture", finetune.Request{Dataset: dataset, Model: writeModel(t, "qwen3"), Runtime: finetune.Runtime{LlamaServer: executable}}, "want gemma3"},
		{"missing executable", finetune.Request{Dataset: dataset, Model: writeModel(t, "gemma3"), Runtime: finetune.Runtime{LlamaServer: filepath.Join(t.TempDir(), "missing")}}, "find llama-server"},
		{"invalid port", finetune.Request{Dataset: dataset, Model: writeModel(t, "gemma3"), Runtime: finetune.Runtime{LlamaServer: executable, Port: 65536}}, "port"},
	}
	backend := New()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := backend.Preflight(t.Context(), test.request)
			if test.want == "" && err != nil {
				t.Fatal(err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("Preflight() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestBackendReturnsStoppedWhenAlreadyCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result, err := (&Backend{train: func(context.Context, finetune.Request, finetune.DatasetContract, finetune.ProgressFunc) (trainingOutput, error) {
		t.Fatal("training called after cancellation")
		return trainingOutput{}, nil
	}, evaluate: func(context.Context, finetune.Request, finetune.DatasetContract, string, finetune.ProgressFunc) (finetune.EvaluationReport, error) {
		t.Fatal("evaluation called after cancellation")
		return finetune.EvaluationReport{}, nil
	}}).Run(ctx, finetune.Request{}, nil)
	if err == nil || result.Status != finetune.StatusStopped {
		t.Fatalf("Run() result = %+v, error = %v", result, err)
	}
}

func TestLlamaServerHealthAndBoundedLogs(t *testing.T) {
	status := http.StatusServiceUnavailable
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(status)
	}))
	defer server.Close()
	client := &http.Client{Timeout: time.Second}
	if ready, err := llamaServerReady(t.Context(), client, server.URL); err != nil || ready {
		t.Fatalf("unavailable health ready = %v, error = %v", ready, err)
	}
	status = http.StatusOK
	if ready, err := llamaServerReady(t.Context(), client, server.URL); err != nil || !ready {
		t.Fatalf("healthy ready = %v, error = %v", ready, err)
	}

	logs := &limitedBuffer{maximum: 5}
	if written, err := logs.Write([]byte("123456789")); err != nil || written != 9 {
		t.Fatalf("Write() = %d, %v", written, err)
	}
	if got := logs.String(); got != "12345" {
		t.Fatalf("bounded logs = %q", got)
	}
}

func TestCompleteChatUsesDeterministicNonStreamingRequest(t *testing.T) {
	messages := []chatMessage{
		{Role: "user", Content: "Earlier question?"},
		{Role: "assistant", Content: "Earlier answer."},
		{Role: "user", Content: "Question?"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/chat/completions" {
			http.Error(response, "unexpected request", http.StatusBadRequest)
			return
		}
		var payload struct {
			Messages    []chatMessage `json:"messages"`
			Temperature float64       `json:"temperature"`
			TopK        int           `json:"top_k"`
			TopP        float64       `json:"top_p"`
			Seed        int64         `json:"seed"`
			Stream      bool          `json:"stream"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		if !slices.Equal(payload.Messages, messages) || payload.Temperature != 0 || payload.TopK != 1 || payload.TopP != 1 || payload.Seed != finetune.DatasetSplitSeed || payload.Stream {
			http.Error(response, "completion request is not deterministic", http.StatusBadRequest)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"  Answer.  "}}]}`))
	}))
	defer server.Close()
	answer, err := completeChat(t.Context(), server.Client(), server.URL, messages, deterministicEvaluationSettings)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "Answer." {
		t.Fatalf("answer = %q", answer)
	}
}

func TestConversationEvaluationUsesKronkChatSampling(t *testing.T) {
	want := chatSettings{Temperature: .8, TopK: 40, TopP: .9, Seed: finetune.DatasetSplitSeed}
	if kronkChatSettings != want {
		t.Fatalf("conversation settings = %+v, want %+v", kronkChatSettings, want)
	}
}

func TestProtectedEvaluationKeepsActualAnswersInOneChatSession(t *testing.T) {
	wantMessages := [][]chatMessage{
		{{Role: "user", Content: "First question?"}},
		{
			{Role: "user", Content: "First question?"},
			{Role: "assistant", Content: "First answer."},
			{Role: "user", Content: "Second question?"},
		},
	}
	requestIndex := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		var payload struct {
			Messages []chatMessage `json:"messages"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		if requestIndex >= len(wantMessages) || !slices.Equal(payload.Messages, wantMessages[requestIndex]) {
			http.Error(response, "evaluation did not preserve the chat session", http.StatusBadRequest)
			return
		}
		answer := "First answer."
		if requestIndex == 1 {
			answer = "Second answer."
		}
		requestIndex++
		response.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(response, `{"choices":[{"message":{"role":"assistant","content":%q}}]}`, answer)
	}))
	defer server.Close()

	records := []finetune.DatasetRecord{
		{Prompt: "First question?", Target: "First answer."},
		{Prompt: "Second question?", Target: "Second answer."},
	}
	results, err := evaluateChatSession(t.Context(), server.Client(), server.URL, records, evaluationSessionTurns, kronkChatSettings, 0, len(records), nil)
	if err != nil {
		t.Fatal(err)
	}
	if requestIndex != 2 || len(results) != 2 || !results[0].ExactMatch || !results[1].ExactMatch {
		t.Fatalf("session evaluation = requests %d, results %+v", requestIndex, results)
	}
}

func TestProtectedEvaluationStartsANewBoundedSession(t *testing.T) {
	var messageCounts []int
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		var payload struct {
			Messages []chatMessage `json:"messages"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		messageCounts = append(messageCounts, len(payload.Messages))
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"answer"}}]}`))
	}))
	defer server.Close()
	records := make([]finetune.DatasetRecord, evaluationSessionTurns+1)
	for index := range records {
		records[index] = finetune.DatasetRecord{Prompt: fmt.Sprintf("Question %d?", index+1), Target: "answer"}
	}
	if _, err := evaluateChatSession(t.Context(), server.Client(), server.URL, records, evaluationSessionTurns, kronkChatSettings, 0, len(records), nil); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(messageCounts, []int{1, 3, 5, 7, 1}) {
		t.Fatalf("message counts = %v", messageCounts)
	}
}

func TestIndependentEvaluationDoesNotCarryConversationHistory(t *testing.T) {
	var messageCounts []int
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		var payload struct {
			Messages []chatMessage `json:"messages"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		messageCounts = append(messageCounts, len(payload.Messages))
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"answer"}}]}`))
	}))
	defer server.Close()
	records := []finetune.DatasetRecord{
		{Prompt: "First question?", Target: "answer"},
		{Prompt: "Unrelated question?", Target: "answer"},
	}
	if _, err := evaluateChatSession(t.Context(), server.Client(), server.URL, records, 1, deterministicEvaluationSettings, 0, len(records), nil); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(messageCounts, []int{1, 1}) {
		t.Fatalf("independent message counts = %v", messageCounts)
	}
}

func TestStabilityRecordsUseDifferentFacts(t *testing.T) {
	records := []finetune.DatasetRecord{
		{Prompt: "First phrasing?", FactID: "one"},
		{Prompt: "Second phrasing?", FactID: "one"},
		{Prompt: "Another fact?", FactID: "two"},
	}
	selected := selectStabilityRecords(records, 2)
	if len(selected) != 2 || selected[0].FactID != "one" || selected[1].FactID != "two" {
		t.Fatalf("stability records = %+v", selected)
	}
}

func TestStabilityRecordsRepeatUniqueFactsInDeterministicOrders(t *testing.T) {
	records := make([]finetune.DatasetRecord, 20)
	for index := range records {
		records[index] = finetune.DatasetRecord{Prompt: fmt.Sprintf("Question %d?", index+1), FactID: fmt.Sprintf("fact-%d", index+1)}
	}
	first := selectStabilityRecords(records, evaluationStabilityRecords)
	second := selectStabilityRecords(records, evaluationStabilityRecords)
	if !slices.Equal(first, second) {
		t.Fatal("stability record order changed between identical selections")
	}
	if len(first) != evaluationStabilityRecords {
		t.Fatalf("stability records = %d, want %d", len(first), evaluationStabilityRecords)
	}
	counts := make(map[string]int, len(records))
	for _, record := range first {
		counts[record.FactID]++
	}
	for _, record := range records {
		if counts[record.FactID] != evaluationStabilityRecords/len(records) {
			t.Fatalf("fact %q evaluated %d times", record.FactID, counts[record.FactID])
		}
	}
}

func TestSubstitutedFactIDsFindsAnotherTrainedTarget(t *testing.T) {
	contract := finetune.DatasetContract{
		Train: []finetune.DatasetRecord{
			{FactID: "slice", Target: "A slice is a descriptor of an array segment."},
			{FactID: "interface", Target: "An interface defines a set of methods."},
			{FactID: "waitgroup", Target: "WaitGroup waits for goroutines to finish."},
		},
		Test: []finetune.DatasetRecord{
			{FactID: "slice", Target: "A slice is a descriptor of an array segment."},
			{FactID: "interface", Target: "An interface defines a set of methods."},
			{FactID: "waitgroup", Target: "WaitGroup waits for goroutines to finish."},
		},
	}
	results := []finetune.EvaluationResult{
		finetune.ScoreAnswer(contract.Test[0], contract.Train[1].Target),
		finetune.ScoreAnswer(contract.Test[1], contract.Train[1].Target),
		finetune.ScoreAnswer(contract.Test[2], contract.Train[0].Target),
	}
	if got := substitutedFactIDs(contract, results); !slices.Equal(got, []string{"slice", "waitgroup"}) {
		t.Fatalf("substituted facts = %v", got)
	}
}

func TestRealGemmaBackendSmoke(t *testing.T) {
	modelPath := os.Getenv("MOXIE_TEST_GEMMA_MODEL")
	if modelPath == "" {
		t.Skip("set MOXIE_TEST_GEMMA_MODEL to run the real Gemma training smoke test")
	}
	dataset := writePreparedDataset(t, 120)
	backend := New()
	manager, err := finetune.NewManager(t.TempDir(), backend)
	if err != nil {
		t.Fatal(err)
	}
	updates := make(chan finetune.RunInfo, 64)
	initial, err := manager.Start(t.Context(), finetune.Request{
		Dataset: dataset,
		Model:   modelPath,
		Runtime: finetune.Runtime{LlamaServer: "llama-server", ContextSize: 512},
	}, func(info finetune.RunInfo) {
		updates <- info
	})
	if err != nil {
		t.Fatal(err)
	}
	var completed finetune.RunInfo
	timer := time.NewTimer(10 * time.Minute)
	defer timer.Stop()
	for completed.Status != finetune.StatusCompleted {
		select {
		case completed = <-updates:
			if completed.Status == finetune.StatusFailed || completed.Status == finetune.StatusStopped {
				t.Fatalf("real Gemma training ended with status %q: %s", completed.Status, completed.Error)
			}
		case <-timer.C:
			t.Fatal("real Gemma training did not finish within ten minutes")
		}
	}
	if completed.SelectedCheckpoint == "" || completed.Report == "" {
		t.Fatalf("real Gemma outputs = %+v", completed)
	}
	var report trainingReport
	readJSONFile(t, completed.Report, &report)
	if report.Evaluation == nil || !report.Evaluation.Decision.Passed || report.Evaluation.Dataset.TestRecords != finetune.MinimumTestRecords {
		t.Fatalf("real protected evaluation = %+v", report.Evaluation)
	}
	for _, name := range []string{"base.jsonl", "adapter.jsonl", "stability.jsonl"} {
		if _, err := os.Stat(filepath.Join(initial.Output, "evaluation", "final", name)); err != nil {
			t.Fatalf("protected evaluation evidence %q: %v", name, err)
		}
	}
	reader, err := gguf.OpenReader(completed.Adapter)
	if err != nil {
		t.Fatal(err)
	}
	artifactType, err := reader.String("general.type")
	if err != nil {
		_ = reader.Close()
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if artifactType != "adapter" {
		t.Fatalf("GGUF general.type = %q, want adapter", artifactType)
	}
	verifyLlamaServerLoadsAdapter(t, modelPath, completed.Adapter)
}

func verifyLlamaServerLoadsAdapter(t *testing.T, modelPath, adapterPath string) {
	t.Helper()
	executable, err := resolveLlamaServer("llama-server")
	if err != nil {
		t.Fatal(err)
	}
	port, err := availablePort()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	command := exec.CommandContext(ctx, executable,
		"-m", modelPath,
		"--lora", adapterPath,
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(port),
		"--ctx-size", "512",
		"--n-gpu-layers", "0",
	)
	logs := &limitedBuffer{maximum: maximumServerLogBytes}
	command.Stdout = logs
	command.Stderr = logs
	if err := command.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	process := newProcessWait(command)
	defer func() {
		cancel()
		process.wait()
	}()
	endpoint := "http://127.0.0.1:" + strconv.Itoa(port)
	client := &http.Client{Timeout: time.Second}
	timer := time.NewTimer(time.Minute)
	defer timer.Stop()
	for {
		ready, _ := llamaServerReady(t.Context(), client, endpoint)
		if ready {
			return
		}
		select {
		case <-process.done:
			t.Fatalf("llama-server rejected exported adapter: %v", processExitError(process.err(), logs.String()))
		case <-timer.C:
			t.Fatalf("llama-server did not load exported adapter: %s", logs.String())
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func writePreparedDataset(t *testing.T, count int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "prepared.jsonl")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	encoder := json.NewEncoder(file)
	for fact := range count / 4 {
		var record finetune.DatasetRecord
		for variant := range 5 {
			record = finetune.DatasetRecord{
				Prompt:   fmt.Sprintf("Question variant %d for fact %03d?", variant, fact),
				Target:   fmt.Sprintf("Detailed answer %03d with enough words to represent generated output.", fact),
				Evidence: fmt.Sprintf("Source passage %03d supports this answer and preserves provenance.", fact),
				FactID:   fmt.Sprintf("fact-%03d", fact),
			}
			if err := encoder.Encode(record); err != nil {
				_ = file.Close()
				t.Fatal(err)
			}
		}
		record.Prompt = fmt.Sprintf("Independent evaluation question for fact %03d?", fact)
		record.IndependentPrompt = true
		if err := encoder.Encode(record); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func writePreparedFactRange(t *testing.T, path string, first, count int) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	encoder := json.NewEncoder(file)
	for fact := first; fact < first+count; fact++ {
		for variant := range 6 {
			record := finetune.DatasetRecord{
				Prompt:            fmt.Sprintf("Question variant %d for fact %03d?", variant, fact),
				Target:            fmt.Sprintf("Detailed answer %03d with enough words to represent generated output.", fact),
				Evidence:          fmt.Sprintf("Source passage %03d supports this answer and preserves provenance.", fact),
				FactID:            fmt.Sprintf("fact-%03d", fact),
				IndependentPrompt: variant == 5,
			}
			if err := encoder.Encode(record); err != nil {
				_ = file.Close()
				t.Fatal(err)
			}
		}
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func waitForTerminalRun(t *testing.T, updates <-chan finetune.RunInfo) finetune.RunInfo {
	t.Helper()
	for {
		select {
		case info := <-updates:
			switch info.Status {
			case finetune.StatusCompleted, finetune.StatusNeedsImprovement, finetune.StatusFailed, finetune.StatusStopped:
				return info
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for managed training run")
		}
	}
}

func readJSONFile(t *testing.T, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatal(err)
	}
}
