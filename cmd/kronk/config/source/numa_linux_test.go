//go:build linux

package source

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCountNUMANodes(t *testing.T) {
	sysfs := func(t *testing.T, names ...string) string {
		t.Helper()
		dir := t.TempDir()
		for _, name := range names {
			if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}

	for name, tt := range map[string]struct {
		dir  string
		want int
	}{
		"one node":     {sysfs(t, "node0", "power"), 1},
		"two nodes":    {sysfs(t, "node0", "node1", "nodex"), 2},
		"no directory": {filepath.Join(t.TempDir(), "missing"), 1},
	} {
		if got := countNUMANodes(tt.dir); got != tt.want {
			t.Errorf("%s: countNUMANodes = %d, want %d", name, got, tt.want)
		}
	}
}
