package tmprl_test

import (
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl"
)

func TestProgressSubscriptions(t *testing.T) {
	var broker tmprl.ProgressBroker[tmprl.Execution, string]
	run := tmprl.Execution{WorkflowID: "training", RunID: "run-one"}
	otherRun := tmprl.Execution{WorkflowID: "training", RunID: "run-two"}
	updates, unsubscribe := broker.Subscribe(run)
	defer unsubscribe()
	other, unsubscribeOther := broker.Subscribe(otherRun)
	defer unsubscribeOther()
	broker.Publish(run, "split")
	broker.Publish(run, "train")
	if got := <-updates; got != "train" {
		t.Fatalf("slow consumer received %q, want latest progress train", got)
	}
	select {
	case got := <-other:
		t.Fatalf("progress leaked to another run: %q", got)
	default:
	}
	unsubscribe()
	unsubscribe()
	broker.Publish(run, "evaluate")
	if _, open := <-updates; open {
		t.Fatal("unsubscribe did not close the subscription")
	}
	broker.Publish(otherRun, "publish")
	if got := <-other; got != "publish" {
		t.Fatalf("unsubscribing another run affected this consumer: %q", got)
	}
}
