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

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/providers"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/uploads"
)

var errExtractionMismatch = errors.New("extraction does not match source or profile")

type extraction struct {
	SourceHash string             `json:"source_hash"`
	Profile    string             `json:"profile"`
	Document   providers.Document `json:"document"`
}

// Run extracts a document and caches it by source identity and extractor settings
// inside the execution workspace. After successful cache publication, retries do
// not upload again. Failed or interrupted conversion may be repeated.
func Run(ctx context.Context, extractor preparation.DocumentExtractor, sourceInfo uploads.File, openSource func(context.Context) (io.ReadCloser, error), workspace *os.Root) (providers.Document, error) {
	profile, err := extractionProfile(extractor, sourceInfo)
	if err != nil {
		return providers.Document{}, err
	}
	cached, err := Read(ctx, extractor, sourceInfo, workspace)
	switch {
	case err == nil:
		return cached, nil
	case errors.Is(err, os.ErrNotExist), errors.Is(err, errExtractionMismatch):
		// Only Run may convert a missing or stale extraction.
	default:
		return providers.Document{}, err
	}
	// Extractors accept a path and may reopen it. Snapshot through the confined
	// root and verify the actual copied bytes before handing over a private path.
	source, err := openSource(ctx)
	if err != nil {
		return providers.Document{}, err
	}
	defer source.Close()
	temporary, err := os.MkdirTemp("", "moxie-extraction-")
	if err != nil {
		return providers.Document{}, err
	}
	defer os.RemoveAll(temporary)
	path := filepath.Join(temporary, "source"+sourceInfo.Format)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return providers.Document{}, err
	}
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(tmprl.ContextReader{Context: ctx, Reader: source}, (100<<20)+1))
	if err := errors.Join(copyErr, file.Close()); err != nil {
		return providers.Document{}, err
	}
	if size > 100<<20 || hex.EncodeToString(hash.Sum(nil)) != sourceInfo.SHA256 {
		return providers.Document{}, fmt.Errorf("%w: dataset source integrity check failed", preparation.ErrInvalidArgument)
	}
	var document providers.Document
	if sourceInfo.Format == ".md" || sourceInfo.Format == ".txt" {
		data, err := os.ReadFile(path)
		if err != nil {
			return providers.Document{}, err
		}
		document.Markdown = string(data)
		structured, err := json.Marshal(map[string]any{"texts": []map[string]string{{"text": string(data)}}})
		if err != nil {
			return providers.Document{}, err
		}
		document.StructuredJSON = structured
	} else {
		var err error
		document, err = extractor.Convert(ctx, path)
		if err != nil {
			return providers.Document{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return providers.Document{}, err
	}
	if strings.TrimSpace(document.Markdown) == "" || !utf8.ValidString(document.Markdown) || !json.Valid(document.StructuredJSON) {
		return providers.Document{}, fmt.Errorf("extractor returned invalid or empty document content")
	}
	if err := tmprl.WriteJSON(workspace, "extraction.json", extraction{sourceInfo.SHA256, profile, document}); err != nil {
		return providers.Document{}, err
	}
	return document, nil
}

// Read returns saved extraction for the exact source and extraction profile.
// It never opens or converts the source document and never writes extraction.
func Read(ctx context.Context, extractor preparation.DocumentExtractor, sourceInfo uploads.File, workspace *os.Root) (providers.Document, error) {
	if err := ctx.Err(); err != nil {
		return providers.Document{}, err
	}
	profile, err := extractionProfile(extractor, sourceInfo)
	if err != nil {
		return providers.Document{}, err
	}
	data, err := workspace.ReadFile("extraction.json")
	if err != nil {
		return providers.Document{}, fmt.Errorf("read extracted document: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return providers.Document{}, err
	}
	var saved extraction
	if err := json.Unmarshal(data, &saved); err != nil {
		return providers.Document{}, fmt.Errorf("decode extracted document: %w", err)
	}
	if saved.SourceHash != sourceInfo.SHA256 || saved.Profile != profile {
		return providers.Document{}, fmt.Errorf("%w: %w", preparation.ErrInvalidArgument, errExtractionMismatch)
	}
	return saved.Document, nil
}

func extractionProfile(extractor preparation.DocumentExtractor, sourceInfo uploads.File) (string, error) {
	if sourceInfo.Format == ".pdf" || sourceInfo.Format == ".docx" {
		if extractor == nil {
			return "", fmt.Errorf("%w: PDF/DOCX extraction is not configured", preparation.ErrInvalidArgument)
		}
		return extractor.ExtractionProfile(), nil
	}
	return "tuning-text-v1", nil
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
