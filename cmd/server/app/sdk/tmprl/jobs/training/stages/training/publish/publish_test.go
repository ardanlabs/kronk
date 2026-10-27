package publish_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training/evaluate"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training/publish"
)

func TestBothAcceptancesRequired(t *testing.T) {
	for _, stored := range []bool{false, true} {
		for _, incoming := range []bool{false, true} {
			root, err := os.OpenRoot(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(evaluate.EvaluationReport{Accepted: stored})
			if err != nil {
				t.Fatal(err)
			}
			if err := root.WriteFile("evaluation.json", data, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := root.WriteFile("candidate.gguf", []byte("adapter"), 0o600); err != nil {
				t.Fatal(err)
			}
			artifacts, err := publish.Run(t.Context(), root, training.Artifacts{Accepted: incoming})
			if (err == nil) != (stored && incoming) {
				t.Fatalf("stored=%v incoming=%v: %v", stored, incoming, err)
			}
			previous := training.Artifacts{Dataset: "dataset", Checkpoint: "checkpoint", Report: "report", Adapter: "old", Accepted: incoming}
			var step training.TrainingStep = publish.Step{}
			result, err := step.Execute(t.Context(), training.StepContext{Workspace: root, Artifacts: previous})
			if (err == nil) != (stored && incoming) {
				t.Fatalf("stage stored=%v incoming=%v: %v", stored, incoming, err)
			}
			if stored && incoming {
				previous.Adapter = artifacts.Adapter
				if step.Name() != "publish" || result.Status != "" || result.Artifacts != previous {
					t.Fatalf("stage result = %+v, want artifacts %+v and continuing status", result, previous)
				}
			}
			if !stored || !incoming {
				if _, err := root.Stat("adapter.gguf"); !os.IsNotExist(err) {
					t.Fatalf("rejected adapter published: %v", err)
				}
			} else {
				data, err := os.ReadFile(artifacts.Adapter)
				if err != nil || string(data) != "adapter" {
					t.Fatalf("published adapter: %q, %v", data, err)
				}
			}
			root.Close()
		}
	}
}
