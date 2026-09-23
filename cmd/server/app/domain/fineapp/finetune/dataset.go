package finetune

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	DatasetContractSchema = "finetune-dataset.v4"
	DatasetSplitSeed      = int64(3407)

	MinimumTrainRecords      = 20
	MinimumValidationRecords = 10
	MinimumTestRecords       = 20
	MinimumKnowledgeFacts    = MinimumValidationRecords + MinimumTestRecords
)

const (
	datasetTaskExact      = "exact"
	datasetTaskGeneration = "generation"

	minimumKnowledgeQuestionVariants = 5
	maximumExactAnswerWords          = 8
)

// DatasetRecord is the strict boundary between Moxie dataset preparation and
// training. Evidence is retained for provenance; model tokenization consumes
// only Prompt and Target.
type DatasetRecord struct {
	Prompt            string `json:"prompt"`
	Target            string `json:"target"`
	Evidence          string `json:"evidence"`
	FactID            string `json:"fact_id,omitempty"`
	SourceDocument    string `json:"source_document,omitempty"`
	IndependentPrompt bool   `json:"independent_prompt,omitempty"`
}

// DatasetContract contains deterministic training, validation, and protected
// test sets. Test must remain untouched until the final adapter evaluation.
type DatasetContract struct {
	Train      []DatasetRecord
	Validation []DatasetRecord
	Test       []DatasetRecord
	Report     DatasetContractReport
}

// DatasetContractReport records the reproducible identity of every split.
type DatasetContractReport struct {
	Schema            string `json:"schema"`
	Task              string `json:"task"`
	Stratified        bool   `json:"stratified"`
	Seed              int64  `json:"seed"`
	SourceRecords     int    `json:"source_records"`
	TrainRecords      int    `json:"train_records"`
	ValidationRecords int    `json:"validation_records"`
	TestRecords       int    `json:"test_records"`
	SourceSHA256      string `json:"source_sha256"`
	TrainSHA256       string `json:"train_sha256"`
	ValidationSHA256  string `json:"validation_sha256"`
	TestSHA256        string `json:"test_sha256"`
	IndependentTest   bool   `json:"independent_test"`
	Ready             bool   `json:"ready"`
}

// InsufficientRecordsError reports a valid prepared dataset that is too small
// to reserve reliable training, validation, and protected test splits.
type InsufficientRecordsError struct {
	Records  int
	Required int
}

// Error implements error.
func (err *InsufficientRecordsError) Error() string {
	return fmt.Sprintf("prepared dataset has %d records; reliable evaluation requires at least %d (%d train, %d validation, %d protected test)",
		err.Records, err.Required, MinimumTrainRecords, MinimumValidationRecords, MinimumTestRecords)
}

// InsufficientFactsError reports a valid knowledge dataset that does not have
// enough independent facts for separate validation and protected test sets.
type InsufficientFactsError struct {
	Facts    int
	Required int
}

// Error implements error.
func (err *InsufficientFactsError) Error() string {
	return fmt.Sprintf("prepared dataset has %d independently testable facts; reliable evaluation requires at least %d", err.Facts, err.Required)
}

type datasetRecordGroup struct {
	records           []DatasetRecord
	independentPrompt *DatasetRecord
	hash              [sha256.Size]byte
}

// BuildDatasetContract validates a prepared Moxie JSONL dataset, creates
// deterministic splits, and safely writes train.jsonl, validation.jsonl,
// test.jsonl, and contract.json into outputDirectory.
func BuildDatasetContract(datasetPath, outputDirectory string) (DatasetContract, error) {
	contract, err := ValidateDatasetContract(datasetPath)
	if err != nil {
		return DatasetContract{}, err
	}
	if err := persistDatasetContract(outputDirectory, contract); err != nil {
		return DatasetContract{}, err
	}
	return contract, nil
}

// ValidateDatasetContract validates and splits a prepared Moxie JSONL dataset
// without writing training artifacts.
func ValidateDatasetContract(datasetPath string) (DatasetContract, error) {
	records, err := loadDatasetRecords(datasetPath)
	if err != nil {
		return DatasetContract{}, err
	}
	contract, err := splitDatasetRecords(records, DatasetSplitSeed)
	if err != nil {
		return DatasetContract{}, err
	}
	return contract, nil
}

// BuildRecoveryDataset replaces failed facts with prepared spare facts while
// preserving the original fact and record counts. It returns false without
// writing a file when a complete deterministic replacement is unavailable.
func BuildRecoveryDataset(datasetPath, sparePath, outputPath string, failedFactIDs []string) (bool, error) {
	if len(failedFactIDs) == 0 {
		return false, nil
	}
	records, err := loadDatasetRecords(datasetPath)
	if err != nil {
		return false, err
	}
	spares, err := loadDatasetRecords(sparePath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	failed := make(map[string]bool, len(failedFactIDs))
	for _, factID := range failedFactIDs {
		failed[factID] = true
	}
	found := make(map[string]bool, len(failed))
	recovered := make([]DatasetRecord, 0, len(records))
	for _, record := range records {
		if failed[record.FactID] {
			found[record.FactID] = true
			continue
		}
		recovered = append(recovered, record)
	}
	if len(found) != len(failed) {
		return false, nil
	}
	replacementsNeeded := len(found)

	usedFacts := make(map[string]bool)
	for _, record := range records {
		usedFacts[record.FactID] = true
	}
	spareFacts := groupRecordsByFact(spares)
	for _, factID := range orderedFactIDs(spares) {
		if replacementsNeeded == 0 {
			break
		}
		if usedFacts[factID] {
			continue
		}
		recovered = append(recovered, spareFacts[factID]...)
		usedFacts[factID] = true
		replacementsNeeded--
	}
	if replacementsNeeded > 0 || len(recovered) != len(records) {
		return false, nil
	}
	if _, err := splitDatasetRecords(recovered, DatasetSplitSeed); err != nil {
		return false, fmt.Errorf("validate recovery dataset: %w", err)
	}
	if err := writeDatasetJSONL(outputPath, recovered); err != nil {
		return false, err
	}
	return true, nil
}

func groupRecordsByFact(records []DatasetRecord) map[string][]DatasetRecord {
	groups := make(map[string][]DatasetRecord)
	for _, record := range records {
		groups[record.FactID] = append(groups[record.FactID], record)
	}
	return groups
}

func orderedFactIDs(records []DatasetRecord) []string {
	seen := make(map[string]bool)
	var order []string
	for _, record := range records {
		if record.FactID != "" && !seen[record.FactID] {
			seen[record.FactID] = true
			order = append(order, record.FactID)
		}
	}
	return order
}

// ReplaceDatasetContract validates a prepared dataset and atomically replaces
// a previously generated contract. The previous contract remains available if
// validation or publication fails.
func ReplaceDatasetContract(datasetPath, outputDirectory string) (DatasetContract, error) {
	parent := filepath.Dir(outputDirectory)
	staging, err := os.MkdirTemp(parent, ".training-")
	if err != nil {
		return DatasetContract{}, fmt.Errorf("create temporary training output: %w", err)
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(staging)
		}
	}()

	contract, err := BuildDatasetContract(datasetPath, staging)
	if err != nil {
		return DatasetContract{}, err
	}
	if err := replaceDatasetContractDirectory(staging, outputDirectory); err != nil {
		return DatasetContract{}, err
	}
	published = true
	return contract, nil
}

func replaceDatasetContractDirectory(staging, outputDirectory string) error {
	info, err := os.Lstat(outputDirectory)
	if os.IsNotExist(err) {
		if err := os.Rename(staging, outputDirectory); err != nil {
			return fmt.Errorf("publish training output: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect previous training output: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("replace training output %q: existing path is not a directory", outputDirectory)
	}

	backup, err := os.MkdirTemp(filepath.Dir(outputDirectory), ".training-previous-")
	if err != nil {
		return fmt.Errorf("reserve previous training output: %w", err)
	}
	if err := os.Remove(backup); err != nil {
		return fmt.Errorf("reserve previous training output: %w", err)
	}
	defer os.RemoveAll(backup)

	if err := os.Rename(outputDirectory, backup); err != nil {
		return fmt.Errorf("preserve previous training output: %w", err)
	}
	if err := os.Rename(staging, outputDirectory); err != nil {
		publishErr := fmt.Errorf("publish training output: %w", err)
		if restoreErr := os.Rename(backup, outputDirectory); restoreErr != nil {
			return errors.Join(publishErr, fmt.Errorf("restore previous training output: %w", restoreErr))
		}
		return publishErr
	}
	return nil
}

func loadDatasetRecords(path string) ([]DatasetRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open prepared dataset %q: %w", path, err)
	}
	defer file.Close()

	var records []DatasetRecord
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for line := 1; scanner.Scan(); line++ {
		raw := scanner.Bytes()
		if len(bytes.TrimSpace(raw)) == 0 {
			return nil, fmt.Errorf("decode prepared dataset %q line %d: blank lines are not allowed", path, line)
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		var record DatasetRecord
		if err := decoder.Decode(&record); err != nil {
			return nil, fmt.Errorf("decode prepared dataset %q line %d: %w", path, line, err)
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			if err == nil {
				err = fmt.Errorf("multiple JSON values are not allowed")
			}
			return nil, fmt.Errorf("decode prepared dataset %q line %d: %w", path, line, err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read prepared dataset %q: %w", path, err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("prepared dataset %q contains no records", path)
	}
	return records, nil
}

func splitDatasetRecords(records []DatasetRecord, seed int64) (DatasetContract, error) {
	if err := validateDatasetRecords("source", records); err != nil {
		return DatasetContract{}, err
	}
	task := inferDatasetTask(records)
	if records[0].FactID != "" {
		return splitKnowledgeDatasetRecords(records, seed)
	}
	if task == datasetTaskGeneration {
		return DatasetContract{}, fmt.Errorf("prepared question-answer dataset has no fact_id; prepare it again so Moxie can evaluate alternate questions instead of unseen facts")
	}
	minimum := MinimumTrainRecords + MinimumValidationRecords + MinimumTestRecords
	if len(records) < minimum {
		return DatasetContract{}, &InsufficientRecordsError{Records: len(records), Required: minimum}
	}

	groups := groupDatasetRecords(records, seed)
	sort.Slice(groups, func(left, right int) bool {
		if order := bytes.Compare(groups[left].hash[:], groups[right].hash[:]); order != 0 {
			return order < 0
		}
		return datasetRecordKey(groups[left].records[0]) < datasetRecordKey(groups[right].records[0])
	})

	testTarget := largerInt(MinimumTestRecords, int(math.Round(float64(len(records))*.1)))
	validationTarget := largerInt(MinimumValidationRecords, int(math.Round(float64(len(records))*.1)))
	labels, stratified := datasetClassificationLabels(records)
	test, remaining := takeDatasetGroups(groups, testTarget, MinimumTrainRecords+MinimumValidationRecords, labels)
	validation, training := takeDatasetGroups(remaining, validationTarget, MinimumTrainRecords, labels)

	contract := DatasetContract{
		Train:      unwrapDatasetGroups(training),
		Validation: unwrapDatasetGroups(validation),
		Test:       unwrapDatasetGroups(test),
	}
	if err := validateDatasetSplits(contract); err != nil {
		return DatasetContract{}, err
	}
	contract.Report = DatasetContractReport{
		Schema: DatasetContractSchema, Task: task, Stratified: stratified, Seed: seed,
		SourceRecords: len(records), TrainRecords: len(contract.Train), ValidationRecords: len(contract.Validation), TestRecords: len(contract.Test),
		SourceSHA256: datasetRecordsHash(records), TrainSHA256: datasetRecordsHash(contract.Train),
		ValidationSHA256: datasetRecordsHash(contract.Validation), TestSHA256: datasetRecordsHash(contract.Test), Ready: true,
	}
	return contract, nil
}

func splitKnowledgeDatasetRecords(records []DatasetRecord, seed int64) (DatasetContract, error) {
	groups := make(map[string][]DatasetRecord)
	independentPrompts := 0
	for index, record := range records {
		if record.FactID == "" {
			return DatasetContract{}, fmt.Errorf("source record %d has no fact_id; prepare this dataset again so Moxie can evaluate alternate questions instead of unseen facts", index+1)
		}
		groups[record.FactID] = append(groups[record.FactID], record)
		if record.IndependentPrompt {
			independentPrompts++
		}
	}
	if independentPrompts == 0 {
		return splitLegacyKnowledgeDatasetRecords(records, groups, seed)
	}
	ordered := make([]datasetRecordGroup, 0, len(groups))
	for factID, grouped := range groups {
		regular := make([]DatasetRecord, 0, len(grouped))
		independent := make([]DatasetRecord, 0, 1)
		for _, record := range grouped {
			if record.IndependentPrompt {
				independent = append(independent, record)
			} else {
				regular = append(regular, record)
			}
		}
		if len(regular) < minimumKnowledgeQuestionVariants {
			return DatasetContract{}, fmt.Errorf("training fact %q has %d training question variants; prepare the dataset again to provide at least %d", factID, len(regular), minimumKnowledgeQuestionVariants)
		}
		if len(independent) != 1 {
			return DatasetContract{}, fmt.Errorf("training fact %q has %d independent evaluation questions; prepare the dataset again to provide exactly 1", factID, len(independent))
		}
		sort.Slice(regular, func(left, right int) bool {
			return datasetRecordKey(regular[left]) < datasetRecordKey(regular[right])
		})
		ordered = append(ordered, datasetRecordGroup{
			records: regular, independentPrompt: &independent[0], hash: datasetOrderHash(factID, seed),
		})
	}
	if len(ordered) < MinimumKnowledgeFacts {
		return DatasetContract{}, &InsufficientFactsError{Facts: len(ordered), Required: MinimumKnowledgeFacts}
	}
	sort.Slice(ordered, func(left, right int) bool {
		return bytes.Compare(ordered[left].hash[:], ordered[right].hash[:]) < 0
	})

	testCount := largerInt(MinimumTestRecords, int(math.Round(float64(len(ordered))*.1)))
	validationCount := largerInt(MinimumValidationRecords, int(math.Round(float64(len(ordered))*.1)))
	testGroups, remaining := takeBalancedKnowledgeGroups(ordered, testCount)
	validationGroups, trainingGroups := takeBalancedKnowledgeGroups(remaining, validationCount)
	ordered = append(testGroups, validationGroups...)
	ordered = append(ordered, trainingGroups...)
	contract := DatasetContract{}
	for index, group := range ordered {
		trainingRecords := group.records
		if index < testCount {
			contract.Test = append(contract.Test, *group.independentPrompt)
			if len(trainingRecords) > minimumKnowledgeQuestionVariants {
				last := len(trainingRecords) - 1
				contract.Test = append(contract.Test, trainingRecords[last])
				trainingRecords = trainingRecords[:last]
			}
		} else if index < testCount+validationCount {
			contract.Validation = append(contract.Validation, *group.independentPrompt)
		} else {
			contract.Train = append(contract.Train, *group.independentPrompt)
		}
		contract.Train = append(contract.Train, trainingRecords...)
	}
	if err := validateDatasetSplits(contract, true); err != nil {
		return DatasetContract{}, err
	}
	contract.Report = DatasetContractReport{
		Schema: DatasetContractSchema, Task: datasetTaskGeneration, Stratified: true, Seed: seed, IndependentTest: true,
		SourceRecords: len(records), TrainRecords: len(contract.Train), ValidationRecords: len(contract.Validation), TestRecords: len(contract.Test),
		SourceSHA256: datasetRecordsHash(records), TrainSHA256: datasetRecordsHash(contract.Train),
		ValidationSHA256: datasetRecordsHash(contract.Validation), TestSHA256: datasetRecordsHash(contract.Test), Ready: true,
	}
	return contract, nil
}

func splitLegacyKnowledgeDatasetRecords(records []DatasetRecord, groups map[string][]DatasetRecord, seed int64) (DatasetContract, error) {
	ordered := make([]datasetRecordGroup, 0, len(groups))
	for factID, grouped := range groups {
		if len(grouped) < minimumKnowledgeQuestionVariants {
			return DatasetContract{}, fmt.Errorf("training fact %q has %d question variants; prepare the dataset again to provide at least %d", factID, len(grouped), minimumKnowledgeQuestionVariants)
		}
		sort.Slice(grouped, func(left, right int) bool {
			return datasetRecordKey(grouped[left]) < datasetRecordKey(grouped[right])
		})
		ordered = append(ordered, datasetRecordGroup{records: grouped, hash: datasetOrderHash(factID, seed)})
	}
	if len(ordered) < MinimumKnowledgeFacts {
		return DatasetContract{}, &InsufficientFactsError{Facts: len(ordered), Required: MinimumKnowledgeFacts}
	}
	sort.Slice(ordered, func(left, right int) bool {
		return bytes.Compare(ordered[left].hash[:], ordered[right].hash[:]) < 0
	})

	testCount := largerInt(MinimumTestRecords, int(math.Round(float64(len(ordered))*.1)))
	validationCount := largerInt(MinimumValidationRecords, int(math.Round(float64(len(ordered))*.1)))
	testGroups, remaining := takeBalancedKnowledgeGroups(ordered, testCount)
	validationGroups, trainingGroups := takeBalancedKnowledgeGroups(remaining, validationCount)
	ordered = append(testGroups, validationGroups...)
	ordered = append(ordered, trainingGroups...)
	contract := DatasetContract{}
	for index, group := range ordered {
		start := 0
		if index < testCount {
			contract.Test = append(contract.Test, group.records[start])
			start++
		}
		if index >= testCount && index < testCount+validationCount {
			contract.Validation = append(contract.Validation, group.records[start])
			start++
		}
		contract.Train = append(contract.Train, group.records[start:]...)
	}
	if err := validateDatasetSplits(contract, true); err != nil {
		return DatasetContract{}, err
	}
	contract.Report = DatasetContractReport{
		Schema: DatasetContractSchema, Task: datasetTaskGeneration, Stratified: true, Seed: seed,
		SourceRecords: len(records), TrainRecords: len(contract.Train), ValidationRecords: len(contract.Validation), TestRecords: len(contract.Test),
		SourceSHA256: datasetRecordsHash(records), TrainSHA256: datasetRecordsHash(contract.Train),
		ValidationSHA256: datasetRecordsHash(contract.Validation), TestSHA256: datasetRecordsHash(contract.Test), Ready: true,
	}
	return contract, nil
}

func validateDatasetRecords(name string, records []DatasetRecord) error {
	promptTargets := make(map[string]string, len(records))
	pairs := make(map[string]int, len(records))
	facts := make(map[string]string)
	factMode := records[0].FactID != ""
	for index, record := range records {
		prompt := normalizeDatasetText(record.Prompt)
		target := normalizeDatasetText(record.Target)
		evidence := normalizeDatasetText(record.Evidence)
		if prompt == "" || target == "" || evidence == "" {
			return fmt.Errorf("%s record %d contains an empty prompt, target, or evidence", name, index+1)
		}
		pair := prompt + "\x00" + target
		if first, exists := pairs[pair]; exists {
			return fmt.Errorf("%s record %d duplicates record %d", name, index+1, first)
		}
		pairs[pair] = index + 1
		if previous, exists := promptTargets[prompt]; exists && previous != target {
			return fmt.Errorf("%s record %d gives a conflicting target for an existing prompt", name, index+1)
		}
		promptTargets[prompt] = target
		if repeatedDatasetTrigramRate(record.Target) > .8 {
			return fmt.Errorf("%s record %d target for prompt %q is excessively repetitive", name, index+1, record.Prompt)
		}
		if (record.FactID != "") != factMode {
			return fmt.Errorf("%s record %d mixes fact-aware and legacy records", name, index+1)
		}
		if record.FactID != "" {
			identity := target + "\x00" + evidence
			if previous, exists := facts[record.FactID]; exists && previous != identity {
				return fmt.Errorf("%s record %d reuses fact_id %q for different source knowledge", name, index+1, record.FactID)
			}
			facts[record.FactID] = identity
		}
	}
	return nil
}

func validateDatasetSplits(contract DatasetContract, allowFactVariants ...bool) error {
	if len(contract.Train) < MinimumTrainRecords || len(contract.Validation) < MinimumValidationRecords || len(contract.Test) < MinimumTestRecords {
		return fmt.Errorf("dataset split produced %d train, %d validation, and %d protected test records; require at least %d, %d, and %d",
			len(contract.Train), len(contract.Validation), len(contract.Test), MinimumTrainRecords, MinimumValidationRecords, MinimumTestRecords)
	}
	sets := []struct {
		name    string
		records []DatasetRecord
	}{
		{"training", contract.Train},
		{"validation", contract.Validation},
		{"protected test", contract.Test},
	}
	promptOwners := make(map[string]string)
	evidenceOwners := make(map[string]string)
	allowEvidenceOverlap := len(allowFactVariants) > 0 && allowFactVariants[0]
	for _, set := range sets {
		if err := validateDatasetRecords(set.name, set.records); err != nil {
			return err
		}
		for index, record := range set.records {
			prompt := normalizeDatasetText(record.Prompt)
			if owner, exists := promptOwners[prompt]; exists && owner != set.name {
				return fmt.Errorf("%s record %d prompt overlaps the %s dataset", set.name, index+1, owner)
			}
			promptOwners[prompt] = set.name
			evidence := normalizeDatasetText(record.Evidence)
			if owner, exists := evidenceOwners[evidence]; exists && owner != set.name && !allowEvidenceOverlap {
				return fmt.Errorf("%s record %d evidence overlaps the %s dataset", set.name, index+1, owner)
			}
			evidenceOwners[evidence] = set.name
		}
	}
	return nil
}

func datasetClassificationLabels(records []DatasetRecord) (map[string]float64, bool) {
	if inferDatasetTask(records) != datasetTaskExact {
		return nil, false
	}
	counts := make(map[string]int)
	for _, record := range records {
		counts[normalizeDatasetText(record.Target)]++
	}
	maximumLabels := smallerInt(20, len(records)/5)
	if len(counts) < 2 || len(counts) > maximumLabels {
		return nil, false
	}
	labels := make(map[string]float64, len(counts))
	for label, count := range counts {
		labels[label] = float64(count) / float64(len(records))
	}
	return labels, true
}

func groupDatasetRecords(records []DatasetRecord, seed int64) []datasetRecordGroup {
	byEvidence := make(map[string][]DatasetRecord)
	for _, record := range records {
		evidence := normalizeDatasetText(record.Evidence)
		byEvidence[evidence] = append(byEvidence[evidence], record)
	}
	groups := make([]datasetRecordGroup, 0, len(byEvidence))
	for evidence, grouped := range byEvidence {
		sort.Slice(grouped, func(left, right int) bool {
			return datasetRecordKey(grouped[left]) < datasetRecordKey(grouped[right])
		})
		groups = append(groups, datasetRecordGroup{records: grouped, hash: datasetOrderHash(evidence, seed)})
	}
	return groups
}

func takeDatasetGroups(groups []datasetRecordGroup, target, minimumRemaining int, labels map[string]float64) ([]datasetRecordGroup, []datasetRecordGroup) {
	remaining := append([]datasetRecordGroup(nil), groups...)
	selected := make([]datasetRecordGroup, 0, target)
	counts := make(map[string]int, len(labels))
	labelNames := make([]string, 0, len(labels))
	for label := range labels {
		labelNames = append(labelNames, label)
	}
	sort.Strings(labelNames)
	selectedRecords := 0
	remainingRecords := datasetGroupRecordCount(remaining)
	for selectedRecords < target && remainingRecords > minimumRemaining {
		best := -1
		if len(labels) > 0 {
			bestScore := math.Inf(1)
			for index, candidate := range remaining {
				if remainingRecords-len(candidate.records) < minimumRemaining {
					continue
				}
				score := datasetGroupLabelScore(counts, selectedRecords, candidate.records, labelNames, labels)
				if score < bestScore {
					best = index
					bestScore = score
				}
			}
		} else {
			for index, candidate := range remaining {
				if remainingRecords-len(candidate.records) >= minimumRemaining {
					best = index
					break
				}
			}
		}
		if best < 0 {
			break
		}
		selected = append(selected, remaining[best])
		for _, record := range remaining[best].records {
			counts[normalizeDatasetText(record.Target)]++
		}
		selectedRecords += len(remaining[best].records)
		remainingRecords -= len(remaining[best].records)
		remaining = append(remaining[:best], remaining[best+1:]...)
	}
	return selected, remaining
}

func datasetGroupLabelScore(selectedCounts map[string]int, selectedRecords int, candidates []DatasetRecord, labelNames []string, labels map[string]float64) float64 {
	total := selectedRecords + len(candidates)
	added := make(map[string]int)
	for _, candidate := range candidates {
		added[normalizeDatasetText(candidate.Target)]++
	}
	var score float64
	for _, label := range labelNames {
		count := selectedCounts[label] + added[label]
		score += math.Abs(float64(count)/float64(total) - labels[label])
	}
	return score
}

func datasetGroupRecordCount(groups []datasetRecordGroup) int {
	count := 0
	for _, group := range groups {
		count += len(group.records)
	}
	return count
}

func takeBalancedKnowledgeGroups(groups []datasetRecordGroup, target int) ([]datasetRecordGroup, []datasetRecordGroup) {
	remaining := append([]datasetRecordGroup(nil), groups...)
	selected := make([]datasetRecordGroup, 0, target)
	exactTotal := 0
	for _, group := range groups {
		if datasetRecordTask(group.records[0]) == datasetTaskExact {
			exactTotal++
		}
	}
	exactSelected := 0
	for len(selected) < target {
		best := 0
		bestScore := math.Inf(1)
		for index, group := range remaining {
			candidateExact := exactSelected
			if datasetRecordTask(group.records[0]) == datasetTaskExact {
				candidateExact++
			}
			score := math.Abs(float64(candidateExact)/float64(len(selected)+1) - float64(exactTotal)/float64(len(groups)))
			if score < bestScore {
				best = index
				bestScore = score
			}
		}
		if datasetRecordTask(remaining[best].records[0]) == datasetTaskExact {
			exactSelected++
		}
		selected = append(selected, remaining[best])
		remaining = append(remaining[:best], remaining[best+1:]...)
	}
	return selected, remaining
}

func unwrapDatasetGroups(groups []datasetRecordGroup) []DatasetRecord {
	result := make([]DatasetRecord, 0, datasetGroupRecordCount(groups))
	for _, group := range groups {
		result = append(result, group.records...)
	}
	return result
}

func inferDatasetTask(records []DatasetRecord) string {
	for _, record := range records {
		if datasetRecordTask(record) == datasetTaskGeneration {
			return datasetTaskGeneration
		}
	}
	return datasetTaskExact
}

func datasetRecordTask(record DatasetRecord) string {
	mode := datasetTaskExact
	if len(strings.Fields(normalizeDatasetText(record.Target))) > maximumExactAnswerWords {
		mode = datasetTaskGeneration
	}
	return mode
}

func datasetOrderHash(key string, seed int64) [sha256.Size]byte {
	data := make([]byte, 8, 8+len(key))
	binary.LittleEndian.PutUint64(data, uint64(seed))
	data = append(data, key...)
	return sha256.Sum256(data)
}

func datasetRecordsHash(records []DatasetRecord) string {
	canonical := make([]string, len(records))
	for index, record := range records {
		canonical[index] = datasetRecordKey(record)
	}
	sort.Strings(canonical)
	hash := sha256.New()
	for _, record := range canonical {
		_, _ = hash.Write([]byte(record))
		_, _ = hash.Write([]byte{'\n'})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func datasetRecordKey(record DatasetRecord) string {
	return strings.Join([]string{
		normalizeDatasetText(record.Prompt),
		normalizeDatasetText(record.Target),
		normalizeDatasetText(record.Evidence),
		record.FactID,
		fmt.Sprint(record.IndependentPrompt),
	}, "\x00")
}

func normalizeDatasetText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func repeatedDatasetTrigramRate(value string) float64 {
	words := strings.Fields(strings.ToLower(value))
	if len(words) < 12 {
		return 0
	}
	total := len(words) - 2
	seen := make(map[string]bool, total)
	repeated := 0
	for index := range total {
		trigram := strings.Join(words[index:index+3], " ")
		if seen[trigram] {
			repeated++
		}
		seen[trigram] = true
	}
	return float64(repeated) / float64(total)
}

func persistDatasetContract(outputDirectory string, contract DatasetContract) error {
	info, err := os.Stat(outputDirectory)
	if err != nil {
		return fmt.Errorf("inspect training output %q: %w", outputDirectory, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("training output %q is not a directory", outputDirectory)
	}

	files := []struct {
		name    string
		records []DatasetRecord
	}{
		{"train.jsonl", contract.Train},
		{"validation.jsonl", contract.Validation},
		{"test.jsonl", contract.Test},
	}
	for _, file := range files {
		path := filepath.Join(outputDirectory, file.name)
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("training dataset artifact %q already exists", path)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect training dataset artifact %q: %w", path, err)
		}
	}
	reportPath := filepath.Join(outputDirectory, "contract.json")
	if _, err := os.Lstat(reportPath); err == nil {
		return fmt.Errorf("training dataset artifact %q already exists", reportPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect training dataset artifact %q: %w", reportPath, err)
	}

	published := make([]string, 0, 4)
	cleanup := func() {
		for _, path := range published {
			_ = os.Remove(path)
		}
	}
	for _, file := range files {
		path := filepath.Join(outputDirectory, file.name)
		if err := writeDatasetJSONL(path, file.records); err != nil {
			cleanup()
			return err
		}
		published = append(published, path)
	}
	if err := writeDatasetJSON(reportPath, contract.Report); err != nil {
		cleanup()
		return err
	}
	return nil
}

func writeDatasetJSONL(path string, records []DatasetRecord) error {
	return writeDatasetFile(path, func(writer io.Writer) error {
		encoder := json.NewEncoder(writer)
		for _, record := range records {
			if err := encoder.Encode(record); err != nil {
				return fmt.Errorf("encode training dataset %q: %w", path, err)
			}
		}
		return nil
	})
}

func writeDatasetJSON(path string, value any) error {
	return writeDatasetFile(path, func(writer io.Writer) error {
		encoder := json.NewEncoder(writer)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(value); err != nil {
			return fmt.Errorf("encode training dataset report %q: %w", path, err)
		}
		return nil
	})
}

func writeDatasetFile(path string, write func(io.Writer) error) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".dataset-*")
	if err != nil {
		return fmt.Errorf("create training dataset artifact in %q: %w", filepath.Dir(path), err)
	}
	temporaryPath := temporary.Name()
	keep := false
	defer func() {
		_ = temporary.Close()
		if !keep {
			_ = os.Remove(temporaryPath)
		}
	}()
	buffered := bufio.NewWriter(temporary)
	if err := write(buffered); err != nil {
		return err
	}
	if err := buffered.Flush(); err != nil {
		return fmt.Errorf("flush training dataset artifact %q: %w", path, err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync training dataset artifact %q: %w", path, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close training dataset artifact %q: %w", path, err)
	}
	if err := os.Chmod(temporaryPath, 0o600); err != nil {
		return fmt.Errorf("secure training dataset artifact %q: %w", path, err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish training dataset artifact %q: %w", path, err)
	}
	keep = true
	return nil
}

func largerInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func smallerInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
