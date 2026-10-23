package status_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ardanlabs/kronk/cmd/kronk/status"
)

func TestReports(t *testing.T) {
	var buf bytes.Buffer
	ctx := status.WithWriter(context.Background(), &buf)
	status.Download(ctx, "model.gguf", 37, 100)
	status.Download(ctx, "model.gguf", 379, 1000) // Same integer percentage.
	status.Download(ctx, "projection.gguf", 1, 0)
	status.Report(ctx, "done", "Installed\nmodel\x1b", -1)
	want := "\x1b]7501;state=working:app=kronk:msg=RG93bmxvYWRpbmcgbW9kZWwuZ2d1Zg==:progress=37\x1b\\" +
		"\x1b]7501;state=working:app=kronk:msg=RG93bmxvYWRpbmcgcHJvamVjdGlvbi5nZ3Vm\x1b\\" +
		"\x1b]7501;state=done:app=kronk:msg=SW5zdGFsbGVkIG1vZGVsIA==\x1b\\"
	if got := buf.String(); got != want {
		t.Fatalf("reports: got %q, want %q", got, want)
	}
}

func TestMessageLimit(t *testing.T) {
	var buf bytes.Buffer
	ctx := status.WithWriter(context.Background(), &buf)
	status.Report(ctx, "error", strings.Repeat("界", 1000), -1)
	encoded := strings.TrimSuffix(strings.SplitN(buf.String(), ":msg=", 2)[1], "\x1b\\")
	msg, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(msg) != 2046 || !utf8.Valid(msg) {
		t.Fatalf("message: got %d bytes, UTF-8 valid %v; want 2046 valid bytes", len(msg), utf8.Valid(msg))
	}
}

func TestDownloadBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name    string
		current int64
		total   int64
		want    string
	}{
		{"start", 0, 100, ":progress=0"},
		{"resumed", 73, 200, ":progress=36"},
		{"complete", 100, 100, ":progress=100"},
		{"over-total", 101, 100, ":progress=100"},
		{"unknown", 37, -1, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			status.Download(status.WithWriter(context.Background(), &buf), "file", tc.current, tc.total)
			got := buf.String()
			if tc.want == "" {
				if strings.Contains(got, "progress=") {
					t.Fatalf("report: got %q, want indeterminate progress", got)
				}
				return
			}
			if !strings.HasSuffix(got, tc.want+"\x1b\\") {
				t.Fatalf("report: got %q, want suffix %q", got, tc.want)
			}
		})
	}
}
