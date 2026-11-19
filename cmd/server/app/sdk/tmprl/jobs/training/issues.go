package training

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// runIssues projects persisted evidence into a whitelist of codes and counts.
// Missing or unreadable evidence never hides the run's execution state or turns
// an unknown failure into a claimed cause. No checkpoint text leaves this owner.
func (j *Job) runIssues(run tmprl.Run[Request, Artifacts]) []RunIssue {
	if !run.Status.Terminal() {
		return nil
	}
	root, err := os.OpenRoot(j.cfg.StorageRoot)
	if err != nil {
		return nil
	}
	defer root.Close()
	if FailureReason(run.FailureTypes) == "training-records-insufficient" {
		file, err := root.Open(filepath.Join(storageDirectory, run.RunID, "steps", "training", "requirements.json"))
		if err != nil {
			return nil
		}
		defer file.Close()
		var issue RunIssue
		if json.NewDecoder(io.LimitReader(file, 4096)).Decode(&issue) == nil && issue.Code == "training-records-insufficient" && issue.Count >= 0 && issue.Required > issue.Count {
			return []RunIssue{issue}
		}
		return nil
	}
	if run.Request.Prepare == nil {
		return nil
	}
	file, err := root.Open(filepath.Join(storageDirectory, run.RunID, "steps", "preparation", "steps.json"))
	if err != nil {
		return nil
	}
	defer file.Close()
	var checkpoint struct {
		Result preparation.PreparationResult `json:"result"`
	}
	if json.NewDecoder(io.LimitReader(file, 32<<20)).Decode(&checkpoint) != nil {
		return nil
	}
	counts := make(map[string]int)
	for _, exclusion := range checkpoint.Result.Exclusions {
		code := "preparation-omissions"
		if strings.HasPrefix(exclusion.Reason, "generation failed after ") {
			code = "preparation-generation-failed"
		}
		if strings.HasPrefix(exclusion.Reason, "verification rejected example ") {
			code = ""
			for _, category := range []string{"grounding", "task_quality", "user_requirements"} {
				if strings.HasSuffix(exclusion.Reason, ": "+category) {
					code = category
					break
				}
			}
		}
		if code != "" {
			counts[code]++
		}
	}
	var issues []RunIssue
	for _, code := range []string{"grounding", "task_quality", "user_requirements", "preparation-generation-failed", "preparation-omissions"} {
		if count := counts[code]; count > 0 {
			issues = append(issues, RunIssue{Code: code, Count: count})
		}
	}
	return issues
}
