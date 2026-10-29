package source

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigFilesByName(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	for name, body := range map[string]string{
		".goreleaser.yaml":      "version: 2\nbuilds: []\n",
		"model_config.yaml":     "version: 2\nmodels: {}\n",
		"model_config.gpu.yaml": "version: 2\nmodels: {}\n",
		"model_config.bad.yaml": "version: 99\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got := ConfigFiles("/srv/model_config.yaml")
	want := []string{"/srv/model_config.yaml", filepath.Join(dir, "model_config.gpu.yaml"), filepath.Join(dir, "model_config.yaml")}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
