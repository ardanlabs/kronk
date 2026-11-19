package apitest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestRunResponseComparisons(t *testing.T) {
	var retryRequests int
	at := NewWithHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Result", r.URL.Path)
		switch r.URL.Path {
		case "/raw":
			if _, err := w.Write([]byte{0xff, 0, 0x7f, 0x80}); err != nil {
				t.Error(err)
			}
		case "/empty":
			w.WriteHeader(http.StatusNoContent)
		case "/retry":
			retryRequests++
			if retryRequests > 1 {
				w.Header().Set("X-Result", "ready")
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			fmt.Fprint(w, `{"value":"copper"}`)
		}
	}))
	var rawChecked, headersChecked, jsonChecked, emptyChecked, retryChecked bool
	at.Run(t, []Table{
		{
			Name: "binary", URL: "/raw", Method: http.MethodGet, StatusCode: http.StatusOK,
			ResponseCmpFunc: func(header http.Header, body []byte) string {
				rawChecked = true
				if header.Get("X-Result") != "/raw" || !bytes.Equal(body, []byte{255, 0, 127, 128}) {
					return fmt.Sprintf("binary response changed: %v %v", header, body)
				}
				return ""
			},
		},
		{
			Name: "JSON-and-headers", URL: "/json", Method: http.MethodGet, StatusCode: http.StatusOK,
			GotResp: &map[string]string{}, ExpResp: "copper",
			ResponseCmpFunc: func(header http.Header, _ []byte) string {
				headersChecked = true
				if header.Get("X-Result") != "/json" {
					return "JSON response headers changed"
				}
				return ""
			},
			CmpFunc: func(got, exp any) string {
				jsonChecked = true
				if (*got.(*map[string]string))["value"] != exp.(string) {
					return fmt.Sprintf("JSON response changed: %v", got)
				}
				return ""
			},
		},
		{
			Name: "empty-and-headers", URL: "/empty", Method: http.MethodGet, StatusCode: http.StatusNoContent,
			ResponseCmpFunc: func(header http.Header, body []byte) string {
				emptyChecked = true
				if header.Get("X-Result") != "/empty" || len(body) != 0 {
					return "204 response headers or body changed"
				}
				return ""
			},
		},
		{
			Name: "retry-headers", URL: "/retry", Method: http.MethodGet, StatusCode: http.StatusNoContent,
			ResponseCmpFunc: func(header http.Header, _ []byte) string {
				if header.Get("X-Result") != "ready" {
					return "response is not ready"
				}
				retryChecked = true
				return ""
			},
		},
	}, "responses", WithRetries(1))
	if !rawChecked || !headersChecked || !jsonChecked || !emptyChecked || !retryChecked {
		t.Fatal("raw, header, JSON, 204, and retry comparisons must all pass")
	}
}

func TestRunStreamingSSE(t *testing.T) {
	// This exceeds Scanner's default line limit and splits the first JSON
	// payload across data fields. Names, comments and CRLF must survive framing.
	text := strings.Repeat("x", 70_000)
	at := Test{mux: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, ": heartbeat\r\nevent: response.delta\r\ndata:{\"text\":\r\ndata: %q}\r\n\r\nevent: response.done\r\ndata: {\"text\":\"final\"}\r\n\r\ndata: [DONE]\r\n\r\n", text)
	})}
	var dataChecked, namesChecked bool
	at.RunStreaming(t, []Table{{
		Name: "named-multiline", URL: "/stream", Method: http.MethodPost, StatusCode: http.StatusOK,
		GotResp: &struct{ Text string }{}, RequireDone: true,
		CmpFunc: func(got, _ any) string {
			if got.(*struct{ Text string }).Text != "final" {
				return "selected response is not the final data event"
			}
			return ""
		},
		StreamCmpFunc: func(events []json.RawMessage) string {
			dataChecked = true
			if len(events) != 2 {
				return "expected two data events excluding DONE"
			}
			var first struct{ Text string }
			if err := json.Unmarshal(events[0], &first); err != nil {
				return err.Error()
			}
			if first.Text != text {
				return "multiline data was truncated or not joined"
			}
			return ""
		},
		SSECmpFunc: func(events []SSEEvent) string {
			namesChecked = true
			if len(events) != 2 || events[0].Event != "response.delta" || events[1].Event != "response.done" {
				return "SSE event names were not preserved"
			}
			return ""
		},
	}}, "stream")
	if !dataChecked || !namesChecked {
		t.Fatal("full-stream validators were not called")
	}
}
