package prepare

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
)

type sourceReferenceRetryCompleter struct {
	requests []CompletionRequest
}

func (c *sourceReferenceRetryCompleter) Complete(_ context.Context, request CompletionRequest) (Completion, error) {
	c.requests = append(c.requests, request)
	prompt := request.Messages[1].Content
	switch {
	case strings.Contains(prompt, "[RECORD 1]"):
		return Completion{Content: `{"record_1":"pass"}`}, nil
	case strings.Contains(prompt, "unavailable source context"):
		return Completion{Content: `{"record_1":"What process leads to improvement through writing and thinking about code?"}`}, nil
	default:
		return Completion{Content: `{"record_1":"What is the source of improvement according to the evidence?"}`}, nil
	}
}

func TestQuestionPoolRetainsReviewedQuestionAndLockedFact(t *testing.T) {
	record := Record{
		Prompt: "Which signal interrupts a program during CPU profiling?", Target: "SIGPROF",
		Evidence: "The profiler uses SIGPROF to interrupt the program.", FactID: "fact-1",
	}
	pools := []generatedQuestionPool{{
		RecordNumber: 1,
		Questions: []string{
			"During processor sampling, what signal pauses the Go program?",
			"A profiler needs to inspect a running process; which signal does it send?",
			"What operating-system signal transfers execution to a CPU profiler?",
			"Name the signal used for periodic profiling interrupts?",
			"When CPU profiling is active, which signal suspends the target process?",
			"What signal lets a sampling profiler capture the program counter?",
		},
	}}
	selected := selectQuestionPools([]Record{record}, pools, nil)
	if len(selected) != questionAnswerRecordsPerFact {
		t.Fatalf("selected records = %d, want %d", len(selected), questionAnswerRecordsPerFact)
	}
	if selected[0].Prompt != record.Prompt {
		t.Fatalf("reviewed question changed from %q to %q", record.Prompt, selected[0].Prompt)
	}
	for _, variant := range selected {
		if variant.Target != record.Target || variant.Evidence != record.Evidence {
			t.Fatalf("locked fact changed: %+v", variant)
		}
		if variant.ValidationPrompt || variant.IndependentPrompt {
			t.Fatalf("training record has an evaluation role: %+v", variant)
		}
	}
}

func TestQuestionPoolPromptRequestsOneGroundedQuestion(t *testing.T) {
	reviewed := Record{Prompt: "Which signal interrupts a program?", Target: "SIGPROF", Evidence: "The profiler uses SIGPROF to interrupt the program.", FactID: "fact-1"}
	prompt := questionPoolMessages([]Record{reviewed}, "")[1].Content
	for _, instruction := range []string{
		"exactly one natural question",
		"LOCKED FACT 1 uses record_1",
		"concise, direct question",
		"describe what the code does without repeating words contained in that identifier",
		"Match the question's scope to the complete target",
		"Name the subject needed to understand the question",
		"this program, this function, this type, this example, it, or they",
		"answer type, direction, scope, code-symbol role, package, and capitalization",
		"registering handlers, for example, does not start a server",
		"FINAL QUESTION CHECK — REQUIRED",
		"A user style request for a large, big, detailed, or contextual question never overrides this standalone requirement",
	} {
		if !strings.Contains(prompt, instruction) {
			t.Fatalf("prompt does not contain %q", instruction)
		}
	}
}

func TestQuestionPoolFailureExplainsUnavailableSourceReference(t *testing.T) {
	fact := Record{Target: "Improvement comes from writing code and thinking about the code I write."}
	pools := []generatedQuestionPool{{RecordNumber: 1, Questions: []string{"What is the source of improvement according to the evidence?"}}}
	reason := questionPoolFailureReason([]Record{fact}, pools, nil)
	if !strings.Contains(reason, "according to the evidence") || !strings.Contains(reason, "unavailable source context") {
		t.Fatalf("failure reason = %q, want the rejected question and actionable cause", reason)
	}
}

func TestQuestionPoolRetryUsesDeterministicFailureFeedback(t *testing.T) {
	model := &sourceReferenceRetryCompleter{}
	previewer, err := NewPreviewer(model)
	if err != nil {
		t.Fatal(err)
	}
	fact := Record{
		Target:   "Improvement comes from writing code and thinking about the code I write.",
		Evidence: "Improvement comes from writing code and thinking about the code I write.",
		FactID:   "fact-1",
	}
	accepted, rejected, metrics, err := previewer.preparePlannedQuestionAnswerBatch(t.Context(), FullBuildRequest{
		Goal: GoalQuestionAnswer, Model: "model", DatasetInstructions: "Create big questions",
	}, []Record{fact}, generationExclusions{}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if len(accepted) != 1 || len(rejected) != 0 || metrics.generationAttempts != 2 || metrics.verificationAttempts != 1 {
		t.Fatalf("accepted=%+v rejected=%+v metrics=%+v", accepted, rejected, metrics)
	}
	if len(model.requests) != 3 || !strings.Contains(model.requests[1].Messages[1].Content, "unavailable source context") {
		t.Fatalf("retry did not receive the deterministic rejection: %+v", model.requests)
	}
}

func TestQuestionPoolSchemaMapsEveryFactToARequiredField(t *testing.T) {
	schema := questionPoolSchema(3)
	properties, ok := schema["properties"].(map[string]any)
	if !ok || len(properties) != 3 {
		t.Fatalf("question properties = %#v, want three explicit fields", schema["properties"])
	}
	required, ok := schema["required"].([]string)
	if !ok || !slices.Equal(required, []string{"record_1", "record_2", "record_3"}) {
		t.Fatalf("required question fields = %#v", schema["required"])
	}
	if _, fragileArray := properties["pools"]; fragileArray {
		t.Fatalf("question schema retained the flattened pool array: %#v", properties)
	}

	flattened, err := json.Marshal(map[string]any{
		"pools": []map[string]any{{"record_number": 1, "questions": []string{"Question one?", "Question two?"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeQuestionPools(string(flattened), 2); err == nil {
		t.Fatal("flattened question array decoded as a valid per-fact response")
	}

	partial, err := json.Marshal(map[string]string{"record_2": "Question two?"})
	if err != nil {
		t.Fatal(err)
	}
	pools, err := decodeQuestionPools(string(partial), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(pools) != 1 || pools[0].RecordNumber != 2 || !slices.Equal(pools[0].Questions, []string{"Question two?"}) {
		t.Fatalf("partially decoded question pools = %+v", pools)
	}
}

func TestQuestionPoolCustomInstructionsReplaceDefaultStyles(t *testing.T) {
	reviewed := Record{Prompt: "Which signal interrupts a program?", Target: "SIGPROF", Evidence: "The profiler uses SIGPROF to interrupt the program.", FactID: "fact-1"}
	messages := questionPoolMessages([]Record{reviewed}, "Create big questions with detailed technical context.")
	for _, defaultStyle := range []string{"concise and direct", "realistic scenario", "conversational"} {
		if strings.Contains(messages[1].Content, defaultStyle) {
			t.Fatalf("custom question pool retained conflicting default style %q: %s", defaultStyle, messages[1].Content)
		}
	}
	for _, requirement := range []string{"Create big questions with detailed technical context.", "Write exactly one natural question"} {
		if !strings.Contains(messages[1].Content, requirement) {
			t.Fatalf("custom question pool omitted %q: %s", requirement, messages[1].Content)
		}
	}
	if !strings.HasPrefix(messages[1].Content, "USER INSTRUCTION — FOLLOW WHEN WRITING EVERY RECORD:\nCreate big questions with detailed technical context.") {
		t.Fatalf("custom question pool did not begin with the exact user instruction: %s", messages[1].Content)
	}
	if !strings.Contains(messages[0].Content, "user's instruction controls how every question is written") {
		t.Fatalf("question pool system prompt did not prioritize custom requirements: %s", messages[0].Content)
	}
}

func TestQuestionAnswerFactBatchesIncludeCustomInstructionsInPromptBudget(t *testing.T) {
	facts := make([]Record, 20)
	for index := range facts {
		facts[index] = Record{
			Target:   fmt.Sprintf("Fact %d describes supported behavior.", index+1),
			Evidence: strings.Repeat(fmt.Sprintf("Evidence %d supports the behavior. ", index+1), 18),
			FactID:   fmt.Sprintf("fact-%d", index+1),
		}
	}
	instructions := strings.Repeat("Use detailed technical context. ", 60)
	batches := questionAnswerFactBatches(facts, instructions)
	if len(batches) < 2 {
		t.Fatalf("batches = %d, want prompt-budget splitting", len(batches))
	}
	count := 0
	for _, batch := range batches {
		count += len(batch)
		if size := len(questionPoolMessages(batch, instructions)[1].Content); size > questionPoolBatchBytes {
			t.Fatalf("batch prompt = %d bytes, exceeds %d-byte budget", size, questionPoolBatchBytes)
		}
	}
	if count != len(facts) {
		t.Fatalf("batched facts = %d, want %d", count, len(facts))
	}
}

func TestQuestionAnswerFactBatchesShareEvidenceWithoutReducingFactCapacity(t *testing.T) {
	facts := make([]Record, 32)
	for index := range facts {
		chunk := index / 8
		facts[index] = Record{
			Target:   fmt.Sprintf("Fact %d describes supported behavior.", index+1),
			Evidence: strings.Repeat(fmt.Sprintf("Chunk %d supplies shared source context. ", chunk+1), 42),
			FactID:   fmt.Sprintf("fact-%d", index+1),
		}
	}

	batches := questionAnswerFactBatches(facts, "")
	if len(batches) != 2 || len(batches[0]) != maximumFactsPerBatch || len(batches[1]) != maximumFactsPerBatch {
		t.Fatalf("shared-evidence batch count = %d, want two full %d-fact batches", len(batches), maximumFactsPerBatch)
	}
	prompt := questionPoolMessages(batches[0], "")[1].Content
	for _, source := range []string{facts[0].Evidence, facts[8].Evidence} {
		if count := strings.Count(prompt, strings.TrimSpace(source)); count != 1 {
			t.Fatalf("shared evidence appears %d times, want once", count)
		}
	}
	if strings.Count(prompt, "[LOCKED FACT ") != maximumFactsPerBatch || strings.Count(prompt, "EVIDENCE BLOCK: ") != maximumFactsPerBatch {
		t.Fatalf("prompt lost fact-to-evidence references: %s", prompt)
	}
}

func TestQuestionPoolRetryNamesQuestionsToAvoid(t *testing.T) {
	fact := Record{Target: "SIGPROF", Evidence: "The profiler uses SIGPROF to interrupt the program.", FactID: "fact-1"}
	prompt := questionPoolMessagesExcluding(
		[]Record{fact},
		"",
		[]string{"Which signal interrupts the program?"},
		[]VerificationResult{{ExpectedTarget: "SIGPROF", Reason: "The question reveals its answer."}},
	)[1].Content
	for _, text := range []string{
		"[QUESTIONS TO AVOID — WRITE DIFFERENT QUESTIONS]",
		"- Which signal interrupts the program?",
		"[VERIFICATION FEEDBACK — CORRECT THESE FAILURES]",
		`Locked target "SIGPROF" was rejected: The question reveals its answer.`,
	} {
		if !strings.Contains(prompt, text) {
			t.Fatalf("retry prompt does not contain %q: %s", text, prompt)
		}
	}
}

func TestQuestionPoolSelectsDiverseQuestionsAcrossAllGeneratedForms(t *testing.T) {
	fact := Record{Prompt: "Which signal interrupts a program?", Target: "SIGPROF", Evidence: "The profiler uses SIGPROF to interrupt the program.", FactID: "fact-1"}
	repetitiveForms := []string{
		"Which signal interrupts a program during CPU profiling?",
		"Which signal interrupts the program while CPU profiling runs?",
		"What signal interrupts a program for CPU profiling?",
		"During CPU profiling, which signal interrupts the program?",
	}
	pools := []generatedQuestionPool{{RecordNumber: 1, Questions: append(repetitiveForms, []string{
		"A profiler needs to inspect a running Go service; what operating-system signal should it send?",
		"What mechanism triggers periodic processor samples in a Go process?",
		"Name the operating-system notification used to collect a CPU profile?",
		"What signal lets a CPU profiler sample an active Go process?",
		"During runtime profiling, which signal triggers sampling?",
		"Which operating-system signal supports periodic CPU samples?",
	}...)}}

	selected := selectQuestionPools([]Record{fact}, pools, nil)
	if len(selected) != questionAnswerRecordsPerFact {
		t.Fatalf("selected records = %d, want a complete question group", len(selected))
	}
	retainedRepetitive := 0
	for _, form := range repetitiveForms {
		if slices.ContainsFunc(selected, func(record Record) bool { return record.Prompt == form }) {
			retainedRepetitive++
		}
	}
	if retainedRepetitive == len(repetitiveForms) {
		t.Fatalf("selected questions blindly retained the first four repetitive forms: %+v", selected)
	}
}

func TestQuestionSimilarityCanaryDetectsRepeatedAnswerCues(t *testing.T) {
	if similarity := maximumQuestionSimilarity("Does the guarantee terminate?", []string{"Is termination guaranteed?"}); similarity != 1 {
		t.Fatalf("inflectional question similarity = %.4f, want 1", similarity)
	}

	successful := []string{
		"I'm observing this program run; what is the one thing I can rely on regarding its completion?",
		"If you execute this specific program, what is the single certainty about when it will stop?",
		"What condition must be met for this program to terminate?",
		"What is the only guarantee regarding program termination in the concurrent program described?",
		"When this program finishes, what is the only rule that was strictly followed?",
		"What is the sole guarantee provided by the design of this program?",
	}
	looping := []string{
		"If you are running this program, what can you be certain of regarding when the program will terminate?",
		"In terms of program termination, what is the only thing guaranteed in this specific program?",
		"What is the definitive rule about when this program will terminate?",
		"What is the one thing that is guaranteed about the program's termination?",
		"What is the only guarantee regarding program termination when running the program with two Goroutines?",
		"What is the sole guarantee concerning the termination of this program and its Goroutines?",
	}
	maximumSimilarity := func(questions []string) float64 {
		maximum := 0.0
		for index, question := range questions {
			maximum = max(maximum, maximumQuestionSimilarity(question, questions[:index]))
		}
		return maximum
	}

	successfulSimilarity := maximumSimilarity(successful)
	loopingSimilarity := maximumSimilarity(looping)
	if successfulSimilarity > .2 || loopingSimilarity < .3 || loopingSimilarity <= successfulSimilarity {
		t.Fatalf("question cue similarities: successful %.4f, looping %.4f", successfulSimilarity, loopingSimilarity)
	}
}

func TestQuestionPoolUsesBestAvailableSynonymCandidates(t *testing.T) {
	fact := Record{Target: "25%", Evidence: "Past 1024 elements, slice capacity grows by 25%.", FactID: "fact-1"}
	pools := []generatedQuestionPool{{RecordNumber: 1, Questions: []string{
		"What is the slice growth rate after 1024 elements?",
		"What is the backing slice growth rate beyond 1024 elements?",
		"After 1024 elements, what is the slice capacity growth rate?",
		"Beyond 1024 elements, what growth rate does the slice use?",
		"What growth rate applies to a slice after 1024 elements?",
		"For a slice beyond 1024 elements, what is its growth rate?",
	}}}

	if selected := selectQuestionPools([]Record{fact}, pools, nil); len(selected) != questionAnswerRecordsPerFact {
		t.Fatalf("selected records = %d, want the best %d questions", len(selected), questionAnswerRecordsPerFact)
	}
}

func TestQuestionSelectionUsesAnswerLeakageOnlyAsDiversityTieBreaker(t *testing.T) {
	candidates := []string{
		"Which vector type uses an underlying slice?",
		"Which generic type uses an underlying slice?",
	}
	selected := selectDistinctQuestions(nil, candidates, 1, "vector")
	if len(selected) != 1 || selected[0] != candidates[1] {
		t.Fatalf("selected questions = %q, want non-revealing candidate for equal diversity", selected)
	}

	existing := []string{"Which generic type uses an underlying slice?"}
	candidates = []string{
		"What does vector represent?",
		"Which parameterized collection is backed by a slice?",
	}
	selected = selectDistinctQuestions(existing, candidates, 1, "vector")
	if len(selected) != 1 || selected[0] != candidates[0] {
		t.Fatalf("selected questions = %q, want v34 diversity to take priority", selected)
	}
}

func TestQuestionPoolUsesAnswerRevealingQuestionsAsFallback(t *testing.T) {
	fact := Record{Target: "vector", Evidence: "The generic vector type uses an underlying slice.", FactID: "fact-1"}
	questions := []string{
		"What does the vector type represent?",
		"Which behavior does vector provide?",
		"How is vector defined?",
		"What storage does vector use?",
		"Which data type underlies vector?",
		"What element constraint does vector apply?",
	}
	selected := selectQuestionPools([]Record{fact}, []generatedQuestionPool{{RecordNumber: 1, Questions: questions}}, nil)
	if len(selected) != questionAnswerRecordsPerFact {
		t.Fatalf("selected records = %d, want fallback to preserve a complete question group", len(selected))
	}
}

func TestQuestionPoolPrefersCompleteGroupWithoutAtomicAnswerLeakage(t *testing.T) {
	fact := Record{Target: "SIGPROF", Evidence: "The profiler uses SIGPROF to interrupt the program.", FactID: "fact-1"}
	questions := []string{
		"Which signal interrupts a program during CPU profiling?",
		"While CPU profiling samples a process, what operating-system signal does it use?",
		"A profiler needs to interrupt a Go service for a sample; what signal should it send?",
		"Does Go profiling use SIGPROF when it samples a program?",
		"What signal lets a CPU profiler sample an active Go process?",
		"Name the signal responsible for Go CPU-profiling interrupts?",
		"Which signal pauses execution for processor sampling?",
	}
	selected := selectQuestionPools([]Record{fact}, []generatedQuestionPool{{RecordNumber: 1, Questions: questions}}, nil)
	if len(selected) != questionAnswerRecordsPerFact {
		t.Fatalf("selected records = %d, want a complete question group", len(selected))
	}
	if slices.ContainsFunc(selected, func(record Record) bool { return strings.Contains(record.Prompt, fact.Target) }) {
		t.Fatalf("selected questions reveal atomic target: %+v", selected)
	}
}

func TestQuestionAnswerTargetPreferencesNeverRemoveFallbacks(t *testing.T) {
	candidates := [][]string{{"36.2%", "SIGPROF", "CPU profiling periodically samples a running program."}}
	passages := []string{"A benchmark reported that a function was 36.2% faster. CPU profiling periodically samples a running program using SIGPROF."}
	prioritizeQuestionAnswerTargets(candidates, passages, []string{"SIGPROF"})
	want := []string{"CPU profiling periodically samples a running program.", "SIGPROF", "36.2%"}
	if !slices.Equal(candidates[0], want) {
		t.Fatalf("ranked candidates = %q, want %q", candidates[0], want)
	}

	prompt := questionAnswerTargetPrompt(candidates, nil, []string{"SIGPROF"}, "")
	for _, text := range []string{"these are preferences", "reuse the best available target", "[PREVIOUSLY SELECTED TARGETS", "- SIGPROF"} {
		if !strings.Contains(prompt, text) {
			t.Fatalf("target prompt does not preserve fallback instruction %q", text)
		}
	}
}

func TestBuildFingerprintIncludesDatasetInstructions(t *testing.T) {
	request := FullBuildRequest{Goal: GoalQuestionAnswer, Model: "model", DatasetInstructions: "Prefer beginner questions."}
	first, err := buildFingerprint([]byte("source"), request, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.DatasetInstructions = "Prefer advanced questions."
	second, err := buildFingerprint([]byte("source"), request, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("changing dataset instructions did not invalidate the build checkpoint fingerprint")
	}
}

func TestQuestionAnswerTargetPromptAlignsAnswerScope(t *testing.T) {
	prompt := questionAnswerTargetPrompt([][]string{{"Routes", "The Routes function registers the sendJSON handler."}}, nil, nil, "")
	for _, instruction := range []string{
		"minimal, non-overlapping set",
		"Do not select every sentence or every candidate",
		"every passage with eligible targets must select at least one",
		"complete answer users will be trained to return",
		"prefer that atomic answer over a sentence containing it",
		"incidental package name, declaration, heading, biography, forward reference, page number, or example detail",
		"behavior, comparison, condition, relationship, or effect",
	} {
		if !strings.Contains(prompt, instruction) {
			t.Fatalf("target prompt does not contain %q", instruction)
		}
	}
}

func TestQuestionAnswerTargetPromptBoundsPreviousTargets(t *testing.T) {
	previous := make([]string, 40)
	for index := range previous {
		previous[index] = fmt.Sprintf("target-%02d", index)
	}
	prompt := questionAnswerTargetPrompt([][]string{{"A supported fact."}}, nil, previous, "")
	if strings.Contains(prompt, "- target-07\n") || !strings.Contains(prompt, "- target-08\n") || !strings.Contains(prompt, "- target-39\n") {
		t.Fatalf("previous-target window is not the most recent %d targets: %s", maxRenderedPreviousTargets, prompt)
	}
	if count := strings.Count(prompt, "\n- target-"); count != maxRenderedPreviousTargets {
		t.Fatalf("rendered previous targets = %d, want %d", count, maxRenderedPreviousTargets)
	}
}

func TestCustomInstructionsControlAnswerDetailDefaults(t *testing.T) {
	const instructions = "Use detailed answers that explain the complete supported behavior."
	previewPrompt := previewUserPrompt(GoalQuestionAnswer, 1, "[PASSAGE 1]\nThe complete supported behavior.", instructions, generationExclusions{})
	if strings.Contains(previewPrompt, "Prefer the shortest atomic") || !strings.Contains(previewPrompt, "Select only a grounded target") {
		t.Fatalf("preview prompt retained the shortest-answer default: %s", previewPrompt)
	}
	targetPrompt := questionAnswerTargetPrompt([][]string{{"The complete supported behavior."}}, nil, nil, instructions)
	if strings.Contains(targetPrompt, "Choose the shortest target") || strings.Contains(targetPrompt, "prefer that atomic answer") || !strings.Contains(targetPrompt, "scope and level of detail by following the user instruction") {
		t.Fatalf("target-selection prompt retained the shortest-answer default: %s", targetPrompt)
	}
	if !strings.HasPrefix(targetPrompt, "USER INSTRUCTION — FOLLOW WHEN WRITING EVERY RECORD:\n"+instructions) {
		t.Fatalf("target-selection prompt did not begin with the exact user instruction: %s", targetPrompt)
	}

	defaultPreview := previewUserPrompt(GoalQuestionAnswer, 1, "[PASSAGE 1]\nThe complete supported behavior.", "", generationExclusions{})
	defaultTarget := questionAnswerTargetPrompt([][]string{{"The complete supported behavior."}}, nil, nil, "")
	if !strings.Contains(defaultPreview, "Prefer the shortest atomic") || !strings.Contains(defaultTarget, "Choose the shortest target") {
		t.Fatal("default generation lost the established concise-answer behavior")
	}
}

func TestQuestionAnswerTargetSelectionCanSkipUnusablePassage(t *testing.T) {
	passages := []string{
		"3.6 Value Semantic Iteration 3.8 Data Semantic Guideline For Built-In",
		"The profiler uses SIGPROF to interrupt the program.",
	}
	candidates := questionAnswerTargetCandidates(passages)
	records, err := validateQuestionAnswerTargets([][]int{{}, {1}}, passages, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Evidence != passages[1] {
		t.Fatalf("selected records = %+v", records)
	}
	prompt := questionAnswerTargetPrompt(candidates, nil, nil, "")
	if !strings.Contains(prompt, "an empty array for navigation, tables of contents, malformed tables") {
		t.Fatalf("target selection prompt cannot skip unusable passages: %s", prompt)
	}
}

func TestQuestionAnswerTargetSelectionRequiresOneArrayPerPassage(t *testing.T) {
	passages := []string{"Table of contents", "Copyright page"}
	if _, err := validateQuestionAnswerTargets(nil, passages, [][]string{nil, nil}); err == nil {
		t.Fatal("top-level empty response accepted without per-passage arrays")
	}
	records, err := validateQuestionAnswerTargets([][]int{{}, {}}, passages, [][]string{nil, nil})
	if err != nil || len(records) != 0 {
		t.Fatalf("per-passage empty selections = %+v, %v", records, err)
	}
	if _, err := validateQuestionAnswerTargets([][]int{{}, {1, 1}}, passages, [][]string{nil, {"A supported fact."}}); err == nil {
		t.Fatal("duplicate target ID accepted")
	}
	prompt := formatQuestionAnswerTargetPassages([][]string{nil})
	if !strings.Contains(prompt, "RETURN AN EMPTY ARRAY FOR THIS PASSAGE") || strings.Contains(prompt, "RETURN 0") {
		t.Fatalf("empty-passage instruction = %q", prompt)
	}
}

func TestPreviewProgressNeverExceedsRequestedRecords(t *testing.T) {
	var got PreviewProgress
	reportPreviewProgress(func(progress PreviewProgress) { got = progress }, 7, 5, "Generating preview records")
	if got.CompletedRecords != 5 || got.TotalRecords != 5 {
		t.Fatalf("preview progress = %+v", got)
	}
}

func TestQuestionAnswerTargetSelectionRejectsContextDependentTarget(t *testing.T) {
	for _, passage := range []string{
		"This test was not required.",
		"It's using the io.Reader interface.",
	} {
		passages := []string{passage}
		if _, err := validateQuestionAnswerTargets([][]int{{1}}, passages, questionAnswerTargetCandidates(passages)); err == nil {
			t.Fatalf("context-dependent target %q passed validation", passage)
		}
	}

	passages := []string{"The profiler uses SIGPROF to interrupt the program."}
	if _, err := validateQuestionAnswerTargets([][]int{{1}}, passages, questionAnswerTargetCandidates(passages)); err != nil {
		t.Fatalf("standalone target failed validation: %v", err)
	}
}

func TestQuestionAnswerTargetCandidatesRemoveNarrativeAnswers(t *testing.T) {
	passages := []string{
		"I've just implemented a new API for this example.",
		"What I need then is a mutex.",
		"I need to make it a leaf function.",
		"In this chapter, we will explore profiling programs.",
	}
	want := [][]string{nil, {"mutex"}, nil, nil}
	got := questionAnswerTargetCandidates(passages)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("target candidates = %#v, want %#v", got, want)
	}
	prompt := formatQuestionAnswerTargetPassages(got)
	if strings.Contains(prompt, passages[0]) || strings.Contains(prompt, passages[1]) || !strings.Contains(prompt, "[NO USABLE TARGETS: RETURN AN EMPTY ARRAY FOR THIS PASSAGE]") || !strings.Contains(prompt, "[TARGET 1] mutex") {
		t.Fatalf("target prompt exposes narrative answers or omits the usable atomic answer: %s", prompt)
	}
}

func TestQuestionAnswerTargetCandidatesPreserveStandaloneFacts(t *testing.T) {
	passages := []string{
		"The profiler uses SIGPROF to interrupt the program.",
		"Sharing the user value up the call stack causes it to escape.",
		"```\nfunc displayName(user *User) string\n*inc++\n```",
	}
	candidates := questionAnswerTargetCandidates(passages)
	want := [][]string{
		{"SIGPROF"},
		{"Sharing the user value up the call stack causes it to escape."},
		{"displayName", "*inc++"},
	}
	for passageIndex, targets := range want {
		for _, target := range targets {
			if !slices.Contains(candidates[passageIndex], target) {
				t.Fatalf("passage %d candidates %q do not contain %q", passageIndex+1, candidates[passageIndex], target)
			}
		}
	}
}

func TestQuestionPromptsPreserveCodeSymbolProvenance(t *testing.T) {
	passages := []string{"func (e *errorString) Error() string"}
	targetPrompt := questionAnswerTargetPrompt(questionAnswerTargetCandidates(passages), nil, nil, "")
	for _, instruction := range []string{"exact role and capitalization", "method name must not become a type", "Do not merge facts from alternative packages or APIs"} {
		if !strings.Contains(targetPrompt, instruction) {
			t.Fatalf("target selection prompt does not contain %q", instruction)
		}
	}

	record := Record{Prompt: "Which method returns an error string?", Target: "Error", Evidence: "func (e *errorString) Error() string", FactID: "fact-1"}
	poolPrompt := questionPoolMessages([]Record{record}, "")[1].Content
	for _, instruction := range []string{"code-symbol role, package, and capitalization", "combine alternate APIs"} {
		if !strings.Contains(poolPrompt, instruction) {
			t.Fatalf("question pool prompt does not contain %q", instruction)
		}
	}
}

func TestNaturalQuestionRejectsPreparationContext(t *testing.T) {
	for _, prompt := range []string{
		"Rewrite the question conversationally: What is a package?",
		"How is the grammar and information order substantially changed in the question: What synchronization primitive is needed?",
		"Which package appears in the provided Go code?",
		"What is the function defined in the source code?",
		"According to the source, which signal is sent?",
		"According to the evidence, which signal is sent?",
		"Which guideline appears in the provided documentation?",
		"What happens in Listing 4?",
		"In the provided orchestration example, which goroutine runs first?",
		"According to Table 12.2, what value is listed?",
		"How many bytes does each character use in the provided string example?",
		"How much faster was numCPU in the provided test output?",
		"In the example program, what timeout was configured?",
		"How much CPU utilization was shown in the second time command output?",
		"How much faster was numCPU than the others in this batch?",
		"How many times does the text recommend repeating the benchmark?",
		"What function appears in the first passage of the second source block?",
		"What change is described in the second target?",
		"Which processor is named in the third passage?",
	} {
		if err := validateNaturalQuestion(prompt); err == nil {
			t.Fatalf("question %q passed natural wording validation", prompt)
		}
	}
	if err := validateNaturalQuestion("Which signal interrupts a program while CPU profiling is active?"); err != nil {
		t.Fatalf("natural question failed validation: %v", err)
	}
	if err := validateNaturalQuestion("Provided that the cache is warm, which operation runs next?"); err != nil {
		t.Fatalf("self-contained conditional question failed validation: %v", err)
	}
	if err := validateNaturalQuestion("How is a Go package represented in the source tree?"); err != nil {
		t.Fatalf("ordinary source-tree question failed validation: %v", err)
	}
}
