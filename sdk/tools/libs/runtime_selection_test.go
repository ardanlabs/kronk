package libs

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/hybridgroup/yzma/pkg/download"
)

func TestParseRuntimeDevices(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		wantState   runtimeProbeState
		wantDevices []string
	}{
		{
			name:      "explicit none",
			output:    "backend logging\nAvailable devices:\n  (none)\n",
			wantState: runtimeProbeNone,
		},
		{
			name:        "one device",
			output:      "Available devices:\n  Vulkan0: NVIDIA GeForce GTX 1070 (8438 MiB, 6825 MiB free)\n",
			wantState:   runtimeProbeDevices,
			wantDevices: []string{"Vulkan0"},
		},
		{
			name:        "multiple devices",
			output:      "Available devices:\n  CUDA0: NVIDIA A (1024 MiB, 900 MiB free)\n  CUDA1: NVIDIA B (2048 MiB, 1800 MiB free)\n",
			wantState:   runtimeProbeDevices,
			wantDevices: []string{"CUDA0", "CUDA1"},
		},
		{
			name:      "missing header",
			output:    "CUDA initialization failed",
			wantState: runtimeProbeUnknown,
		},
		{
			name:      "header embedded in warning",
			output:    "warning: Available devices:\nVulkan0: diagnostic text (1024 MiB, 900 MiB free)",
			wantState: runtimeProbeUnknown,
		},
		{
			name:      "malformed body",
			output:    "Available devices:\nmaybe",
			wantState: runtimeProbeUnknown,
		},
		{
			name:      "colon bearing warning",
			output:    "Available devices:\nwarning: backend initialization incomplete",
			wantState: runtimeProbeUnknown,
		},
		{
			name:      "device row before header",
			output:    "CUDA0: GPU (1024 MiB, 900 MiB free)\nAvailable devices:\n  (none)\n",
			wantState: runtimeProbeNone,
		},
		{
			name:        "crlf device row",
			output:      "Available devices:\r\n  Vulkan0: GPU (1024 MiB, 900 MiB free)\r\n",
			wantState:   runtimeProbeDevices,
			wantDevices: []string{"Vulkan0"},
		},
		{
			name:      "device mixed with warning",
			output:    "Available devices:\nVulkan0: GPU (1024 MiB, 900 MiB free)\nwarning: incomplete",
			wantState: runtimeProbeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, devices := parseRuntimeDevices(tt.output)
			if state != tt.wantState {
				t.Errorf("state: got %v, want %v", state, tt.wantState)
			}
			if !slices.Equal(devices, tt.wantDevices) {
				t.Errorf("devices: got %v, want %v", devices, tt.wantDevices)
			}
		})
	}
}

func TestChooseRuntime(t *testing.T) {
	cuda := runtimeCandidate{processor: download.CUDA, path: "/cuda", version: "b100"}
	rocm := runtimeCandidate{processor: download.ROCm, path: "/rocm", version: "b100"}
	vulkan := runtimeCandidate{processor: download.Vulkan, path: "/vulkan", version: "b100"}

	tests := []struct {
		name       string
		probes     []runtimeProbe
		want       runtimeCandidate
		wantSelect bool
	}{
		{
			name: "first positively verified alternative",
			probes: []runtimeProbe{
				{candidate: rocm, state: runtimeProbeUnknown},
				{candidate: vulkan, state: runtimeProbeDevices},
			},
			want:       vulkan,
			wantSelect: true,
		},
		{
			name: "candidate order is honored",
			probes: []runtimeProbe{
				{candidate: cuda, state: runtimeProbeDevices},
				{candidate: vulkan, state: runtimeProbeDevices},
			},
			want:       cuda,
			wantSelect: true,
		},
		{
			name: "no positive alternative",
			probes: []runtimeProbe{
				{candidate: rocm, state: runtimeProbeUnknown},
				{candidate: vulkan, state: runtimeProbeNone},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := chooseRuntime(tt.probes)
			if ok != tt.wantSelect {
				t.Errorf("selected: got %v, want %v", ok, tt.wantSelect)
			}
			if got != tt.want {
				t.Errorf("candidate: got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestPrependRuntimePath(t *testing.T) {
	separator := string(os.PathListSeparator)
	tests := []struct {
		name string
		env  []string
		key  string
		path string
		want []string
	}{
		{
			name: "existing value",
			env:  []string{"A=1", "PATH=/usr/bin"},
			key:  "PATH",
			path: "/runtime",
			want: []string{"A=1", "PATH=/runtime" + separator + "/usr/bin"},
		},
		{
			name: "missing value",
			env:  []string{"A=1"},
			key:  "PATH",
			path: "/runtime",
			want: []string{"A=1", "PATH=/runtime"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := prependRuntimePath(tt.env, tt.key, tt.path)
			if !slices.Equal(got, tt.want) {
				t.Errorf("environment: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSelectInstalledRuntime(t *testing.T) {
	scrubKronkEnv(t)

	root := filepath.Join(t.TempDir(), localFolder)
	arch, err := download.ParseArch("amd64")
	if err != nil {
		t.Fatalf("parse arch: %v", err)
	}
	opSys, err := download.ParseOS("linux")
	if err != nil {
		t.Fatalf("parse os: %v", err)
	}

	cudaPath := installPathFor(root, arch, opSys, download.CUDA)
	vulkanPath := installPathFor(root, arch, opSys, download.Vulkan)
	for path, processor := range map[string]download.Processor{
		cudaPath:   download.CUDA,
		vulkanPath: download.Vulkan,
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := writeVersionFile(path, "b100", arch, opSys, processor); err != nil {
			t.Fatalf("write version %s: %v", path, err)
		}
	}

	primary := &Libs{
		root:      root,
		path:      cudaPath,
		arch:      arch,
		os:        opSys,
		processor: download.CUDA,
	}

	probeFn := func(_ context.Context, candidate runtimeCandidate) runtimeProbe {
		state := runtimeProbeNone
		if candidate.processor.Equal(download.Vulkan) {
			state = runtimeProbeDevices
		}
		return runtimeProbe{candidate: candidate, state: state}
	}

	selected, decision, err := primary.selectInstalledRuntime(context.Background(), noopLog, probeFn)
	if err != nil {
		t.Fatalf("select runtime: %v", err)
	}
	if got, want := selected.Processor(), "vulkan"; got != want {
		t.Errorf("processor: got %q, want %q", got, want)
	}
	if got, want := selected.LibsPath(), vulkanPath; got != want {
		t.Errorf("path: got %q, want %q", got, want)
	}
	if got, want := decision.PreferredProcessor, "cuda"; got != want {
		t.Errorf("preferred processor: got %q, want %q", got, want)
	}
	if got, want := primary.Processor(), "cuda"; got != want {
		t.Errorf("primary processor mutated: got %q, want %q", got, want)
	}
	if got, want := primary.LibsPath(), cudaPath; got != want {
		t.Errorf("primary path mutated: got %q, want %q", got, want)
	}
}
