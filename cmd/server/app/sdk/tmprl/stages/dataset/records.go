// Package dataset owns record validation, identity, training split policy,
// and shared file operations for datasets and their derived artifacts.
package dataset

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/model"
)

func Normalized(text string) string { return strings.Join(strings.Fields(text), " ") }
func Key(text string) string        { return strings.ToLower(Normalized(text)) }

func FactID(evidence, target string) string {
	hash := sha256.Sum256([]byte(Key(evidence) + "\x00" + Key(target)))
	return hex.EncodeToString(hash[:16])
}

func ValidateReviewedRecords(records []model.Record) error {
	for _, r := range records {
		if r.ValidationPrompt || r.IndependentPrompt {
			return fmt.Errorf("reviewed examples must be training records")
		}
		if !r.UserApproved && !strings.Contains(Normalized(r.Evidence), Normalized(r.Target)) {
			return fmt.Errorf("target must be quoted from evidence unless explicitly approved")
		}
	}
	return ValidateRecords(records)
}

func ValidateRecords(records []model.Record) error {
	prompts := make(map[string]bool)
	facts := make(map[string]string)
	factIDs := make(map[string]string)
	for i, r := range records {
		if utf8.RuneCountInString(strings.TrimSpace(r.Prompt)) < 3 || Normalized(r.Target) == "" || Normalized(r.Evidence) == "" {
			return fmt.Errorf("record %d requires prompt, target and evidence", i+1)
		}
		if r.ValidationPrompt && r.IndependentPrompt || r.UserApproved && (r.ValidationPrompt || r.IndependentPrompt) {
			return fmt.Errorf("record %d has conflicting split/approval flags", i+1)
		}
		key := Key(r.Prompt)
		if prompts[key] {
			return fmt.Errorf("record %d repeats a prompt", i+1)
		}
		prompts[key] = true
		if r.FactID != "" {
			identity := Key(r.Evidence) + "\x00" + Key(r.Target)
			if previous, exists := facts[r.FactID]; exists && previous != identity {
				return fmt.Errorf("record %d reuses a fact identity for different knowledge", i+1)
			}
			if previous, exists := factIDs[identity]; exists && previous != r.FactID {
				return fmt.Errorf("record %d assigns different fact IDs to the same knowledge", i+1)
			}
			facts[r.FactID] = identity
			factIDs[identity] = r.FactID
		}
		if len(records) > 0 && (r.FactID == "") != (records[0].FactID == "") {
			return fmt.Errorf("mixed fact and non-fact records")
		}
		if r.FactID == "" && (r.ValidationPrompt || r.IndependentPrompt) {
			return fmt.Errorf("evaluation records require fact identities")
		}
	}
	return nil
}

// Strict JSONL parsing is SDK-owned. Unknown fields, blank rows, trailing JSON,
// conflicting facts and duplicate prompts are rejected by this parser and import.
// A backend consuming other references must validate its own inputs.
func ReadRecords(ctx context.Context, reader io.Reader) ([]model.Record, error) {
	scanner := bufio.NewScanner(ContextReader{Context: ctx, Reader: reader})
	scanner.Buffer(make([]byte, 4096), 16<<20)
	var records []model.Record
	for scanner.Scan() {
		decoder := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
		decoder.DisallowUnknownFields()
		var r model.Record
		if err := decoder.Decode(&r); err != nil {
			return nil, fmt.Errorf("JSONL record %d: %w", len(records)+1, err)
		}
		var trailing any
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("JSONL record %d contains trailing data", len(records)+1)
		}
		records = append(records, r)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("empty dataset")
	}
	return records, ValidateRecords(records)
}
