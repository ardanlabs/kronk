package verify_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/preparation/verify"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/training"
)

type completer struct {
	content string
	err     error
}

func (c completer) Complete(context.Context, preparation.CompletionRequest) (preparation.Completion, error) {
	return preparation.Completion{Content: c.content, PromptTokens: 9}, c.err
}

func TestRun(t *testing.T) {
	records := []training.Record{{Prompt: "Where?", Target: "here", Evidence: "It is here."}, {Prompt: "When?", Target: "now", Evidence: "It is now."}, {Prompt: "Who?", Target: "Ada", Evidence: "Ada works."}, {Prompt: "What?", Target: "boat", Evidence: "A boat."}}
	for _, tc := range []struct {
		name, response string
		warnings       []string
		invalid        bool
	}{
		{name: "all pass", response: `{"record_1":"pass","record_2":"pass","record_3":"pass","record_4":"pass"}`},
		{name: "rejections", response: `{"record_4":"user_requirements","record_2":"grounding","record_1":"pass","record_3":"task_quality"}`, warnings: []string{"record 2: grounding", "record 3: task_quality", "record 4: user_requirements"}},
		{name: "missing", response: `{"record_1":"pass"}`, invalid: true},
		{name: "wrong keys", response: `{"record_1":"pass","record_2":"pass","record_3":"pass","other":"pass"}`, invalid: true},
		{name: "unknown outcome", response: `{"record_1":"yes","record_2":"pass","record_3":"pass","record_4":"pass"}`, invalid: true},
		{name: "malformed", response: `{`, invalid: true},
		{name: "trailing", response: `{} {}`, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			warnings, usage, err := verify.Run(t.Context(), completer{content: tc.response}, preparation.PreparationRequest{Goal: preparation.GoalQuestionAnswer}, records)
			if (err != nil) != tc.invalid {
				t.Fatalf("error = %v", err)
			}
			if !tc.invalid && !reflect.DeepEqual(warnings, tc.warnings) {
				t.Fatalf("warnings = %v, want %v", warnings, tc.warnings)
			}
			if usage.PromptTokens != 9 {
				t.Fatalf("lost usage: %+v", usage)
			}
		})
	}
	want := errors.New("completion failed")
	if _, _, err := verify.Run(t.Context(), completer{err: want}, preparation.PreparationRequest{}, records); !errors.Is(err, want) {
		t.Fatalf("error = %v", err)
	}
}
