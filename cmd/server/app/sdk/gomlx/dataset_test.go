package gomlx

import (
	"fmt"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/fineapp/finetune"
)

func TestRuntimeDatasetPreservesLegacyConversationsAndProtectsTest(t *testing.T) {
	var training []finetune.DatasetRecord
	for fact := range 10 {
		for variant := range 3 {
			training = append(training, finetune.DatasetRecord{Prompt: fmt.Sprintf("fact %d variant %d", fact, variant), Target: "answer", FactID: fmt.Sprint(fact)})
		}
	}
	contract := finetune.DatasetContract{Train: training, Validation: []finetune.DatasetRecord{{Prompt: "validate", Target: "answer", FactID: "0", IndependentPrompt: true}}, Test: []finetune.DatasetRecord{{Prompt: "protected", Target: "protected"}}}
	dataset := runtimeDataset(contract)
	if len(dataset.Train) != 30 || len(dataset.Validation) != 1 || len(dataset.TrainingConversations) != 10 || len(dataset.ValidationConversations) != 1 {
		t.Fatalf("dataset = %+v", dataset)
	}
	for index, conversation := range dataset.TrainingConversations {
		if len(conversation) != 4 || conversation[3].Prompt != fmt.Sprintf("fact %d variant 2", index) {
			t.Fatalf("conversation = %+v", conversation)
		}
		for step, record := range conversation[:3] {
			wantFact := (index + 5 + step) % 10
			if record.Prompt != fmt.Sprintf("fact %d variant 0", wantFact) {
				t.Fatalf("history = %+v", conversation)
			}
		}
	}
	validation := dataset.ValidationConversations[0]
	if len(validation) != 4 || validation[3].Prompt != "validate" {
		t.Fatalf("validation = %+v", validation)
	}
	contract.Train = training[:9]
	if dataset := runtimeDataset(contract); len(dataset.TrainingConversations) != 0 || len(dataset.ValidationConversations) != 0 {
		t.Fatal("fewer than four facts must not produce conversations")
	}
}
