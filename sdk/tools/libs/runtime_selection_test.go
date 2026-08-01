package libs

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/hybridgroup/yzma/pkg/download"
)

func TestWithDetectRespectsExplicitOptions(t *testing.T) {
	scrubKronkEnv(t)

	arch, err := download.ParseArch("arm64")
	if err != nil {
		t.Fatalf("parse arch: %v", err)
	}
	opSys, err := download.ParseOS("darwin")
	if err != nil {
		t.Fatalf("parse os: %v", err)
	}
	processor, err := download.ParseProcessor("cpu")
	if err != nil {
		t.Fatalf("parse processor: %v", err)
	}

	tests := []struct {
		name string
		opts []Option
	}{
		{
			name: "detect before explicit options",
			opts: []Option{
				WithDetect(context.Background(), noopLog),
				WithArch(arch),
				WithOS(opSys),
				WithProcessor(processor),
			},
		},
		{
			name: "detect after explicit options",
			opts: []Option{
				WithArch(arch),
				WithOS(opSys),
				WithProcessor(processor),
				WithDetectOverrides(context.Background(), noopLog, "", "amd64", "linux", "cuda"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := append([]Option{WithBasePath(t.TempDir())}, tt.opts...)
			lib, err := New(opts...)
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			if got, want := lib.Arch(), "arm64"; got != want {
				t.Errorf("arch: got %q, want %q", got, want)
			}
			if got, want := lib.OS(), "darwin"; got != want {
				t.Errorf("os: got %q, want %q", got, want)
			}
			if got, want := lib.Processor(), "cpu"; got != want {
				t.Errorf("processor: got %q, want %q", got, want)
			}
		})
	}
}

func TestWithDetectOverrides(t *testing.T) {
	scrubKronkEnv(t)

	lib, err := New(
		WithBasePath(t.TempDir()),
		WithDetectOverrides(context.Background(), noopLog, "", "amd64", "linux", "cpu"),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if got, want := lib.Arch(), "amd64"; got != want {
		t.Errorf("arch: got %q, want %q", got, want)
	}
	if got, want := lib.OS(), "linux"; got != want {
		t.Errorf("os: got %q, want %q", got, want)
	}
	if got, want := lib.Processor(), "cpu"; got != want {
		t.Errorf("processor: got %q, want %q", got, want)
	}
}

func TestNewAutomaticallySelectsCompatibleHostRuntime(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix shell scripts")
	}

	scrubKronkEnv(t)

	bin := t.TempDir()
	nvidiaSMI := `#!/bin/sh
if [ "$#" -eq 0 ]; then
  echo "CUDA Version: 13.0"
  exit 0
fi
echo "0, GPU-pascal, 6.1"
`
	vulkanInfo := `#!/bin/sh
echo "deviceType = PHYSICAL_DEVICE_TYPE_DISCRETE_GPU"
`
	for name, content := range map[string]string{
		"nvidia-smi": nvidiaSMI,
		"vulkaninfo": vulkanInfo,
	} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(content), 0o755); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	t.Setenv("PATH", bin)

	basePath := t.TempDir()
	root := filepath.Join(basePath, localFolder)
	arch, err := download.ParseArch("amd64")
	if err != nil {
		t.Fatalf("parse arch: %v", err)
	}
	opSys, err := download.ParseOS("linux")
	if err != nil {
		t.Fatalf("parse os: %v", err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	if err := writeVersionFile(root, "b100", arch, opSys, download.CUDA); err != nil {
		t.Fatalf("write version: %v", err)
	}

	lib, err := New(WithBasePath(basePath))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got, want := lib.Processor(), "vulkan"; got != want {
		t.Errorf("processor: got %q, want %q", got, want)
	}
}

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
			name:             "invalid visibility token terminates the list",
			output:           "0, GPU-ampere, 8.6\n1, GPU-pascal, 6.1\n",
			visible:          "0,-1,1",
			filter:           true,
			wantState:        hostCUDASupported,
			wantCapabilities: []string{"8.6"},
		},
		{
			name:      "out of range visibility token exposes no devices",
			output:    "0, GPU-ampere, 8.6\n1, GPU-pascal, 6.1\n",
			visible:   "99,1",
			filter:    true,
			wantState: hostCUDAUnavailable,
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
			output: "Agent 1\n  Device Type: GPU\n  Feature: KERNEL_DISPATCH\n",
			want:   true,
		},
		{
			name:   "gpu with combined dispatch features",
			output: "Agent 1\n  Device Type: GPU\n  Features: KERNEL_DISPATCH & AGENT_DISPATCH\n",
			want:   true,
		},
		{
			name:   "cpu agent only",
			output: "Agent 1\n  Device Type: CPU\n  Feature: KERNEL_DISPATCH\n",
		},
		{
			name:   "gpu without kernel dispatch",
			output: "Agent 1\n  Device Type: GPU\n  Feature: AGENT_DISPATCH\n",
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
