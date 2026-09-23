package prepare

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

type qualityBenchmarkCase struct {
	name                string
	valid               bool
	deterministicReject bool
	record              Record
}

func qualityBenchmark() []qualityBenchmarkCase {
	return []qualityBenchmarkCase{
		{name: "direct fact", valid: true, record: Record{Prompt: "Who founded Northwind Labs?", Target: "Mira Chen founded Northwind Labs.", Evidence: "Northwind Labs was founded by Mira Chen in 2018."}},
		{name: "faithful paraphrase", valid: true, record: Record{Prompt: "Where does the service operate?", Target: "It operates in Toronto.", Evidence: "The service is based in Toronto and supports customers throughout Canada."}},
		{name: "focused source sentence", valid: true, record: Record{Prompt: "Who are the three project guides?", Target: "The project guides are Bill, Brian, and Erik.", Evidence: "For this project, you could not hope for better guides than Bill, Brian, and Erik."}},
		{name: "function name", valid: true, record: Record{Prompt: "What function is called after all results arrive?", Target: "Display", Evidence: "After every result arrives, the main function calls Display and then returns."}},
		{name: "package name", valid: true, record: Record{Prompt: "What is the name of the package containing counters.go?", Target: "counters", Evidence: "The counters.go file declares package counters."}},
		{name: "focused package sentence", valid: true, record: Record{Prompt: "What is the name of the package containing counters.go?", Target: "The file lives in package counters.", Evidence: "The counters.go file lives in package counters."}},
		{name: "focused function sentence", valid: true, record: Record{Prompt: "What function is called after all results arrive?", Target: "The Display function is called.", Evidence: "After every result arrives, the Display function is called."}},
		{name: "string value", valid: true, record: Record{Prompt: "What is the prefix string assigned to the trace logger?", Target: `"TRACE: "`, Evidence: `The trace logger is initialized with the prefix string "TRACE: ".`}},
		{name: "percentage", valid: true, record: Record{Prompt: "What percentage of the paper is recycled?", Target: "15 percent", Evidence: "The paper contains at least 15 percent recycled material."}},
		{name: "concise declaration", valid: true, record: Record{Prompt: "What is the type of the interrupt field?", Target: "chan os.Signal", Evidence: "The interrupt field is declared as a channel of os.Signal values: interrupt chan os.Signal."}},
		{name: "supported qualifier", valid: true, record: Record{Prompt: "When did the service launch?", Target: "The service launched publicly in March 2022.", Evidence: "Following a private beta, the service launched publicly in March 2022."}},
		{name: "supported qualifier synonym", valid: true, record: Record{Prompt: "Where is the service available?", Target: "The service is available globally.", Evidence: "The service is available worldwide."}},
		{name: "supported exclusivity synonym", valid: true, record: Record{Prompt: "Who can approve a release?", Target: "Only maintainers can approve a release.", Evidence: "Release approval is reserved solely for maintainers."}},
		{name: "subject section", valid: true, record: Record{Prompt: "Which section of the API handles authentication?", Target: "The security section handles authentication.", Evidence: "Authentication is handled by the security section of the API."}},
		{name: "ordinary synonym", valid: true, record: Record{Prompt: "What does the cache retain?", Target: "It keeps recently requested records.", Evidence: "The cache retains recently requested records for five minutes."}},

		{name: "function declaration", deterministicReject: true, record: Record{Prompt: "What function is called after all results arrive?", Target: "func Display(results chan *Result) {", Evidence: "After every result arrives, main calls Display(results)."}},
		{name: "string punctuation", deterministicReject: true, record: Record{Prompt: "What is the prefix string assigned to the trace logger?", Target: `"TRACE: ",`, Evidence: `The trace logger is initialized with the prefix string "TRACE: ".`}},
		{name: "source line residue", deterministicReject: true, record: Record{Prompt: "What value is assigned to checkMark?", Target: `09 const checkMark = "\u2713"`, Evidence: `Line 09 declares const checkMark = "\u2713".`}},
		{name: "assignment instead of value", deterministicReject: true, record: Record{Prompt: "What value is assigned to timeout?", Target: "const timeout = 3 * time.Second", Evidence: "The timeout declaration is const timeout = 3 * time.Second."}},
		{name: "document index", deterministicReject: true, record: Record{Prompt: "What constant is associated with L on page 189?", Target: "Ldate constant 189", Evidence: "Index: Ldate constant 189."}},
		{name: "table of contents target", deterministicReject: true, record: Record{Prompt: "Which operations retrieve and remove map entries?", Target: "3.23 Lookups and Deleting Map Keys................................65", Evidence: "3.23 Lookups and Deleting Map Keys................................65"}},
		{name: "unnamed method target", deterministicReject: true, record: Record{Prompt: "Which method decrements a WaitGroup?", Target: "That call decrements the WaitGroup by 1.", Evidence: "Inside each goroutine is a call to Done. That call decrements the WaitGroup by 1."}},
		{name: "isolated unlabeled value", deterministicReject: true, record: Record{Prompt: "How many pages does the Northwind handbook contain?", Target: "424 pages", Evidence: "424 pages"}},
		{name: "wrong struct field", deterministicReject: true, record: Record{Prompt: "What is assigned to the name field?", Target: `"abc"`, Evidence: `var data = []struct { input []byte }{{input: []byte("abc")}}`}},
		{name: "partial printed output", deterministicReject: true, record: Record{Prompt: "What string is printed when the name is Bill?", Target: `"My Name Is"`, Evidence: `fmt.Println("My Name Is", d.name)`}},
		{name: "complete printed literal", valid: true, record: Record{Prompt: "What string is printed when the request ends?", Target: `"request ended"`, Evidence: `log.Println("request ended")`}},
		{name: "first item with complete list", deterministicReject: true, record: Record{Prompt: "What is the first step in Wes Dyer's order?", Target: "Make it correct, make it clear, make it concise, make it fast. In that order.", Evidence: "Make it correct, make it clear, make it concise, make it fast. In that order."}},
		{name: "one field with surrounding fields", deterministicReject: true, record: Record{Prompt: "Which field is inaccessible outside the package?", Target: "Name and ID are accessible, but password is not.", Evidence: "Name and ID are accessible, but password is not."}},
		{name: "yes no mismatch", deterministicReject: true, record: Record{Prompt: "The Context package controls cancellation; is this correct?", Target: "Use the Context package.", Evidence: "Use the Context package."}},
		{name: "explicit yes answer", valid: true, record: Record{Prompt: "Does the Context package control cancellation?", Target: "Yes, the Context package controls cancellation.", Evidence: "Yes, the Context package controls cancellation."}},
		{name: "why answer repeats conclusion", deterministicReject: true, record: Record{Prompt: "Why must the value be allocated on the heap?", Target: "Therefore, the value must be allocated on the heap.", Evidence: "The caller retains its address. Therefore, the value must be allocated on the heap."}},
		{name: "why answer gives reason", valid: true, record: Record{Prompt: "Why must the value be allocated on the heap?", Target: "Because the caller retains its address after the function returns.", Evidence: "Because the caller retains its address after the function returns, the value must be allocated on the heap."}},
		{name: "self answering prompt", deterministicReject: true, record: Record{Prompt: "Once collection finishes, every processor serves the application and it returns to full speed; what is the resulting state?", Target: "Every processor serves the application and it returns to full speed.", Evidence: "Every processor serves the application and it returns to full speed."}},
		{name: "literal answer leak", deterministicReject: true, record: Record{Prompt: "Which email is assigned to the user with email bill@ardanlabs.com?", Target: `"bill@ardanlabs.com"`, Evidence: `The user has email "bill@ardanlabs.com".`}},
		{name: "quoted one-word answer leak", deterministicReject: true, record: Record{Prompt: `What prefix is configured when the literal "TRACE: " is used?`, Target: `"TRACE: "`, Evidence: `The logger prefix is "TRACE: ".`}},
		{name: "quoted answer leak with alternate quotes", deterministicReject: true, record: Record{Prompt: "When the name is initialized to 'Bill', what is its initial value?", Target: `"Bill"`, Evidence: `The name is initialized to "Bill".`}},
		{name: "unresolved declared type", deterministicReject: true, record: Record{Prompt: "What struct type is declared to represent a linked-list node?", Target: "This struct type is declared to represent a node for the linked list.", Evidence: "The type node[T any] is declared to represent a node for the linked list."}},
		{name: "unresolved standalone target", deterministicReject: true, record: Record{Prompt: "What happens when the user value is shared up the call stack?", Target: "This share will cause the user value to escape.", Evidence: "Sharing the user value up the call stack causes it to escape."}},
		{name: "enable disable contradiction", deterministicReject: true, record: Record{Prompt: "Which flag enables inlining in the terminal view?", Target: "--noinlines", Evidence: "Add --noinlines to turn off inlining in the terminal view."}},
		{name: "valid disabling flag", valid: true, record: Record{Prompt: "Which flag disables inlining in the terminal view?", Target: "--noinlines", Evidence: "Add --noinlines to turn off inlining in the terminal view."}},
		{name: "environment variable mismatch", deterministicReject: true, record: Record{Prompt: "Which environment variable enables detailed GC pacing traces?", Target: "gcpacertrace=1", Evidence: "Set GODEBUG=gcpacertrace=1 for detailed GC pacing traces."}},
		{name: "valid environment variable", valid: true, record: Record{Prompt: "Which environment variable enables detailed GC pacing traces?", Target: "GODEBUG", Evidence: "Set GODEBUG=gcpacertrace=1 for detailed GC pacing traces."}},
		{name: "package name does not answer significance", record: Record{Prompt: "What is the significance of importing a package with a blank identifier?", Target: `"net/http/pprof"`, Evidence: `Importing _ "net/http/pprof" calls its init function and registers profiling routes.`}},
		{name: "type declaration does not answer type name", record: Record{Prompt: "What is the name of the generic struct type used for a linked-list node?", Target: "type node[T any] struct {", Evidence: "type node[T any] struct { Data T }"}},
		{name: "method name does not answer interface name", record: Record{Prompt: "What is the name of Go's built-in error-handling interface?", Target: "Error", Evidence: "type error interface { Error() string }"}},
		{name: "wrong target", record: Record{Prompt: "Who founded Northwind Labs?", Target: "Toronto", Evidence: "Northwind Labs was founded by Mira Chen in 2018."}},
		{name: "wrong evidence", record: Record{Prompt: "Where does the service operate?", Target: "Toronto", Evidence: "Northwind Labs was founded by Mira Chen in 2018."}},
		{name: "unsupported qualifier", deterministicReject: true, record: Record{Prompt: "When did the service launch?", Target: "The service launched globally in March 2022.", Evidence: "The service launched publicly in March 2022."}},
		{name: "outside knowledge", record: Record{Prompt: "What language is the service written in?", Target: "Go", Evidence: "The service processes customer reports every hour."}},
	}
}

func TestDeterministicQualityBenchmark(t *testing.T) {
	for _, benchmark := range qualityBenchmark() {
		if !benchmark.valid && !benchmark.deterministicReject {
			continue
		}
		t.Run(benchmark.name, func(t *testing.T) {
			results, err := validateVerification(GoalQuestionAnswer, []generatedVerification{{RecordNumber: 1, ExpectedTargetID: 1}}, []Record{benchmark.record})
			if err != nil {
				t.Fatal(err)
			}
			if got := results[0].Passed; got != benchmark.valid {
				t.Fatalf("quality decision = %t, want %t: %s", got, benchmark.valid, results[0].Reason)
			}
		})
	}
}

func TestLiveQualityBenchmark(t *testing.T) {
	model := os.Getenv("MOXIE_QUALITY_MODEL")
	if model == "" {
		t.Skip("MOXIE_QUALITY_MODEL is not set")
	}
	client := newLiveKronkCompleter()
	previewer, err := NewPreviewer(client)
	if err != nil {
		t.Fatal(err)
	}

	benchmark := qualityBenchmark()
	records := make([]Record, len(benchmark))
	for index := range benchmark {
		records[index] = benchmark[index].record
	}

	const batchSize = 5
	var trueAccept, trueReject, falseAccept, falseReject int
	for start := 0; start < len(records); start += batchSize {
		end := min(start+batchSize, len(records))
		results, _, err := previewer.verifyRecords(t.Context(), model, GoalQuestionAnswer, "", records[start:end])
		if err != nil {
			t.Fatalf("verify benchmark cases %d-%d: %v", start+1, end, err)
		}
		for index, result := range results {
			benchmarkCase := benchmark[start+index]
			switch {
			case benchmarkCase.valid && result.Passed:
				trueAccept++
			case benchmarkCase.valid:
				falseReject++
				t.Logf("false reject: %s", benchmarkCase.name)
			case result.Passed:
				falseAccept++
				t.Logf("false accept: %s", benchmarkCase.name)
			default:
				trueReject++
			}
		}
	}

	precision := qualityRatio(trueAccept, trueAccept+falseAccept)
	recall := qualityRatio(trueAccept, trueAccept+falseReject)
	specificity := qualityRatio(trueReject, trueReject+falseAccept)
	t.Logf("true_accept=%d true_reject=%d false_accept=%d false_reject=%d precision=%.3f recall=%.3f specificity=%.3f",
		trueAccept, trueReject, falseAccept, falseReject, precision, recall, specificity)
	if falseAccept != 0 || recall < .9 {
		t.Fatalf("quality benchmark requires zero false accepts and at least 90%% recall")
	}
}

func TestLiveQuestionFormCanary(t *testing.T) {
	model := os.Getenv("MOXIE_QUALITY_MODEL")
	if model == "" {
		t.Skip("MOXIE_QUALITY_MODEL is not set")
	}
	client := newLiveKronkCompleter()
	previewer, err := NewPreviewer(client)
	if err != nil {
		t.Fatal(err)
	}

	facts := []Record{
		{Target: "SIGPROF", Evidence: "Go CPU profiling uses SIGPROF to interrupt a running program for sampling."},
		{Target: "context", Evidence: "The standard context package lets callers control cancellation and deadlines for blocking operations."},
		{Target: "go test", Evidence: "The go test tool runs Go tests."},
		{Target: "Callers must process bytes returned by Read before considering its error.", Evidence: "Callers must process bytes returned by Read before considering its error."},
		{Target: "A table test uses one test function for multiple values and conditions.", Evidence: "A table test uses one test function for multiple values and conditions."},
	}
	for index := range facts {
		facts[index].FactID = recordFactID(facts[index].Evidence, facts[index].Target)
	}

	completion, err := client.Complete(t.Context(), CompletionRequest{
		Model:          model,
		Messages:       questionPoolMessages(facts, ""),
		ResponseSchema: questionPoolSchema(len(facts)),
		MaxTokens:      max(generationMaxTokens, len(facts)*questionCandidatePoolSize*96),
	})
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Pools []generatedQuestionPool `json:"pools"`
	}
	if err := json.Unmarshal([]byte(completion.Content), &response); err != nil {
		t.Fatalf("decode question pool: %v\n%s", err, completion.Content)
	}
	t.Logf("raw generated pools: %s", completion.Content)
	expanded := selectQuestionPools(facts, response.Pools, nil)
	if len(expanded) == 0 {
		t.Fatal("no source fact produced usable alternate questions")
	}
	verification, review, err := previewer.verifyRecords(t.Context(), model, GoalQuestionAnswer, "", expanded)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("canary tokens: prompt=%d completion=%d total=%d", completion.PromptTokens+review.promptTokens, completion.CompletionTokens+review.completionTokens, completion.PromptTokens+completion.CompletionTokens+review.promptTokens+review.completionTokens)

	passedByFact := make(map[string]int, len(facts))
	for index, result := range verification {
		if result.Passed {
			passedByFact[expanded[index].FactID]++
		}
	}
	for _, fact := range facts {
		var questions []string
		for _, record := range expanded {
			if record.FactID == fact.FactID {
				questions = append(questions, record.Prompt)
			}
		}
		t.Logf("target %q question forms:\n%s", fact.Target, strings.Join(questions, "\n"))
		if len(questions) != questionAnswerRequiredVariants+1 {
			t.Errorf("target %q produced %d question forms, want %d", fact.Target, len(questions), questionAnswerRequiredVariants+1)
			continue
		}
		if passedByFact[fact.FactID] != len(questions) {
			t.Errorf("target %q passed %d/%d generated questions", fact.Target, passedByFact[fact.FactID], len(questions))
		}
		for left := range questions {
			if questionRepeatsAtomicTarget(questions[left], fact.Target) {
				t.Errorf("target %q is revealed by question %q", fact.Target, questions[left])
			}
			for right := left + 1; right < len(questions); right++ {
				if similarity := maximumQuestionSimilarity(questions[left], []string{questions[right]}); similarity >= 0.8 {
					t.Errorf("target %q has overly similar questions %.2f: %s", fact.Target, similarity, fmt.Sprintf("%q and %q", questions[left], questions[right]))
				}
			}
		}
	}
}

func TestLiveCustomDatasetInstructionsAffectQuestions(t *testing.T) {
	model := os.Getenv("MOXIE_QUALITY_MODEL")
	if model == "" {
		t.Skip("MOXIE_QUALITY_MODEL is not set")
	}
	client := newLiveKronkCompleter()
	previewer, err := NewPreviewer(client)
	if err != nil {
		t.Fatal(err)
	}

	facts := []Record{
		{Target: "SIGPROF", Evidence: "Go CPU profiling uses SIGPROF to interrupt a running program for sampling."},
		{Target: "context", Evidence: "The standard context package lets callers control cancellation and deadlines for blocking operations."},
		{Target: "go test", Evidence: "The go test tool compiles and runs Go tests."},
	}
	for index := range facts {
		facts[index].FactID = recordFactID(facts[index].Evidence, facts[index].Target)
	}

	generate := func(instructions string) []Record {
		t.Helper()
		completion, err := client.Complete(t.Context(), CompletionRequest{
			Model:          model,
			Messages:       questionPoolMessages(facts, instructions),
			ResponseSchema: questionPoolSchema(len(facts)),
			MaxTokens:      max(generationMaxTokens, len(facts)*questionCandidatePoolSize*96),
			Seed:           preparationGenerationSeed,
		})
		if err != nil {
			t.Fatal(err)
		}
		var response struct {
			Pools []generatedQuestionPool `json:"pools"`
		}
		if err := json.Unmarshal([]byte(completion.Content), &response); err != nil {
			t.Fatalf("decode question pool: %v\n%s", err, completion.Content)
		}
		selected := selectQuestionPools(facts, response.Pools, nil)
		if want := len(facts) * (questionAnswerRequiredVariants + 1); len(selected) != want {
			t.Fatalf("selected %d usable questions, want %d: %s", len(selected), want, completion.Content)
		}
		return selected
	}

	defaultRecords := generate("")
	const instructions = "create big questions"
	customRecords := generate(instructions)
	averageWords := func(records []Record) float64 {
		total := 0
		for _, record := range records {
			total += len(strings.Fields(record.Prompt))
		}
		return float64(total) / float64(len(records))
	}
	defaultAverage := averageWords(defaultRecords)
	customAverage := averageWords(customRecords)
	t.Logf("average question words: default=%.1f custom=%.1f", defaultAverage, customAverage)
	if customAverage < defaultAverage+2 {
		t.Fatalf("custom instructions did not materially increase question detail: default=%.1f custom=%.1f", defaultAverage, customAverage)
	}

	results, _, err := previewer.verifyRecords(t.Context(), model, GoalQuestionAnswer, instructions, customRecords)
	if err != nil {
		t.Fatal(err)
	}
	for index, result := range results {
		if !result.Passed {
			t.Errorf("custom question %q failed verification: %s", customRecords[index].Prompt, result.Reason)
		}
	}
}

func qualityRatio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}
