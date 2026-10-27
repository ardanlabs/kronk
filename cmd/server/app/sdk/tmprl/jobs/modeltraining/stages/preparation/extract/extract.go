// Package extract converts source documents into cached text and bounded passages
// for training-data preparation.
package extract

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/imports"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// Run extracts a document and caches it by source identity and extractor settings
// inside the execution workspace. After successful cache publication, retries do
// not upload again. Failed or interrupted conversion may be repeated.
func Run(ctx context.Context, extractor preparation.DocumentExtractor, sourceInfo imports.PreparationSource, sourceRoot, workspace *os.Root) (preparation.Document, error) {
	profile := "tuning-text-v1"
	if sourceInfo.Format == ".pdf" || sourceInfo.Format == ".docx" {
		if extractor == nil {
			return preparation.Document{}, fmt.Errorf("%w: PDF/DOCX extraction is not configured", preparation.ErrInvalidArgument)
		}
		profile = extractor.ExtractionProfile()
	}
	type extraction struct {
		SourceHash string               `json:"source_hash"`
		Profile    string               `json:"profile"`
		Document   preparation.Document `json:"document"`
	}
	cache := "extraction.json"
	if data, err := workspace.ReadFile(cache); err == nil {
		var saved extraction
		if err := json.Unmarshal(data, &saved); err != nil {
			return preparation.Document{}, err
		}
		if saved.SourceHash == sourceInfo.SHA256 && saved.Profile == profile {
			return saved.Document, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return preparation.Document{}, err
	}
	// Extractors accept a path and may reopen it. Snapshot through the confined
	// root and verify the actual copied bytes before handing over a private path.
	source, err := sourceRoot.Open(filepath.Join("imports", sourceInfo.ID, "source"+sourceInfo.Format))
	if err != nil {
		return preparation.Document{}, err
	}
	defer source.Close()
	temporary, err := os.MkdirTemp("", "moxie-extraction-")
	if err != nil {
		return preparation.Document{}, err
	}
	defer os.RemoveAll(temporary)
	path := filepath.Join(temporary, "source"+sourceInfo.Format)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return preparation.Document{}, err
	}
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(tmprl.ContextReader{Context: ctx, Reader: source}, (100<<20)+1))
	if err := errors.Join(copyErr, file.Close()); err != nil {
		return preparation.Document{}, err
	}
	if size > 100<<20 || hex.EncodeToString(hash.Sum(nil)) != sourceInfo.SHA256 {
		return preparation.Document{}, fmt.Errorf("%w: dataset source integrity check failed", preparation.ErrInvalidArgument)
	}
	var document preparation.Document
	if sourceInfo.Format == ".md" || sourceInfo.Format == ".txt" {
		data, err := os.ReadFile(path)
		if err != nil {
			return preparation.Document{}, err
		}
		document.Markdown = string(data)
		structured, err := json.Marshal(map[string]any{"texts": []map[string]string{{"text": string(data)}}})
		if err != nil {
			return preparation.Document{}, err
		}
		document.StructuredJSON = structured
	} else {
		var err error
		document, err = extractor.Convert(ctx, path)
		if err != nil {
			return preparation.Document{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return preparation.Document{}, err
	}
	if strings.TrimSpace(document.Markdown) == "" || !utf8.ValidString(document.Markdown) || !json.Valid(document.StructuredJSON) {
		return preparation.Document{}, fmt.Errorf("extractor returned invalid or empty document content")
	}
	if err := tmprl.WriteJSON(workspace, cache, extraction{sourceInfo.SHA256, profile, document}); err != nil {
		return preparation.Document{}, err
	}
	return document, nil
}

// Passages selects bounded, source-quoted candidates and counts excluded pieces.
// These candidates prevent the model from inventing targets. Paragraphs retain
// context; sentence-sized passages bound model payloads.
func Passages(markdown string) ([]string, int) {
	var passages []string
	excluded := 0
	seen := make(map[string]bool)
	for paragraph := range strings.SplitSeq(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n\n") {
		var lines []string
		for line := range strings.SplitSeq(paragraph, "\n") {
			line = strings.TrimSpace(line)
			if line == "<!-- image -->" || strings.HasPrefix(line, "#") && strings.HasPrefix(strings.TrimLeft(line, "#"), " ") {
				continue
			}
			lines = append(lines, line)
		}
		paragraph = training.Normalized(strings.Join(lines, " "))
		if paragraph == "" {
			continue
		}
		pieces := []string{paragraph}
		if utf8.RuneCountInString(paragraph) > 400 {
			pieces = strings.SplitAfter(paragraph, ". ")
		}
		for _, piece := range pieces {
			piece = training.Normalized(piece)
			if piece == "" {
				continue
			}
			// Do not cut long code, tables or prose at arbitrary character
			// boundaries and present a truncated statement as a complete fact.
			if utf8.RuneCountInString(piece) > 400 {
				excluded++
				continue
			}
			if !seen[piece] {
				seen[piece] = true
				passages = append(passages, piece)
			}
		}
	}
	return passages, excluded
}
