package training

import (
	"crypto/rand"
	"encoding/json"
	"os"
)

// WriteRecords atomically publishes JSONL records within the caller's workspace.
func WriteRecords(root *os.Root, path string, records []Record) error {
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
