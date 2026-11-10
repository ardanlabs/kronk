// Package extract converts source documents into cached text and structural units
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
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/generate"
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
func Run(ctx context.Context, extractor preparation.DocumentExtractor, sourceInfo uploads.File, openSource func(context.Context) (io.ReadCloser, error), workspace *os.Root, options ...providers.DocumentOptions) (providers.Document, error) {
	profile, err := extractionProfile(extractor, sourceInfo, options...)
	if err != nil {
		return providers.Document{}, err
	}
	cached, err := Read(ctx, extractor, sourceInfo, workspace, options...)
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
		var conversion providers.DocumentOptions
		if len(options) > 0 {
			conversion = options[0]
		}
		document, err = extractor.Convert(ctx, path, conversion)
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
func Read(ctx context.Context, extractor preparation.DocumentExtractor, sourceInfo uploads.File, workspace *os.Root, options ...providers.DocumentOptions) (providers.Document, error) {
	if err := ctx.Err(); err != nil {
		return providers.Document{}, err
	}
	profile, err := extractionProfile(extractor, sourceInfo, options...)
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

func extractionProfile(extractor preparation.DocumentExtractor, sourceInfo uploads.File, options ...providers.DocumentOptions) (string, error) {
	if len(options) > 1 {
		return "", fmt.Errorf("%w: only one set of extraction options is allowed", preparation.ErrInvalidArgument)
	}
	var pages providers.DocumentOptions
	if len(options) > 0 {
		pages = options[0]
	}
	if err := pages.Validate(); err != nil {
		return "", err
	}
	if pages != (providers.DocumentOptions{}) && sourceInfo.Format != ".pdf" {
		return "", fmt.Errorf("%w: page ranges require a PDF", preparation.ErrInvalidArgument)
	}
	if sourceInfo.Format == ".pdf" || sourceInfo.Format == ".docx" {
		if extractor == nil {
			return "", fmt.Errorf("%w: PDF/DOCX extraction is not configured", preparation.ErrInvalidArgument)
		}
		profile := extractor.ExtractionProfile()
		if pages.PageStart > 0 {
			profile += fmt.Sprintf("-pages-%d-%d", pages.PageStart, pages.PageEnd)
		}
		return profile, nil
	}
	return "tuning-text-v1", nil
}

const (
	sourceEvidenceRunes = 3000
	sourceTargetRunes   = 800
	sourcePassageRunes  = 1200
)

type sourceHeading struct {
	text  string
	level int
}

type sourceBlock struct {
	reference string
	text      string
	kind      string
	headings  []sourceHeading
	section   int
}

type sourceBlocks struct {
	blocks     []sourceBlock
	headings   []sourceHeading
	section    int
	exclusions []preparation.SourceExclusion
}

func (s *sourceBlocks) exclude(reference, reason string) {
	s.exclusions = append(s.exclusions, preparation.SourceExclusion{Reference: reference, Reason: reason})
}

func (s *sourceBlocks) heading(text string, level int) {
	s.section++
	if level <= 0 {
		s.headings = nil // Without levels, each heading starts a new section.
	} else {
		for len(s.headings) > 0 && (s.headings[len(s.headings)-1].level >= level || s.headings[len(s.headings)-1].level == 0) {
			s.headings = s.headings[:len(s.headings)-1]
		}
	}
	s.headings = append(s.headings, sourceHeading{text: text, level: level})
}

func (s *sourceBlocks) append(reference, text, kind string) {
	if strings.TrimSpace(text) != "" {
		s.blocks = append(s.blocks, sourceBlock{reference, text, kind, slices.Clone(s.headings), s.section})
	}
}

// SourceUnits groups adjacent prose into passages without cutting source blocks.
// Lists, code and picture OCR remain separate passage types. Recognized
// Docling documents fail closed; synthetic text JSON uses the Markdown fallback.
func SourceUnits(document providers.Document, goal preparation.Goal) ([]preparation.SourceUnit, []preparation.SourceExclusion, error) {
	var source sourceBlocks
	if !utf8.Valid(document.StructuredJSON) || !utf8.ValidString(document.Markdown) {
		return nil, nil, fmt.Errorf("source content is not valid UTF-8")
	}
	var header struct {
		SchemaName string `json:"schema_name"`
	}
	if len(document.StructuredJSON) > 0 {
		if err := json.Unmarshal(document.StructuredJSON, &header); err != nil {
			return nil, nil, fmt.Errorf("decode source structure: %w", err)
		}
	}
	if header.SchemaName == "DoclingDocument" {
		if err := source.docling(document.StructuredJSON); err != nil {
			return nil, source.exclusions, err
		}
	} else {
		if header.SchemaName != "" {
			source.exclude("structured_json", "unsupported schema; using Markdown blocks")
		}
		source.markdown(document.Markdown)
	}

	// Group only adjacent prose in the same section. The size is a soft packing
	// limit: an individual larger block remains whole, subject to evidence bounds.
	var passages []sourceBlock
	for _, block := range source.blocks {
		if len(passages) > 0 {
			last := &passages[len(passages)-1]
			if last.kind == "text" && block.kind == "text" && last.section == block.section && utf8.RuneCountInString(last.text)+2+utf8.RuneCountInString(block.text) <= sourcePassageRunes {
				last.reference += "," + block.reference
				last.text += "\n\n" + block.text
				continue
			}
		}
		passages = append(passages, block)
	}
	source.blocks = passages
	var units []preparation.SourceUnit
	for i, block := range source.blocks {
		targets := generate.Targets(block.text, goal)
		invalidTarget := false
		for _, target := range targets {
			if strings.TrimSpace(target) == "" || (goal != preparation.GoalQuestionAnswer && goal != preparation.GoalTextGeneration && utf8.RuneCountInString(target) > sourceTargetRunes) {
				invalidTarget = true
				break
			}
		}
		if invalidTarget || len(targets) == 0 {
			source.exclude(block.reference, "no targets or target set contains empty or oversized candidates (800 rune target budget)")
			continue
		}
		start, end := i, i
		evidence := renderSource(source.blocks, i, start, end)
		if utf8.RuneCountInString(evidence) > sourceEvidenceRunes {
			source.exclude(block.reference, "required focal block, heading ancestry and markers exceed 3000 rune evidence budget")
			continue
		}
		// Optional neighbors are never cut or replaced by more distant blocks.
		if i > 0 && source.blocks[i-1].section == block.section {
			candidate := renderSource(source.blocks, i, i-1, end)
			if utf8.RuneCountInString(candidate) <= sourceEvidenceRunes {
				start, evidence = i-1, candidate
			}
		}
		if i+1 < len(source.blocks) && source.blocks[i+1].section == block.section {
			candidate := renderSource(source.blocks, i, start, i+1)
			if utf8.RuneCountInString(candidate) <= sourceEvidenceRunes {
				evidence = candidate
			}
		}
		units = append(units, preparation.SourceUnit{Reference: block.reference, Evidence: evidence, Targets: targets})
	}
	return units, source.exclusions, nil
}

func renderSource(blocks []sourceBlock, focal, start, end int) string {
	var out strings.Builder
	fmt.Fprintf(&out, "[SOURCE %s]\n", blocks[focal].reference)
	for _, heading := range blocks[focal].headings {
		out.WriteString(heading.text)
		out.WriteString("\n\n")
	}
	for i := start; i <= end; i++ {
		if i == focal {
			out.WriteString("[FOCAL BEGIN]\n")
		}
		out.WriteString(blocks[i].text)
		if i == focal {
			out.WriteString("\n[FOCAL END]")
		}
		out.WriteString("\n\n")
	}
	return strings.TrimSpace(out.String())
}

type doclingRef struct {
	Ref string `json:"$ref"`
}

type doclingNode struct {
	SelfRef      string       `json:"self_ref"`
	Label        string       `json:"label"`
	ContentLayer string       `json:"content_layer"`
	Text         *string      `json:"text"`
	Level        *int         `json:"level"`
	Children     []doclingRef `json:"children"`
}

func (s *sourceBlocks) docling(data []byte) error {
	var document struct {
		Body     *doclingNode  `json:"body"`
		Groups   []doclingNode `json:"groups"`
		Texts    []doclingNode `json:"texts"`
		Pictures []doclingNode `json:"pictures"`
		Tables   []doclingNode `json:"tables"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("decode Docling document: %w", err)
	}
	if document.Body == nil || document.Body.SelfRef != "#/body" || document.Body.Children == nil {
		return fmt.Errorf("malformed Docling body")
	}
	nodes := map[string]doclingNode{"#/body": *document.Body}
	collections := []struct {
		name  string
		nodes []doclingNode
	}{{"groups", document.Groups}, {"texts", document.Texts}, {"pictures", document.Pictures}, {"tables", document.Tables}}
	for _, collection := range collections {
		for i, node := range collection.nodes {
			reference := fmt.Sprintf("#/%s/%d", collection.name, i)
			if node.SelfRef != reference || node.Label == "" || node.Level != nil && *node.Level <= 0 {
				return fmt.Errorf("malformed Docling node %s", reference)
			}
			nodes[reference] = node
		}
	}
	// Validate even omitted structures: a bad reference must never trigger fallback.
	for reference, node := range nodes {
		for _, child := range node.Children {
			if _, ok := nodes[child.Ref]; !ok {
				return fmt.Errorf("invalid Docling child reference in %s: %q", reference, child.Ref)
			}
		}
	}
	state := make(map[string]uint8)
	pictureDepth := 0
	var visit func(string, bool, int) error
	visit = func(reference string, omitted bool, depth int) error {
		if depth > 256 {
			s.exclude(reference, "Docling structure exceeds 256 nesting level resource budget")
			return fmt.Errorf("source structure exceeds 256 Docling nesting levels at %s", reference)
		}
		if state[reference] != 0 {
			return fmt.Errorf("cyclic or repeated Docling reference %s", reference)
		}
		state[reference] = 1
		node := nodes[reference]
		pictureStart := len(s.blocks)
		pictureHeadings, pictureSection := slices.Clone(s.headings), s.section
		picture := !omitted && strings.HasPrefix(reference, "#/pictures/")
		if picture {
			pictureDepth++
		}
		if !omitted {
			switch {
			case node.ContentLayer == "furniture" || node.Label == "page_header" || node.Label == "page_footer":
				s.exclude(reference, "document furniture")
				omitted = true
			case strings.HasPrefix(reference, "#/tables/"):
				s.exclude(reference, "unsupported table structure")
				s.section++ // Do not pack prose across an omitted structure.
				omitted = true
			case reference == "#/body", strings.HasPrefix(reference, "#/groups/"):
				// Containers only determine child reading order.
			case strings.HasPrefix(reference, "#/pictures/"):
				s.exclude(reference, "visual relationships unsupported; OCR retained as one picture passage")
			case node.Label == "section_header" || node.Label == "title":
				if node.Text == nil || strings.TrimSpace(*node.Text) == "" {
					return fmt.Errorf("missing Docling heading text at %s", reference)
				}
				if pictureDepth > 0 {
					s.append(reference, *node.Text, "picture")
					break
				}
				level := 0
				if node.Level != nil {
					level = *node.Level
				}
				s.heading(*node.Text, level)
			case node.Label == "text" || node.Label == "paragraph" || node.Label == "list_item" || node.Label == "code" || node.Label == "caption" || node.Label == "footnote":
				if node.Text == nil {
					return fmt.Errorf("missing Docling block text at %s", reference)
				}
				kind := node.Label
				if kind == "paragraph" {
					kind = "text"
				}
				s.append(reference, *node.Text, kind)
			default:
				s.exclude(reference, "unsupported Docling label: "+node.Label)
				s.section++
			}
		}
		for _, child := range node.Children {
			if err := visit(child.Ref, omitted, depth+1); err != nil {
				return err
			}
		}
		if picture {
			pictureDepth--
			var text []string
			for _, block := range s.blocks[pictureStart:] {
				text = append(text, block.text)
			}
			s.blocks = s.blocks[:pictureStart]
			s.headings, s.section = pictureHeadings, pictureSection
			s.append(reference, strings.Join(text, "\n\n"), "picture")
		}
		state[reference] = 2
		return nil
	}
	if err := visit("#/body", false, 0); err != nil {
		return err
	}
	for _, collection := range collections {
		for _, node := range collection.nodes {
			if state[node.SelfRef] == 0 {
				s.exclude(node.SelfRef, "outside body reading order")
				if err := visit(node.SelfRef, true, 0); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

var sourceATX = regexp.MustCompile(`^ {0,3}(#{1,6})(?:[ \t]+|$)`)
var sourceList = regexp.MustCompile(`^([ \t]*)(?:[-+*]|[0-9]+[.)])[ \t]+`)
var sourceSetext = regexp.MustCompile(`^ {0,3}(=+|-+)[ \t]*$`)
var sourceTableSeparator = regexp.MustCompile(`^\s*\|?\s*:?-+:?\s*(?:\|\s*:?-+:?\s*)*\|?\s*$`)

func sourceTableAt(lines []string, i int) bool {
	return i+1 < len(lines) && strings.Contains(lines[i], "|") && sourceTableSeparator.MatchString(lines[i+1])
}

func sourceImage(line string) bool {
	return strings.TrimSpace(line) == "<!-- image -->" || strings.HasPrefix(strings.TrimSpace(line), "![")
}

func sourceFence(line string) (byte, int, string) {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 || len(trimmed) < 3 || trimmed[0] != '`' && trimmed[0] != '~' {
		return 0, 0, ""
	}
	n := 0
	for n < len(trimmed) && trimmed[n] == trimmed[0] {
		n++
	}
	if n < 3 {
		return 0, 0, ""
	}
	return trimmed[0], n, trimmed[n:]
}

func (s *sourceBlocks) markdown(markdown string) {
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); {
		start := i
		line := lines[i]
		reference := "markdown:line:" + strconv.Itoa(i+1)
		if strings.TrimSpace(line) == "" {
			i++
			continue
		}
		if marker, count, _ := sourceFence(line); count > 0 {
			i++
			closed := false
			for i < len(lines) {
				endMarker, endCount, rest := sourceFence(lines[i])
				i++
				if endMarker == marker && endCount >= count && strings.TrimSpace(rest) == "" {
					closed = true
					break
				}
			}
			if !closed {
				s.exclude(reference, "unterminated fenced code block")
				continue
			}
			s.append(reference, strings.Join(lines[start:i], "\n"), "code")
			continue
		}
		if match := sourceATX.FindStringSubmatch(line); match != nil {
			s.heading(line, len(match[1]))
			i++
			continue
		}
		if sourceTableAt(lines, i) {
			i += 2
			for i < len(lines) && strings.TrimSpace(lines[i]) != "" && strings.Contains(lines[i], "|") {
				i++
			}
			s.exclude(reference, "unsupported Markdown table structure")
			s.section++
			continue
		}
		if i+1 < len(lines) && sourceSetext.MatchString(lines[i+1]) {
			level := 2
			if strings.HasPrefix(strings.TrimSpace(lines[i+1]), "=") {
				level = 1
			}
			s.heading(strings.Join(lines[i:i+2], "\n"), level)
			i += 2
			continue
		}
		if sourceImage(line) {
			s.exclude(reference, "unsupported image resource")
			s.section++
			i++
			continue
		}
		i++
		for i < len(lines) && strings.TrimSpace(lines[i]) != "" {
			_, count, _ := sourceFence(lines[i])
			if sourceATX.MatchString(lines[i]) || sourceList.MatchString(lines[i]) || count > 0 || sourceImage(lines[i]) || sourceTableAt(lines, i) {
				break
			}
			if i+1 < len(lines) && sourceSetext.MatchString(lines[i+1]) {
				break
			}
			i++
		}
		kind := "text"
		if sourceList.MatchString(line) {
			kind = "list_item"
		}
		s.append(reference, strings.Join(lines[start:i], "\n"), kind)
	}
}
