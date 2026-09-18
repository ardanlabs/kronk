package libs

import (
	"slices"
	"strings"
	"testing"

	"github.com/hybridgroup/yzma/pkg/download"
)

func TestOpenVINOCombinations(t *testing.T) {
	tests := []struct {
		name  string
		arch  string
		opSys string
		want  bool
	}{
		{name: "linux amd64", arch: "amd64", opSys: "linux", want: true},
		{name: "windows amd64", arch: "amd64", opSys: "windows", want: true},
		{name: "linux arm64", arch: "arm64", opSys: "linux"},
		{name: "windows arm64", arch: "arm64", opSys: "windows"},
		{name: "darwin amd64", arch: "amd64", opSys: "darwin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSupported(tt.arch, tt.opSys, "openvino"); got != tt.want {
				t.Errorf("IsSupported(%q, %q, openvino): got %t, want %t", tt.arch, tt.opSys, got, tt.want)
			}
		})
	}
}

func TestOpenVINOResolver(t *testing.T) {
	tests := []struct {
		name  string
		opSys download.OS
	}{
		{name: "linux", opSys: download.Linux},
		{name: "windows", opSys: download.Windows},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			urls, err := download.DefaultResolver.Resolve(download.Target{
				Arch:      download.AMD64,
				OS:        tt.opSys,
				Processor: download.OpenVINO,
				Version:   versionTag(defaultVersion),
			})
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}

			if !slices.ContainsFunc(urls, func(url string) bool {
				return strings.Contains(url, "-openvino-2026.4-x64")
			}) {
				t.Errorf("Resolve() URLs = %q, want OpenVINO 2026.4 x64 asset", urls)
			}
		})
	}
}
