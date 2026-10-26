package dataset

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

type ContextReader struct {
	Context context.Context
	Reader  io.Reader
}

func (r ContextReader) Read(p []byte) (int, error) {
	if err := r.Context.Err(); err != nil {
		return 0, err
	}
	return r.Reader.Read(p)
}

// WriteJSON atomically publishes a file, so readers see the previous complete
// value or the new complete value, never a partially written JSON document.
func WriteJSON(root *os.Root, path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	temporary := filepath.Join(filepath.Dir(path), ".publish-"+rand.Text())
	file, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer root.Remove(temporary)
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return err
	}
	return root.Rename(temporary, path)
}
