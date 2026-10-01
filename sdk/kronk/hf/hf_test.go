package hf

import "testing"

func TestRepoFilesURLPreservesRevisionWithPath(t *testing.T) {
	got := repoFilesURL("owner", "repo", "feature/revision", "quantized/Q4 K", false)
	want := "https://huggingface.co/api/models/owner/repo/tree/feature%2Frevision/quantized/Q4%20K"
	if got != want {
		t.Errorf("repoFilesURL: got %q, want %q", got, want)
	}
}

func TestRepoFilesURLRecursiveIgnoresPath(t *testing.T) {
	got := repoFilesURL("owner", "repo", "branch", "ignored/path", true)
	want := "https://huggingface.co/api/models/owner/repo/tree/branch?recursive=true"
	if got != want {
		t.Errorf("repoFilesURL: got %q, want %q", got, want)
	}
}
