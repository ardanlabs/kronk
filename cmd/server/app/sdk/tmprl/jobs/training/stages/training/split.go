package training

import (
	"crypto/sha256"
	"fmt"
	"slices"
	"strings"
)

// Evaluation sizing: a dataset holds back test questions for 5% of its training
// records, between MinimumTestQuestions and MaximumTestQuestions, and half as many
// validation questions, at least MinimumValidationQuestions.
const (
	MinimumTrainingRecords     = 20
	MinimumTestQuestions       = 16
	MaximumTestQuestions       = 128
	MinimumValidationQuestions = 8
)

// EvaluationNeeds returns the validation and test questions a dataset with the
// given number of training records needs.
func EvaluationNeeds(trainingRecords int) (validation, test int) {
	test = min(max(trainingRecords*5/100, MinimumTestQuestions), MaximumTestQuestions)
	return max(MinimumValidationQuestions, test/2), test
}

// TrainingReadiness checks whether records can be split for training without
// performing training. Explicit fact-mode evaluation rows stay separate;
// unflagged exact-answer records are split by evidence without sharing source
// groups across splits.
func TrainingReadiness(records []Record) error {
	_, err := Split(records)
	return err
}

// Split validates readiness and allocates records with a fixed seed. A dataset
// without a complete evaluation set still trains on its training records when
// it has at least MinimumTrainingRecords; it then has no validation or test
// records, so training is not compared with the base model.
func Split(records []Record) (TrainingDataset, error) {
	if err := ValidateRecords(records); err != nil {
		return TrainingDataset{}, err
	}
	dataset, err := split(records)
	if err == nil {
		return dataset, nil
	}
	train := slices.DeleteFunc(slices.Clone(records), func(r Record) bool { return r.ValidationPrompt || r.IndependentPrompt })
	if len(train) < MinimumTrainingRecords {
		return TrainingDataset{}, err
	}
	return TrainingDataset{Train: train}, nil
}

// split allocates evaluation records. Explicit evaluation flags are
// authoritative; unflagged fact variants and non-fact examples are split
// without introducing source-group leakage.
func split(records []Record) (TrainingDataset, error) {
	if len(records) == 0 {
		return TrainingDataset{}, fmt.Errorf("empty dataset")
	}
	dataset := TrainingDataset{}
	validation, test := 10, 20
	if records[0].FactID != "" {
		hasValidation := slices.ContainsFunc(records, func(r Record) bool { return r.ValidationPrompt })
		hasTest := slices.ContainsFunc(records, func(r Record) bool { return r.IndependentPrompt })
		groups := make(map[string][]Record)
		for _, r := range records {
			groups[r.FactID] = append(groups[r.FactID], r)
		}
		keys := make([]string, 0, len(groups))
		for key := range groups {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		if hasValidation {
			// Flagged evaluation questions are sized to the training records.
			validation, test = EvaluationNeeds(len(keys))
		}
		if !hasValidation && len(keys) < 30 {
			return TrainingDataset{}, fmt.Errorf("unflagged evaluation requires at least 30 independently testable facts")
		}
		for _, key := range keys {
			rows := groups[key]
			if hasValidation {
				train, validation, test := 0, 0, 0
				for _, r := range rows {
					switch {
					case r.ValidationPrompt:
						validation++
						dataset.Validation = append(dataset.Validation, r)
					case r.IndependentPrompt:
						test++
						dataset.Test = append(dataset.Test, r)
					default:
						train++
						dataset.Train = append(dataset.Train, r)
					}
				}
				if train != 1 || validation > 1 || test > 1 {
					return TrainingDataset{}, fmt.Errorf("each fact requires one training question and at most one question per evaluation split")
				}
				continue
			}
			independent := slices.DeleteFunc(slices.Clone(rows), func(r Record) bool { return !r.IndependentPrompt })
			rows = slices.DeleteFunc(slices.Clone(rows), func(r Record) bool { return r.IndependentPrompt })
			if len(rows) < 5 || (hasTest && len(independent) != 1) {
				return TrainingDataset{}, fmt.Errorf("unflagged facts require five question variants and, when present, one independent test question per fact")
			}
			for _, r := range rows {
				if r.UserApproved {
					dataset.Train = append(dataset.Train, r)
				}
			}
			rows = slices.DeleteFunc(rows, func(r Record) bool { return r.UserApproved })
			needed := 1
			if len(independent) == 0 {
				needed++
			}
			if len(rows) < needed {
				return TrainingDataset{}, fmt.Errorf("unflagged facts require unapproved questions for validation and protected testing")
			}
			slices.SortFunc(rows, func(a, b Record) int { return compareTrainingOrder(Key(a.Prompt), Key(b.Prompt)) })
			dataset.Validation = append(dataset.Validation, rows[0])
			rows = rows[1:]
			if len(independent) > 0 {
				dataset.Test = append(dataset.Test, independent...)
			} else {
				dataset.Test = append(dataset.Test, rows[0])
				rows = rows[1:]
			}
			dataset.Train = append(dataset.Train, rows...)
		}
	} else {
		groups := make(map[string][]Record)
		for _, r := range records {
			if len(strings.Fields(r.Target)) > 8 {
				return TrainingDataset{}, fmt.Errorf("generation datasets require fact identities and independent evaluation prompts")
			}
			key := Key(r.Evidence)
			groups[key] = append(groups[key], r)
		}
		keys := make([]string, 0, len(groups))
		for key := range groups {
			keys = append(keys, key)
		}
		slices.SortFunc(keys, compareTrainingOrder)
		for _, key := range keys {
			rows := groups[key]
			switch {
			case slices.ContainsFunc(rows, func(r Record) bool { return r.UserApproved }):
				dataset.Train = append(dataset.Train, rows...)
			case len(dataset.Test) < 20:
				dataset.Test = append(dataset.Test, rows...)
			case len(dataset.Validation) < 10:
				dataset.Validation = append(dataset.Validation, rows...)
			default:
				dataset.Train = append(dataset.Train, rows...)
			}
		}
	}
	if len(dataset.Train) < MinimumTrainingRecords || len(dataset.Validation) < validation || len(dataset.Test) < test {
		return TrainingDataset{}, fmt.Errorf("training splits require %d train, %d validation and %d protected test rows", MinimumTrainingRecords, validation, test)
	}
	return dataset, nil
}

func compareTrainingOrder(a, b string) int {
	ah, bh := sha256.Sum256([]byte("3407/"+a)), sha256.Sum256([]byte("3407/"+b))
	return slices.Compare(ah[:], bh[:])
}
