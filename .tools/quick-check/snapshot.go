package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
)

// A snapshot records the result of checking the full artifacts, not a second
// copy of their logs or generated answers. Hashes identify evidence; they are
// not a signature or a substitute for retaining that evidence for investigation.
type comparisonSnapshot struct {
	Version  int                     `json:"version"`
	Summary  matrixSummary           `json:"summary"`
	Cells    map[string]snapshotCell `json:"cells"`
	Evidence map[string]string       `json:"evidence_sha256"`
	Problems []string                `json:"validation_problems"`
	Digest   string                  `json:"snapshot_sha256"`
}

type snapshotCell struct {
	Identity   comparisonRun        `json:"identity"`
	Metrics    map[string][]float64 `json:"metric_samples"`
	Model      map[string]any       `json:"model_metadata"`
	Native     map[string]string    `json:"native_parameters"`
	Inputs     string               `json:"controlled_inputs_sha256"`
	TargetOnly bool                 `json:"target_only"`
}

func controlledInputHash(input comparisonInput) string {
	if input.snapshot != nil {
		return input.snapshot.Inputs
	}
	var requests []any
	for _, value := range comparisonRequests(input) {
		request := value.(map[string]any)
		body, _ := request["body"].(map[string]any)
		controlled := make(map[string]any, len(body))
		maps.Copy(controlled, body)
		if messages, ok := body["messages"].([]any); ok {
			filtered := []any{}
			for _, value := range messages {
				message, _ := value.(map[string]any)
				if message["role"] != "assistant" {
					filtered = append(filtered, value)
				}
			}
			controlled["messages"] = filtered
		}
		requests = append(requests, map[string]any{"phase": request["phase"], "repetition": request["repetition"], "body": controlled})
	}
	data, _ := json.Marshal(requests)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func (input comparisonInput) metricSamples() map[string][]float64 {
	if input.snapshot != nil {
		return input.snapshot.Metrics
	}
	return comparisonMetrics(input.report)
}

func snapshotDigest(s comparisonSnapshot) string {
	s.Digest = ""
	data, _ := json.Marshal(s)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func readSnapshot(dir string) (comparisonSnapshot, error) {
	var s comparisonSnapshot
	data, err := os.ReadFile(filepath.Join(dir, "comparison-snapshot.json"))
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, err
	}
	if s.Version != 1 || s.Digest != snapshotDigest(s) || len(s.Summary.Cells) == 0 || len(s.Cells) != len(s.Summary.Cells) ||
		(s.Summary.Status != "PASS" && s.Summary.Status != "FAIL" && s.Summary.Status != "INTERRUPTED") {
		return s, fmt.Errorf("invalid comparison snapshot version, checksum, status or cells")
	}
	seen := map[string]bool{}
	for _, cell := range s.Summary.Cells {
		c, ok := s.Cells[cell.ID]
		if !ok || seen[cell.ID] || cell.ID == "" || c.Identity.Status != cell.Status ||
			c.Identity.ReferenceTokens != cell.ReferenceTokens || c.Identity.Executed != cell.RepetitionsExecuted || c.Identity.Requested != cell.RepetitionsRequested ||
			c.Identity.Version != cell.ServerVersion ||
			(cell.Status != "NOT_RUN" && len(c.Identity.Problems) == 0 && cell.ServerRevision != s.Summary.ServerRevision) {
			return s, fmt.Errorf("inconsistent snapshot cell %s", cell.ID)
		}
		seen[cell.ID] = true
		if len(c.Identity.Problems) > 0 && len(c.Metrics) > 0 {
			return s, fmt.Errorf("snapshot cell %s has measurements despite invalid evidence", cell.ID)
		}
	}
	return s, nil
}

func snapshotInput(s comparisonSnapshot) matrixInput {
	input := matrixInput{summary: s.Summary, cells: map[string]comparisonInput{}, problems: s.Problems}
	for _, cell := range s.Summary.Cells {
		c := s.Cells[cell.ID]
		input.cells[cell.ID] = comparisonInput{
			summary: cell, snapshot: &c, native: c.Native,
			report: report{TargetOnly: c.TargetOnly, Metadata: map[string]any{"model": c.Model}},
		}
	}
	return input
}

func makeSnapshot(dir string) (comparisonSnapshot, error) {
	if _, err := os.Stat(filepath.Join(dir, "summary.json")); os.IsNotExist(err) {
		return readSnapshot(dir)
	}
	s := comparisonSnapshot{Version: 1, Cells: map[string]snapshotCell{}, Evidence: map[string]string{}}
	input, validationErr := readFullComparison(dir)
	if validationErr != nil {
		// Preserve failed, interrupted, or damaged evidence honestly. A recorded
		// PASS is not rewritten, but unvalidated measurements cannot be compared.
		data, err := os.ReadFile(filepath.Join(dir, "summary.json"))
		if err != nil {
			return s, err
		}
		if err := json.Unmarshal(data, &input.summary); err != nil {
			return s, err
		}
		s.Problems = []string{validationErr.Error()}
	}
	s.Summary = input.summary
	for i := range s.Summary.Cells {
		cell := &s.Summary.Cells[i]
		c := snapshotCell{Identity: comparisonRun{Version: cell.ServerVersion, Status: cell.Status, Executed: cell.RepetitionsExecuted, Requested: cell.RepetitionsRequested, ReferenceTokens: cell.ReferenceTokens}}
		if validationErr != nil {
			c.Identity.Problems = append(append([]string{}, cell.Findings...), "evidence validation: "+validationErr.Error())
		} else if cell.Status != "NOT_RUN" {
			ci := input.cells[cell.ID]
			c.Identity, c.Native, c.Inputs, c.TargetOnly = comparisonIdentity("", ci), ci.native, controlledInputHash(ci), ci.report.TargetOnly
			model, _ := ci.report.Metadata["model"].(map[string]any)
			c.Model = make(map[string]any)
			for _, key := range []string{"size", "quantization", "file_type", "template", "metadata", "model_config"} {
				c.Model[key] = model[key]
			}
			if len(c.Identity.Problems) == 0 {
				c.Metrics = ci.metricSamples()
			}
		}
		// Measurements live once as per-repetition metric samples. No generated
		// content, raw request bodies, or server events enter the snapshot.
		cell.Measurements, cell.WarmupRequest = nil, nil
		for j := range cell.Repetitions {
			cell.Repetitions[j].Results = nil
		}
		s.Cells[cell.ID] = c
	}
	for _, name := range []string{"summary.json", "events.ndjson", "tool.log"} {
		f, err := os.Open(filepath.Join(dir, name))
		if os.IsNotExist(err) {
			continue // The validation problem above records the missing evidence.
		}
		if err != nil {
			return s, err
		}
		h := sha256.New()
		_, err = io.Copy(h, f)
		f.Close()
		if err != nil {
			return s, err
		}
		s.Evidence[name] = hex.EncodeToString(h.Sum(nil))
	}
	s.Digest = snapshotDigest(s)
	return s, nil
}

func snapshotCommand(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("quick-check snapshot", flag.ContinueOnError)
	flags.SetOutput(out)
	parent := flags.String("out", ".tools/quick-check/output", "retained queue")
	dest := flags.String("dest", "", "new directory for compact queue snapshots")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *dest == "" || flags.NArg() != 0 {
		return fmt.Errorf("usage: quick-check snapshot -out DIR -dest NEW_DIR")
	}
	runs, err := savedRuns(*parent)
	if err != nil {
		return err
	}
	if len(runs) == 0 {
		return fmt.Errorf("no retained runs to snapshot")
	}
	if err := os.Mkdir(*dest, 0755); err != nil {
		return err
	}
	for _, run := range runs {
		s, err := makeSnapshot(run.path)
		if err != nil {
			return fmt.Errorf("%s: %w", run.path, err)
		}
		dir := filepath.Join(*dest, filepath.Base(run.path))
		if err := os.Mkdir(dir, 0755); err != nil {
			return err
		}
		data, err := json.MarshalIndent(s, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "comparison-snapshot.json"), append(data, '\n'), 0644); err != nil {
			return err
		}
		if _, err := readSnapshot(dir); err != nil {
			return err
		}
		if run.promoted != "" {
			data, err := os.ReadFile(filepath.Join(run.path, "promotion.json"))
			if err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(dir, "promotion.json"), data, 0644); err != nil {
				return err
			}
		}
		fmt.Fprintf(out, "Saved comparison snapshot: %s\n", dir)
	}
	return nil
}
