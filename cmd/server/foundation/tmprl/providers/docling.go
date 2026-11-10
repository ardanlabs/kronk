package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
func (*DoclingExtractor) ExtractionProfile() string { return "tuning-docling-dlparse_v4-accurate-v1" }

// Convert uploads a PDF or DOCX and waits for its converted content.
func (e *DoclingExtractor) Convert(ctx context.Context, path string, options DocumentOptions) (Document, error) {
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
		if options.PageStart > 0 {
			fields = append(fields, [2]string{"page_range", strconv.Itoa(options.PageStart)}, [2]string{"page_range", strconv.Itoa(options.PageEnd)})
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
	if _, err := e.request(ctx, http.MethodPost, "/v1/convert/file/async", reader, form.FormDataContentType(), &task); err != nil {
		return Document{}, err
	}
	if task.ID == "" {
		return Document{}, fmt.Errorf("docling returned an empty task ID")
	}
	id := task.ID
	for task.Status != "success" {
		switch task.Status {
		case "pending", "started":
		default:
			return Document{}, fmt.Errorf("docling conversion did not succeed: %s", task.Status)
		}
		select {
		case <-ctx.Done():
			return Document{}, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
		if err := e.read(ctx, "/v1/status/poll/"+url.PathEscape(id), &task); err != nil {
			return Document{}, err
		}
	}
	var result struct {
		Status         string  `json:"status"`
		ProcessingTime float64 `json:"processing_time"`
		Document       struct {
			Markdown string          `json:"md_content"`
			JSON     json.RawMessage `json:"json_content"`
		} `json:"document"`
	}
	if err := e.read(ctx, "/v1/result/"+url.PathEscape(id), &result); err != nil {
		return Document{}, err
	}
	var structured struct {
		Texts    []json.RawMessage `json:"texts"`
		Tables   []json.RawMessage `json:"tables"`
		Pictures []json.RawMessage `json:"pictures"`
	}
	if result.Status != "success" || strings.TrimSpace(result.Document.Markdown) == "" || json.Unmarshal(result.Document.JSON, &structured) != nil || len(structured.Texts)+len(structured.Tables)+len(structured.Pictures) == 0 {
		return Document{}, fmt.Errorf("docling returned invalid or empty document content")
	}
	return Document{Markdown: result.Document.Markdown, StructuredJSON: result.Document.JSON, ProcessingTime: result.ProcessingTime}, nil
}

func (e *DoclingExtractor) read(ctx context.Context, path string, out any) error {
	var err error
	for attempt := range 3 {
		var status int
		status, err = e.request(ctx, http.MethodGet, path, nil, "", out)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if attempt == 2 || (status != 0 && status != 408 && status != 429 && status < 500) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 250 * time.Millisecond):
		}
	}
	return err
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
