package apitest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

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
