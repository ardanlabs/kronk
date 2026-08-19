package malina

import "testing"

func TestCommandTree(t *testing.T) {
	for _, path := range [][]string{
		{"libs"},
		{"model", "catalog"},
		{"model", "list"},
		{"model", "pull"},
		{"model", "remove"},
	} {
		if _, _, err := Cmd.Find(path); err != nil {
			t.Errorf("Cmd.Find(%v) error = %v", path, err)
		}
	}
}
