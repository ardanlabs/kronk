package prepare

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultDoclingEndpoint          = "http://127.0.0.1:5001"
	maximumDoclingResponseBodyBytes = 256 * 1024 * 1024
	doclingReadAttempts             = 3
	doclingRetryDelay               = 250 * time.Millisecond
)

// DoclingExtractionProfile identifies the pinned service and settings that
// affect persisted extraction output.
const DoclingExtractionProfile = "docling-v1.12.0-dlparse_v4-accurate-v1"

// DoclingExtractor converts documents through an already-running
// docling-serve instance. Container lifecycle remains outside the server.
type DoclingExtractor struct {
	endpoint     string
	httpClient   *http.Client
	pollInterval time.Duration
	retryDelay   time.Duration
}

// NewDoclingExtractor constructs an extractor. An empty endpoint uses the
// local service started by make docling-up, and a nil client uses
// http.DefaultClient.
func NewDoclingExtractor(endpoint string, httpClient *http.Client) (*DoclingExtractor, error) {
	if endpoint == "" {
		endpoint = defaultDoclingEndpoint
	}

	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse Docling endpoint: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("parse Docling endpoint: endpoint must include a scheme and host")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &DoclingExtractor{
		endpoint:     strings.TrimRight(endpoint, "/"),
		httpClient:   httpClient,
		pollInterval: 500 * time.Millisecond,
		retryDelay:   doclingRetryDelay,
	}, nil
}

// ExtractionProfile identifies this extractor's reproducible conversion
// settings.
func (e *DoclingExtractor) ExtractionProfile() string {
	return DoclingExtractionProfile
}

// Convert uploads a PDF or DOCX, waits for conversion, and returns Markdown
// plus Docling's structured JSON.
func (e *DoclingExtractor) Convert(ctx context.Context, sourcePath string) (Document, error) {
	mediaType, err := doclingSourceMediaType(sourcePath)
	if err != nil {
		return Document{}, err
	}

	reader, writer := io.Pipe()
	form := multipart.NewWriter(writer)
	go func() {
		if err := writeDoclingConversionRequest(form, sourcePath, mediaType); err != nil {
			_ = writer.CloseWithError(err)
			return
		}
		_ = writer.Close()
	}()
	defer reader.Close()

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint+"/v1/convert/file/async", reader)
	if err != nil {
		return Document{}, fmt.Errorf("create Docling conversion request: %w", err)
	}
	request.Header.Set("Content-Type", form.FormDataContentType())

	var task doclingConversionTask
	if err := e.execute(request, &task); err != nil {
		return Document{}, err
	}
	if task.ID == "" {
		return Document{}, fmt.Errorf("convert document: Docling returned an empty task id")
	}

	for {
		switch task.Status {
		case "success":
			return e.result(ctx, task.ID)
		case "failure":
			return Document{}, fmt.Errorf("convert document: Docling task failed")
		case "pending", "started":
		case "":
			return Document{}, fmt.Errorf("convert document: Docling returned an empty task status")
		default:
			return Document{}, fmt.Errorf("convert document: Docling returned unknown task status %q", task.Status)
		}

		timer := time.NewTimer(e.pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return Document{}, fmt.Errorf("convert document: %w", ctx.Err())
		case <-timer.C:
		}

		pollRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, e.endpoint+"/v1/status/poll/"+url.PathEscape(task.ID), nil)
		if err != nil {
			return Document{}, fmt.Errorf("create Docling status request: %w", err)
		}
		if err := e.executeRead(pollRequest, &task); err != nil {
			return Document{}, err
		}
	}
}

type doclingConversionTask struct {
	ID     string `json:"task_id"`
	Status string `json:"task_status"`
}

type doclingConversionResponse struct {
	Document struct {
		Markdown       string          `json:"md_content"`
		StructuredJSON json.RawMessage `json:"json_content"`
	} `json:"document"`
	Status         string  `json:"status"`
	ProcessingTime float64 `json:"processing_time"`
}

func (e *DoclingExtractor) result(ctx context.Context, taskID string) (Document, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, e.endpoint+"/v1/result/"+url.PathEscape(taskID), nil)
	if err != nil {
		return Document{}, fmt.Errorf("create Docling result request: %w", err)
	}

	var result doclingConversionResponse
	if err := e.executeRead(request, &result); err != nil {
		return Document{}, err
	}
	if result.Status != "success" {
		return Document{}, fmt.Errorf("convert document: Docling reported %q status", result.Status)
	}
	if strings.TrimSpace(result.Document.Markdown) == "" {
		return Document{}, fmt.Errorf("convert document: Docling returned empty Markdown")
	}
	if !validDoclingStructuredJSON(result.Document.StructuredJSON) {
		return Document{}, fmt.Errorf("convert document: Docling returned invalid structured JSON")
	}

	return Document{
		Markdown:       result.Document.Markdown,
		StructuredJSON: result.Document.StructuredJSON,
		ProcessingTime: result.ProcessingTime,
	}, nil
}

type doclingStatusError struct {
	code    int
	status  string
	message string
}

func (err *doclingStatusError) Error() string {
	return fmt.Sprintf("call Docling: server returned %s: %s", err.status, err.message)
}

func (e *DoclingExtractor) executeRead(request *http.Request, target any) error {
	var lastErr error
	for attempt := range doclingReadAttempts {
		if err := e.execute(request, target); err == nil {
			return nil
		} else {
			lastErr = err
			if attempt == doclingReadAttempts-1 || !retryableDoclingRead(request.Context(), err) {
				break
			}
		}
		timer := time.NewTimer(time.Duration(attempt+1) * e.retryDelay)
		select {
		case <-request.Context().Done():
			timer.Stop()
			return fmt.Errorf("call Docling: %w", request.Context().Err())
		case <-timer.C:
		}
	}
	return lastErr
}

func retryableDoclingRead(ctx context.Context, err error) bool {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if statusErr, ok := errors.AsType[*doclingStatusError](err); ok {
		return statusErr.code == http.StatusRequestTimeout || statusErr.code == http.StatusTooManyRequests || statusErr.code >= http.StatusInternalServerError
	}
	_, networkError := errors.AsType[net.Error](err)
	return networkError
}

func (e *DoclingExtractor) execute(request *http.Request, target any) error {
	response, err := e.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("call Docling: %w; start it with make docling-up", err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		return &doclingStatusError{code: response.StatusCode, status: response.Status, message: strings.TrimSpace(string(message))}
	}
	if response.ContentLength > maximumDoclingResponseBodyBytes {
		return fmt.Errorf("decode Docling response: response exceeds %d bytes", maximumDoclingResponseBodyBytes)
	}

	data, err := io.ReadAll(io.LimitReader(response.Body, maximumDoclingResponseBodyBytes+1))
	if err != nil {
		return fmt.Errorf("read Docling response: %w", err)
	}
	if len(data) > maximumDoclingResponseBodyBytes {
		return fmt.Errorf("decode Docling response: response exceeds %d bytes", maximumDoclingResponseBodyBytes)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode Docling response: %w", err)
	}

	return nil
}

func writeDoclingConversionRequest(form *multipart.Writer, sourcePath, mediaType string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open source document: %w", err)
	}
	defer source.Close()

	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{
		"name":     "files",
		"filename": filepath.Base(sourcePath),
	}))
	header.Set("Content-Type", mediaType)

	part, err := form.CreatePart(header)
	if err != nil {
		return fmt.Errorf("create document form field: %w", err)
	}
	if _, err := io.Copy(part, source); err != nil {
		return fmt.Errorf("copy source document: %w", err)
	}

	fields := []struct {
		name  string
		value string
	}{
		{name: "to_formats", value: "md"},
		{name: "to_formats", value: "json"},
		{name: "include_images", value: "false"},
		{name: "table_mode", value: "accurate"},
		{name: "image_export_mode", value: "placeholder"},
	}
	if strings.EqualFold(filepath.Ext(sourcePath), ".pdf") {
		fields = append(fields, struct {
			name  string
			value string
		}{name: "pdf_backend", value: "dlparse_v4"})
	}

	for _, field := range fields {
		if err := form.WriteField(field.name, field.value); err != nil {
			return fmt.Errorf("write %s form field: %w", field.name, err)
		}
	}
	if err := form.Close(); err != nil {
		return fmt.Errorf("close conversion form: %w", err)
	}

	return nil
}

func doclingSourceMediaType(sourcePath string) (string, error) {
	switch strings.ToLower(filepath.Ext(sourcePath)) {
	case ".pdf":
		return "application/pdf", nil
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document", nil
	default:
		return "", fmt.Errorf("convert document: unsupported file type %q", filepath.Ext(sourcePath))
	}
}
