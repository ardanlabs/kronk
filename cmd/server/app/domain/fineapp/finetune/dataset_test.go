package finetune

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestBuildDatasetContractCreatesDeterministicProtectedSplits(t *testing.T) {
	records := testDatasetRecords(98)
	firstInput := writeTestDataset(t, records)
	firstOutput := t.TempDir()
	first, err := BuildDatasetContract(firstInput, firstOutput)
	if err != nil {
		t.Fatal(err)
	}
	reversed := append([]DatasetRecord(nil), records...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	secondInput := writeTestDataset(t, reversed)
	secondOutput := t.TempDir()
	second, err := BuildDatasetContract(secondInput, secondOutput)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(first, second) {
		t.Fatal("same prepared dataset produced a different contract")
	}
	if !first.Report.Ready || first.Report.Seed != DatasetSplitSeed || first.Report.SourceRecords != 98 {
		t.Fatalf("report = %+v", first.Report)
	}
	if len(first.Train) != 68 || len(first.Validation) != 10 || len(first.Test) != 20 {
		t.Fatalf("split sizes = %d/%d/%d", len(first.Train), len(first.Validation), len(first.Test))
	}
	for _, hash := range []string{first.Report.SourceSHA256, first.Report.TrainSHA256, first.Report.ValidationSHA256, first.Report.TestSHA256} {
		if len(hash) != 64 {
			t.Fatalf("invalid report hash %q", hash)
		}
	}
	for _, name := range []string{"train.jsonl", "validation.jsonl", "test.jsonl", "contract.json"} {
		info, err := os.Stat(filepath.Join(firstOutput, name))
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s permissions = %o", name, info.Mode().Perm())
		}
	}

	writtenTrain := readTestDataset(t, filepath.Join(firstOutput, "train.jsonl"))
	if !reflect.DeepEqual(writtenTrain, first.Train) || writtenTrain[0].Evidence == "" {
		t.Fatal("persisted training split lost records or evidence")
	}
	var report DatasetContractReport
	readTestJSON(t, filepath.Join(firstOutput, "contract.json"), &report)
	if !reflect.DeepEqual(report, first.Report) {
		t.Fatalf("written report = %+v, want %+v", report, first.Report)
	}
}

func TestBuildDatasetContractStratifiesClassificationTargets(t *testing.T) {
	records := make([]DatasetRecord, 0, 300)
	for _, label := range []string{"good", "neutral", "bad"} {
		for index := range 100 {
			records = append(records, DatasetRecord{
				Prompt: fmt.Sprintf("Classify %s sample %d", label, index), Target: label,
				Evidence: fmt.Sprintf("The measured sample %d has a %s status.", index, label),
			})
		}
	}
	contract, err := BuildDatasetContract(writeTestDataset(t, records), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !contract.Report.Stratified || contract.Report.Task != datasetTaskExact {
		t.Fatalf("report = %+v", contract.Report)
	}
	for name, set := range map[string][]DatasetRecord{"train": contract.Train, "validation": contract.Validation, "test": contract.Test} {
		counts := make(map[string]int)
		for _, record := range set {
			counts[record.Target]++
		}
		if counts["good"] != counts["neutral"] || counts["neutral"] != counts["bad"] {
			t.Fatalf("%s targets are not balanced: %v", name, counts)
		}
	}
}

func TestBuildDatasetContractKeepsSharedEvidenceInOneSplit(t *testing.T) {
	records := testDatasetRecords(60)
	for index := range 4 {
		records[index].Evidence = "One source passage supports several related questions."
	}
	contract, err := BuildDatasetContract(writeTestDataset(t, records), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	owners := make(map[string]string)
	for name, split := range map[string][]DatasetRecord{"train": contract.Train, "validation": contract.Validation, "test": contract.Test} {
		for _, record := range split {
			evidence := normalizeDatasetText(record.Evidence)
			if owner, exists := owners[evidence]; exists && owner != name {
				t.Fatalf("evidence occurs in %s and %s", owner, name)
			}
			owners[evidence] = name
		}
	}
}

func TestBuildDatasetContractKeepsUserApprovedEvidenceGroupInTraining(t *testing.T) {
	records := testDatasetRecords(60)
	records[0].Prompt = "User-edited prompt?"
	records[0].Target = "User-edited answer."
	records[0].Evidence = "Shared evidence for the user-approved records."
	records[0].UserApproved = true
	records[1].Evidence = records[0].Evidence

	contract, err := BuildDatasetContract(writeTestDataset(t, records), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range contract.Train {
		if record.Prompt == records[0].Prompt && record.Target == records[0].Target && record.UserApproved {
			return
		}
	}
	t.Fatalf("user-approved record is not in training: %+v", contract)
}

func TestBuildDatasetContractHoldsOutAlternateQuestionsNotUnseenFacts(t *testing.T) {
	records := make([]DatasetRecord, 0, 150)
	for fact := range 30 {
		for variant := range minimumKnowledgeQuestionVariants + 1 {
			records = append(records, DatasetRecord{
				Prompt:            fmt.Sprintf("Question variant %d for fact %d?", variant, fact),
				Target:            fmt.Sprintf("Detailed answer for fact %d with enough words to require generation.", fact),
				Evidence:          fmt.Sprintf("Source evidence for fact %d supports the detailed answer.", fact),
				FactID:            fmt.Sprintf("fact-%d", fact),
				IndependentPrompt: variant == minimumKnowledgeQuestionVariants,
			})
		}
	}
	contract, err := BuildDatasetContract(writeTestDataset(t, records), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(contract.Train) != 150 || len(contract.Validation) != 10 || len(contract.Test) != 20 {
		t.Fatalf("split sizes = %d/%d/%d", len(contract.Train), len(contract.Validation), len(contract.Test))
	}
	if !contract.Report.IndependentTest {
		t.Fatal("dataset contract did not record independent evaluation coverage")
	}
	trainingFacts := make(map[string]bool)
	trainingFactCounts := make(map[string]int)
	trainingPrompts := make(map[string]bool)
	for _, record := range contract.Train {
		trainingFacts[record.FactID] = true
		trainingFactCounts[record.FactID]++
		trainingPrompts[record.Prompt] = true
	}
	validationFacts := make(map[string]bool)
	for _, record := range contract.Validation {
		validationFacts[record.FactID] = true
	}
	for _, record := range contract.Test {
		if validationFacts[record.FactID] {
			t.Fatalf("fact %q occurs in validation and protected test", record.FactID)
		}
	}
	for _, record := range append(append([]DatasetRecord(nil), contract.Validation...), contract.Test...) {
		if !record.IndependentPrompt {
			t.Fatalf("evaluation prompt %q was not independently generated", record.Prompt)
		}
		if !trainingFacts[record.FactID] {
			t.Fatalf("evaluation fact %q was not represented in training", record.FactID)
		}
		if trainingFactCounts[record.FactID] < minimumKnowledgeQuestionVariants {
			t.Fatalf("evaluation fact %q has only %d training questions", record.FactID, trainingFactCounts[record.FactID])
		}
		if trainingPrompts[record.Prompt] {
			t.Fatalf("evaluation prompt %q leaked into training", record.Prompt)
		}
	}
}

func TestBuildDatasetContractUsesPerFactTrainValidationAndTestRoles(t *testing.T) {
	records := make([]DatasetRecord, 0, MinimumKnowledgeFacts+MinimumValidationRecords+MinimumTestRecords)
	for fact := range MinimumKnowledgeFacts {
		record := DatasetRecord{
			Prompt:         fmt.Sprintf("Training question for current fact %d?", fact),
			Target:         fmt.Sprintf("Detailed current answer for fact %d with enough words to require generation.", fact),
			Evidence:       fmt.Sprintf("Source chunk evidence for current fact %d supports its answer.", fact),
			FactID:         fmt.Sprintf("current-fact-%d", fact),
			SourceDocument: "guide.pdf",
			SourceChunkID:  fmt.Sprintf("chunk-%d", fact),
		}
		records = append(records, record)
		if fact < MinimumValidationRecords {
			validation := record
			validation.Prompt = fmt.Sprintf("Validation question for current fact %d?", fact)
			validation.ValidationPrompt = true
			records = append(records, validation)
		}
		test := record
		test.Prompt = fmt.Sprintf("Protected test question for current fact %d?", fact)
		test.IndependentPrompt = true
		records = append(records, test)
	}
	contract, err := BuildDatasetContract(writeTestDataset(t, records), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(contract.Train) != MinimumKnowledgeFacts || len(contract.Validation) != MinimumValidationRecords || len(contract.Test) != MinimumTestRecords {
		t.Fatalf("split sizes = %d/%d/%d", len(contract.Train), len(contract.Validation), len(contract.Test))
	}
	trainingPrompts := make(map[string]bool, len(contract.Train))
	for _, record := range contract.Train {
		trainingPrompts[record.Prompt] = true
		if record.ValidationPrompt || record.IndependentPrompt || record.SourceChunkID == "" {
			t.Fatalf("invalid training role or lineage: %+v", record)
		}
	}
	for _, record := range contract.Validation {
		if !record.ValidationPrompt || record.IndependentPrompt || trainingPrompts[record.Prompt] {
			t.Fatalf("invalid validation role: %+v", record)
		}
	}
	for _, record := range contract.Test {
		if record.ValidationPrompt || !record.IndependentPrompt || trainingPrompts[record.Prompt] {
			t.Fatalf("invalid protected-test role: %+v", record)
		}
	}
}

func TestBuildDatasetContractKeepsUserApprovedQuestionInTraining(t *testing.T) {
	records := make([]DatasetRecord, 0, 180)
	for fact := range 30 {
		for variant := range minimumKnowledgeQuestionVariants + 1 {
			records = append(records, DatasetRecord{
				Prompt:            fmt.Sprintf("Question variant %d for fact %d?", variant, fact),
				Target:            fmt.Sprintf("Detailed answer for fact %d with enough words to require generation.", fact),
				Evidence:          fmt.Sprintf("Source evidence for fact %d supports the detailed answer.", fact),
				FactID:            fmt.Sprintf("fact-%d", fact),
				IndependentPrompt: variant == minimumKnowledgeQuestionVariants,
				UserApproved:      fact == 0 && variant == 0,
			})
		}
	}

	contract, err := BuildDatasetContract(writeTestDataset(t, records), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(contract.Train, func(record DatasetRecord) bool { return record.UserApproved }) {
		t.Fatal("user-approved question is not in training")
	}
	for _, record := range append(append([]DatasetRecord(nil), contract.Validation...), contract.Test...) {
		if record.UserApproved {
			t.Fatalf("user-approved question was placed in evaluation: %+v", record)
		}
	}
}

func TestBuildRecoveryDatasetReplacesFailedFactsAndPreservesThirtyFacts(t *testing.T) {
	records := knowledgeRecords(0, 30)
	spares := knowledgeRecords(30, 3)
	directory := t.TempDir()
	dataset := filepath.Join(directory, "dataset.jsonl")
	sparePath := filepath.Join(directory, "dataset-spares.jsonl")
	output := filepath.Join(directory, "recovery.jsonl")
	if err := os.WriteFile(dataset, []byte(encodeTestDataset(t, records)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sparePath, []byte(encodeTestDataset(t, spares)), 0o600); err != nil {
		t.Fatal(err)
	}

	built, err := BuildRecoveryDataset(dataset, sparePath, output, []string{"fact-0", "fact-1", "fact-2"})
	if err != nil {
		t.Fatal(err)
	}
	if !built {
		t.Fatal("recovery dataset was not built")
	}
	recovered := readTestDataset(t, output)
	if len(recovered) != 180 {
		t.Fatalf("recovery records = %d, want 180", len(recovered))
	}
	facts := make(map[string]int)
	for _, record := range recovered {
		facts[record.FactID]++
	}
	if len(facts) != 30 {
		t.Fatalf("recovery facts = %d, want 30", len(facts))
	}
	for _, factID := range []string{"fact-0", "fact-1", "fact-2"} {
		if facts[factID] != 0 {
			t.Fatalf("failed fact %q remains in recovery dataset", factID)
		}
	}
	for _, factID := range []string{"fact-30", "fact-31", "fact-32"} {
		if facts[factID] != 6 {
			t.Fatalf("spare fact %q has %d records, want 6", factID, facts[factID])
		}
	}
}

func TestBuildRecoveryDatasetDoesNotReplaceUserApprovedFact(t *testing.T) {
	records := knowledgeRecords(0, 30)
	records[0].UserApproved = true
	directory := t.TempDir()
	dataset := filepath.Join(directory, "dataset.jsonl")
	sparePath := filepath.Join(directory, "dataset-spares.jsonl")
	output := filepath.Join(directory, "recovery.jsonl")
	if err := os.WriteFile(dataset, []byte(encodeTestDataset(t, records)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sparePath, []byte(encodeTestDataset(t, knowledgeRecords(30, 1))), 0o600); err != nil {
		t.Fatal(err)
	}

	built, err := BuildRecoveryDataset(dataset, sparePath, output, []string{"fact-0"})
	if err != nil {
		t.Fatal(err)
	}
	if built {
		t.Fatal("recovery replaced a user-approved fact")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("recovery output exists: %v", err)
	}
}

func knowledgeRecords(first, count int) []DatasetRecord {
	records := make([]DatasetRecord, 0, count*6)
	for fact := first; fact < first+count; fact++ {
		for variant := range 6 {
			records = append(records, DatasetRecord{
				Prompt:            fmt.Sprintf("Question variant %d for fact %d?", variant, fact),
				Target:            fmt.Sprintf("Detailed answer for fact %d with enough words to require generation.", fact),
				Evidence:          fmt.Sprintf("Source evidence for fact %d supports the detailed answer.", fact),
				FactID:            fmt.Sprintf("fact-%d", fact),
				IndependentPrompt: variant == 5,
			})
		}
	}
	return records
}

func TestBuildDatasetContractHoldsOutAnAdditionalAvailableQuestion(t *testing.T) {
	records := make([]DatasetRecord, 0, 180)
	for fact := range 30 {
		for variant := range minimumKnowledgeQuestionVariants + 2 {
			records = append(records, DatasetRecord{
				Prompt:            fmt.Sprintf("Question variant %d for fact %d?", variant, fact),
				Target:            fmt.Sprintf("Detailed answer for fact %d with enough words to require generation.", fact),
				Evidence:          fmt.Sprintf("Source evidence for fact %d supports the detailed answer.", fact),
				FactID:            fmt.Sprintf("fact-%d", fact),
				IndependentPrompt: variant == minimumKnowledgeQuestionVariants+1,
			})
		}
	}
	contract, err := BuildDatasetContract(writeTestDataset(t, records), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(contract.Train) != 160 || len(contract.Validation) != 10 || len(contract.Test) != 40 {
		t.Fatalf("split sizes = %d/%d/%d", len(contract.Train), len(contract.Validation), len(contract.Test))
	}
	trainingPrompts := make(map[string]bool, len(contract.Train))
	for _, record := range contract.Train {
		trainingPrompts[record.Prompt] = true
	}
	testFacts := make(map[string]int)
	for _, record := range contract.Test {
		testFacts[record.FactID]++
		if trainingPrompts[record.Prompt] {
			t.Fatalf("protected prompt %q leaked into training", record.Prompt)
		}
	}
	for factID, count := range testFacts {
		if count != 2 {
			t.Fatalf("protected fact %q has %d held-out questions, want 2", factID, count)
		}
	}
}

func TestBuildDatasetContractBalancesKnowledgeAnswerModes(t *testing.T) {
	records := make([]DatasetRecord, 0, 170)
	for fact := range 34 {
		target := fmt.Sprintf("Exact answer %d", fact)
		if fact >= 19 {
			target = fmt.Sprintf("Detailed answer for fact %d contains enough words to require generated prose from the adapter.", fact)
		}
		for variant := range minimumKnowledgeQuestionVariants + 1 {
			records = append(records, DatasetRecord{
				Prompt:            fmt.Sprintf("Question variant %d for fact %d?", variant, fact),
				Target:            target,
				Evidence:          fmt.Sprintf("Source evidence for fact %d supports its answer.", fact),
				FactID:            fmt.Sprintf("fact-%d", fact),
				IndependentPrompt: variant == minimumKnowledgeQuestionVariants,
			})
		}
	}
	contract, err := BuildDatasetContract(writeTestDataset(t, records), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	reversed := append([]DatasetRecord(nil), records...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	repeated, err := BuildDatasetContract(writeTestDataset(t, reversed), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(contract, repeated) {
		t.Fatal("balanced knowledge split changed with source record order")
	}
	if !contract.Report.Stratified {
		t.Fatal("knowledge answer modes were not reported as stratified")
	}
	countExact := func(records []DatasetRecord, independentOnly bool) int {
		count := 0
		for _, record := range records {
			if (!independentOnly || record.IndependentPrompt) && datasetRecordTask(record) == datasetTaskExact {
				count++
			}
		}
		return count
	}
	if exact := countExact(contract.Test, false); exact != 11 {
		t.Fatalf("protected test exact answers = %d, want 11 of 20", exact)
	}
	if exact := countExact(contract.Validation, false); exact != 6 {
		t.Fatalf("validation exact answers = %d, want 6 of 10", exact)
	}
	if exact := countExact(contract.Train, true); exact != 2 {
		t.Fatalf("training-only independent exact answers = %d, want 2 of 4", exact)
	}
}

func TestBuildDatasetContractIdentifiesKnowledgeDatasetThatNeedsMoreFacts(t *testing.T) {
	records := make([]DatasetRecord, 0, 90)
	for fact := range 18 {
		for variant := range minimumKnowledgeQuestionVariants + 1 {
			records = append(records, DatasetRecord{
				Prompt:            fmt.Sprintf("Question variant %d for fact %d?", variant, fact),
				Target:            fmt.Sprintf("Detailed answer for fact %d with enough words to require generation.", fact),
				Evidence:          fmt.Sprintf("Source evidence for fact %d supports the detailed answer.", fact),
				FactID:            fmt.Sprintf("fact-%d", fact),
				IndependentPrompt: variant == minimumKnowledgeQuestionVariants,
			})
		}
	}

	_, err := BuildDatasetContract(writeTestDataset(t, records), t.TempDir())
	var insufficient *InsufficientFactsError
	if !errors.As(err, &insufficient) {
		t.Fatalf("error = %v, want InsufficientFactsError", err)
	}
	if insufficient.Facts != 18 || insufficient.Required != MinimumValidationRecords+MinimumTestRecords {
		t.Fatalf("insufficient fact details = %+v", insufficient)
	}
}

func TestBuildDatasetContractRejectsFactsWithoutQuestionVariants(t *testing.T) {
	records := make([]DatasetRecord, 60)
	for index := range records {
		records[index] = DatasetRecord{
			Prompt: fmt.Sprintf("Question %d?", index), Target: "A sufficiently detailed generated answer for this source fact.",
			Evidence: fmt.Sprintf("Evidence %d", index), FactID: fmt.Sprintf("fact-%d", index),
		}
	}
	_, err := BuildDatasetContract(writeTestDataset(t, records), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "at least 5") {
		t.Fatalf("error = %v, want missing question variants", err)
	}
}

func TestBuildDatasetContractSupportsLegacyDataButRejectsPartialIndependentEvaluation(t *testing.T) {
	records := make([]DatasetRecord, 0, 121)
	for fact := range 30 {
		for variant := range minimumKnowledgeQuestionVariants {
			records = append(records, DatasetRecord{
				Prompt:   fmt.Sprintf("Training question %d for fact %d?", variant, fact),
				Target:   fmt.Sprintf("Detailed answer for fact %d with enough words to require generation.", fact),
				Evidence: fmt.Sprintf("Source evidence for fact %d supports the detailed answer.", fact),
				FactID:   fmt.Sprintf("fact-%d", fact),
			})
		}
	}
	legacy, err := BuildDatasetContract(writeTestDataset(t, records), t.TempDir())
	if err != nil {
		t.Fatalf("legacy prepared dataset: %v", err)
	}
	if legacy.Report.IndependentTest {
		t.Fatal("legacy dataset incorrectly claims independent evaluation coverage")
	}
	records = append(records, DatasetRecord{
		Prompt: "Independent question for fact 0?", Target: records[0].Target,
		Evidence: records[0].Evidence, FactID: records[0].FactID, IndependentPrompt: true,
	})
	_, err = BuildDatasetContract(writeTestDataset(t, records), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "exactly 1") {
		t.Fatalf("error = %v, want missing independent evaluation question", err)
	}
}

func TestBuildDatasetContractRejectsUnsafePreparedData(t *testing.T) {
	valid := testDatasetRecords(50)
	tests := map[string]struct {
		data string
		want string
	}{
		"too few records":    {encodeTestDataset(t, valid[:49]), "at least 50"},
		"blank line":         {encodeTestDataset(t, valid[:25]) + "\n" + encodeTestDataset(t, valid[25:]), "blank lines"},
		"unknown field":      {strings.Replace(encodeTestDataset(t, valid), `"evidence":`, `"unknown":"value","evidence":`, 1), "unknown field"},
		"trailing JSON":      {strings.Replace(encodeTestDataset(t, valid), "}\n", "} {}\n", 1), "multiple JSON values"},
		"empty evidence":     {replaceTestRecord(t, valid, 0, func(record *DatasetRecord) { record.Evidence = " " }), "empty prompt, target, or evidence"},
		"duplicate pair":     {replaceTestRecord(t, valid, 1, func(record *DatasetRecord) { *record = valid[0] }), "duplicates record"},
		"conflicting target": {replaceTestRecord(t, valid, 1, func(record *DatasetRecord) { record.Prompt = valid[0].Prompt }), "conflicting target"},
		"repetitive target": {replaceTestRecord(t, valid, 0, func(record *DatasetRecord) {
			record.Target = strings.Repeat("one two three ", 10)
		}), "excessively repetitive"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			input := filepath.Join(t.TempDir(), "dataset.jsonl")
			if err := os.WriteFile(input, []byte(test.data), 0o600); err != nil {
				t.Fatal(err)
			}
			output := t.TempDir()
			if _, err := BuildDatasetContract(input, output); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			for _, artifact := range []string{"train.jsonl", "validation.jsonl", "test.jsonl", "contract.json"} {
				if _, err := os.Stat(filepath.Join(output, artifact)); !os.IsNotExist(err) {
					t.Fatalf("unsafe dataset published %s", artifact)
				}
			}
		})
	}
}

func TestBuildDatasetContractIdentifiesValidDatasetThatNeedsMoreRecords(t *testing.T) {
	output := t.TempDir()
	_, err := BuildDatasetContract(writeTestDataset(t, testDatasetRecords(11)), output)
	var insufficient *InsufficientRecordsError
	if !errors.As(err, &insufficient) {
		t.Fatalf("error = %v, want InsufficientRecordsError", err)
	}
	if insufficient.Records != 11 || insufficient.Required != 50 {
		t.Fatalf("insufficient record details = %+v", insufficient)
	}
	for _, artifact := range []string{"train.jsonl", "validation.jsonl", "test.jsonl", "contract.json"} {
		if _, err := os.Stat(filepath.Join(output, artifact)); !os.IsNotExist(err) {
			t.Fatalf("undersized dataset published %s", artifact)
		}
	}
}

func TestBuildDatasetContractDoesNotOverwriteArtifacts(t *testing.T) {
	output := t.TempDir()
	path := filepath.Join(output, "train.jsonl")
	if err := os.WriteFile(path, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := BuildDatasetContract(writeTestDataset(t, testDatasetRecords(50)), output)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("error = %v", err)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil || string(data) != "keep me" {
		t.Fatalf("existing artifact changed: data=%q error=%v", data, readErr)
	}
}

func TestReplaceDatasetContractReplacesOnlyAfterValidation(t *testing.T) {
	parent := t.TempDir()
	output := filepath.Join(parent, "training")
	if err := os.Mkdir(output, 0o700); err != nil {
		t.Fatal(err)
	}
	first, err := BuildDatasetContract(writeTestDataset(t, testDatasetRecords(50)), output)
	if err != nil {
		t.Fatal(err)
	}

	badDataset := writeTestDataset(t, testDatasetRecords(49))
	if _, err := ReplaceDatasetContract(badDataset, output); err == nil {
		t.Fatal("invalid replacement dataset was accepted")
	}
	var report DatasetContractReport
	readTestJSON(t, filepath.Join(output, "contract.json"), &report)
	if !reflect.DeepEqual(report, first.Report) {
		t.Fatalf("failed replacement changed the previous contract: got %+v want %+v", report, first.Report)
	}

	replacement, err := ReplaceDatasetContract(writeTestDataset(t, testDatasetRecords(76)), output)
	if err != nil {
		t.Fatal(err)
	}
	readTestJSON(t, filepath.Join(output, "contract.json"), &report)
	if !reflect.DeepEqual(report, replacement.Report) || report.SourceRecords != 76 {
		t.Fatalf("replacement report = %+v, want %+v", report, replacement.Report)
	}
	if matches, err := filepath.Glob(filepath.Join(parent, ".training-*")); err != nil || len(matches) != 0 {
		t.Fatalf("temporary training directories remain: matches=%v error=%v", matches, err)
	}
}

func testDatasetRecords(count int) []DatasetRecord {
	records := make([]DatasetRecord, count)
	for index := range records {
		records[index] = DatasetRecord{
			Prompt:   fmt.Sprintf("Question %03d?", index),
			Target:   fmt.Sprintf("Answer %03d.", index),
			Evidence: fmt.Sprintf("Source passage %03d supports this answer and preserves its provenance.", index),
		}
	}
	return records
}

func writeTestDataset(t *testing.T, records []DatasetRecord) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dataset.jsonl")
	if err := os.WriteFile(path, []byte(encodeTestDataset(t, records)), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func encodeTestDataset(t *testing.T, records []DatasetRecord) string {
	t.Helper()
	var encoded strings.Builder
	encoder := json.NewEncoder(&encoded)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			t.Fatal(err)
		}
	}
	return encoded.String()
}

func replaceTestRecord(t *testing.T, records []DatasetRecord, index int, replace func(*DatasetRecord)) string {
	t.Helper()
	changed := append([]DatasetRecord(nil), records...)
	replace(&changed[index])
	return encodeTestDataset(t, changed)
}

func readTestDataset(t *testing.T, path string) []DatasetRecord {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var records []DatasetRecord
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record DatasetRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return records
}

func readTestJSON(t *testing.T, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatal(err)
	}
}
