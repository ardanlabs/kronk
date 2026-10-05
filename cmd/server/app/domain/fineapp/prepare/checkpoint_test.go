package prepare

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadBuildCheckpointDiscardsUnsafeStateWithoutFailing(t *testing.T) {
	fact := Record{FactID: "fact-1", Target: "supported target", Evidence: "supporting evidence"}
	tests := map[string]func(*buildCheckpoint){
		"unknown rejected fact": func(checkpoint *buildCheckpoint) {
			checkpoint.RejectedFacts[0].FactID = "unknown"
		},
		"duplicate rejected fact": func(checkpoint *buildCheckpoint) {
			checkpoint.RejectedFacts = append(checkpoint.RejectedFacts, checkpoint.RejectedFacts[0])
		},
		"changed rejected target": func(checkpoint *buildCheckpoint) {
			checkpoint.RejectedFacts[0].Target = "different target"
		},
		"missing rejection reason": func(checkpoint *buildCheckpoint) {
			checkpoint.RejectedFacts[0].Reason = ""
		},
		"negative metric": func(checkpoint *buildCheckpoint) {
			checkpoint.GenerationAttempts = -1
		},
	}
	for name, corrupt := range tests {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			checkpoint := buildCheckpoint{
				Version: datasetContract, PlanVersion: datasetPlanVersion, Fingerprint: "current", CompletedBatches: 1,
				RejectedFacts: []RejectedFact{{FactID: fact.FactID, Target: fact.Target, Evidence: fact.Evidence, Reason: "bounded attempts exhausted"}},
			}
			corrupt(&checkpoint)
			writeCheckpointFixture(t, directory, checkpoint)

			if loaded, found := loadBuildCheckpoint(directory, "current", 1, [][]Record{{fact}}); found || loaded.Fingerprint != "" {
				t.Fatalf("loadBuildCheckpoint() = (%+v, %t), want a clean rebuild", loaded, found)
			}
			if _, err := os.Stat(filepath.Join(directory, buildCheckpointFile)); !os.IsNotExist(err) {
				t.Fatalf("unsafe checkpoint was not discarded: %v", err)
			}
		})
	}
}

func TestLoadBuildCheckpointKeepsValidState(t *testing.T) {
	directory := t.TempDir()
	fact := Record{FactID: "fact-1", Target: "supported target", Evidence: "supporting evidence", UserApproved: true}
	want := buildCheckpoint{
		Version: datasetContract, PlanVersion: datasetPlanVersion, Fingerprint: "current", CompletedBatches: 0,
		RejectedFacts:      []RejectedFact{{FactID: fact.FactID, Target: fact.Target, Evidence: fact.Evidence, UserApproved: true, Reason: "bounded attempts exhausted"}},
		GenerationAttempts: 3, VerificationAttempts: 1, PromptTokens: 20, CompletionTokens: 10,
	}
	writeCheckpointFixture(t, directory, want)

	got, found := loadBuildCheckpoint(directory, "current", 1, [][]Record{{fact}})
	if !found || len(got.RejectedFacts) != 1 || got.RejectedFacts[0].FactID != fact.FactID {
		t.Fatalf("loadBuildCheckpoint() = (%+v, %t), want valid checkpoint", got, found)
	}
}

func writeCheckpointFixture(t *testing.T, directory string, checkpoint buildCheckpoint) {
	t.Helper()
	data, err := json.Marshal(checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, buildCheckpointFile), data, 0o600); err != nil {
		t.Fatal(err)
	}
}
