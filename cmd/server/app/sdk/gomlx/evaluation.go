package gomlx

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/fineapp/finetune"
)

const (
	maximumEvaluationResponseBytes = 16 << 20
	minimumEvaluationContextSize   = 8192
	evaluationSessionTurns         = 4
	evaluationStabilityRecords     = 100
)

type chatSettings struct {
	Temperature float64
	TopK        int
	TopP        float64
	Seed        int64
}

var (
	deterministicEvaluationSettings = chatSettings{TopK: 1, TopP: 1, Seed: finetune.DatasetSplitSeed}
	kronkChatSettings               = chatSettings{Temperature: .8, TopK: 40, TopP: .9, Seed: finetune.DatasetSplitSeed}
)

type evaluateFunc func(context.Context, finetune.Request, finetune.DatasetContract, string, finetune.ProgressFunc) (finetune.EvaluationReport, error)

func evaluateProtectedTest(ctx context.Context, request finetune.Request, contract finetune.DatasetContract, candidate string, progress finetune.ProgressFunc) (finetune.EvaluationReport, error) {
	if len(contract.Test) != contract.Report.TestRecords {
		return finetune.EvaluationReport{}, fmt.Errorf("protected test has %d records; contract requires %d", len(contract.Test), contract.Report.TestRecords)
	}
	evaluationDirectory := filepath.Join(request.Output, "evaluation")
	if err := os.Mkdir(evaluationDirectory, 0o700); err != nil {
		return finetune.EvaluationReport{}, fmt.Errorf("create evaluation directory: %w", err)
	}
	finalDirectory := filepath.Join(evaluationDirectory, "final")
	if err := os.RemoveAll(finalDirectory); err != nil {
		return finetune.EvaluationReport{}, fmt.Errorf("clear incomplete protected evaluation: %w", err)
	}
	if err := os.Mkdir(finalDirectory, 0o700); err != nil {
		return finetune.EvaluationReport{}, fmt.Errorf("create protected evaluation directory: %w", err)
	}
	stabilityRecords := selectStabilityRecords(contract.Test, evaluationStabilityRecords)
	total := len(contract.Test)*2 + len(stabilityRecords)
	reportProgress(progress, finetune.Progress{Stage: finetune.StageFinalEvaluation, Total: total, Message: "Evaluating frozen base model"})
	base, _, err := evaluateModel(ctx, request, contract.Test, nil, "", 0, total, progress)
	if err != nil {
		return finetune.EvaluationReport{}, fmt.Errorf("evaluate frozen base model: %w", err)
	}
	if err := writeEvaluationResults(filepath.Join(finalDirectory, "base.jsonl"), base); err != nil {
		return finetune.EvaluationReport{}, err
	}
	reportProgress(progress, finetune.Progress{
		Stage: finetune.StageFinalEvaluation, Completed: len(contract.Test), Total: total,
		Message: "Evaluating selected adapter",
	})
	adapted, stability, err := evaluateModel(ctx, request, contract.Test, stabilityRecords, candidate, len(contract.Test), total, progress)
	if err != nil {
		return finetune.EvaluationReport{}, fmt.Errorf("evaluate selected adapter: %w", err)
	}
	if err := writeEvaluationResults(filepath.Join(finalDirectory, "adapter.jsonl"), adapted); err != nil {
		return finetune.EvaluationReport{}, err
	}
	if err := writeEvaluationResults(filepath.Join(finalDirectory, "stability.jsonl"), stability); err != nil {
		return finetune.EvaluationReport{}, err
	}
	report, err := finetune.BuildEvaluationReport(contract.Report, base, adapted, stability)
	if err != nil {
		return report, err
	}
	if !report.Decision.Passed {
		report.SubstitutedFactIDs = substitutedFactIDs(contract, adapted)
	}
	message := "Protected evaluation passed"
	if !report.Decision.Passed {
		message = "Protected evaluation did not pass"
	}
	reportProgress(progress, finetune.Progress{Stage: finetune.StageFinalEvaluation, Completed: total, Total: total, Message: message})
	return report, nil
}

func substitutedFactIDs(contract finetune.DatasetContract, results []finetune.EvaluationResult) []string {
	targets := make(map[string]string)
	for _, records := range [][]finetune.DatasetRecord{contract.Train, contract.Validation, contract.Test} {
		for _, record := range records {
			if record.FactID != "" {
				targets[record.FactID] = record.Target
			}
		}
	}
	seen := make(map[string]bool)
	var substituted []string
	for index, result := range results {
		if index >= len(contract.Test) {
			break
		}
		record := contract.Test[index]
		if record.FactID == "" || seen[record.FactID] {
			continue
		}
		for factID, target := range targets {
			if factID == record.FactID {
				continue
			}
			alternative := finetune.ScoreAnswer(finetune.DatasetRecord{Target: target}, result.Actual)
			if alternative.ExactMatch ||
				alternative.TokenF1 >= .6 && alternative.AnswerCoverage >= .7 && alternative.TokenF1 >= result.TokenF1+.1 {
				seen[record.FactID] = true
				substituted = append(substituted, record.FactID)
				break
			}
		}
	}
	return substituted
}

func evaluateModel(ctx context.Context, request finetune.Request, records, stabilityRecords []finetune.DatasetRecord, adapter string, completed, total int, progress finetune.ProgressFunc) ([]finetune.EvaluationResult, []finetune.EvaluationResult, error) {
	client := &http.Client{Timeout: 2 * time.Minute}
	evaluationRequest := request
	evaluationRequest.Runtime.ContextSize = max(evaluationRequest.Runtime.ContextSize, minimumEvaluationContextSize)
	endpoint, stop, err := startLlamaServer(ctx, evaluationRequest, adapter, client)
	if err != nil {
		return nil, nil, err
	}
	defer stop()
	results, err := evaluateChatSession(ctx, client, endpoint, records, 1, deterministicEvaluationSettings, completed, total, progress)
	if err != nil {
		return nil, nil, err
	}
	var stability []finetune.EvaluationResult
	if len(stabilityRecords) > 0 {
		reportProgress(progress, finetune.Progress{
			Stage: finetune.StageFinalEvaluation, Completed: completed + len(records), Total: total,
			Message: "Checking same-session answer stability",
		})
		stability, err = evaluateChatSession(ctx, client, endpoint, stabilityRecords, evaluationSessionTurns, kronkChatSettings, completed+len(records), total, progress)
		if err != nil {
			return nil, nil, err
		}
	}
	if err := stop(); err != nil {
		return nil, nil, err
	}
	return results, stability, nil
}

func evaluateAdapterRecords(ctx context.Context, request finetune.Request, records []finetune.DatasetRecord, adapter string) ([]finetune.EvaluationResult, error) {
	client := &http.Client{Timeout: 2 * time.Minute}
	evaluationRequest := request
	evaluationRequest.Runtime.ContextSize = max(evaluationRequest.Runtime.ContextSize, minimumEvaluationContextSize)
	endpoint, stop, err := startLlamaServer(ctx, evaluationRequest, adapter, client)
	if err != nil {
		return nil, err
	}
	defer stop()
	results, err := evaluateChatSession(ctx, client, endpoint, records, 1, deterministicEvaluationSettings, 0, len(records), nil)
	if err != nil {
		return nil, err
	}
	if err := stop(); err != nil {
		return nil, err
	}
	return results, nil
}

func selectStabilityRecords(records []finetune.DatasetRecord, limit int) []finetune.DatasetRecord {
	if limit <= 0 {
		return nil
	}
	unique := make([]finetune.DatasetRecord, 0, min(limit, len(records)))
	facts := make(map[string]struct{}, cap(unique))
	for _, record := range records {
		fact := record.FactID
		if fact == "" {
			fact = record.Prompt
		}
		if _, exists := facts[fact]; exists {
			continue
		}
		facts[fact] = struct{}{}
		unique = append(unique, record)
		if len(unique) == limit {
			break
		}
	}
	selected := make([]finetune.DatasetRecord, 0, limit)
	for pass := 0; len(selected) < limit && len(unique) > 0; pass++ {
		ordered := append([]finetune.DatasetRecord(nil), unique...)
		if pass > 0 {
			random := rand.New(rand.NewPCG(uint64(finetune.DatasetSplitSeed), uint64(pass)))
			random.Shuffle(len(ordered), func(first, second int) {
				ordered[first], ordered[second] = ordered[second], ordered[first]
			})
		}
		selected = append(selected, ordered[:min(len(ordered), limit-len(selected))]...)
	}
	return selected
}

func evaluateChatSession(ctx context.Context, client *http.Client, endpoint string, records []finetune.DatasetRecord, sessionTurns int, settings chatSettings, completed, total int, progress finetune.ProgressFunc) ([]finetune.EvaluationResult, error) {
	results := make([]finetune.EvaluationResult, len(records))
	history := make([]chatMessage, 0, sessionTurns*2)
	for index, record := range records {
		if err := context.Cause(ctx); err != nil {
			return nil, err
		}
		if index > 0 && index%sessionTurns == 0 {
			history = history[:0]
		}
		history = append(history, chatMessage{Role: "user", Content: record.Prompt})
		answer, err := completeChat(ctx, client, endpoint, history, settings)
		if err != nil {
			return nil, fmt.Errorf("generate protected answer %d: %w", index+1, err)
		}
		history = append(history, chatMessage{Role: "assistant", Content: answer})
		results[index] = finetune.ScoreAnswer(record, answer)
		reportProgress(progress, finetune.Progress{
			Stage: finetune.StageFinalEvaluation, Completed: completed + index + 1, Total: total,
			Message: fmt.Sprintf("Generated protected answer %d of %d", completed+index+1, total),
		})
	}
	return results, nil
}

func completeChat(ctx context.Context, client *http.Client, endpoint string, messages []chatMessage, settings chatSettings) (string, error) {
	if len(messages) == 0 || messages[len(messages)-1].Role != "user" {
		return "", fmt.Errorf("completion request must end with a user message")
	}
	payload, err := json.Marshal(struct {
		Messages    []chatMessage `json:"messages"`
		MaxTokens   int           `json:"max_tokens"`
		Temperature float64       `json:"temperature"`
		TopK        int           `json:"top_k"`
		TopP        float64       `json:"top_p"`
		Seed        int64         `json:"seed"`
		Stream      bool          `json:"stream"`
	}{
		Messages: messages, MaxTokens: 192,
		Temperature: settings.Temperature, TopK: settings.TopK, TopP: settings.TopP, Seed: settings.Seed,
	})
	if err != nil {
		return "", fmt.Errorf("encode completion request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create completion request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("complete prompt: %w", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maximumEvaluationResponseBytes+1))
	if err != nil {
		return "", fmt.Errorf("read completion response: %w", err)
	}
	if len(data) > maximumEvaluationResponseBytes {
		return "", fmt.Errorf("completion response exceeds %d bytes", maximumEvaluationResponseBytes)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("completion response status %s: %s", response.Status, strings.TrimSpace(string(data)))
	}
	var completion struct {
		Choices []struct {
			Message chatMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &completion); err != nil {
		return "", fmt.Errorf("decode completion response: %w", err)
	}
	if len(completion.Choices) == 0 {
		return "", fmt.Errorf("completion response contains no choices")
	}
	return strings.TrimSpace(completion.Choices[0].Message.Content), nil
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func writeEvaluationResults(path string, results []finetune.EvaluationResult) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create evaluation results %q: %w", path, err)
	}
	written := false
	defer func() {
		_ = file.Close()
		if !written {
			_ = os.Remove(path)
		}
	}()
	buffer := bufio.NewWriter(file)
	encoder := json.NewEncoder(buffer)
	for _, result := range results {
		if err := encoder.Encode(result); err != nil {
			return fmt.Errorf("encode evaluation results %q: %w", path, err)
		}
	}
	if err := buffer.Flush(); err != nil {
		return fmt.Errorf("flush evaluation results %q: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close evaluation results %q: %w", path, err)
	}
	written = true
	return nil
}
