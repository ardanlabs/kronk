package training

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// FailureEntry is a public diagnostic event. Messages and codes are fixed by
// this owner; backend error text, model output and filesystem paths are excluded.
type FailureEntry struct {
	Step    string    `json:"step"`
	Code    string    `json:"code"`
	Message string    `json:"message"`
	Time    time.Time `json:"time,omitzero"`
	Attempt int32     `json:"attempt,omitzero"`
}

// failureLog reconstructs the recorded sequence, not every intermediate retry.
// It also works with retained execution history from runs created before logs
// were added. Unknown errors are never classified by searching private text.
func failureLog(run tmprl.Run[Request, Artifacts]) []FailureEntry {
	if run.Status != tmprl.StatusFailed {
		return nil
	}
	labels := map[string]string{"extract": "Document extraction", "preview": "Preview generation", "generate": "Record generation", "verify": "Record verification", "split": "Dataset split", "train": "Adapter training", "evaluate": "Adapter evaluation", "publish": "Adapter publication"}
	code, message := "unclassified-failure", "The saved run does not contain a recoverable failure cause."
	if run.Error != "" {
		message = "An internal error stopped this step. Its recorded error has no public diagnostic category."
	}
	// Only known type identities can supply a cause. Arbitrary exception names
	// and messages are never echoed into the saved catalog or HTTP response.
	messages := map[string]string{
		"dataset-document-exists":              "This document and page range are already in the dataset.",
		"dataset-questions-exist":              "Every generated question already exists in the dataset.",
		"dataset-changed":                      "The dataset changed before the records could be saved.",
		"preparation-no-examples":              "The model did not produce usable examples from the selected content.",
		"preparation-no-source-units":          "No supported source passages were available for this preparation goal.",
		"preparation-request-budget-exhausted": "Preparation reached its generation request limit before resolving all source passages.",
		"preparation-no-verified-examples":     "None of the generated records passed verification.",
		"preparation-checkpoint-version":       "Saved progress uses an older checkpoint format and cannot be resumed.",
		"invalid-training":                     "The run input failed validation.",
		"preparation-extractor-unavailable":    "The document conversion service (Docling) was unavailable.",
		"preparation-page-range":               "The selected page range starts after the document's last page.",
		"preparation-extraction-failed":        "The document conversion service could not read the document.",
		"training-records-insufficient":        "The dataset has too few training records.",
		"invalid-step-output":                  "The step returned an invalid result.",
		"invalid-step-checkpoint":              "The saved progress checkpoint is missing, changed, or invalid.",
		"SyntaxError":                          "The structured response could not be read as valid JSON.",
		"UnmarshalTypeError":                   "The structured response contained a value of the wrong type.",
		"timeout:HEARTBEAT":                    "The worker stopped reporting progress before the heartbeat deadline.",
		"timeout:START_TO_CLOSE":               "The step exceeded its execution time limit.",
		"timeout:SCHEDULE_TO_CLOSE":            "The activity exceeded its total time limit, including retries.",
		"timeout:SCHEDULE_TO_START":            "No worker started the activity before its queue deadline.",
		"workflow-timeout":                     "The run exceeded its workflow time limit.",
		"workflow-terminated":                  "The workflow was terminated before it finished.",
		"deadlineExceededError":                "The operation did not finish before its request deadline.",
		"PathError":                            "A required file could not be accessed.",
	}
	for _, kind := range run.FailureTypes {
		if text, ok := messages[kind]; ok {
			code, message = kind, text
			break
		}
	}
	var log []FailureEntry
	failed := false
	for _, stage := range run.Stages {
		for _, step := range stage.Steps {
			label, ok := labels[step.Name]
			if !ok {
				continue
			}
			switch step.Status {
			case tmprl.StatusCompleted:
				log = append(log, FailureEntry{Step: step.Name, Code: "step-completed", Message: label + " completed.", Time: step.ClosedAt, Attempt: step.Attempt})
			case tmprl.StatusFailed:
				if !step.StartedAt.IsZero() {
					log = append(log, FailureEntry{Step: step.Name, Code: "step-started", Message: label + " started.", Time: step.StartedAt})
				}
				log = append(log, FailureEntry{Step: step.Name, Code: code, Message: message, Time: step.ClosedAt, Attempt: step.Attempt})
				failed = true
			}
		}
	}
	if !failed {
		step := ""
		if _, ok := labels[run.Step]; ok {
			step = run.Step
		}
		log = append(log, FailureEntry{Step: step, Code: code, Message: message, Time: run.ClosedAt})
	}
	return log
}

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
