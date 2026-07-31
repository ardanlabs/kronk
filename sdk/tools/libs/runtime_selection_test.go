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

func TestParseCUDA13Host(t *testing.T) {
	tests := []struct {
		name             string
		output           string
		visible          string
		filter           bool
		wantState        hostCUDAState
		wantCapabilities []string
	}{
		{
			name:             "pascal is unsupported",
			output:           "0, GPU-pascal, 6.1\n",
			wantState:        hostCUDAUnsupported,
			wantCapabilities: []string{"6.1"},
		},
		{
			name:             "volta is unsupported",
			output:           "0, GPU-volta, 7.0\n",
			wantState:        hostCUDAUnsupported,
			wantCapabilities: []string{"7.0"},
		},
		{
			name:             "turing is supported",
			output:           "0, GPU-turing, 7.5\n",
			wantState:        hostCUDASupported,
			wantCapabilities: []string{"7.5"},
		},
		{
			name:             "mixed generations are unsupported",
			output:           "0, GPU-ampere, 8.6\n1, GPU-pascal, 6.1\n",
			wantState:        hostCUDAUnsupported,
			wantCapabilities: []string{"8.6", "6.1"},
		},
		{
			name:             "hidden pascal does not disable cuda",
			output:           "0, GPU-ampere, 8.6\n1, GPU-pascal, 6.1\n",
			visible:          "0",
			filter:           true,
			wantState:        hostCUDASupported,
			wantCapabilities: []string{"8.6"},
		},
		{
			name:      "empty visibility mask makes cuda unavailable",
			output:    "0, GPU-ampere, 8.6\n",
			filter:    true,
			wantState: hostCUDAUnavailable,
		},
		{
			name:             "unsupported capability wins over malformed row",
			output:           "0, GPU-pascal, 6.1\nmalformed\n",
			wantState:        hostCUDAUnsupported,
			wantCapabilities: []string{"6.1"},
		},
		{
			name:      "empty output is unknown",
			wantState: hostCUDAUnknown,
		},
		{
			name:      "unexpected output is unknown",
			output:    "N/A\n",
			wantState: hostCUDAUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCUDA13Host(tt.output, tt.visible, tt.filter)
			if got.state != tt.wantState {
				t.Errorf("state: got %v, want %v", got.state, tt.wantState)
			}
			if !slices.Equal(got.capabilities, tt.wantCapabilities) {
				t.Errorf("capabilities: got %v, want %v", got.capabilities, tt.wantCapabilities)
			}
		})
	}
}

func TestHasROCmGPU(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "gpu with kernel dispatch",
			output: "Agent 1\n  Device Type: GPU\n  Kernel Dispatch: FULL\n",
			want:   true,
		},
		{
			name:   "cpu agent only",
			output: "Agent 1\n  Device Type: CPU\n  Kernel Dispatch: FULL\n",
		},
		{
			name:   "gpu without kernel dispatch",
			output: "Agent 1\n  Device Type: GPU\n  Kernel Dispatch: NONE\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasROCmGPU(tt.output); got != tt.want {
				t.Errorf("ROCm GPU: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasVulkanGPU(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "discrete GPU",
			output: "deviceType = PHYSICAL_DEVICE_TYPE_DISCRETE_GPU\n",
			want:   true,
		},
		{
			name:   "integrated GPU",
			output: "deviceType = PHYSICAL_DEVICE_TYPE_INTEGRATED_GPU\n",
			want:   true,
		},
		{
			name:   "software CPU device",
			output: "deviceType = PHYSICAL_DEVICE_TYPE_CPU\n",
		},
		{
			name:   "instance without devices",
			output: "Vulkan Instance Version: 1.3.280\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasVulkanGPU(tt.output); got != tt.want {
				t.Errorf("Vulkan GPU: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSelectHostRuntime(t *testing.T) {
	arch, err := download.ParseArch("amd64")
	if err != nil {
		t.Fatalf("parse arch: %v", err)
	}
	opSys, err := download.ParseOS("linux")
	if err != nil {
		t.Fatalf("parse os: %v", err)
	}

	root := filepath.Join(t.TempDir(), localFolder)
	primary := &Libs{
		root:      root,
		path:      installPathFor(root, arch, opSys, download.CUDA),
		arch:      arch,
		os:        opSys,
		processor: download.CUDA,
	}

	probeFn := func(context.Context) hostCUDAProbe {
		return hostCUDAProbe{
			state:        hostCUDAUnsupported,
			capabilities: []string{"6.1"},
		}
	}
	fallbackFn := func(context.Context) download.Processor {
		return download.Vulkan
	}

	selected, decision := primary.selectHostRuntime(context.Background(), noopLog, probeFn, fallbackFn)
	if got, want := selected.Processor(), "vulkan"; got != want {
		t.Errorf("processor: got %q, want %q", got, want)
	}
	if got, want := selected.LibsPath(), installPathFor(root, arch, opSys, download.Vulkan); got != want {
		t.Errorf("path: got %q, want %q", got, want)
	}
	if got, want := decision.PreferredProcessor, "cuda"; got != want {
		t.Errorf("preferred processor: got %q, want %q", got, want)
	}
	if got, want := decision.SelectedProcessor, "vulkan"; got != want {
		t.Errorf("selected processor: got %q, want %q", got, want)
	}
	if got, want := primary.Processor(), "cuda"; got != want {
		t.Errorf("primary processor mutated: got %q, want %q", got, want)
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
