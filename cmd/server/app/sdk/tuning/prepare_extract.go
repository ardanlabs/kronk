package tuning

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
)

// Extraction output is cached by source identity and extractor settings inside
// the execution workspace. After successful cache publication, retries do not
// upload again. Failed or interrupted conversion may be repeated.
func (b *preparationEngine) extractDocument(ctx context.Context, dataset PreparationSource, sourceRoot, workspace *os.Root) (Document, error) {
	profile := "tuning-text-v1"
	if dataset.Format == ".pdf" || dataset.Format == ".docx" {
		if b.Extractor == nil {
			return Document{}, fmt.Errorf("%w: PDF/DOCX extraction is not configured", ErrInvalidArgument)
		}
		profile = b.Extractor.ExtractionProfile()
	}
	type extraction struct {
		SourceHash string   `json:"source_hash"`
		Profile    string   `json:"profile"`
		Document   Document `json:"document"`
	}
	cache := "extraction.json"
	if data, err := workspace.ReadFile(cache); err == nil {
		var saved extraction
		if err := json.Unmarshal(data, &saved); err != nil {
			return Document{}, err
		}
		if saved.SourceHash == dataset.SHA256 && saved.Profile == profile {
			return saved.Document, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Document{}, err
	}
	// Extractors accept a path and may reopen it. Snapshot through the confined
	// root and verify the actual copied bytes before handing over a private path.
	source, err := sourceRoot.Open(filepath.Join("imports", dataset.ID, "source"+dataset.Format))
	if err != nil {
		return Document{}, err
	}
	defer source.Close()
	temporary, err := os.MkdirTemp("", "moxie-extraction-")
	if err != nil {
		return Document{}, err
	}
	defer os.RemoveAll(temporary)
	path := filepath.Join(temporary, "source"+dataset.Format)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return Document{}, err
	}
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(contextReader{Context: ctx, Reader: source}, (100<<20)+1))
	if err := errors.Join(copyErr, file.Close()); err != nil {
		return Document{}, err
	}
	if size > 100<<20 || hex.EncodeToString(hash.Sum(nil)) != dataset.SHA256 {
		return Document{}, fmt.Errorf("%w: dataset source integrity check failed", ErrInvalidArgument)
	}
	var document Document
	if dataset.Format == ".md" || dataset.Format == ".txt" {
		data, err := os.ReadFile(path)
		if err != nil {
			return Document{}, err
		}
		document.Markdown = string(data)
		structured, err := json.Marshal(map[string]any{"texts": []map[string]string{{"text": string(data)}}})
		if err != nil {
			return Document{}, err
		}
		document.StructuredJSON = structured
	} else {
		var err error
		document, err = b.Extractor.Convert(ctx, path)
		if err != nil {
			return Document{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	if strings.TrimSpace(document.Markdown) == "" || !utf8.ValidString(document.Markdown) || !json.Valid(document.StructuredJSON) {
		return Document{}, fmt.Errorf("extractor returned invalid or empty document content")
	}
	if err := writeJSON(workspace, cache, extraction{dataset.SHA256, profile, document}); err != nil {
		return Document{}, err
	}
	return document, nil
}

// Bounded, source-quoted candidates prevent the model from inventing targets.
// Paragraphs retain context; sentence-sized passages bound model payloads.
func sourcePassages(markdown string) ([]string, int) {
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
		paragraph = normalized(strings.Join(lines, " "))
		if paragraph == "" {
			continue
		}
		pieces := []string{paragraph}
		if utf8.RuneCountInString(paragraph) > 400 {
			pieces = strings.SplitAfter(paragraph, ". ")
		}
		for _, piece := range pieces {
			piece = normalized(piece)
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
