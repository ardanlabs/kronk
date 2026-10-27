// Package definition owns the ordered training job and its execution policies.
package definition

import (
	_ "embed"

	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// jobYAML is immutable embedded source, never a cached decoded job.
//
//go:embed job.yaml
var jobYAML string

// Load parses the embedded job, returning fresh, caller-owned stages and steps.
func Load() (tmprl.Job, error) {
	return tmprl.ParseJob([]byte(jobYAML))
}
