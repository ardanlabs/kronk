package tmprl_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl"
	"github.com/stretchr/testify/mock"
	commonpb "go.temporal.io/api/common/v1"
	workflowpb "go.temporal.io/api/workflow/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/mocks"
)

func TestHistory(t *testing.T) {
	c := &mocks.Client{}
	c.On("ListWorkflow", mock.Anything, &workflowservice.ListWorkflowExecutionsRequest{
		PageSize: 3, NextPageToken: []byte("page-two"), Query: "WorkflowType = 'example.Export'",
	}).Return(&workflowservice.ListWorkflowExecutionsResponse{
		Executions: []*workflowpb.WorkflowExecutionInfo{
			{Execution: &commonpb.WorkflowExecution{WorkflowId: "export-a", RunId: "run-2"}},
			{Execution: &commonpb.WorkflowExecution{WorkflowId: "export-b", RunId: "run-7"}},
		},
		NextPageToken: []byte("page-three"),
	}, nil)

	runs, next, err := tmprl.History(t.Context(), c, "example.Export", 3, []byte("page-two"))
	if err != nil {
		t.Fatal(err)
	}
	want := []tmprl.Execution{{WorkflowID: "export-a", RunID: "run-2"}, {WorkflowID: "export-b", RunID: "run-7"}}
	if !reflect.DeepEqual(runs, want) || string(next) != "page-three" {
		t.Fatalf("history: got %v, token %q; want %v, token page-three", runs, next, want)
	}

	for _, size := range []int{-1, 0, 101} {
		if _, _, err := tmprl.History(t.Context(), c, "example.Export", size, nil); !errors.Is(err, tmprl.ErrInvalidPageSize) {
			t.Fatalf("page size %d: %v", size, err)
		}
	}
}
