package prepare

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestQuoteCandidatesPreservesNamesWithAbbreviations(t *testing.T) {
	passage := "William Kennedy wrote the book. Brian Ketelsen and Erik St. Martin are the organizers of GopherCon."
	want := []string{
		"William Kennedy wrote the book.",
		"Brian Ketelsen and Erik St. Martin are the organizers of GopherCon.",
	}

	if got := quoteCandidates(passage); !reflect.DeepEqual(got, want) {
		t.Fatalf("quotes: got %#v want %#v", got, want)
	}
}

func TestQuoteCandidatesPreservesUppercaseNameWithAbbreviation(t *testing.T) {
	passage := "WITH BRIAN KETELSEN AND ERIK ST. MARTIN"
	want := []string{"WITH BRIAN KETELSEN AND ERIK ST. MARTIN"}

	if got := quoteCandidates(passage); !reflect.DeepEqual(got, want) {
		t.Fatalf("quotes: got %#v want %#v", got, want)
	}
}

func TestQuoteCandidatesPreservesInitialAndDecimal(t *testing.T) {
	passage := "J. Smith recorded 3.5 volts. The reading was stable."
	want := []string{
		"J. Smith recorded 3.5 volts.",
		"The reading was stable.",
	}

	if got := quoteCandidates(passage); !reflect.DeepEqual(got, want) {
		t.Fatalf("quotes: got %#v want %#v", got, want)
	}
}

func TestQuoteCandidatesSeparatesCollapsedNumberedCode(t *testing.T) {
	passage := "```\n11 12 // timeout is the number of seconds. 13 const timeout = 3 * time.Second 14 15 // main is the entry point. 16 func main() { 17 log.Println(\"Starting work.\") 18 19 // Create a timer. 20 r := runner.New(timeout)\n```"
	quotes := quoteCandidates(passage)

	if !containsQuote(quotes, "const timeout = 3 * time.Second") {
		t.Fatalf("quotes do not contain the timeout declaration: %#v", quotes)
	}
	if !containsQuote(quotes, `log.Println("Starting work.")`) {
		t.Fatalf("quotes do not contain the log statement: %#v", quotes)
	}
	if !containsQuote(quotes, `"Starting work."`) {
		t.Fatalf("quotes do not contain the string argument: %#v", quotes)
	}
	for _, quote := range quotes {
		if utf8.RuneCountInString(quote) > maxTargetRunes {
			t.Fatalf("oversized quote with %d runes: %q", utf8.RuneCountInString(quote), quote)
		}
	}
}

func TestQuoteCandidatesDoesNotTreatNumericValueAsLineNumber(t *testing.T) {
	passage := "```\n13 const ( 14 numberGoroutines = 4 // Number to use. 15 taskLoad = 10 // Amount of work. 16 ) 17 18 // main starts here. 19 func main() {}\n```"
	quotes := quoteCandidates(passage)

	if !containsQuote(quotes, "taskLoad = 10 // Amount of work.") {
		t.Fatalf("numeric value split the source line: %#v", quotes)
	}
}

func TestQuoteCandidatesExtractsAssignmentFromCollapsedFunction(t *testing.T) {
	passage := "```\nfunc TestDownload(t *testing.T) { statusCode := 200 server := mockServer() defer server.Close() }\n```"
	quotes := quoteCandidates(passage)

	if !containsQuote(quotes, "statusCode := 200") {
		t.Fatalf("quotes do not contain the concise assignment: %#v", quotes)
	}
	if !containsQuote(quotes, "200") {
		t.Fatalf("quotes do not contain the assigned value: %#v", quotes)
	}
}

func TestQuoteCandidatesExtractsAssignedExpression(t *testing.T) {
	quotes := quoteCandidates("```\nconst timeout = 3 * time.Second // maximum duration\n```")
	if !containsQuote(quotes, "3 * time.Second") {
		t.Fatalf("quotes do not contain the assigned expression: %#v", quotes)
	}
}

func TestQuoteCandidatesExtractsFunctionPackageAndTypeNames(t *testing.T) {
	passage := "```\n01 package counters\n02 type Table struct { buckets int }\n03 func Display(results chan *Result) {\n04 }\n```"
	quotes := quoteCandidates(passage)

	for _, want := range []string{"counters", "Table", "Display"} {
		if !containsQuote(quotes, want) {
			t.Fatalf("quotes do not contain %q: %#v", want, quotes)
		}
	}
}

func TestQuoteCandidatesRejectsIncompleteDeclarationButKeepsTypeName(t *testing.T) {
	quotes := quoteCandidates("```\ntype Table[K comparable, V any] struct { <-- this is a data table\n```")

	if containsQuote(quotes, "type Table[K comparable, V any] struct {") {
		t.Fatalf("quotes contain an incomplete declaration: %#v", quotes)
	}
	if !containsQuote(quotes, "Table") {
		t.Fatalf("quotes do not contain the usable type name: %#v", quotes)
	}
}

func TestQuoteCandidatesExtractsCommandAndMetricFromTerminalOutput(t *testing.T) {
	passage := "```\n$ export GODEBUG=gctrace=1,gcpacertrace=1 ./app Sample output: gc 5 Average: 0.0248 secs Requests/sec: 3711.3009\n```"
	quotes := quoteCandidates(passage)

	for _, want := range []string{"$ export GODEBUG=gctrace=1,gcpacertrace=1 ./app", "Average: 0.0248 secs", "Requests/sec: 3711.3009"} {
		if !containsQuote(quotes, want) {
			t.Fatalf("quotes do not contain %q: %#v", want, quotes)
		}
	}
}

func TestQuoteCandidatesExtractsAtomicPercentageAndBooleanValues(t *testing.T) {
	passage := "That's an improvement of 88% on requests.\n\n```\nType Zero Value Boolean false Integer 0\n```"
	quotes := quoteCandidates(passage)

	for _, want := range []string{"88%", "false"} {
		if !containsQuote(quotes, want) {
			t.Fatalf("quotes do not contain %q: %#v", want, quotes)
		}
	}
}

func TestQuoteCandidatesExtractsAtomicProseAnswers(t *testing.T) {
	passage := "The profiler uses SIGPROF. Disable inlining with --noinlines. Add gcpacertrace=1 for details. The total is 4, which is 4. Output is hey. What I need then is a mutex. Bill is the output once again. Name and ID are accessible, but password is not. I added a variable named lFound. A cache line is 64-byte data. The heap is 4 meg."
	quotes := quoteCandidates(passage)

	for _, want := range []string{"SIGPROF", "--noinlines", "gcpacertrace=1", "4", "hey", "mutex", "Bill", "password", "lFound", "64-byte", "4 meg"} {
		if !containsQuote(quotes, want) {
			t.Fatalf("quotes do not contain %q: %#v", want, quotes)
		}
	}
}

func TestQuoteCandidatesRemovesCodePresentationAnnotation(t *testing.T) {
	passage := "```\n98 atomic.AddInt32(&found, 1) <-- CHANGED 99 continue 100 }\n```"
	quotes := quoteCandidates(passage)

	if !containsQuote(quotes, "atomic.AddInt32(&found, 1)") {
		t.Fatalf("quotes do not contain clean function call: %#v", quotes)
	}
	for _, quote := range quotes {
		if strings.Contains(quote, "<--") {
			t.Fatalf("quote contains presentation annotation: %q", quote)
		}
	}
}

func TestQuoteCandidatesDecodesHTMLEntities(t *testing.T) {
	want := []string{"L1 -> L2 -> L3 -> main memory."}
	if got := quoteCandidates("L1 -&gt; L2 -&gt; L3 -&gt; main memory."); !reflect.DeepEqual(got, want) {
		t.Fatalf("quotes: got %#v want %#v", got, want)
	}
}

func TestQuoteCandidatesCanonicalizesProseWithoutChangingCode(t *testing.T) {
	passage := "-  The  go test  tool is used to run tests.\n- (2) Callers  process bytes before errors.\n\n```\nvalue  :=  source\n```"
	quotes := quoteCandidates(passage)

	for _, want := range []string{
		"The go test tool is used to run tests.",
		"Callers process bytes before errors.",
		"value  :=  source",
	} {
		if !containsQuote(quotes, want) {
			t.Fatalf("quotes do not contain canonical candidate %q: %#v", want, quotes)
		}
	}
}

func TestQuoteCandidatesExtractsSemicolonSeparatedPropositions(t *testing.T) {
	passage := "The cache retains recent records; expired records are removed by the janitor."
	quotes := quoteCandidates(passage)

	for _, want := range []string{"The cache retains recent records", "expired records are removed by the janitor."} {
		if !containsQuote(quotes, want) {
			t.Fatalf("quotes do not contain proposition %q: %#v", want, quotes)
		}
	}
}

func TestQuoteCandidatesSeparatesMarkdownTableCells(t *testing.T) {
	passage := "| Section | Page |\n| --- | ---: |\n| Packaging and tooling | 39 |\n| Mutexes | 145 |"
	quotes := quoteCandidates(passage)

	for _, want := range []string{"Packaging and tooling", "39", "Mutexes", "145", "Section: Packaging and tooling; Page: 39", "Section: Mutexes; Page: 145"} {
		if !containsQuote(quotes, want) {
			t.Fatalf("quotes do not contain table cell %q: %#v", want, quotes)
		}
	}
	if containsQuote(quotes, "---") || containsQuote(quotes, "---:") {
		t.Fatalf("quotes contain table separators: %#v", quotes)
	}
}

func TestQuoteCandidatesAllowsShortScalarTargets(t *testing.T) {
	want := []string{"3.5"}
	if got := quoteCandidates("3.5"); !reflect.DeepEqual(got, want) {
		t.Fatalf("quotes: got %#v want %#v", got, want)
	}
}

func TestQuoteCandidatesRejectsIncompleteProseFragments(t *testing.T) {
	passage := "Development editor:\n\nJennifer Stout\n\nSTEVE FRANCIA GOPHER AND CREATOR OF HUGO,"
	want := []string{"Jennifer Stout"}
	if got := quoteCandidates(passage); !reflect.DeepEqual(got, want) {
		t.Fatalf("quotes: got %#v want %#v", got, want)
	}
}

func TestQuoteCandidatesSeparatesMarkdownQuotations(t *testing.T) {
	passage := "- \"There are two kinds of software projects: those that fail, and those that turn into legacy horrors.\" - Peter Weinberger (inventor of AWK)\n- \"Legacy software is an unappreciated but serious problem. Legacy code may be the downfall of our civilization.\" - Chuck Moore (inventor of Forth)"
	want := []string{
		`"There are two kinds of software projects: those that fail, and those that turn into legacy horrors." - Peter Weinberger (inventor of AWK)`,
		`"Legacy software is an unappreciated but serious problem. Legacy code may be the downfall of our civilization." - Chuck Moore (inventor of Forth)`,
	}

	if got := quoteCandidates(passage); !reflect.DeepEqual(got, want) {
		t.Fatalf("quotes: got %#v want %#v", got, want)
	}
}

func TestQuoteCandidatesKeepsCompleteMultiSentenceQuotation(t *testing.T) {
	passage := `- "Make it correct, make it clear, make it concise, make it fast. In that order." Wes Dyer`
	want := []string{`"Make it correct, make it clear, make it concise, make it fast. In that order." Wes Dyer`}

	if got := quoteCandidates(passage); !reflect.DeepEqual(got, want) {
		t.Fatalf("quotes: got %#v want %#v", got, want)
	}
}

func TestQuoteCandidatesPreservesNestedQuotesAndApostrophes(t *testing.T) {
	passages := map[string]string{
		`- "Good engineering is less about finding the "perfect" solution and more about understanding tradeoffs." - JBD`: `"Good engineering is less about finding the "perfect" solution and more about understanding tradeoffs." - JBD`,
		`- “It’s important to ask questions. That’s how we learn.” - Ada`:                                                 `“It’s important to ask questions. That’s how we learn.” - Ada`,
	}
	for passage, want := range passages {
		if got := quoteCandidates(passage); !reflect.DeepEqual(got, []string{want}) {
			t.Fatalf("quotes: got %#v want %#v", got, []string{want})
		}
	}
}

func TestQuoteCandidatesRejectsUnbalancedOpeningQuotation(t *testing.T) {
	for _, passage := range []string{
		`"An incomplete straight quotation.`,
		`“An incomplete curly quotation.`,
	} {
		if got := quoteCandidates(passage); len(got) != 0 {
			t.Fatalf("quotes for %q: got %#v want none", passage, got)
		}
	}
}

func TestQuestionAnswerShapeRequiresAtomicRequestedValues(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		target string
		valid  bool
	}{
		{name: "percentage", prompt: "What percentage improved?", target: "88%", valid: true},
		{name: "percentage written as words", prompt: "What percent was recycled?", target: "15 percent", valid: true},
		{name: "percentage with context", prompt: "What percentage improved?", target: "It improved 88% after processing 6960 requests.", valid: false},
		{name: "written percentage with context", prompt: "What percent was recycled?", target: "The paper was 15 percent recycled.", valid: false},
		{name: "Boolean zero value", prompt: "What is the zero value for Boolean?", target: "false", valid: true},
		{name: "Boolean table", prompt: "What is the zero value for Boolean?", target: "Boolean false Integer 0", valid: false},
		{name: "string argument", prompt: "What string argument is passed?", target: `"active"`, valid: true},
		{name: "string argument call", prompt: "What string argument is passed?", target: `run("active")`, valid: false},
		{name: "prefix string", prompt: "What is the prefix string assigned to the Trace logger?", target: `"TRACE: "`, valid: true},
		{name: "prefix string with comma", prompt: "What is the prefix string assigned to the Trace logger?", target: `"TRACE: ",`, valid: false},
		{name: "function name", prompt: "What function is called when processing completes?", target: "Display", valid: true},
		{name: "focused function sentence", prompt: "What function is called when processing completes?", target: "The Display function is called.", valid: true},
		{name: "function declaration", prompt: "What function is called when processing completes?", target: "func Display(results chan *Result) {", valid: false},
		{name: "struct type name", prompt: "What is the name of the struct type used for the hash table?", target: "Table", valid: true},
		{name: "struct declaration instead of name", prompt: "What is the name of the struct type used for the hash table?", target: "type Table struct { buckets int }", valid: false},
		{name: "requested struct definition", prompt: "Which struct definition contains the bucket count?", target: "type Table struct { buckets int }", valid: true},
		{name: "package name", prompt: "What is the name of the package containing counters.go?", target: "counters", valid: true},
		{name: "focused package sentence", prompt: "What is the name of the package containing counters.go?", target: "The file lives in package counters.", valid: true},
		{name: "source line residue", prompt: "What value is assigned to checkMark?", target: `09 const checkMark = "\u2713"`, valid: false},
		{name: "assignment instead of value", prompt: "What value is assigned to timeout?", target: "const timeout = 3 * time.Second", valid: false},
		{name: "assigned expression", prompt: "What value is assigned to timeout?", target: "3 * time.Second", valid: true},
		{name: "document page", prompt: "What constant is associated with L on page 189?", target: "Ldate", valid: false},
		{name: "document page range", prompt: "What page range covers imports?", target: "43-44", valid: false},
		{name: "document section number", prompt: "Which section number explains imports?", target: "Section 4", valid: false},
		{name: "document subsection", prompt: "Which subsection can I find the cache example in?", target: "Caching", valid: false},
		{name: "generated passage label", prompt: "According to Passage 3, what does the cache store?", target: "Responses", valid: false},
		{name: "subject section", prompt: "Which section of the API handles authentication?", target: "The security section handles authentication.", valid: true},
		{name: "answer revealing confirmation", prompt: "Is the default duration 1 second?", target: "1 second", valid: false},
		{name: "ordinary answer wording", prompt: "What is the default duration?", target: "1 second", valid: true},
		{name: "focused sentence", prompt: "Who are the three guides?", target: "The guides are Bill, Brian, and Erik.", valid: true},
		{name: "ordinary explanation", prompt: "What do channels carry?", target: "Channels carry values between goroutines.", valid: true},
		{name: "focused flag", prompt: "What flag enables GC pacer details?", target: "gcpacertrace=1", valid: true},
		{name: "flag with explanation", prompt: "What flag enables GC pacer details?", target: "Add the gcpacertrace=1 flag for details.", valid: false},
		{name: "annotation", prompt: "What call fixes the race?", target: "atomic.AddInt32(&found, 1) <-- CHANGED", valid: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRecordShape(GoalQuestionAnswer, tt.prompt, tt.target)
			if (err == nil) != tt.valid {
				t.Fatalf("validateRecordShape() error = %v, valid = %v", err, tt.valid)
			}
		})
	}
}

func TestValidateReviewedRecordsNormalizesEvidence(t *testing.T) {
	records, err := ValidateReviewedRecords([]Record{{
		Prompt:   "What is the cache order?",
		Target:   "L1 -&gt; L2.",
		Evidence: "L1 -&gt; L2.",
	}})
	if err != nil {
		t.Fatalf("ValidateReviewedRecords() error = %v", err)
	}
	if got, want := records[0].Target, "L1 -> L2."; got != want {
		t.Fatalf("target = %q, want %q", got, want)
	}
	if got, want := records[0].Evidence, "L1 -> L2."; got != want {
		t.Fatalf("evidence = %q, want %q", got, want)
	}
}

func TestValidateReviewedRecordsCanonicalizesOldProseTarget(t *testing.T) {
	records, err := ValidateReviewedRecords([]Record{{
		Prompt:   "Which tool runs Go tests?",
		Target:   " The  go test  tool is used to run tests.",
		Evidence: "-  The  go test  tool is used to run tests.",
		FactID:   "old-fact-id",
	}})
	if err != nil {
		t.Fatalf("ValidateReviewedRecords() error = %v", err)
	}
	if got, want := records[0].Target, "The go test tool is used to run tests."; got != want {
		t.Fatalf("target = %q, want %q", got, want)
	}
	if records[0].FactID == "old-fact-id" || records[0].FactID == "" {
		t.Fatalf("fact ID was not refreshed: %q", records[0].FactID)
	}
}

func containsQuote(quotes []string, want string) bool {
	return slices.Contains(quotes, want)
}
