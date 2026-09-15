package finetune

import (
	"math"
	"strings"
	"testing"
)

func TestScoreAnswerNormalizesHarmlessExactDifferences(t *testing.T) {
	result := ScoreAnswer(DatasetRecord{Prompt: "Status?", Target: "Ready"}, "  READY.\n")
	if !result.ExactMatch || result.TokenF1 != 1 || result.ContentTokenF1 != 1 || result.AnswerCoverage != 1 {
		t.Fatalf("score = %+v", result)
	}
}

func TestScoreAnswerAcceptsConciseContentWithoutAcceptingStopWords(t *testing.T) {
	concise := ScoreAnswer(DatasetRecord{Target: "This constant represents the maximum number of bytes required, which is 4."}, "4 bytes")
	if concise.AnswerCoverage >= 1 || concise.ContentTokenF1 >= .6 || concise.TokenF1 >= .7 {
		t.Fatalf("concise score = %+v", concise)
	}
	partial := ScoreAnswer(DatasetRecord{Target: "The default -benchtime is 1 second."}, "second")
	if partial.AnswerCoverage >= 1 || partial.ContentTokenF1 >= .6 {
		t.Fatalf("incomplete short answer score = %+v", partial)
	}
	stopWord := ScoreAnswer(DatasetRecord{Target: "The expected answer is Ready."}, "the")
	if stopWord.AnswerCoverage != 0 {
		t.Fatalf("stop-word score = %+v", stopWord)
	}
}

func TestProtectedExactEvaluationPassesOnlyWithMeasuredGain(t *testing.T) {
	contract := DatasetContractReport{Ready: true, Task: datasetTaskExact, TestRecords: 20}
	base := evaluationResults(20, 10)
	adapter := evaluationResults(20, 19)
	report, err := BuildEvaluationReport(contract, base, adapter, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Decision.Passed || report.Decision.AdapterScore != .95 || math.Abs(report.Decision.Improvement-.45) > 1e-12 {
		t.Fatalf("report = %+v", report)
	}

	unchanged, err := BuildEvaluationReport(contract, adapter, adapter, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Decision.Passed || !strings.Contains(strings.Join(unchanged.Decision.RejectionReasons, "\n"), "improvement") {
		t.Fatalf("unchanged adapter decision = %+v", unchanged.Decision)
	}
}

func TestProtectedGenerationEvaluationRecordsAllFailures(t *testing.T) {
	contract := DatasetContractReport{Ready: true, Task: datasetTaskGeneration, TestRecords: 20}
	base := make([]EvaluationResult, 20)
	adapter := make([]EvaluationResult, 19)
	for index := range base {
		base[index] = EvaluationResult{
			Expected: "A detailed expected answer containing enough words to require generated prose.",
			Actual:   "base", TokenF1: .55, AnswerCoverage: .65, RepeatedTrigramRate: .01,
		}
	}
	for index := range adapter {
		adapter[index] = EvaluationResult{
			Expected: "A detailed expected answer containing enough words to require generated prose.",
			Actual:   "", TokenF1: .55, AnswerCoverage: .66, RepeatedTrigramRate: .05,
		}
	}
	_, err := BuildEvaluationReport(contract, base, adapter, adapter)
	if err == nil || !strings.Contains(err.Error(), "record counts") {
		t.Fatalf("record-count error = %v", err)
	}

	adapter = append(adapter, EvaluationResult{
		Expected: "A detailed expected answer containing enough words to require generated prose.",
		Actual:   "", TokenF1: .55, AnswerCoverage: .66, RepeatedTrigramRate: .05,
	})
	report, err := BuildEvaluationReport(contract, base, adapter, adapter)
	if err != nil {
		t.Fatal(err)
	}
	reasons := strings.Join(report.Decision.RejectionReasons, "\n")
	if report.Decision.Passed {
		t.Fatalf("decision passed: %+v", report.Decision)
	}
	for _, want := range []string{"answer coverage", "protected answers", "improvement", "empty answers"} {
		if !strings.Contains(reasons, want) {
			t.Errorf("reasons %q missing %q", reasons, want)
		}
	}
}

func TestProtectedGenerationEvaluationCountsLoopingAnswerOnce(t *testing.T) {
	contract := DatasetContractReport{Ready: true, Task: datasetTaskGeneration, TestRecords: 20}
	base := make([]EvaluationResult, 20)
	target := "The only guarantee in this program is that the program won't terminate until the two Goroutines are done."
	adapter := make([]EvaluationResult, 20)
	for index := range adapter {
		adapter[index] = ScoreAnswer(DatasetRecord{Prompt: "When does the program terminate?", Target: target}, target)
	}
	adapter[0] = ScoreAnswer(
		DatasetRecord{Prompt: "What is the sole guarantee?", Target: target},
		"The only guarantee in this program is that the only guarantee is that the only guarantee is that the only guarantee is that the only guarantee is",
	)

	report, err := BuildEvaluationReport(contract, base, adapter, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Decision.Passed || report.Adapter.RecordsMeetingScore != 19 || report.Adapter.RecordPassingRate != .95 {
		t.Fatalf("single looping answer should count as one failed answer: %+v", report)
	}
}

func TestGenerationEvaluationRejectsAnswerRepetitionBeyondExpectedWording(t *testing.T) {
	policy, err := defaultReliabilityPolicy(datasetTaskGeneration)
	if err != nil {
		t.Fatal(err)
	}
	target := "The test function continues and the test function reports more failures."
	exact := ScoreAnswer(DatasetRecord{Prompt: "What does the test function do?", Target: target}, target)
	if exact.RepeatedTrigramRate == 0 || !evaluationResultMeetsPolicy(exact, policy) {
		t.Fatalf("naturally repetitive exact answer rejected: %+v", exact)
	}

	repeated := ScoreAnswer(DatasetRecord{Prompt: "What sequence is expected?", Target: "alpha beta gamma"}, "alpha beta gamma alpha beta gamma")
	if repeated.TokenF1 < policy.MinimumAnswerTokenF1 || repeated.AnswerCoverage < policy.MinimumAnswerCoverage {
		t.Fatalf("test answer must pass lexical scoring before repetition: %+v", repeated)
	}
	if evaluationResultMeetsPolicy(repeated, policy) {
		t.Fatalf("answer repetition beyond expected wording passed: %+v", repeated)
	}
}

func TestProtectedGenerationEvaluationRequiresMostAnswersToPassIndividually(t *testing.T) {
	contract := DatasetContractReport{Ready: true, Task: datasetTaskGeneration, TestRecords: 20}
	base := make([]EvaluationResult, 20)
	adapter := make([]EvaluationResult, 20)
	for index := range adapter {
		adapter[index] = EvaluationResult{
			Prompt:   "What is the expected answer?",
			Expected: "A detailed expected answer containing enough words to require generated prose.",
			Actual:   "answer", TokenF1: .59, AnswerCoverage: .8,
		}
	}
	report, err := BuildEvaluationReport(contract, base, adapter, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if report.Decision.Passed || report.Adapter.RecordsMeetingScore != 0 || report.Adapter.RecordPassingRate != 0 {
		t.Fatalf("weak individual answers passed: %+v", report)
	}
}

func TestProtectedGenerationEvaluationAllowsBoundedWeakAnswers(t *testing.T) {
	contract := DatasetContractReport{Ready: true, Task: datasetTaskGeneration, TestRecords: 20}
	base := make([]EvaluationResult, 20)
	adapter := make([]EvaluationResult, 20)
	for index := range adapter {
		adapter[index] = EvaluationResult{
			Prompt:   "What is the expected answer?",
			Expected: "A detailed expected answer containing enough words to require generated prose.",
			Actual:   "answer", TokenF1: .8, AnswerCoverage: .8,
		}
		if index >= 14 {
			adapter[index].TokenF1 = .4
		}
	}
	report, err := BuildEvaluationReport(contract, base, adapter, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Decision.Passed || report.Adapter.RecordsMeetingScore != 14 || report.Adapter.RecordPassingRate != .7 {
		t.Fatalf("bounded weak answers rejected: %+v", report)
	}
}

func TestProtectedGenerationEvaluationAcceptsConciseShortAnswersButRejectsContradictions(t *testing.T) {
	contract := DatasetContractReport{Ready: true, Task: datasetTaskGeneration, TestRecords: 20}
	base := make([]EvaluationResult, 20)
	adapter := make([]EvaluationResult, 20)
	for index := range adapter {
		adapter[index] = ScoreAnswer(DatasetRecord{Target: "The default -benchtime is 1 second."}, "1 second")
	}
	report, err := BuildEvaluationReport(contract, base, adapter, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Decision.Passed || report.Adapter.RecordsMeetingScore != 20 {
		t.Fatalf("concise correct answers rejected: %+v", report)
	}
	for index := 13; index < len(adapter); index++ {
		adapter[index] = ScoreAnswer(DatasetRecord{Target: `"localhost:8000"`}, "127.0.0.1")
	}
	report, err = BuildEvaluationReport(contract, base, adapter, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if report.Decision.Passed || report.Adapter.RecordsMeetingScore != 13 {
		t.Fatalf("contradictory short answers passed: %+v", report)
	}
}

func TestGenerationEvaluationRecognizesSupportedConciseAnswers(t *testing.T) {
	policy, err := defaultReliabilityPolicy(datasetTaskGeneration)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		record DatasetRecord
		actual string
		pass   bool
	}{
		{"essential name", DatasetRecord{Prompt: "What is the starting thread called?", Target: "The initial thread for each process is called the main thread."}, "main", true},
		{"essential value", DatasetRecord{Prompt: "What zero value does a map hold?", Target: "The zero value for a map variable is nil."}, "nil", true},
		{"omits citation numbers", DatasetRecord{Prompt: "Which call configures two logical processors?", Target: "The example in listing 6.7 configures two logical processors with the GOMAXPROCS call on line 14."}, "GOMAXPROCS", true},
		{"missing number", DatasetRecord{Prompt: "What is the default benchmark duration?", Target: "The default duration is 1 second."}, "second", false},
		{"wrong number with shared wording", DatasetRecord{Prompt: "What is the default benchmark duration?", Target: "The default duration is 1 second."}, "The default duration is 0 seconds.", false},
		{"wrong source line with shared wording", DatasetRecord{Prompt: "What happens at line 113?", Target: "An interface conversion occurs at line 113."}, "An interface conversion occurs at line 123.", false},
		{"wrong email domain", DatasetRecord{Prompt: "Which email address is listed?", Target: "The address is john@yahoo.com."}, "The address is john@ardanlabs.com.", false},
		{"substituted name", DatasetRecord{Prompt: "What is the person's name?", Target: "The person's name is Gene."}, "The person's name is Joan.", false},
		{"shared wording without enough answer", DatasetRecord{Prompt: "How are strings copied?", Target: "Copying a string has constant cost regardless of length because its two-word header is copied."}, "Copying a string has constant cost regardless of length.", false},
		{"repeats question", DatasetRecord{Prompt: "What zero value does a map hold?", Target: "The zero value for a map variable is nil."}, "map value", false},
		{"contradiction", DatasetRecord{Prompt: "Which host is configured?", Target: "The configured host is localhost:8000."}, "127.0.0.1", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := ScoreAnswer(test.record, test.actual)
			if passed := evaluationResultMeetsPolicy(result, policy); passed != test.pass {
				t.Fatalf("evaluationResultMeetsPolicy() = %v, want %v: %+v", passed, test.pass, result)
			}
		})
	}
}

func TestEvaluationPassingRateUsesPublicationAnswerRules(t *testing.T) {
	results := []EvaluationResult{
		ScoreAnswer(DatasetRecord{Prompt: "What is the duration?", Target: "1 second"}, "1 second"),
		ScoreAnswer(DatasetRecord{Prompt: "What is the duration?", Target: "1 second"}, "0 seconds"),
	}
	rate, err := EvaluationPassingRate(datasetTaskGeneration, results)
	if err != nil {
		t.Fatal(err)
	}
	if rate != .5 {
		t.Fatalf("passing rate = %v, want .5", rate)
	}
}

func TestProtectedEvaluationReportsConversationStabilitySeparately(t *testing.T) {
	contract := DatasetContractReport{Ready: true, Task: datasetTaskGeneration, TestRecords: 20}
	base := make([]EvaluationResult, 20)
	adapter := make([]EvaluationResult, 20)
	for index := range adapter {
		adapter[index] = ScoreAnswer(DatasetRecord{Target: "The expected protected answer is correct."}, "The expected protected answer is correct.")
	}
	stability := append([]EvaluationResult(nil), adapter[:12]...)
	for index := 8; index < len(stability); index++ {
		stability[index] = ScoreAnswer(DatasetRecord{Target: "The expected protected answer is correct."}, "unrelated")
	}
	report, err := BuildEvaluationReport(contract, base, adapter, stability)
	if err != nil {
		t.Fatal(err)
	}
	if report.Decision.Passed || report.Adapter.RecordPassingRate != 1 || report.Stability.RecordPassingRate != float64(8)/12 {
		t.Fatalf("unstable adapter report = %+v", report)
	}
	if reasons := strings.Join(report.Decision.RejectionReasons, "\n"); !strings.Contains(reasons, "same-session") {
		t.Fatalf("stability rejection reasons = %q", reasons)
	}

	stability[8] = adapter[0]
	report, err = BuildEvaluationReport(contract, base, adapter, stability)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Decision.Passed || report.Stability.RecordPassingRate != .75 {
		t.Fatalf("stable adapter report = %+v", report)
	}
}

func evaluationResults(records, exact int) []EvaluationResult {
	results := make([]EvaluationResult, records)
	for index := range results {
		results[index] = EvaluationResult{Actual: "answer", ExactMatch: index < exact, TokenF1: fraction(exact, records)}
	}
	return results
}
