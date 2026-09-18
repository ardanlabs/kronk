//go:build openvino_integration

package diagnose

import (
	"context"
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk"
)

func TestOpenVINOInference(t *testing.T) {
	if runtime.GOARCH != "amd64" || (runtime.GOOS != "linux" && runtime.GOOS != "windows") {
		t.Fatalf("OpenVINO integration requires linux/amd64 or windows/amd64; got %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	device := os.Getenv("GGML_OPENVINO_DEVICE")
	if device == "" {
		device = "CPU"
		t.Setenv("GGML_OPENVINO_DEVICE", device)
	}
	t.Setenv("KRONK_PROCESSOR", "openvino")
	t.Logf("OpenVINO target device: %s", device)

	report, err := Collect(
		t.Context(),
		func(_ context.Context, msg string, args ...any) {
			t.Log(append([]any{msg}, args...)...)
		},
		WithKronkVersion(kronk.Version),
		WithInstall(true),
		WithProcessor("openvino"),
		WithEngineProbe(func() error {
			return kronk.Init()
		}),
	)
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	backendIdx := slices.IndexFunc(report.Llama.Backends, func(backend Backend) bool {
		return backend.Processor == "openvino"
	})
	if backendIdx < 0 {
		t.Fatalf("installed backends = %v, want openvino", installedProcessors(report.Llama.Backends))
	}
	backend := report.Llama.Backends[backendIdx]
	assertCommandsSucceeded(t, "OpenVINO backend probe", backend.Commands)
	assertOpenVINODidNotFallback(t, backend.Commands)
	if !slices.ContainsFunc(backend.Devices, func(device Device) bool {
		return strings.HasPrefix(strings.ToUpper(device.ID), "OPENVINO")
	}) {
		t.Fatalf("OpenVINO devices = %+v, want OPENVINO device", backend.Devices)
	}

	if !report.Engine.Probed || !report.Engine.Loaded {
		t.Fatalf("engine = %+v, want loaded OpenVINO bundle", report.Engine)
	}
	if report.Engine.Processor != "openvino" {
		t.Errorf("engine processor = %q, want openvino", report.Engine.Processor)
	}

	if report.Bench.Processor != "openvino" || report.Bench.Model == "" {
		t.Fatalf("benchmark = %+v, want OpenVINO model benchmark", report.Bench)
	}
	assertCommandsSucceeded(t, "OpenVINO benchmark", report.Bench.Commands)
	assertOpenVINODidNotFallback(t, report.Bench.Commands)
}

func assertCommandsSucceeded(t *testing.T, name string, commands []Command) {
	t.Helper()

	if len(commands) == 0 {
		t.Fatalf("%s commands are empty", name)
	}
	for _, command := range commands {
		t.Logf("%s: %s\n%s", name, command.Cmd, command.Output)
		if command.Err != "" {
			t.Errorf("%s command %q error = %s", name, command.Cmd, command.Err)
		}
	}
}

func assertOpenVINODidNotFallback(t *testing.T, commands []Command) {
	t.Helper()

	for _, command := range commands {
		if strings.Contains(command.Output, "is not available, fallback to CPU") {
			t.Fatalf("OpenVINO silently fell back to CPU:\n%s", command.Output)
		}
	}
}
