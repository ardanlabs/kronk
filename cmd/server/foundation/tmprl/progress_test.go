package tmprl_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

func TestProgressRemaining(t *testing.T) {
	start := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name                      string
		initial, completed, total int
		elapsed                   time.Duration
		want                      time.Duration
	}{
		{"new work", 0, 4, 20, 2 * time.Minute, 8 * time.Minute},
		{"resumed work", 12, 16, 20, 2 * time.Minute, 2 * time.Minute},
		{"no new work", 12, 12, 20, time.Minute, 0},
		{"counter reset", 12, 3, 20, time.Minute, 0},
		{"finished", 0, 20, 20, time.Minute, 0},
		{"unknown total", 0, 4, 0, time.Minute, 0},
		{"no timed sample", 0, 4, 20, 0, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := tmprl.Progress{InitialCompleted: test.initial, Completed: test.completed, Total: test.total, StartedAt: start, UpdatedAt: start.Add(test.elapsed)}
			data, err := json.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			var restored tmprl.Progress
			if err := json.Unmarshal(data, &restored); err != nil {
				t.Fatal(err)
			}
			if got := restored.Remaining(); got != test.want {
				t.Fatalf("restored progress estimate = %v, want %v", got, test.want)
			}
		})
	}
	if got := (tmprl.Progress{Completed: 4, Total: 20}).Remaining(); got != 0 {
		t.Fatalf("old heartbeat without timing produced estimate %v", got)
	}
}

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
