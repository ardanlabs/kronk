package catalog

import (
	"testing"
)

func TestParseShorthand(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		owner    string
		repo     string
		tag      string
		revision string
		ok       bool
	}{
		{
			name:     "basic",
			input:    "bartowski/Qwen3-8B-GGUF:Q4_K_M",
			owner:    "bartowski",
			repo:     "Qwen3-8B-GGUF",
			tag:      "Q4_K_M",
			revision: "main",
			ok:       true,
		},
		{
			name:     "with revision",
			input:    "owner/repo:Q8_0@dev",
			owner:    "owner",
			repo:     "repo",
			tag:      "Q8_0",
			revision: "dev",
			ok:       true,
		},
		{
			name:     "empty revision defaults to main",
			input:    "owner/repo:Q4_K_M@",
			owner:    "owner",
			repo:     "repo",
			tag:      "Q4_K_M",
			revision: "main",
			ok:       true,
		},
		{
			name:     "hf.co prefix",
			input:    "hf.co/owner/repo:Q4_K_M",
			owner:    "owner",
			repo:     "repo",
			tag:      "Q4_K_M",
			revision: "main",
			ok:       true,
		},
		{
			name:     "https prefix",
			input:    "https://huggingface.co/owner/repo:Q4_K_M",
			owner:    "owner",
			repo:     "repo",
			tag:      "Q4_K_M",
			revision: "main",
			ok:       true,
		},
		{
			name:     "case-insensitive prefix",
			input:    "HF.CO/Owner/Repo:Q4_K_M",
			owner:    "Owner",
			repo:     "Repo",
			tag:      "Q4_K_M",
			revision: "main",
			ok:       true,
		},
		{
			name:  "reject .gguf",
			input: "owner/repo/model.gguf",
			ok:    false,
		},
		{
			name:  "reject /resolve/",
			input: "https://huggingface.co/owner/repo/resolve/main/model.gguf",
			ok:    false,
		},
		{
			name:  "reject no colon",
			input: "owner/repo",
			ok:    false,
		},
		{
			name:  "reject empty tag",
			input: "owner/repo:",
			ok:    false,
		},
		{
			name:  "reject empty tag with revision",
			input: "owner/repo:@main",
			ok:    false,
		},
		{
			name:  "reject too many path segments",
			input: "a/b/c:Q4",
			ok:    false,
		},
		{
			name:  "reject empty owner",
			input: "/repo:Q4",
			ok:    false,
		},
		{
			name:  "reject empty repo",
			input: "owner/:Q4",
			ok:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, tag, revision, ok := parseShorthand(tt.input)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if !ok {
				return
			}
			if owner != tt.owner {
				t.Errorf("owner = %q, want %q", owner, tt.owner)
			}
			if repo != tt.repo {
				t.Errorf("repo = %q, want %q", repo, tt.repo)
			}
			if tag != tt.tag {
				t.Errorf("tag = %q, want %q", tag, tt.tag)
			}
			if revision != tt.revision {
				t.Errorf("revision = %q, want %q", revision, tt.revision)
			}
		})
	}
}

func TestModelID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Model-Q4_K_M.gguf", "Model-Q4_K_M"},
		{"Model-Q4_K_M.GGUF", "Model-Q4_K_M"},
		{"Model-Q8_0-00001-of-00002.gguf", "Model-Q8_0"},
		{"subdir/Model-Q4_K_M.gguf", "subdir/Model-Q4_K_M"},
		{"no-extension", "no-extension"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := modelID(tt.input)
			if got != tt.want {
				t.Errorf("modelID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGroupByModelID_CaseInsensitive(t *testing.T) {
	files := []HFRepoFile{
		{Filename: "Model-Q4_K_M.gguf"},
		{Filename: "model-Q4_K_M.gguf"},
	}

	groups := groupByModelID(files)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group (case-insensitive), got %d: %v", len(groups), groups)
	}
}

func TestClassifyGGUFFiles(t *testing.T) {
	files := []HFRepoFile{
		{Filename: "model-Q4_K_M.gguf"},
		{Filename: "mmproj-model-Q4_K_M.gguf"},
		{Filename: "README.md"},
		{Filename: "config.json"},
	}

	gguf, proj := classifyGGUFFiles(files)
	if len(gguf) != 1 || gguf[0].Filename != "model-Q4_K_M.gguf" {
		t.Errorf("gguf = %v, want [model-Q4_K_M.gguf]", gguf)
	}
	if len(proj) != 1 || proj[0].Filename != "mmproj-model-Q4_K_M.gguf" {
		t.Errorf("proj = %v, want [mmproj-model-Q4_K_M.gguf]", proj)
	}
}

func TestMatchByTag(t *testing.T) {
	files := []HFRepoFile{
		{Filename: "Model-Q4_K_M.gguf"},
		{Filename: "Model-Q8_0.gguf"},
		{Filename: "Model-IQ4_XS.gguf"},
	}

	matched := matchByTag(files, "Q4_K_M")
	if len(matched) != 1 || matched[0].Filename != "Model-Q4_K_M.gguf" {
		t.Errorf("matchByTag Q4_K_M = %v, want [Model-Q4_K_M.gguf]", matched)
	}

	matched = matchByTag(files, "q4_k_m")
	if len(matched) != 1 {
		t.Errorf("matchByTag q4_k_m (case-insensitive) = %v, want 1 match", matched)
	}
}

func TestValidateSplitCompleteness(t *testing.T) {
	tests := []struct {
		name    string
		files   []HFRepoFile
		wantErr bool
	}{
		{
			name:    "single file",
			files:   []HFRepoFile{{Filename: "model-Q4.gguf"}},
			wantErr: false,
		},
		{
			name: "complete 1-based split",
			files: []HFRepoFile{
				{Filename: "model-Q4-00001-of-00002.gguf"},
				{Filename: "model-Q4-00002-of-00002.gguf"},
			},
			wantErr: false,
		},
		{
			name: "complete 0-based split",
			files: []HFRepoFile{
				{Filename: "model-Q4-00000-of-00002.gguf"},
				{Filename: "model-Q4-00001-of-00002.gguf"},
			},
			wantErr: false,
		},
		{
			name: "incomplete 1-based split",
			files: []HFRepoFile{
				{Filename: "model-Q4-00001-of-00003.gguf"},
				{Filename: "model-Q4-00003-of-00003.gguf"},
			},
			wantErr: true,
		},
		{
			name: "inconsistent totals",
			files: []HFRepoFile{
				{Filename: "model-Q4-00001-of-00002.gguf"},
				{Filename: "model-Q4-00001-of-00003.gguf"},
			},
			wantErr: true,
		},
		{
			name: "multiple non-split files",
			files: []HFRepoFile{
				{Filename: "model-Q4.gguf"},
				{Filename: "model-Q8.gguf"},
			},
			wantErr: true,
		},
		{
			name: "mixed split and non-split",
			files: []HFRepoFile{
				{Filename: "model-Q4.gguf"},
				{Filename: "model-Q4-00001-of-00002.gguf"},
			},
			wantErr: true,
		},
		{
			name: "non-contiguous 0-based parts",
			files: []HFRepoFile{
				{Filename: "model-Q4-00000-of-00002.gguf"},
				{Filename: "model-Q4-00002-of-00002.gguf"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSplitCompleteness(tt.files)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSplitCompleteness() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDetectProjection(t *testing.T) {
	tests := []struct {
		name            string
		projFiles       []HFRepoFile
		selectedModelID string
		tag             string
		want            string
	}{
		{
			name:            "no proj files",
			projFiles:       nil,
			selectedModelID: "Model-Q4_K_M",
			tag:             "Q4_K_M",
			want:            "",
		},
		{
			name:            "single match",
			projFiles:       []HFRepoFile{{Filename: "mmproj-Model-Q4_K_M.gguf"}},
			selectedModelID: "Model-Q4_K_M",
			tag:             "Q4_K_M",
			want:            "mmproj-Model-Q4_K_M.gguf",
		},
		{
			name: "prefer same directory",
			projFiles: []HFRepoFile{
				{Filename: "other/mmproj-Model-Q4_K_M.gguf"},
				{Filename: "subdir/mmproj-Model-Q4_K_M.gguf"},
			},
			selectedModelID: "subdir/Model-Q4_K_M",
			tag:             "Q4_K_M",
			want:            "subdir/mmproj-Model-Q4_K_M.gguf",
		},
		{
			name: "fallback to tag match",
			projFiles: []HFRepoFile{
				{Filename: "mmproj-Other.gguf"},
				{Filename: "mmproj-with-Q4_K_M.gguf"},
			},
			selectedModelID: "Unrelated-Model",
			tag:             "Q4_K_M",
			want:            "mmproj-with-Q4_K_M.gguf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectProjection(tt.projFiles, tt.selectedModelID, tt.tag)
			if got != tt.want {
				t.Errorf("detectProjection() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProjBaseModelID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"mmproj-Model-Q4_K_M.gguf", "Model-Q4_K_M"},
		{"mmproj_Model-Q4_K_M.gguf", "Model-Q4_K_M"},
		{"subdir/mmproj-Model-Q4_K_M.gguf", "Model-Q4_K_M"},
		{"MMPROJ-Model-Q4_K_M.gguf", "Model-Q4_K_M"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := projBaseModelID(tt.input)
			if got != tt.want {
				t.Errorf("projBaseModelID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBaseName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"file.gguf", "file.gguf"},
		{"subdir/file.gguf", "file.gguf"},
		{"a/b/c/file.gguf", "file.gguf"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := baseName(tt.input)
			if got != tt.want {
				t.Errorf("baseName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDirName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"file.gguf", ""},
		{"subdir/file.gguf", "subdir"},
		{"a/b/c/file.gguf", "a/b/c"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := dirName(tt.input)
			if got != tt.want {
				t.Errorf("dirName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRepoRelativePath(t *testing.T) {
	tests := []struct {
		name  string
		ref   string
		owner string
		repo  string
		want  string
	}{
		{
			name:  "short form top-level",
			ref:   "owner/repo/file.gguf",
			owner: "owner",
			repo:  "repo",
			want:  "file.gguf",
		},
		{
			name:  "short form subdirectory",
			ref:   "owner/repo/subdir/file.gguf",
			owner: "owner",
			repo:  "repo",
			want:  "subdir/file.gguf",
		},
		{
			name:  "full URL main",
			ref:   "https://huggingface.co/owner/repo/resolve/main/file.gguf",
			owner: "owner",
			repo:  "repo",
			want:  "file.gguf",
		},
		{
			name:  "full URL non-main revision",
			ref:   "https://huggingface.co/owner/repo/resolve/dev/subdir/file.gguf",
			owner: "owner",
			repo:  "repo",
			want:  "subdir/file.gguf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := repoRelativePath(tt.ref, tt.owner, tt.repo)
			if got != tt.want {
				t.Errorf("repoRelativePath(%q, %q, %q) = %q, want %q", tt.ref, tt.owner, tt.repo, got, tt.want)
			}
		})
	}
}
