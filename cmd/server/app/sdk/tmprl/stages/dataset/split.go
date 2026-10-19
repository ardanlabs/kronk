package dataset

import (
	"crypto/sha256"
	"fmt"
	"slices"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/model"
)

// Readiness is a gate, not a training implementation. Explicit fact-mode
// evaluation rows stay separate; unflagged exact-answer records must be
// splittable by evidence without sharing source groups across splits.
// Generated non-QA datasets do not currently create evaluation rows and may
// complete preparation without passing this gate. Their training policy is
// deferred until a goal-aware training backend is implemented.
func TrainingReadiness(records []model.Record) error {
	_, err := Split(records)
	return err
}

// Split owns readiness validation and allocation with a fixed seed.
// Current fact-mode evaluation flags are authoritative; legacy variants and
// non-fact examples are split without introducing source-group leakage.
func Split(records []model.Record) (model.TrainingDataset, error) {
	if err := ValidateRecords(records); err != nil {
		return model.TrainingDataset{}, err
	}
	if len(records) == 0 {
		return model.TrainingDataset{}, fmt.Errorf("empty dataset")
	}
	dataset := model.TrainingDataset{}
	if records[0].FactID != "" {
		hasValidation := slices.ContainsFunc(records, func(r model.Record) bool { return r.ValidationPrompt })
		hasTest := slices.ContainsFunc(records, func(r model.Record) bool { return r.IndependentPrompt })
		groups := make(map[string][]model.Record)
		for _, r := range records {
			groups[r.FactID] = append(groups[r.FactID], r)
		}
		keys := make([]string, 0, len(groups))
		for key := range groups {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		if !hasValidation && len(keys) < 30 {
			return model.TrainingDataset{}, fmt.Errorf("legacy evaluation requires at least 30 independently testable facts")
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
					return model.TrainingDataset{}, fmt.Errorf("each fact requires one training question and at most one question per evaluation split")
				}
				continue
			}
			independent := slices.DeleteFunc(slices.Clone(rows), func(r model.Record) bool { return !r.IndependentPrompt })
			rows = slices.DeleteFunc(slices.Clone(rows), func(r model.Record) bool { return r.IndependentPrompt })
			if len(rows) < 5 || (hasTest && len(independent) != 1) {
				return model.TrainingDataset{}, fmt.Errorf("legacy facts require five question variants and, when present, one independent test question per fact")
			}
			for _, r := range rows {
				if r.UserApproved {
					dataset.Train = append(dataset.Train, r)
				}
			}
			rows = slices.DeleteFunc(rows, func(r model.Record) bool { return r.UserApproved })
			needed := 1
			if len(independent) == 0 {
				needed++
			}
			if len(rows) < needed {
				return model.TrainingDataset{}, fmt.Errorf("legacy facts require unapproved questions for validation and protected testing")
			}
			slices.SortFunc(rows, func(a, b model.Record) int { return compareTrainingOrder(Key(a.Prompt), Key(b.Prompt)) })
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
		groups := make(map[string][]model.Record)
		for _, r := range records {
			if len(strings.Fields(r.Target)) > 8 {
				return model.TrainingDataset{}, fmt.Errorf("generation datasets require fact identities and independent evaluation prompts")
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
			case slices.ContainsFunc(rows, func(r model.Record) bool { return r.UserApproved }):
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
	if len(dataset.Train) < 20 || len(dataset.Validation) < 10 || len(dataset.Test) < 20 {
		return model.TrainingDataset{}, fmt.Errorf("training splits require 20 train, 10 validation and 20 protected test rows")
	}
	return dataset, nil
}

func compareTrainingOrder(a, b string) int {
	ah, bh := sha256.Sum256([]byte("3407/"+a)), sha256.Sum256([]byte("3407/"+b))
	return slices.Compare(ah[:], bh[:])
}
