package finetune

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

const EvaluationReportSchema = "finetune-evaluation.v8"

// EvaluationResult is the auditable score for one generated answer.
type EvaluationResult struct {
	Prompt              string  `json:"prompt"`
	Expected            string  `json:"expected"`
	Actual              string  `json:"actual"`
	ExactMatch          bool    `json:"exact_match"`
	TokenPrecision      float64 `json:"token_precision"`
	TokenRecall         float64 `json:"token_recall"`
	TokenF1             float64 `json:"token_f1"`
	ContentTokenF1      float64 `json:"content_token_f1"`
	AnswerCoverage      float64 `json:"answer_coverage"`
	ConflictingAnswer   bool    `json:"conflicting_answer"`
	RepeatedTrigramRate float64 `json:"repeated_trigram_rate"`
}

// EvaluationSummary aggregates one model's protected-test answers.
type EvaluationSummary struct {
	Records                 int     `json:"records"`
	ExactMatches            int     `json:"exact_matches"`
	RecordsMeetingScore     int     `json:"records_meeting_score"`
	RecordPassingRate       float64 `json:"record_passing_rate"`
	EmptyAnswers            int     `json:"empty_answers"`
	MeanTokenPrecision      float64 `json:"mean_token_precision"`
	MeanTokenRecall         float64 `json:"mean_token_recall"`
	MeanTokenF1             float64 `json:"mean_token_f1"`
	MeanAnswerCoverage      float64 `json:"mean_answer_coverage"`
	MeanRepeatedTrigramRate float64 `json:"mean_repeated_trigram_rate"`
}

// ReliabilityPolicy records Moxie's automatic task-aware publication gate.
// These settings are internal product defaults, not user tuning controls.
type ReliabilityPolicy struct {
	Task                          string  `json:"task"`
	MinimumTestRecords            int     `json:"minimum_test_records"`
	MinimumExactAccuracy          float64 `json:"minimum_exact_accuracy,omitempty"`
	MinimumAnswerCoverage         float64 `json:"minimum_answer_coverage,omitempty"`
	MinimumAnswerTokenF1          float64 `json:"minimum_answer_token_f1,omitempty"`
	MinimumRecordPassingRate      float64 `json:"minimum_record_passing_rate,omitempty"`
	MinimumStabilityRecords       int     `json:"minimum_stability_records"`
	MinimumStabilityPassingRate   float64 `json:"minimum_stability_passing_rate"`
	MinimumImprovement            float64 `json:"minimum_improvement"`
	MaximumEmptyAnswers           int     `json:"maximum_empty_answers"`
	MaximumAnswerRepetitionExcess float64 `json:"maximum_answer_repetition_excess"`
}

// ReliabilityDecision explains whether Moxie may publish the adapter.
type ReliabilityDecision struct {
	BaseScore        float64  `json:"base_score"`
	AdapterScore     float64  `json:"adapter_score"`
	Improvement      float64  `json:"improvement"`
	Passed           bool     `json:"passed"`
	RejectionReasons []string `json:"rejection_reasons,omitempty"`
}

// EvaluationReport compares the frozen base with the selected adapter on the
// untouched protected test split.
type EvaluationReport struct {
	Schema             string                `json:"schema"`
	Dataset            DatasetContractReport `json:"dataset"`
	Policy             ReliabilityPolicy     `json:"policy"`
	Base               EvaluationSummary     `json:"base"`
	Adapter            EvaluationSummary     `json:"adapter"`
	Stability          EvaluationSummary     `json:"stability"`
	Decision           ReliabilityDecision   `json:"decision"`
	SubstitutedFactIDs []string              `json:"substituted_fact_ids,omitempty"`
}

// ScoreAnswer computes deterministic lexical evidence. Exact answers ignore
// harmless case, surrounding punctuation, and whitespace differences.
func ScoreAnswer(record DatasetRecord, actual string) EvaluationResult {
	expectedTokens := normalizedAnswerTokens(record.Target)
	actualTokens := normalizedAnswerTokens(actual)
	overlap := tokenOverlap(expectedTokens, actualTokens)
	precision := fraction(overlap, len(actualTokens))
	recall := fraction(overlap, len(expectedTokens))
	f1 := harmonicMean(precision, recall)
	expectedContent := contentAnswerTokens(expectedTokens)
	actualContent := contentAnswerTokens(actualTokens)
	contentOverlap := tokenOverlap(expectedContent, actualContent)
	contentPrecision := fraction(contentOverlap, len(actualContent))
	coverage := fraction(contentOverlap, len(expectedContent))
	contentF1 := harmonicMean(contentPrecision, coverage)
	return EvaluationResult{
		Prompt: record.Prompt, Expected: record.Target, Actual: strings.TrimSpace(actual),
		ExactMatch:     canonicalAnswer(actual) == canonicalAnswer(record.Target),
		TokenPrecision: precision, TokenRecall: recall, TokenF1: f1, ContentTokenF1: contentF1, AnswerCoverage: coverage,
		ConflictingAnswer:   hasConflictingAnswer(record, actual),
		RepeatedTrigramRate: repeatedTokenNGramRate(actualTokens, 3),
	}
}

// BuildEvaluationReport verifies protected-test cardinality and applies the
// task-aware publication policy.
func BuildEvaluationReport(contract DatasetContractReport, base, adapter, stability []EvaluationResult) (EvaluationReport, error) {
	report := EvaluationReport{Schema: EvaluationReportSchema, Dataset: contract}
	if !contract.Ready {
		return report, errors.New("dataset contract is not ready")
	}
	if len(base) != contract.TestRecords || len(adapter) != contract.TestRecords {
		return report, fmt.Errorf("evaluation record counts base=%d adapter=%d do not match protected test records=%d", len(base), len(adapter), contract.TestRecords)
	}
	policy, err := defaultReliabilityPolicy(contract.Task)
	if err != nil {
		return report, err
	}
	report.Policy = policy
	report.Base = summarizeEvaluation(base, policy)
	report.Adapter = summarizeEvaluation(adapter, policy)
	report.Stability = summarizeEvaluation(stability, policy)
	report.Decision = decideReliability(policy, report.Base, report.Adapter)
	if report.Stability.Records < policy.MinimumStabilityRecords {
		report.Decision.RejectionReasons = append(report.Decision.RejectionReasons, fmt.Sprintf(
			"conversation stability evaluated %d records; require at least %d",
			report.Stability.Records, policy.MinimumStabilityRecords,
		))
	}
	if report.Stability.RecordPassingRate < policy.MinimumStabilityPassingRate {
		report.Decision.RejectionReasons = append(report.Decision.RejectionReasons, fmt.Sprintf(
			"%.4f of same-session answers meet the task-aware score; require at least %.4f",
			report.Stability.RecordPassingRate, policy.MinimumStabilityPassingRate,
		))
	}
	report.Decision.Passed = len(report.Decision.RejectionReasons) == 0
	return report, nil
}

// EvaluationPassingRate applies the same per-answer rules used by the final
// publication gate. Training backends use it only on validation records when
// choosing between candidates; protected test records remain untouched.
func EvaluationPassingRate(task string, results []EvaluationResult) (float64, error) {
	policy, err := defaultReliabilityPolicy(task)
	if err != nil {
		return 0, err
	}
	return summarizeEvaluation(results, policy).RecordPassingRate, nil
}

func defaultReliabilityPolicy(task string) (ReliabilityPolicy, error) {
	policy := ReliabilityPolicy{
		Task: task, MinimumTestRecords: MinimumTestRecords, MinimumImprovement: .05,
		MinimumStabilityRecords: 12, MinimumStabilityPassingRate: .7,
		MaximumEmptyAnswers: 0, MaximumAnswerRepetitionExcess: .1,
	}
	switch task {
	case datasetTaskExact:
		policy.MinimumExactAccuracy = .9
	case datasetTaskGeneration:
		policy.MinimumAnswerCoverage = .7
		policy.MinimumAnswerTokenF1 = .6
		policy.MinimumRecordPassingRate = .7
	default:
		return ReliabilityPolicy{}, fmt.Errorf("unknown dataset task %q", task)
	}
	return policy, nil
}

func decideReliability(policy ReliabilityPolicy, base, adapter EvaluationSummary) ReliabilityDecision {
	decision := ReliabilityDecision{}
	if adapter.Records != base.Records {
		decision.RejectionReasons = append(decision.RejectionReasons, fmt.Sprintf("adapter evaluated %d records; base evaluated %d", adapter.Records, base.Records))
	}
	if adapter.Records < policy.MinimumTestRecords {
		decision.RejectionReasons = append(decision.RejectionReasons, fmt.Sprintf("protected test has %d records; require at least %d", adapter.Records, policy.MinimumTestRecords))
	}
	switch policy.Task {
	case datasetTaskExact:
		decision.BaseScore = fraction(base.ExactMatches, base.Records)
		decision.AdapterScore = fraction(adapter.ExactMatches, adapter.Records)
		if decision.AdapterScore < policy.MinimumExactAccuracy {
			decision.RejectionReasons = append(decision.RejectionReasons, fmt.Sprintf("exact accuracy %.4f is below %.4f", decision.AdapterScore, policy.MinimumExactAccuracy))
		}
	case datasetTaskGeneration:
		decision.BaseScore = base.RecordPassingRate
		decision.AdapterScore = adapter.RecordPassingRate
		if adapter.MeanAnswerCoverage < policy.MinimumAnswerCoverage && adapter.RecordPassingRate < policy.MinimumRecordPassingRate {
			decision.RejectionReasons = append(decision.RejectionReasons, fmt.Sprintf("mean answer coverage %.4f is below %.4f", adapter.MeanAnswerCoverage, policy.MinimumAnswerCoverage))
		}
		if adapter.RecordPassingRate < policy.MinimumRecordPassingRate {
			decision.RejectionReasons = append(decision.RejectionReasons, fmt.Sprintf(
				"%.4f of protected answers meet the task-aware score; require at least %.4f",
				adapter.RecordPassingRate, policy.MinimumRecordPassingRate,
			))
		}
	default:
		decision.RejectionReasons = append(decision.RejectionReasons, fmt.Sprintf("unknown dataset task %q", policy.Task))
	}
	decision.Improvement = decision.AdapterScore - decision.BaseScore
	if decision.Improvement < policy.MinimumImprovement {
		decision.RejectionReasons = append(decision.RejectionReasons, fmt.Sprintf("improvement %.4f is below %.4f", decision.Improvement, policy.MinimumImprovement))
	}
	if adapter.EmptyAnswers > policy.MaximumEmptyAnswers {
		decision.RejectionReasons = append(decision.RejectionReasons, fmt.Sprintf("empty answers %d exceed maximum %d", adapter.EmptyAnswers, policy.MaximumEmptyAnswers))
	}
	decision.Passed = len(decision.RejectionReasons) == 0
	decision.RejectionReasons = slices.Clip(decision.RejectionReasons)
	return decision
}

func summarizeEvaluation(results []EvaluationResult, policy ReliabilityPolicy) EvaluationSummary {
	summary := EvaluationSummary{Records: len(results)}
	for _, result := range results {
		if result.ExactMatch {
			summary.ExactMatches++
		}
		if evaluationResultMeetsPolicy(result, policy) {
			summary.RecordsMeetingScore++
		}
		if strings.TrimSpace(result.Actual) == "" {
			summary.EmptyAnswers++
		}
		summary.MeanTokenPrecision += result.TokenPrecision
		summary.MeanTokenRecall += result.TokenRecall
		summary.MeanTokenF1 += result.TokenF1
		summary.MeanAnswerCoverage += result.AnswerCoverage
		summary.MeanRepeatedTrigramRate += result.RepeatedTrigramRate
	}
	if len(results) > 0 {
		total := float64(len(results))
		summary.MeanTokenPrecision /= total
		summary.MeanTokenRecall /= total
		summary.MeanTokenF1 /= total
		summary.MeanAnswerCoverage /= total
		summary.MeanRepeatedTrigramRate /= total
		summary.RecordPassingRate = float64(summary.RecordsMeetingScore) / total
	}
	return summary
}

func evaluationResultMeetsPolicy(result EvaluationResult, policy ReliabilityPolicy) bool {
	expectedRepetition := repeatedTokenNGramRate(normalizedAnswerTokens(result.Expected), 3)
	if result.RepeatedTrigramRate > expectedRepetition+policy.MaximumAnswerRepetitionExcess {
		return false
	}
	switch policy.Task {
	case datasetTaskExact:
		return result.ExactMatch
	case datasetTaskGeneration:
		if result.ConflictingAnswer {
			return false
		}
		if result.TokenF1 >= policy.MinimumAnswerTokenF1 && result.AnswerCoverage >= policy.MinimumAnswerCoverage {
			return true
		}
		return supportedConciseAnswer(result)
	default:
		return false
	}
}

var concreteAnswerPattern = regexp.MustCompile(`(?i)(?:https?://|www\.)\S+|[\w.+-]+@[\w.-]+\.[a-z]{2,}|\b\d+(?:\.\d+)?(?:%|[a-z]+)?\b`)

func hasConflictingAnswer(record DatasetRecord, actual string) bool {
	expectedConcrete := concreteAnswerPattern.FindAllString(strings.ToLower(record.Target), -1)
	actualConcrete := concreteAnswerPattern.FindAllString(strings.ToLower(actual), -1)
	if asksForConcreteValue(record.Prompt) && !isSubset(expectedConcrete, actualConcrete) {
		return true
	}
	for _, value := range actualConcrete {
		if (strings.Contains(value, "@") || strings.Contains(value, "://")) && !slices.Contains(expectedConcrete, value) {
			return true
		}
	}
	if !asksForFocusedAnswer(record.Prompt) {
		return false
	}

	promptTokens := contentAnswerTokens(normalizedAnswerTokens(record.Prompt))
	expectedValues := tokensNotIn(contentAnswerTokens(normalizedAnswerTokens(record.Target)), promptTokens)
	actualValues := tokensNotIn(contentAnswerTokens(normalizedAnswerTokens(actual)), promptTokens)
	return tokenOverlap(expectedValues, actualValues) < len(expectedValues) &&
		tokenOverlap(expectedValues, actualValues) < len(actualValues)
}

func asksForConcreteValue(prompt string) bool {
	prompt = strings.ToLower(prompt)
	for _, phrase := range []string{"how many", "how much", "how long", "percent", "duration", "email", "url", "address", "line "} {
		if strings.Contains(prompt, phrase) {
			return true
		}
	}
	return false
}

func asksForFocusedAnswer(prompt string) bool {
	prompt = strings.ToLower(prompt)
	for _, phrase := range []string{"who ", "whose ", " name", "which routine", "which function", "which method", "which call", "which host", "email", "url", "address", " called"} {
		if strings.Contains(prompt, phrase) {
			return true
		}
	}
	return false
}

func isSubset(expected, actual []string) bool {
	for _, value := range expected {
		if !slices.Contains(actual, value) {
			return false
		}
	}
	return true
}

func tokensNotIn(tokens, excluded []string) []string {
	values := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if !slices.Contains(excluded, token) {
			values = append(values, token)
		}
	}
	return values
}

func supportedConciseAnswer(result EvaluationResult) bool {
	expected := contentAnswerTokens(normalizedAnswerTokens(result.Expected))
	actual := contentAnswerTokens(normalizedAnswerTokens(result.Actual))
	if len(actual) == 0 || len(actual) > 3 || tokenOverlap(expected, actual) != len(actual) {
		return false
	}
	prompt := contentAnswerTokens(normalizedAnswerTokens(result.Prompt))
	if tokenOverlap(prompt, actual) == len(actual) {
		return false
	}
	actualTokens := make(map[string]bool, len(actual))
	for _, token := range actual {
		actualTokens[token] = true
	}
	if len(expected) <= maximumExactAnswerWords {
		for _, token := range expected {
			if containsDigit(token) && !actualTokens[token] {
				return false
			}
		}
	}
	return true
}

func containsDigit(value string) bool {
	for _, character := range value {
		if unicode.IsDigit(character) {
			return true
		}
	}
	return false
}

func harmonicMean(left, right float64) float64 {
	if left+right == 0 {
		return 0
	}
	return 2 * left * right / (left + right)
}

func canonicalAnswer(value string) string {
	value = strings.TrimFunc(strings.TrimSpace(value), unicode.IsPunct)
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func normalizedAnswerTokens(value string) []string {
	return strings.FieldsFunc(strings.ToLower(value), func(character rune) bool {
		return unicode.IsSpace(character) || unicode.IsPunct(character)
	})
}

func contentAnswerTokens(tokens []string) []string {
	content := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if !answerStopWords[token] {
			content = append(content, token)
		}
	}
	return content
}

var answerStopWords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true,
	"be": true, "by": true, "for": true, "from": true, "in": true, "is": true,
	"it": true, "of": true, "on": true, "or": true, "that": true, "the": true,
	"this": true, "to": true, "was": true, "when": true, "which": true, "with": true,
}

func tokenOverlap(expected, actual []string) int {
	available := make(map[string]int, len(expected))
	for _, token := range expected {
		available[token]++
	}
	overlap := 0
	for _, token := range actual {
		if available[token] > 0 {
			overlap++
			available[token]--
		}
	}
	return overlap
}

func repeatedTokenNGramRate(tokens []string, size int) float64 {
	total := len(tokens) - size + 1
	if total <= 0 {
		return 0
	}
	seen := make(map[string]bool, total)
	repeated := 0
	for index := range total {
		gram := strings.Join(tokens[index:index+size], "\x00")
		if seen[gram] {
			repeated++
		}
		seen[gram] = true
	}
	return fraction(repeated, total)
}

func fraction(value, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(value) / float64(total)
}
