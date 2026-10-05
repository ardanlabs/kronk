package prepare

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
)

const (
	maxVerificationAttempts          = 2
	maxVerificationRecordsPerRequest = 16
	maxVerificationTokens            = 2048
	maxVerificationEvidenceRunes     = 1200
	verificationPass                 = "pass"
	verificationGrounding            = "grounding"
	verificationTaskQuality          = "task_quality"
	verificationUserRequirements     = "user_requirements"
)

// VerificationResult records the semantic grounding decision for one preview record.
type VerificationResult struct {
	RecordNumber   int    `json:"record_number"`
	Passed         bool   `json:"passed"`
	ExpectedTarget string `json:"expected_target"`
	Reason         string `json:"reason"`
}

type generatedVerification struct {
	RecordNumber     int `json:"record_number"`
	ExpectedTargetID int `json:"expected_target_id"`
	FailureCategory  string
}

type replacementHistory struct {
	evidenceIDs map[int]struct{}
	prompts     []string
}

type verificationStructureError struct {
	err error
}

func (err verificationStructureError) Error() string {
	return err.err.Error()
}

func (err verificationStructureError) Unwrap() error {
	return err.err
}

func (p *Previewer) verifyAndRepair(ctx context.Context, request PreviewRequest, passages []string, records []Record, expectedCount int) ([]Record, []VerificationResult, previewMetrics, error) {
	var metrics previewMetrics
	history := replacementHistory{evidenceIDs: make(map[int]struct{})}

	for repair := range maxRepairAttempts {
		verification, verificationMetrics, err := p.verifyRecords(ctx, request.Model, request.Goal, request.DatasetInstructions, records)
		metrics.add(verificationMetrics)
		if err != nil {
			return nil, nil, metrics, err
		}

		failed := failedVerification(verification)
		passedRecords, passedVerification := passingRecords(records, verification)
		if len(passedRecords) >= expectedCount {
			return passedRecords[:expectedCount], passedVerification[:expectedCount], metrics, nil
		}
		if repair == maxRepairAttempts-1 {
			return reviewablePreview(records, verification, expectedCount, metrics)
		}

		exclusions := replacementExclusions(records, passages, failed, &history)
		if repair > 0 {
			releaseFailedEvidence(records, passages, failed, &history)
		}
		availablePassages := filterPassages(passages, history.evidenceIDs)
		needed := expectedCount - len(passedRecords)
		if len(availablePassages) < needed {
			releaseFailedEvidence(records, passages, failed, &history)
			availablePassages = filterPassages(passages, history.evidenceIDs)
		}
		if len(availablePassages) < needed {
			return reviewablePreview(records, verification, expectedCount, metrics)
		}

		candidateCount := min(len(availablePassages), needed*2)
		replacements, generationMetrics, err := p.generateRecords(ctx, request, formatPassages(availablePassages), availablePassages, candidateCount, exclusions, false, maxPreviewGenerationAttempts, len(passedRecords), expectedCount)
		metrics.add(generationMetrics)
		if err != nil && (!isGeneratedRecordsValidationError(err) || len(replacements) < needed) {
			return nil, verification, metrics, err
		}
		records = append(passedRecords, replacements...)
	}

	return nil, nil, metrics, fmt.Errorf("build preview: semantic verification did not complete")
}

func reviewablePreview(records []Record, verification []VerificationResult, expectedCount int, metrics previewMetrics) ([]Record, []VerificationResult, previewMetrics, error) {
	count := min(len(records), len(verification), expectedCount)
	if count == 0 {
		return nil, nil, metrics, recordQualityError{err: fmt.Errorf("build preview: model produced no reviewable records")}
	}
	return records[:count], verification[:count], metrics, nil
}

func passingRecords(records []Record, verification []VerificationResult) ([]Record, []VerificationResult) {
	passedRecords := make([]Record, 0, len(records))
	passedVerification := make([]VerificationResult, 0, len(records))
	for index, result := range verification {
		if !result.Passed {
			continue
		}
		passedRecords = append(passedRecords, records[index])
		result.RecordNumber = len(passedRecords)
		passedVerification = append(passedVerification, result)
	}
	return passedRecords, passedVerification
}

func filterPassages(passages []string, excluded map[int]struct{}) []string {
	available := make([]string, 0, len(passages)-len(excluded))
	for index, passage := range passages {
		if _, skip := excluded[index+1]; !skip {
			available = append(available, passage)
		}
	}
	return available
}

func (p *Previewer) verifyRecords(ctx context.Context, model string, goal Goal, datasetInstructions string, records []Record) ([]VerificationResult, previewMetrics, error) {
	results := make([]VerificationResult, 0, len(records))
	var metrics previewMetrics
	totalBatches := (len(records) + maxVerificationRecordsPerRequest - 1) / maxVerificationRecordsPerRequest
	for start := 0; start < len(records); start += maxVerificationRecordsPerRequest {
		end := min(start+maxVerificationRecordsPerRequest, len(records))
		batch, batchMetrics, err := p.verifyRecordBatch(ctx, model, goal, datasetInstructions, records[start:end])
		metrics.add(batchMetrics)
		if err != nil {
			return nil, metrics, fmt.Errorf("verification batch %d of %d: %w", start/maxVerificationRecordsPerRequest+1, totalBatches, err)
		}
		for index := range batch {
			batch[index].RecordNumber += start
		}
		results = append(results, batch...)
	}
	return results, metrics, nil
}

func (p *Previewer) verifyRecordBatch(ctx context.Context, model string, goal Goal, datasetInstructions string, records []Record) ([]VerificationResult, previewMetrics, error) {
	verification, metrics, err := p.verifyRecordBatchAttempts(ctx, model, goal, datasetInstructions, records)
	if err == nil {
		return verification, metrics, nil
	}
	if _, ok := errors.AsType[verificationStructureError](err); !ok {
		return nil, metrics, err
	}
	if len(records) == 1 {
		return []VerificationResult{{
			RecordNumber:   1,
			ExpectedTarget: records[0].Target,
			Reason:         "The verifier could not return a complete structured result for this record.",
		}}, metrics, nil
	}

	middle := len(records) / 2
	left, leftMetrics, err := p.verifyRecordBatch(ctx, model, goal, datasetInstructions, records[:middle])
	metrics.add(leftMetrics)
	if err != nil {
		return nil, metrics, err
	}
	right, rightMetrics, err := p.verifyRecordBatch(ctx, model, goal, datasetInstructions, records[middle:])
	metrics.add(rightMetrics)
	if err != nil {
		return nil, metrics, err
	}
	for index := range right {
		right[index].RecordNumber += middle
	}
	return append(left, right...), metrics, nil
}

func (p *Previewer) verifyRecordBatchAttempts(ctx context.Context, model string, goal Goal, datasetInstructions string, records []Record) ([]VerificationResult, previewMetrics, error) {
	userPrompt := verificationUserPrompt(records, datasetInstructions)
	enforceDatasetInstructions := datasetInstructions != ""
	messages := verificationMessages(goal, enforceDatasetInstructions, userPrompt, "")

	var metrics previewMetrics
	var validationErr error
	structureFailure := false

	for attempt := range maxVerificationAttempts {
		metrics.verificationAttempts++
		completion, err := p.completer.Complete(ctx, CompletionRequest{
			Model:          model,
			Messages:       messages,
			ResponseSchema: verificationSchema(len(records)),
			MaxTokens:      verificationTokenLimit(len(records)),
			Seed:           preparationGenerationSeed,
		})
		if err != nil {
			if errors.Is(err, ErrCompletionTruncated) {
				return nil, metrics, verificationStructureError{err: fmt.Errorf("build preview: verify records: %w", err)}
			}
			return nil, metrics, fmt.Errorf("build preview: verify records: %w", err)
		}
		metrics.promptTokens += completion.PromptTokens
		metrics.completionTokens += completion.CompletionTokens

		structureFailure = false
		var verdicts map[string]string
		if err := json.Unmarshal([]byte(completion.Content), &verdicts); err != nil {
			validationErr = fmt.Errorf("decode verification results: %w", err)
			structureFailure = true
		} else {
			generated, err := generatedVerifications(verdicts, len(records), enforceDatasetInstructions)
			validationErr = err
			if validationErr == nil {
				verification, err := validateVerification(goal, generated, records)
				validationErr = err
				if validationErr == nil {
					return verification, metrics, nil
				}
			}
		}

		messages = verificationMessages(goal, enforceDatasetInstructions, userPrompt, fmt.Sprintf("Retry %d: the previous response failed validation: %s. Return a fresh response containing every required record_N property with a permitted outcome.", attempt+2, validationErr))
	}

	err := fmt.Errorf("build preview: verification response failed validation after %d attempts: %w", maxVerificationAttempts, validationErr)
	if structureFailure {
		return nil, metrics, verificationStructureError{err: err}
	}
	return nil, metrics, recordQualityError{err: err}
}

func verificationTokenLimit(recordCount int) int {
	tokens := 64 + recordCount*40
	if tokens > maxVerificationTokens {
		return maxVerificationTokens
	}
	return tokens
}

func verificationMessages(goal Goal, enforceDatasetInstructions bool, userPrompt, correction string) []Message {
	if correction != "" {
		userPrompt += "\n\nCORRECTION:\n" + correction
	}
	return []Message{
		{Role: "system", Content: verificationSystemPrompt(goal, enforceDatasetInstructions)},
		{Role: "user", Content: userPrompt},
	}
}

func verificationSystemPrompt(goal Goal, enforceDatasetInstructions bool) string {
	rules := "The pair must be a useful standalone training example for its stated task. The prompt must contain the subject and setup needed to answer it without an unseen document, table, listing, example, or earlier conversation. Reject unresolved references such as 'this manner'. The target must contain every fact requested by the prompt and no additional fact or clause. The prompt and target must request the same answer type and action: reject a flag when an environment-variable name was requested, and reject a disabling target when the prompt asks how to enable the feature. Reject a prompt that states or substantially restates the answer before asking for it. Reject targets with clearly unrelated code, logs, terminal output, or surrounding prose. Do not reject a focused source sentence merely because a shorter answer could be written when no concise target candidate is available."
	switch goal {
	case GoalQuestionAnswer:
		rules += " The prompt must be a natural question ending with a question mark unless the record is explicitly labeled USER-APPROVED EDIT; for an approved edit, accept any non-empty prompt style and judge its grounding and meaning without rewriting it. Reject document-navigation questions about section, page, or index locations. If the question asks for one value, percentage, Boolean zero value, identifier, declaration, command, function, or result, reject an entire function, transcript, trace, benchmark report, multi-sentence passage, or target containing surrounding fields, metrics, annotations, or explanations."
	case GoalClassification:
		rules += " The prompt must identify the input being classified and the target must be only the supported category."
	case GoalNumericPrediction:
		rules += " The prompt must identify the input and requested measurement and the target must be a concise supported numeric value."
	case GoalTextGeneration:
		rules += " The prompt must clearly request the target text and the response must be focused on that request."
	}
	instructionRules := "No user-provided dataset requirements apply. Do not reject merely for stylistic preference."
	outcomeRules := "Return one outcome for each record_N property: pass when the record satisfies every requirement; grounding when a necessary fact is unsupported, contradictory, ambiguous, or requires outside knowledge; or task_quality when the prompt and target do not form a useful standalone example for the requested task."
	if enforceDatasetInstructions {
		instructionRules = "The user's instruction controls how every record is written and is an acceptance requirement. Reject a record that does not follow it. Only source grounding, factual correctness, and the required output structure take priority over the user's instruction."
		outcomeRules = "Return one outcome for each record_N property: pass when the record satisfies every requirement; grounding when a necessary fact is unsupported, contradictory, ambiguous, or requires outside knowledge; task_quality when the prompt and target do not form a useful standalone example for the requested task; or user_requirements when the record is otherwise valid but does not follow the user-provided dataset requirements."
	}
	return "You are a careful verifier for supervised fine-tuning data. Evaluate the prompt, source evidence, and selected target. " + instructionRules + " " + outcomeRules + " Accept faithful paraphrases, ordinary synonyms, concise answers, and focused source sentences when the relationship remains exact. Shared topic words are not enough: use grounding when the prompt names a different field, entity, operation, or measurement than the target, or when the target is only one fragment of the requested output. Preserve the exact role and capitalization of code symbols: reject a prompt that reinterprets a method as a type or interface, attributes an API to the wrong package, or combines alternate APIs. A code declaration proves only what is shown in that declaration; for example, the presence of a function named main does not by itself prove that it is the program entry point. A why question must be answered with the causal reason, not merely a repetition of its conclusion. A yes-or-no question must receive an explicit yes or no. Reject a prompt whose premise conflicts with the evidence, supplies the answer, or depends on an unseen example or output. Do not require the target to repeat its subject when the evidence clearly connects that subject to the answer. Use grounding for a defect where a necessary entity, relationship, number, or qualifier is unsupported, the relationship is ambiguous, or the target does not answer what was asked. In particular, reject an isolated value when its label or subject from the prompt is absent from the evidence. " + rules + " Use grounding if answering requires outside knowledge or another source passage. Return every required record_N property."
}

func verificationUserPrompt(records []Record, datasetInstructions string) string {
	var prompt strings.Builder
	if datasetInstructions != "" {
		prompt.WriteString(datasetInstructionsPrompt(datasetInstructions))
		prompt.WriteString("\n\n")
	}
	prompt.WriteString("Verify whether each selected target is explicitly grounded by its evidence and independently satisfies its prompt, task quality requirements, and the user instruction when one is provided. TARGET 1 is the selected training target. Each record references exactly one numbered source-evidence block.\n")
	evidence, evidenceIDs := sharedEvidenceReferences(records)
	for index, source := range evidence {
		fmt.Fprintf(&prompt, "\n[SOURCE EVIDENCE %d]\n%s\n", index+1, source)
	}
	for index, record := range records {
		fmt.Fprintf(&prompt, "\n[RECORD %d]\n", index+1)
		if record.UserEdited {
			prompt.WriteString("USER-APPROVED EDIT: preserve the prompt and target exactly as written; do not reject solely because the prompt is not phrased as a question.\n")
		}
		fmt.Fprintf(&prompt, "PROMPT: %s\nEVIDENCE BLOCK: %d\n", record.Prompt, evidenceIDs[index])
		fmt.Fprintf(&prompt, "[TARGET 1] %s\n", record.Target)
	}
	return strings.TrimSpace(prompt.String())
}

func sharedEvidenceReferences(records []Record) ([]string, []int) {
	evidence := make([]string, 0, len(records))
	evidenceIDs := make([]int, len(records))
	ids := make(map[string]int, len(records))
	for index, record := range records {
		source := strings.TrimSpace(record.Evidence)
		id, exists := ids[source]
		if !exists {
			evidence = append(evidence, source)
			id = len(evidence)
			ids[source] = id
		}
		evidenceIDs[index] = id
	}
	return evidence, evidenceIDs
}

func verificationEvidence(record Record) string {
	evidence := []rune(strings.TrimSpace(record.Evidence))
	if len(evidence) <= maxVerificationEvidenceRunes {
		return string(evidence)
	}

	targetStart := 0
	if byteIndex := strings.Index(string(evidence), record.Target); byteIndex >= 0 {
		targetStart = len([]rune(string(evidence)[:byteIndex]))
	}
	targetLength := len([]rune(record.Target))
	contextRunes := max(maxVerificationEvidenceRunes-targetLength, 0)
	start := max(targetStart-contextRunes/2, 0)
	end := min(start+maxVerificationEvidenceRunes, len(evidence))
	start = max(end-maxVerificationEvidenceRunes, 0)
	return strings.TrimSpace(string(evidence[start:end]))
}

func verificationSchema(count int) map[string]any {
	properties := make(map[string]any, count)
	required := make([]string, count)
	for index := range count {
		name := fmt.Sprintf("record_%d", index+1)
		properties[name] = map[string]any{"type": "string"}
		required[index] = name
	}

	return map[string]any{
		"type":       "object",
		"properties": properties,
		"required":   required,
	}
}

func generatedVerifications(verdicts map[string]string, count int, enforceDatasetInstructions bool) ([]generatedVerification, error) {
	if len(verdicts) != count {
		return nil, fmt.Errorf("model returned %d verification results; expected %d", len(verdicts), count)
	}

	generated := make([]generatedVerification, count)
	for index := range count {
		name := fmt.Sprintf("record_%d", index+1)
		outcome, exists := verdicts[name]
		if !exists {
			return nil, fmt.Errorf("model omitted verification result %q", name)
		}
		expectedTargetID := 0
		failureCategory := outcome
		switch outcome {
		case verificationPass:
			expectedTargetID = 1
			failureCategory = ""
		case verificationGrounding, verificationTaskQuality:
		case verificationUserRequirements:
			if !enforceDatasetInstructions {
				return nil, fmt.Errorf("model returned outcome %q for %s without user-provided dataset requirements", outcome, name)
			}
		default:
			return nil, fmt.Errorf("model returned invalid verification outcome %q for %s", outcome, name)
		}
		generated[index] = generatedVerification{RecordNumber: index + 1, ExpectedTargetID: expectedTargetID, FailureCategory: failureCategory}
	}
	return generated, nil
}

func validateVerification(goal Goal, generated []generatedVerification, records []Record) ([]VerificationResult, error) {
	if len(generated) != len(records) {
		return nil, fmt.Errorf("model returned %d verification results; expected %d", len(generated), len(records))
	}

	results := make([]VerificationResult, len(records))
	seen := make(map[int]struct{}, len(records))
	for index := range generated {
		result := &generated[index]
		if result.RecordNumber < 1 || result.RecordNumber > len(records) {
			return nil, fmt.Errorf("verification result %d has invalid record number %d", index+1, result.RecordNumber)
		}
		if _, exists := seen[result.RecordNumber]; exists {
			return nil, fmt.Errorf("verification result %d duplicates record number %d", index+1, result.RecordNumber)
		}
		record := records[result.RecordNumber-1]
		if result.ExpectedTargetID < 0 || result.ExpectedTargetID > 1 {
			return nil, fmt.Errorf("verification result %d has invalid expected_target_id %d", index+1, result.ExpectedTargetID)
		}
		passed := result.ExpectedTargetID == 1
		reason := ""
		expectedTarget := record.Target
		if passed {
			if err := validateVerifiedRecordShape(goal, record); err != nil {
				passed = false
				reason = err.Error()
			} else if err := validateRecordEvidence(goal, record.Prompt, record.Target, record.Evidence); err != nil {
				passed = false
				reason = err.Error()
			} else if isUnsupportedIsolatedValue(record) {
				passed = false
				reason = "target is an isolated value whose subject or label is absent from the evidence"
			} else if qualifier := unsupportedTargetQualifier(record); qualifier != "" {
				passed = false
				reason = fmt.Sprintf("target introduces unsupported qualifier %q", qualifier)
			}
		}
		if !passed {
			if reason == "" {
				reason = verificationFailureReason(result.FailureCategory)
			}
		} else {
			reason = "The prompt and selected target satisfy the task quality contract."
		}
		seen[result.RecordNumber] = struct{}{}
		results[result.RecordNumber-1] = VerificationResult{
			RecordNumber:   result.RecordNumber,
			Passed:         passed,
			ExpectedTarget: expectedTarget,
			Reason:         reason,
		}
	}

	return results, nil
}

func validateVerifiedRecordShape(goal Goal, record Record) error {
	if record.UserEdited {
		return nil
	}
	return validateRecordShape(goal, record.Prompt, record.Target)
}

func verificationFailureReason(category string) string {
	switch category {
	case verificationGrounding:
		return "The selected target is not grounded in the source evidence."
	case verificationUserRequirements:
		return "The record does not satisfy the user-provided dataset requirements."
	default:
		return "The prompt and selected target do not form a useful standalone example for the requested task."
	}
}

func isUnsupportedIsolatedValue(record Record) bool {
	target := normalizeSourceText(record.Target)
	evidence := normalizeSourceText(record.Evidence)
	return target == evidence && len(strings.Fields(target)) <= 3 && !strings.ContainsAny(target, ".?!")
}

func unsupportedTargetQualifier(record Record) string {
	targetWords := normalizedWordSet(record.Target)
	evidenceWords := normalizedWordSet(record.Evidence)
	qualifierGroups := [][]string{
		{"always", "invariably"},
		{"exclusively", "only", "solely"},
		{"globally", "worldwide"},
		{"never"},
	}
	for _, group := range qualifierGroups {
		for _, qualifier := range group {
			if targetWords[qualifier] && !containsAnyWord(evidenceWords, group) {
				return qualifier
			}
		}
	}
	return ""
}

func containsAnyWord(words map[string]bool, candidates []string) bool {
	for _, candidate := range candidates {
		if words[candidate] {
			return true
		}
	}
	return false
}

func normalizedWordSet(value string) map[string]bool {
	words := strings.FieldsFunc(strings.ToLower(value), func(char rune) bool {
		return !unicode.IsLetter(char) && !unicode.IsNumber(char)
	})
	set := make(map[string]bool, len(words))
	for _, word := range words {
		set[word] = true
	}
	return set
}

func failedVerification(results []VerificationResult) []VerificationResult {
	failed := make([]VerificationResult, 0, len(results))
	for _, result := range results {
		if !result.Passed {
			failed = append(failed, result)
		}
	}
	return failed
}

func replacementExclusions(records []Record, passages []string, failed []VerificationResult, history *replacementHistory) generationExclusions {
	failedRecords := make(map[int]struct{}, len(failed))
	for _, result := range failed {
		failedRecords[result.RecordNumber-1] = struct{}{}
	}

	for recordIndex, record := range records {
		if _, failed := failedRecords[recordIndex]; !failed {
			history.prompts = append(history.prompts, record.Prompt)
		}
		for index, passage := range passages {
			if passage == record.Evidence {
				history.evidenceIDs[index+1] = struct{}{}
			}
		}
	}

	return generationExclusions{
		prompts:  history.prompts,
		feedback: failed,
	}
}

func releaseFailedEvidence(records []Record, passages []string, failed []VerificationResult, history *replacementHistory) {
	for _, result := range failed {
		index := result.RecordNumber - 1
		if index < 0 || index >= len(records) {
			continue
		}
		for passageIndex, passage := range passages {
			if passage == records[index].Evidence {
				delete(history.evidenceIDs, passageIndex+1)
				break
			}
		}
	}
}
