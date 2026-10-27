package definition_test

import (
	"testing"
	"time"

	trainingdefinition "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/definition"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

func TestLoad(t *testing.T) {
	job := load(t)
	if job.Name != "model-training" || job.QueueTimeout != 24*time.Hour || job.ReservationTimeout != 168*time.Hour || job.SessionAttempts != 3 {
		t.Fatalf("job: got %+v, want model-training with 24h queue, 168h reservation, 3 attempts", job)
	}
	want := []struct {
		name    string
		timeout time.Duration
		steps   []string
	}{
		{"preparation", 24 * time.Hour, []string{"extract", "generate", "verify"}},
		{"training", 144 * time.Hour, []string{"split", "train", "evaluate", "publish"}},
	}
	if len(job.Stages) != len(want) {
		t.Fatalf("stages: got %d, want %d", len(job.Stages), len(want))
	}
	for i, stage := range job.Stages {
		if stage.Name != want[i].name || stage.Timeout != want[i].timeout || len(stage.Steps) != len(want[i].steps) {
			t.Fatalf("stage %d: got %+v, want %+v", i, stage, want[i])
		}
		for k, step := range stage.Steps {
			retry := tmprl.Retry{MaximumAttempts: 3, InitialInterval: time.Second, BackoffCoefficient: 2, MaximumInterval: time.Minute}
			if step.Name != want[i].steps[k] || step.Timeout != 24*time.Hour || step.HeartbeatTimeout != 5*time.Second || step.Retry != retry {
				t.Errorf("step %d/%d: got %+v, want %s with 24h timeout, 5s heartbeat and %+v", i, k, step, want[i].steps[k], retry)
			}
		}
	}
	if err := job.Validate(); err != nil {
		t.Errorf("Validate: got %v, want nil", err)
	}

	// Neither repeated loads nor aliased YAML retry policies share mutable data.
	job.Stages[0].Steps[0].Retry.MaximumAttempts = 99
	if got := job.Stages[0].Steps[1].Retry.MaximumAttempts; got != 3 {
		t.Errorf("aliased retry attempts: got %d, want 3", got)
	}
	job.Stages[0].Steps[0].Name = "changed"
	job.Stages[0].Name = "changed"
	fresh := load(t)
	if fresh.Stages[0].Name != "preparation" || fresh.Stages[0].Steps[0].Name != "extract" || fresh.Stages[0].Steps[0].Retry.MaximumAttempts != 3 {
		t.Errorf("fresh load: got %+v, want unchanged preparation/extract", fresh)
	}
}

func load(t *testing.T) tmprl.Job {
	t.Helper()
	job, err := trainingdefinition.Load()
	if err != nil {
		t.Fatalf("Load: got %v, want nil", err)
	}
	return job
}
