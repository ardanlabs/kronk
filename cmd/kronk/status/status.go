// Package status reports CLI activity using the OSC 7501 program status protocol.
package status

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"sync"
	"unicode"
)

type contextKey struct{}

type reporter struct {
	mu   sync.Mutex
	w    io.Writer
	last string
}

// WithWriter enables reports to w for this invocation. The caller must ensure
// that w is a terminal; without this context, reporting is a no-op.
func WithWriter(ctx context.Context, w io.Writer) context.Context {
	return context.WithValue(ctx, contextKey{}, &reporter{w: w})
}

// Report replaces the invocation's status. A negative progress omits the
// percentage. Completion and error records deliberately survive process exit.
func Report(ctx context.Context, state string, msg string, progress int) {
	r, ok := ctx.Value(contextKey{}).(*reporter)
	if !ok {
		return
	}

	// Protocol text must be a single line, without control characters, and no
	// more than 2048 UTF-8 bytes. Truncate at a rune boundary before encoding.
	var text strings.Builder
	for _, ch := range msg {
		if unicode.IsControl(ch) {
			ch = ' '
		}
		if text.Len()+len(string(ch)) > 2048 {
			break
		}
		text.WriteRune(ch)
	}

	seq := "\x1b]7501;state=" + state + ":app=kronk:msg=" + base64.StdEncoding.EncodeToString([]byte(text.String()))
	if progress >= 0 && (state == "working" || state == "blocked") {
		seq += fmt.Sprintf(":progress=%d", min(progress, 100))
	}
	seq += "\x1b\\"

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.w == nil || seq == r.last {
		return
	}
	if _, err := io.WriteString(r.w, seq); err != nil {
		// Status is best-effort presentation, never a reason to fail a command.
		r.w = nil
		return
	}
	r.last = seq
}

// Download reports per-file progress, or indeterminate work when size is unknown.
func Download(ctx context.Context, src string, current int64, total int64) {
	progress := -1
	if total > 0 {
		progress = int(max(0, min(100, float64(current)/float64(total)*100)))
	}
	Report(ctx, "working", "Downloading "+src, progress)
}
