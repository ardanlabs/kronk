package diagnose

import (
	"strings"
	"testing"
)

func TestBackendSelectionHints(t *testing.T) {
	noneCommand := Command{
		Cmd:    "/cuda/llama-bench --list-devices",
		Output: "Available devices:\n  (none)\n",
	}
	vulkanCommand := Command{
		Cmd:    "/vulkan/llama-bench --list-devices",
		Output: "Available devices:\n  Vulkan0: NVIDIA GeForce GTX 1070 (8438 MiB, 6825 MiB free)\n",
	}

	tests := []struct {
		name     string
		engine   Engine
		backends []Backend
		wantHint bool
	}{
		{
			name:   "selected backend has verified alternative",
			engine: Engine{Probed: true, Loaded: true, Processor: "cuda", LibPath: "/cuda"},
			backends: []Backend{
				{Processor: "cuda", Version: "b100", BinDir: "/cuda", Commands: []Command{noneCommand}},
				{Processor: "vulkan", Version: "b100", BinDir: "/vulkan", Devices: []Device{{ID: "Vulkan0", Name: "NVIDIA GeForce GTX 1070"}}, Commands: []Command{vulkanCommand}},
			},
			wantHint: true,
		},
		{
			name:   "engine not probed",
			engine: Engine{Processor: "cuda", LibPath: "/cuda"},
			backends: []Backend{
				{Processor: "cuda", Version: "b100", BinDir: "/cuda", Commands: []Command{noneCommand}},
				{Processor: "vulkan", Version: "b100", BinDir: "/vulkan", Devices: []Device{{ID: "Vulkan0", Name: "GPU"}}, Commands: []Command{vulkanCommand}},
			},
		},
		{
			name:   "selected probe failed",
			engine: Engine{Probed: true, Processor: "cuda", LibPath: "/cuda"},
			backends: []Backend{
				{Processor: "cuda", Version: "b100", BinDir: "/cuda", Commands: []Command{{Cmd: noneCommand.Cmd, Output: noneCommand.Output, Err: "exit 1"}}},
				{Processor: "vulkan", Version: "b100", BinDir: "/vulkan", Devices: []Device{{ID: "Vulkan0", Name: "GPU"}}, Commands: []Command{vulkanCommand}},
			},
		},
		{
			name:   "selected backend has a device",
			engine: Engine{Probed: true, Processor: "cuda", LibPath: "/cuda"},
			backends: []Backend{
				{Processor: "cuda", Version: "b100", BinDir: "/cuda", Devices: []Device{{ID: "CUDA0", Name: "GPU"}}, Commands: []Command{{Cmd: noneCommand.Cmd, Output: "Available devices:\n CUDA0: GPU"}}},
				{Processor: "vulkan", Version: "b100", BinDir: "/vulkan", Devices: []Device{{ID: "Vulkan0", Name: "GPU"}}, Commands: []Command{vulkanCommand}},
			},
		},
		{
			name:   "alternative version differs",
			engine: Engine{Probed: true, Processor: "cuda", LibPath: "/cuda"},
			backends: []Backend{
				{Processor: "cuda", Version: "b100", BinDir: "/cuda", Commands: []Command{noneCommand}},
				{Processor: "vulkan", Version: "b99", BinDir: "/vulkan", Devices: []Device{{ID: "Vulkan0", Name: "GPU"}}, Commands: []Command{vulkanCommand}},
			},
		},
		{
			name:   "alternative warning contains colon",
			engine: Engine{Probed: true, Processor: "cuda", LibPath: "/cuda"},
			backends: []Backend{
				{Processor: "cuda", Version: "b100", BinDir: "/cuda", Commands: []Command{noneCommand}},
				{Processor: "vulkan", Version: "b100", BinDir: "/vulkan", Devices: []Device{{ID: "Vulkan0", Name: "stale parse"}}, Commands: []Command{{Cmd: vulkanCommand.Cmd, Output: "Available devices:\nwarning: no device"}}},
			},
		},
		{
			name:   "alternative header embedded in warning",
			engine: Engine{Probed: true, Processor: "cuda", LibPath: "/cuda"},
			backends: []Backend{
				{Processor: "cuda", Version: "b100", BinDir: "/cuda", Commands: []Command{noneCommand}},
				{Processor: "vulkan", Version: "b100", BinDir: "/vulkan", Devices: []Device{{ID: "Vulkan0", Name: "stale parse"}}, Commands: []Command{{Cmd: vulkanCommand.Cmd, Output: "warning: Available devices:\nVulkan0: diagnostic text (1024 MiB, 900 MiB free)"}}},
			},
		},
		{
			name:   "device row before alternative header",
			engine: Engine{Probed: true, Processor: "cuda", LibPath: "/cuda"},
			backends: []Backend{
				{Processor: "cuda", Version: "b100", BinDir: "/cuda", Commands: []Command{noneCommand}},
				{Processor: "vulkan", Version: "b100", BinDir: "/vulkan", Devices: []Device{{ID: "Vulkan0", Name: "stale parse"}}, Commands: []Command{{Cmd: vulkanCommand.Cmd, Output: "Vulkan0: GPU (1024 MiB, 900 MiB free)\nAvailable devices:\n(none)"}}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hints := backendSelectionHints(tt.engine, tt.backends)
			if got := len(hints) > 0; got != tt.wantHint {
				t.Errorf("hint: got %v, want %v", got, tt.wantHint)
			}
			if tt.wantHint {
				if !strings.Contains(hints[0].Remedy, "KRONK_PROCESSOR=vulkan") {
					t.Errorf("remedy: got %q, want Vulkan processor override", hints[0].Remedy)
				}
			}
		})
	}
}

func TestEngineCPUFallbackHints(t *testing.T) {
	tests := []struct {
		name     string
		engine   Engine
		wantHint bool
	}{
		{
			name:     "accelerator bundle running on cpu",
			engine:   Engine{Probed: true, Loaded: true, Processor: "vulkan", Backend: "cpu"},
			wantHint: true,
		},
		{
			name:     "rocm bundle running on cpu",
			engine:   Engine{Probed: true, Loaded: true, Processor: "rocm", Backend: "cpu"},
			wantHint: true,
		},
		{
			name:   "bundle and backend agree",
			engine: Engine{Probed: true, Loaded: true, Processor: "vulkan", Backend: "vulkan"},
		},
		{
			// Empty is "unknown" (no probe result), never "cpu".
			name:   "backend unknown",
			engine: Engine{Probed: true, Loaded: true, Processor: "vulkan"},
		},
		{
			// darwin/arm64: the cpu bundle carries Metal and runs on it.
			name:   "cpu bundle running on metal",
			engine: Engine{Probed: true, Loaded: true, Processor: "cpu", Backend: "metal"},
		},
		{
			name:   "cpu bundle running on cpu",
			engine: Engine{Probed: true, Loaded: true, Processor: "cpu", Backend: "cpu"},
		},
		{
			name:   "engine did not load",
			engine: Engine{Probed: true, Processor: "vulkan", Backend: "cpu"},
		},
		{
			name:   "engine not probed",
			engine: Engine{Loaded: true, Processor: "vulkan", Backend: "cpu"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hints := engineCPUFallbackHints(tt.engine)
			if got := len(hints) > 0; got != tt.wantHint {
				t.Errorf("hint: got %v, want %v", got, tt.wantHint)
			}
			if !tt.wantHint {
				return
			}
			if !strings.Contains(hints[0].Message, tt.engine.Processor) {
				t.Errorf("message: got %q, want the %s bundle named", hints[0].Message, tt.engine.Processor)
			}
			if !strings.Contains(hints[0].Remedy, "KRONK_PROCESSOR=cpu") {
				t.Errorf("remedy: got %q, want the cpu override", hints[0].Remedy)
			}
			if hints[0].Severity != "warn" {
				t.Errorf("severity: got %q, want %q", hints[0].Severity, "warn")
			}
		})
	}
}
