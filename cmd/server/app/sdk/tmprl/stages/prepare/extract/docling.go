package extract

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
	"strings"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/shared/model"
)

type doclingExtractor struct {
	endpoint string
	client   *http.Client
}

// NewDoclingExtractor connects to an existing Docling service, never starts
// containers. A custom HTTP client can supply authentication and timeouts.
func NewDoclingExtractor(endpoint string, client *http.Client) (model.DocumentExtractor, error) {
	if endpoint == "" {
		endpoint = "http://127.0.0.1:5001"
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("%w: invalid Docling endpoint", model.ErrInvalidArgument)
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &doclingExtractor{strings.TrimRight(endpoint, "/"), client}, nil
}

func (*doclingExtractor) ExtractionProfile() string { return "tuning-docling-dlparse_v4-accurate-v1" }

func (e *doclingExtractor) Convert(ctx context.Context, path string) (model.Document, error) {
	mediaType := "application/pdf"
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf":
	case ".docx":
		mediaType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	default:
		return model.Document{}, fmt.Errorf("%w: Docling accepts PDF or DOCX", model.ErrInvalidArgument)
	}
	file, err := os.Open(path)
	if err != nil {
		return model.Document{}, err
	}
	reader, writer := io.Pipe()
	defer reader.Close()
	form := multipart.NewWriter(writer)
	go func() {
		defer file.Close()
		options := [][2]string{{"to_formats", "md"}, {"to_formats", "json"}, {"include_images", "false"}, {"table_mode", "accurate"}, {"image_export_mode", "placeholder"}}
		if mediaType == "application/pdf" {
			options = append(options, [2]string{"pdf_backend", "dlparse_v4"})
		}
		for _, option := range options {
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
		return model.Document{}, err
	}
	if task.ID == "" {
		return model.Document{}, fmt.Errorf("docling returned an empty task ID")
	}
	id := task.ID
	for task.Status != "success" {
		switch task.Status {
		case "pending", "started":
		default:
			return model.Document{}, fmt.Errorf("docling conversion did not succeed: %s", task.Status)
		}
		select {
		case <-ctx.Done():
			return model.Document{}, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
		if err := e.read(ctx, "/v1/status/poll/"+url.PathEscape(id), &task); err != nil {
			return model.Document{}, err
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
		return model.Document{}, err
	}
	var structured struct {
		Texts    []json.RawMessage `json:"texts"`
		Tables   []json.RawMessage `json:"tables"`
		Pictures []json.RawMessage `json:"pictures"`
	}
	if result.Status != "success" || strings.TrimSpace(result.Document.Markdown) == "" || json.Unmarshal(result.Document.JSON, &structured) != nil || len(structured.Texts)+len(structured.Tables)+len(structured.Pictures) == 0 {
		return model.Document{}, fmt.Errorf("docling returned invalid or empty document content")
	}
	return model.Document{Markdown: result.Document.Markdown, StructuredJSON: result.Document.JSON, ProcessingTime: result.ProcessingTime}, nil
}

func (e *doclingExtractor) read(ctx context.Context, path string, out any) error {
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

func (e *doclingExtractor) request(ctx context.Context, method, path string, body io.Reader, contentType string, out any) (int, error) {
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
