package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// Errors that classify why a document conversion failed.
var (
	ErrExtractorUnavailable = errors.New("document extractor is unavailable")
	ErrPageRangePastEnd     = errors.New("page range starts after the last page")
	ErrExtractionFailed     = errors.New("document extraction failed")
)

// Document preserves converted content and conversion metadata.
type Document struct {
	Markdown       string          `json:"markdown"`
	StructuredJSON json.RawMessage `json:"structured_json"`
	ProcessingTime float64         `json:"processing_time"`
}

// DocumentOptions bounds PDF conversion to inclusive, one-based pages.
// Both zero selects the entire document.
type DocumentOptions struct {
	PageStart int
	PageEnd   int
}

// Validate rejects partial, reversed, or nonpositive page ranges.
func (o DocumentOptions) Validate() error {
	if o != (DocumentOptions{}) && (o.PageStart < 1 || o.PageEnd < o.PageStart) {
		return fmt.Errorf("%w: page range requires a positive first page and a last page at or after it", tmprl.ErrInvalidArgument)
	}
	return nil
}

// DoclingExtractor converts documents using an existing Docling service.
type DoclingExtractor struct {
	endpoint string
	client   *http.Client
}

// NewDoclingExtractor connects to an existing Docling service, never starts
// containers. A custom HTTP client can supply authentication and timeouts.
func NewDoclingExtractor(endpoint string, client *http.Client) (*DoclingExtractor, error) {
	if endpoint == "" {
		endpoint = "http://127.0.0.1:5001"
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("%w: invalid Docling endpoint", tmprl.ErrInvalidArgument)
	}
	if client == nil {
		client = http.DefaultClient
	}
	e := DoclingExtractor{strings.TrimRight(endpoint, "/"), client}
	return &e, nil
}

// ExtractionProfile identifies conversion settings for cache identity.
func (*DoclingExtractor) ExtractionProfile() string { return "tuning-docling-dlparse_v4-accurate-v2" }

// Docling holds a whole conversion in memory, so PDFs are converted a bounded
// number of pages at a time whatever their length.
const doclingPageBatch = 10

// doclingRecovery bounds how long a batch waits for an unreachable or
// restarted Docling service before the conversion fails.
const doclingRecovery = 2 * time.Minute

// DocumentCheckpoint supplies persistent batch storage scoped to one source and
// extraction profile. Load returns os.ErrNotExist for an unsaved batch. Save
// must commit the batch before checking Stop; its error prevents further work.
type DocumentCheckpoint struct {
	Load func(start, end int) (Document, error)
	Save func(start, end int, document Document) error
}

// Convert uploads a PDF or DOCX and waits for its converted content. PDFs are
// converted in page batches that are joined into one document; after each
// batch, progress, when not nil, receives the number of pages read so far.
// An optional checkpoint reuses completed batches and saves new ones.
func (e *DoclingExtractor) Convert(ctx context.Context, path string, options DocumentOptions, progress func(pages int), checkpoints ...DocumentCheckpoint) (Document, error) {
	if len(checkpoints) > 1 {
		return Document{}, fmt.Errorf("%w: only one document checkpoint is allowed", tmprl.ErrInvalidArgument)
	}
	if err := options.Validate(); err != nil {
		return Document{}, err
	}
	mediaType := "application/pdf"
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf":
	case ".docx":
		mediaType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	default:
		return Document{}, fmt.Errorf("%w: Docling accepts PDF or DOCX", tmprl.ErrInvalidArgument)
	}
	if options != (DocumentOptions{}) && mediaType != "application/pdf" {
		return Document{}, fmt.Errorf("%w: page ranges require a PDF", tmprl.ErrInvalidArgument)
	}
	convert := func(start, end int) (Document, error) {
		if err := ctx.Err(); err != nil {
			return Document{}, err
		}
		var checkpoint DocumentCheckpoint
		if len(checkpoints) > 0 {
			checkpoint = checkpoints[0]
		}
		if checkpoint.Load != nil {
			document, err := checkpoint.Load(start, end)
			switch {
			case err == nil:
				return document, nil
			case errors.Is(err, os.ErrNotExist):
			default:
				return Document{}, err
			}
		}
		document, err := e.convertRecovering(ctx, path, mediaType, start, end)
		if err != nil {
			return Document{}, err
		}
		if checkpoint.Save != nil {
			if err := checkpoint.Save(start, end, document); err != nil {
				return Document{}, err
			}
		}
		return document, nil
	}
	var batches []Document
	if mediaType != "application/pdf" {
		document, err := convert(0, 0)
		if err != nil {
			return Document{}, err
		}
		batches = append(batches, document)
	}
	read := 0
	for start := max(options.PageStart, 1); mediaType == "application/pdf" && (options.PageEnd == 0 || start <= options.PageEnd); start += doclingPageBatch {
		end := start + doclingPageBatch - 1
		if options.PageEnd > 0 {
			end = min(end, options.PageEnd)
		}
		document, err := convert(start, end)
		if errors.Is(err, ErrPageRangePastEnd) && len(batches) > 0 {
			break // The previous batch ended on the last page.
		}
		if err != nil {
			return Document{}, err
		}
		var pages struct {
			Pages map[string]json.RawMessage `json:"pages"`
		}
		_ = json.Unmarshal(document.StructuredJSON, &pages) // Invalid structure fails validation after the join.
		if len(batches) > 0 && len(pages.Pages) == 0 {
			break // A batch past the end converted no pages.
		}
		batches = append(batches, document)
		read += len(pages.Pages)
		if progress != nil && len(pages.Pages) > 0 {
			progress(read)
		}
	}
	document, err := joinDocling(batches)
	if err != nil {
		return Document{}, err
	}
	var structured struct {
		Texts    []json.RawMessage `json:"texts"`
		Tables   []json.RawMessage `json:"tables"`
		Pictures []json.RawMessage `json:"pictures"`
	}
	if strings.TrimSpace(document.Markdown) == "" || json.Unmarshal(document.StructuredJSON, &structured) != nil || len(structured.Texts)+len(structured.Tables)+len(structured.Pictures) == 0 {
		return Document{}, fmt.Errorf("%w: docling returned invalid or empty document content", ErrExtractionFailed)
	}
	return document, nil
}

// convertRecovering converts one batch, resubmitting it while Docling is
// unreachable or has lost the task in a restart.
func (e *DoclingExtractor) convertRecovering(ctx context.Context, path, mediaType string, start, end int) (Document, error) {
	deadline := time.Now().Add(doclingRecovery)
	wait := time.Second
	for {
		document, err := e.convert(ctx, path, mediaType, start, end)
		if !errors.Is(err, ErrExtractorUnavailable) || time.Now().Add(wait).After(deadline) {
			return document, err
		}
		select {
		case <-ctx.Done():
			return Document{}, ctx.Err()
		case <-time.After(wait):
		}
		wait = min(2*wait, 15*time.Second)
	}
}

// convert submits one conversion of pages start through end, or the whole
// document when start is zero.
func (e *DoclingExtractor) convert(ctx context.Context, path, mediaType string, start, end int) (Document, error) {
	file, err := os.Open(path)
	if err != nil {
		return Document{}, err
	}
	reader, writer := io.Pipe()
	defer reader.Close()
	form := multipart.NewWriter(writer)
	go func() {
		defer file.Close()
		fields := [][2]string{{"to_formats", "md"}, {"to_formats", "json"}, {"include_images", "false"}, {"table_mode", "accurate"}, {"image_export_mode", "placeholder"}}
		if mediaType == "application/pdf" {
			fields = append(fields, [2]string{"pdf_backend", "dlparse_v4"})
		}
		if start > 0 {
			fields = append(fields, [2]string{"page_range", strconv.Itoa(start)}, [2]string{"page_range", strconv.Itoa(end)})
		}
		for _, option := range fields {
			if err := form.WriteField(option[0], option[1]); err != nil {
				writer.CloseWithError(err)
				return
			}
		}
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", fmt.Sprintf("form-data; name=\"files\"; filename=%q", filepath.Base(path)))
		header.Set("Content-Type", mediaType)
		part, err := form.CreatePart(header)
		if err != nil {
			writer.CloseWithError(err)
			return
		}
		if _, err := io.Copy(part, file); err != nil {
			writer.CloseWithError(err)
			return
		}
		writer.CloseWithError(form.Close())
	}()
	var task struct {
		ID     string `json:"task_id"`
		Status string `json:"task_status"`
	}
	if status, err := e.request(ctx, http.MethodPost, "/v1/convert/file/async", reader, form.FormDataContentType(), &task); err != nil {
		return Document{}, unavailable(ctx, status, err)
	}
	if task.ID == "" {
		return Document{}, fmt.Errorf("%w: docling returned an empty task ID", ErrExtractionFailed)
	}
	id := task.ID
	for task.Status != "success" {
		switch task.Status {
		case "pending", "started":
		default:
			return Document{}, fmt.Errorf("%w: docling task status %s", ErrExtractionFailed, task.Status)
		}
		select {
		case <-ctx.Done():
			return Document{}, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
		if status, err := e.read(ctx, "/v1/status/poll/"+url.PathEscape(id), &task); err != nil {
			return Document{}, unavailable(ctx, status, err)
		}
	}
	var result struct {
		Status         string  `json:"status"`
		ProcessingTime float64 `json:"processing_time"`
		Errors         []struct {
			Message string `json:"error_message"`
		} `json:"errors"`
		Document struct {
			Markdown string          `json:"md_content"`
			JSON     json.RawMessage `json:"json_content"`
		} `json:"document"`
	}
	if status, err := e.read(ctx, "/v1/result/"+url.PathEscape(id), &result); err != nil {
		return Document{}, unavailable(ctx, status, err)
	}
	if result.Status != "success" {
		var messages []string
		for _, failure := range result.Errors {
			// Docling reports a start past the last page as a page_range input error.
			if strings.Contains(failure.Message, "page_range") {
				return Document{}, fmt.Errorf("%w: %s", ErrPageRangePastEnd, failure.Message)
			}
			messages = append(messages, failure.Message)
		}
		return Document{}, fmt.Errorf("%w: docling status %s: %s", ErrExtractionFailed, result.Status, strings.Join(messages, "; "))
	}
	if !json.Valid(result.Document.JSON) {
		return Document{}, fmt.Errorf("%w: docling returned invalid document structure", ErrExtractionFailed)
	}
	return Document{Markdown: result.Document.Markdown, StructuredJSON: result.Document.JSON, ProcessingTime: result.ProcessingTime}, nil
}

// unavailable marks transport failures, server errors, and tasks a restarted
// service no longer knows as a temporarily unavailable Docling service.
func unavailable(ctx context.Context, status int, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if status == 0 || status == http.StatusNotFound || status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500 {
		return fmt.Errorf("%w: %w", ErrExtractorUnavailable, err)
	}
	return fmt.Errorf("%w: %w", ErrExtractionFailed, err)
}

// doclingCollections are the DoclingDocument arrays that "#/<name>/<index>"
// references address.
var doclingCollections = []string{"groups", "texts", "pictures", "tables", "key_value_items", "form_items"}

// joinDocling joins batch conversions in page order. Each later batch's
// collection references are shifted past the items already joined, its body
// and furniture children are appended, and its pages are added.
func joinDocling(batches []Document) (Document, error) {
	if len(batches) == 1 {
		return batches[0], nil
	}
	var joined Document
	var merged map[string]any
	var markdown []string
	for _, batch := range batches {
		joined.ProcessingTime += batch.ProcessingTime
		if strings.TrimSpace(batch.Markdown) != "" {
			markdown = append(markdown, batch.Markdown)
		}
		decoder := json.NewDecoder(bytes.NewReader(batch.StructuredJSON))
		decoder.UseNumber()
		var document map[string]any
		if err := decoder.Decode(&document); err != nil {
			return Document{}, fmt.Errorf("%w: decode docling batch: %w", ErrExtractionFailed, err)
		}
		if merged == nil {
			merged = document
			continue
		}
		offsets := make(map[string]int, len(doclingCollections))
		for _, name := range doclingCollections {
			items, _ := merged[name].([]any)
			offsets[name] = len(items)
		}
		shiftReferences(document, offsets)
		for _, name := range doclingCollections {
			items, _ := merged[name].([]any)
			added, _ := document[name].([]any)
			merged[name] = append(items, added...)
		}
		for _, name := range []string{"body", "furniture"} {
			into, _ := merged[name].(map[string]any)
			from, _ := document[name].(map[string]any)
			if into == nil || from == nil {
				continue
			}
			children, _ := into["children"].([]any)
			added, _ := from["children"].([]any)
			into["children"] = append(children, added...)
		}
		pages, _ := merged["pages"].(map[string]any)
		added, _ := document["pages"].(map[string]any)
		if pages == nil {
			pages = map[string]any{}
			merged["pages"] = pages
		}
		maps.Copy(pages, added)
	}
	data, err := json.Marshal(merged)
	if err != nil {
		return Document{}, err
	}
	joined.Markdown, joined.StructuredJSON = strings.Join(markdown, "\n\n"), data
	return joined, nil
}

// shiftReferences offsets every "$ref" and "self_ref" that addresses a
// collection item.
func shiftReferences(value any, offsets map[string]int) {
	switch value := value.(type) {
	case map[string]any:
		for key, item := range value {
			if reference, ok := item.(string); ok && (key == "$ref" || key == "self_ref") {
				value[key] = shiftReference(reference, offsets)
				continue
			}
			shiftReferences(item, offsets)
		}
	case []any:
		for _, item := range value {
			shiftReferences(item, offsets)
		}
	}
}

func shiftReference(reference string, offsets map[string]int) string {
	rest, ok := strings.CutPrefix(reference, "#/")
	if !ok {
		return reference
	}
	name, index, ok := strings.Cut(rest, "/")
	offset, known := offsets[name]
	n, err := strconv.Atoi(index)
	if !ok || !known || err != nil {
		return reference
	}
	return "#/" + name + "/" + strconv.Itoa(n+offset)
}

func (e *DoclingExtractor) read(ctx context.Context, path string, out any) (int, error) {
	var status int
	var err error
	for attempt := range 3 {
		status, err = e.request(ctx, http.MethodGet, path, nil, "", out)
		if err == nil {
			return status, nil
		}
		if ctx.Err() != nil {
			return status, ctx.Err()
		}
		if attempt == 2 || (status != 0 && status != 408 && status != 429 && status < 500) {
			return status, err
		}
		select {
		case <-ctx.Done():
			return status, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 250 * time.Millisecond):
		}
	}
	return status, err
}

func (e *DoclingExtractor) request(ctx context.Context, method, path string, body io.Reader, contentType string, out any) (int, error) {
	request, err := http.NewRequestWithContext(ctx, method, e.endpoint+path, body)
	if err != nil {
		return 0, err
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response, err := e.client.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return response.StatusCode, fmt.Errorf("docling returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (256<<20)+1))
	if err != nil {
		return response.StatusCode, err
	}
	if len(data) > 256<<20 {
		return response.StatusCode, fmt.Errorf("docling response exceeds limit")
	}
	return response.StatusCode, json.Unmarshal(data, out)
}
