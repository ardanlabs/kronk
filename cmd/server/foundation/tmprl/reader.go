package tmprl

import (
	"context"
	"io"
)

// ContextReader checks cancellation before each read from the borrowed reader.
// Cancellation cannot interrupt a read that is already blocked in the reader.
type ContextReader struct {
	Context context.Context
	Reader  io.Reader
}

// Read checks the context before reading into p.
func (r ContextReader) Read(p []byte) (int, error) {
	if err := r.Context.Err(); err != nil {
		return 0, err
	}
	return r.Reader.Read(p)
}
