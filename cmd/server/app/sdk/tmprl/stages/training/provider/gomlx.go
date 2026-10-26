// Package provider adapts GoMLX and worker-local llama-server computation to
// tuning's trainer contract. It does not orchestrate stages.
package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/gomlx/engine"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/shared/model"
)

// goMLXTrainer uses the existing in-process Gemma 3 runtime, not the legacy
// application's business models. llama-server supplies the exact tokenizer and
// generates protected answers from the frozen base or exported adapter.
type goMLXTrainer struct{ executable string }

// NewGoMLX returns the built-in training provider.
func NewGoMLX(executable string) model.Trainer { return goMLXTrainer{executable: executable} }

// Preflight validates all splits only for the built-in provider.
func Preflight(ctx context.Context, trainer model.Trainer, job model.TrainingJob) error {
	t, ok := trainer.(goMLXTrainer)
	if !ok {
		return nil
	}
	endpoint, stop, err := t.server(ctx, job, "")
	if err != nil {
		return err
	}
	defer stop()
	return t.validateContext(ctx, endpoint, job.Dataset)
}

const predictionTokens = 192

func (t goMLXTrainer) Train(ctx context.Context, job model.TrainingJob, publish func(model.Progress)) (model.TrainingCandidate, error) {
	runtime, err := engine.NewLoRARuntime(job.Model, engine.AdapterConfig{Rank: job.Training.LoRARank, Alpha: job.Training.LoRAAlpha, AdapterScale: 1, Seed: job.Training.Seed})
	if err != nil {
		return model.TrainingCandidate{}, err
	}
	defer runtime.Close()
	endpoint, stop, err := t.server(ctx, job, "")
	if err != nil {
		return model.TrainingCandidate{}, err
	}
	defer stop()
	tokenizer, err := engine.New(endpoint, &http.Client{Timeout: 2 * time.Minute})
	if err != nil {
		return model.TrainingCandidate{}, err
	}
	dataset := engine.Dataset{}
	for _, r := range job.Dataset.Train {
		dataset.Train = append(dataset.Train, engine.Record{Prompt: r.Prompt, Target: r.Target})
	}
	for _, r := range job.Dataset.Validation {
		dataset.Validation = append(dataset.Validation, engine.Record{Prompt: r.Prompt, Target: r.Target})
	}
	prepared, err := runtime.PrepareDataset(ctx, dataset, tokenizer, func(p engine.TokenizationProgress) {
		publish(model.Progress{Stage: model.StageTrain, Completed: p.Completed, Total: p.Total, Message: "Tokenizing " + p.Split})
	})
	if err != nil {
		return model.TrainingCandidate{}, err
	}
	stop() // Free the inference model before allocating training graphs.
	result, err := runtime.TrainDataset(ctx, prepared, engine.TrainingConfig{
		Epochs: job.Training.MaximumEpochs, BatchSize: job.Training.BatchSize,
		LearningRate: job.Training.LearningRate, Seed: uint64(job.Training.Seed),
		EarlyStoppingPatience: 3, MinimumValidationImprovement: 0.0001,
		AdapterCheckpointDirectory: filepath.Join(job.Workdir, "epochs"),
	}, func(p engine.TrainingProgress) {
		publish(model.Progress{Stage: model.StageTrain, Completed: p.CompletedSteps, Total: p.TotalSteps, Message: fmt.Sprintf("Training epoch %d", p.Epoch)})
	})
	if err != nil {
		return model.TrainingCandidate{}, err
	}
	if err := ctx.Err(); err != nil {
		return model.TrainingCandidate{}, err
	}
	path := filepath.Join(job.Workdir, "candidate.gguf")
	if err := runtime.ExportBestAdapter(result, path); err != nil {
		return model.TrainingCandidate{}, err
	}
	candidate := model.TrainingCandidate{Path: path, BestEpoch: result.BestEpoch, ValidationLoss: result.BestValidationLoss}
	for _, metrics := range result.Epochs {
		if metrics.Epoch == result.BestEpoch {
			candidate.TrainingLoss = metrics.TrainingLoss
		}
	}
	return candidate, nil
}

func (t goMLXTrainer) Predict(ctx context.Context, job model.TrainingJob, adapter string, records []model.Record) ([]string, error) {
	endpoint, stop, err := t.server(ctx, job, adapter)
	if err != nil {
		return nil, err
	}
	defer stop()
	if err := t.validateContext(ctx, endpoint, model.TrainingDataset{Test: records}); err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	answers := make([]string, 0, len(records))
	for _, record := range records {
		// Targets/evidence are never sent to inference. Each protected question
		// gets an independent conversation under the same deterministic settings.
		var output struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}
		payload := map[string]any{"messages": []map[string]string{{"role": "user", "content": record.Prompt}}, "temperature": 0, "seed": job.Training.Seed, "max_tokens": predictionTokens, "stream": false}
		if err := inferenceJSON(ctx, client, endpoint+"/v1/chat/completions", payload, &output); err != nil {
			return nil, err
		}
		if len(output.Choices) != 1 || output.Choices[0].FinishReason != "stop" {
			return nil, fmt.Errorf("protected prediction is missing or truncated")
		}
		answers = append(answers, output.Choices[0].Message.Content)
	}
	return answers, nil
}

// validateContext checks the actual chat template and tokenizer, without
// generating answers or allowing protected targets into optimization.
func (t goMLXTrainer) validateContext(ctx context.Context, endpoint string, dataset model.TrainingDataset) error {
	client := &http.Client{Timeout: 2 * time.Minute}
	var props struct {
		BOS      string `json:"bos_token"`
		Settings struct {
			Context int `json:"n_ctx"`
		} `json:"default_generation_settings"`
	}
	if err := inferenceJSON(ctx, client, endpoint+"/props", nil, &props); err != nil {
		return err
	}
	if props.Settings.Context < 1 || props.BOS == "" {
		return fmt.Errorf("inference server returned invalid context or BOS metadata")
	}
	tokenizer, err := engine.New(endpoint, client)
	if err != nil {
		return err
	}
	for _, split := range []struct {
		name      string
		rows      []model.Record
		protected bool
	}{
		{"training", dataset.Train, false}, {"validation", dataset.Validation, false}, {"protected test", dataset.Test, true},
	} {
		for i, record := range split.rows {
			var formatted struct {
				Prompt string `json:"prompt"`
			}
			content := record.Prompt
			if !split.protected {
				content = strings.TrimSpace(content) // Match GoMLX's training formatting.
			}
			payload := map[string]any{"messages": []map[string]string{{"role": "user", "content": content}}}
			if err := inferenceJSON(ctx, client, endpoint+"/apply-template", payload, &formatted); err != nil {
				return err
			}
			if formatted.Prompt == "" {
				return fmt.Errorf("inference server returned an empty chat template")
			}
			prompt := formatted.Prompt
			if !strings.HasPrefix(prompt, props.BOS) {
				prompt = props.BOS + prompt
			}
			reserve := predictionTokens
			if !split.protected {
				// Match the complete Gemma training turn; targets are sent only
				// to tokenization, never to protected-answer inference.
				prompt += strings.TrimSpace(record.Target) + "<end_of_turn>"
				reserve = -1 // Training uses next-token pairs, excluding the final token.
			}
			tokens, err := tokenizer.Tokenize(ctx, prompt)
			if err != nil {
				return err
			}
			if len(tokens)+reserve > props.Settings.Context {
				return fmt.Errorf("%w: %s record %d needs %d tokens including answer space; model context is %d", model.ErrInvalidArgument, split.name, i+1, len(tokens)+reserve, props.Settings.Context)
			}
		}
	}
	return nil
}

func inferenceJSON(ctx context.Context, client *http.Client, endpoint string, input any, output any) error {
	method := http.MethodGet
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body, method = bytes.NewReader(data), http.MethodPost
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("inference request failed: HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil {
		return err
	}
	if len(data) > 1<<20 {
		return fmt.Errorf("inference response exceeds limit")
	}
	return json.Unmarshal(data, output)
}

// server is worker-local: loopback-only, bounded startup, no unbounded logs and
// cooperative process cancellation. Every exit is reaped, including failures.
func (t goMLXTrainer) server(ctx context.Context, job model.TrainingJob, adapter string) (string, func(), error) {
	name := t.executable
	if name == "" {
		name = "llama-server"
	}
	executable, err := exec.LookPath(name)
	if err != nil {
		return "", nil, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	if err := listener.Close(); err != nil {
		return "", nil, err
	}
	arguments := []string{"-m", job.Model, "--host", "127.0.0.1", "--port", port, "--ctx-size", "0", "--parallel", "1", "--no-context-shift", "--n-gpu-layers", "0", "--log-verbosity", "0"}
	if adapter != "" {
		arguments = append(arguments, "--lora", adapter)
	}
	processCtx, cancel := context.WithCancel(ctx)
	command := exec.CommandContext(processCtx, executable, arguments...)
	var diagnostics serverErrorOutput
	command.Stdout, command.Stderr = io.Discard, &diagnostics
	if err := command.Start(); err != nil {
		cancel()
		return "", nil, err
	}
	done := make(chan struct{})
	var waitErr error
	go func() { waitErr = command.Wait(); close(done) }()
	stop := func() { cancel(); <-done }
	endpoint := "http://127.0.0.1:" + port
	startup, finish := context.WithTimeout(ctx, time.Minute)
	defer finish()
	healthClient := &http.Client{Timeout: time.Second}
	for {
		select {
		case <-done:
			cancel()
			return "", nil, fmt.Errorf("llama-server exited during startup: %v: %s", waitErr, diagnostics.data)
		case <-startup.Done():
			stop()
			return "", nil, fmt.Errorf("llama-server startup: %w: %s", startup.Err(), diagnostics.data)
		default:
		}
		request, err := http.NewRequestWithContext(startup, http.MethodGet, endpoint+"/health", nil)
		if err != nil {
			stop()
			return "", nil, err
		}
		response, err := healthClient.Do(request)
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return endpoint, stop, nil
			}
		}
		select {
		case <-startup.Done():
			stop()
			return "", nil, fmt.Errorf("llama-server startup: %w: %s", startup.Err(), diagnostics.data)
		case <-done:
			cancel()
			return "", nil, fmt.Errorf("llama-server exited during startup: %v: %s", waitErr, diagnostics.data)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// Keep only the last 4 KiB of stderr. exec writes it from one goroutine; callers
// read it only after Wait has finished, so no mutex or logging service is needed.
type serverErrorOutput struct{ data []byte }

func (o *serverErrorOutput) Write(p []byte) (int, error) {
	n := len(p)
	if len(p) >= 4096 {
		o.data = append(o.data[:0], p[len(p)-4096:]...)
		return n, nil
	}
	if excess := len(o.data) + len(p) - 4096; excess > 0 {
		o.data = append(o.data[:0], o.data[excess:]...)
	}
	o.data = append(o.data, p...)
	return n, nil
}

var _ model.Trainer = goMLXTrainer{}
