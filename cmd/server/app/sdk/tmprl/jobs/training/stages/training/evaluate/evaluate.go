// Package evaluate compares a trained candidate with the frozen base model
// against protected test answers and persists the acceptance decision.
package evaluate

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training/train"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// Run snapshots the candidate into the caller-owned scratch directory and
// writes the report to workspace. The caller owns both directory lifetimes.
// Publish, when not nil, receives answers given so far: the base model answers
// every test question, then the candidate does.
func Run(ctx context.Context, workspace *os.Root, trainer training.Trainer, job training.TrainingJob, publish func(tmprl.Progress)) (training.Artifacts, error) {
	file, err := workspace.Open("candidate.gguf")
	if err != nil {
		return training.Artifacts{}, err
	}
	defer file.Close()
	private, err := os.OpenRoot(job.Workdir)
	if err != nil {
		return training.Artifacts{}, err
	}
	defer private.Close()
	if err := train.CopyArtifact(ctx, private, "candidate.gguf", file); err != nil {
		return training.Artifacts{}, err
	}
	// A dataset without test questions trains without the comparison; a report
	// with no answers records that the adapter was not checked.
	if len(job.Dataset.Test) == 0 {
		if err := tmprl.WriteJSON(workspace, "evaluation.json", EvaluationReport{Accepted: true}); err != nil {
			return training.Artifacts{}, err
		}
		return training.Artifacts{Report: filepath.Join(workspace.Name(), "evaluation.json"), Accepted: true, NotCompared: true}, nil
	}
	total := 2 * len(job.Dataset.Test)
	answered := func(offset int) func(int) {
		if publish == nil {
			return nil
		}
		return func(n int) { publish(tmprl.Progress{Completed: offset + n, Total: total}) }
	}
	base, err := trainer.Predict(ctx, job, "", job.Dataset.Test, answered(0))
	if err != nil {
		return training.Artifacts{}, err
	}
	adapted, err := trainer.Predict(ctx, job, filepath.Join(job.Workdir, "candidate.gguf"), job.Dataset.Test, answered(len(job.Dataset.Test)))
	if err != nil {
		return training.Artifacts{}, err
	}
	if len(base) != len(job.Dataset.Test) || len(adapted) != len(base) {
		return training.Artifacts{}, training.InvalidOutput("evaluate")
	}
	report := EvaluationReport{Total: len(base)}
	empty := 0
	for i, record := range job.Dataset.Test {
		adapterPassed := passes(record.Prompt, adapted[i], record.Target)
		report.Answers = append(report.Answers, EvaluationAnswer{Prompt: record.Prompt, Expected: record.Target, BaseAnswer: base[i], AdapterAnswer: adapted[i], AdapterPassed: new(adapterPassed)})
		if passes(record.Prompt, base[i], record.Target) {
			report.BaseCorrect++
		}
		if adapterPassed {
			report.AdapterCorrect++
		}
		if strings.TrimSpace(adapted[i]) == "" {
			empty++
		}
	}
	// The adapter is accepted when at least 70% of its answers pass, it passes at
	// least 5 points more of them than the base model, and none is empty.
	report.Checks = []AcceptanceCheck{
		{Code: "answer-score", Actual: report.AdapterCorrect, Required: (report.Total*7 + 9) / 10, Passed: report.AdapterCorrect*10 >= report.Total*7},
		{Code: "gain-over-base", Actual: report.AdapterCorrect - report.BaseCorrect, Required: (report.Total + 19) / 20, Passed: (report.AdapterCorrect-report.BaseCorrect)*20 >= report.Total},
		{Code: "empty-answers", Actual: empty, Required: 0, Passed: empty == 0},
		{Code: "test-questions", Actual: report.Total, Required: training.MinimumTestQuestions, Passed: report.Total >= training.MinimumTestQuestions},
	}
	report.Accepted = !slices.ContainsFunc(report.Checks, func(check AcceptanceCheck) bool { return !check.Passed })
	if err := tmprl.WriteJSON(workspace, "evaluation.json", report); err != nil {
		return training.Artifacts{}, err
	}
	return training.Artifacts{Report: filepath.Join(workspace.Name(), "evaluation.json"), Accepted: report.Accepted}, nil
}

// =============================================================================
// Answer scoring compares words, not exact sentences, so a reworded correct
// answer passes. Text is lowercased and split on spaces and punctuation;
// content words leave out a few stop words such as "the" and "of".

// passes reports whether answer gives target's answer to prompt. It passes when
// it covers at least 70% of target's content words with a word F1 of at least
// 0.6, or when it is a short answer made only of target's words. It fails when
// it repeats itself more than target does or conflicts with a number, address,
// or name that prompt asks for.
func passes(prompt, answer, target string) bool {
	expected, actual := answerTokens(target), answerTokens(answer)
	if repetition(actual) > repetition(expected)+.1 || conflicts(prompt, answer, target) {
		return false
	}
	overlap := tokenOverlap(expected, actual)
	f1 := harmonicMean(fraction(overlap, len(actual)), fraction(overlap, len(expected)))
	expectedContent, actualContent := contentTokens(expected), contentTokens(actual)
	if f1 >= .6 && fraction(tokenOverlap(expectedContent, actualContent), len(expectedContent)) >= .7 {
		return true
	}
	return conciseAnswer(prompt, expectedContent, actualContent)
}

// conciseAnswer accepts at most three content words, all from the expected
// answer and not all from the question. A short expected answer's numbers must
// all be present.
func conciseAnswer(prompt string, expected, actual []string) bool {
	if len(actual) == 0 || len(actual) > 3 || tokenOverlap(expected, actual) != len(actual) {
		return false
	}
	if tokenOverlap(contentTokens(answerTokens(prompt)), actual) == len(actual) {
		return false
	}
	if len(expected) <= 8 {
		for _, token := range expected {
			if strings.ContainsFunc(token, unicode.IsDigit) && !slices.Contains(actual, token) {
				return false
			}
		}
	}
	return true
}

var concreteAnswer = regexp.MustCompile(`(?i)(?:https?://|www\.)\S+|[\w.+-]+@[\w.-]+\.[a-z]{2,}|\b\d+(?:\.\d+)?(?:%|[a-z]+)?\b`)

// conflicts reports an answer that leaves out a number a "how many" question
// needs, names an address the expected answer does not, or gives a different
// name than a "who" or "which" question needs.
func conflicts(prompt, answer, target string) bool {
	expected := concreteAnswer.FindAllString(strings.ToLower(target), -1)
	actual := concreteAnswer.FindAllString(strings.ToLower(answer), -1)
	if asks(prompt, "how many", "how much", "how long", "percent", "duration", "email", "url", "address", "line ") && !isSubset(expected, actual) {
		return true
	}
	for _, value := range actual {
		if (strings.Contains(value, "@") || strings.Contains(value, "://")) && !slices.Contains(expected, value) {
			return true
		}
	}
	if !asks(prompt, "who ", "whose ", " name", "which routine", "which function", "which method", "which call", "which host", "email", "url", "address", " called") {
		return false
	}
	question := contentTokens(answerTokens(prompt))
	want := tokensNotIn(contentTokens(answerTokens(target)), question)
	got := tokensNotIn(contentTokens(answerTokens(answer)), question)
	return tokenOverlap(want, got) < len(want) && tokenOverlap(want, got) < len(got)
}

func asks(prompt string, phrases ...string) bool {
	prompt = strings.ToLower(prompt)
	return slices.ContainsFunc(phrases, func(phrase string) bool { return strings.Contains(prompt, phrase) })
}

func isSubset(expected, actual []string) bool {
	return !slices.ContainsFunc(expected, func(value string) bool { return !slices.Contains(actual, value) })
}

func tokensNotIn(tokens, excluded []string) []string {
	return slices.DeleteFunc(slices.Clone(tokens), func(token string) bool { return slices.Contains(excluded, token) })
}

func answerTokens(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return unicode.IsSpace(r) || unicode.IsPunct(r) })
}

var stopWords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true,
	"be": true, "by": true, "for": true, "from": true, "in": true, "is": true,
	"it": true, "of": true, "on": true, "or": true, "that": true, "the": true,
	"this": true, "to": true, "was": true, "when": true, "which": true, "with": true,
}

func contentTokens(tokens []string) []string {
	return slices.DeleteFunc(slices.Clone(tokens), func(token string) bool { return stopWords[token] })
}

// tokenOverlap counts actual tokens found in expected, each expected token once.
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

// repetition is the share of word trigrams that repeat an earlier one.
func repetition(tokens []string) float64 {
	total := len(tokens) - 2
	if total <= 0 {
		return 0
	}
	seen := make(map[string]bool, total)
	repeated := 0
	for i := range total {
		gram := strings.Join(tokens[i:i+3], "\x00")
		if seen[gram] {
			repeated++
		}
		seen[gram] = true
	}
	return fraction(repeated, total)
}

func harmonicMean(left, right float64) float64 {
	if left+right == 0 {
		return 0
	}
	return 2 * left * right / (left + right)
}

func fraction(value, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(value) / float64(total)
}
