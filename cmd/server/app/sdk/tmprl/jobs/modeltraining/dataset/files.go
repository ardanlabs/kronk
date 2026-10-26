package dataset

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"os"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/model"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
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
	return tmprl.WriteJSON(root, path, value)
}

// WriteRecords atomically publishes JSONL records within the caller's workspace.
func WriteRecords(root *os.Root, path string, records []model.Record) error {
	temporary := ".records-" + rand.Text()
	file, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer root.Remove(temporary)
	encoder := json.NewEncoder(file)
	for _, r := range records {
		if err := encoder.Encode(r); err != nil {
			file.Close()
			return err
		}
	}
	if err := file.Close(); err != nil {
		return err
	}
	return root.Rename(temporary, path)
}
