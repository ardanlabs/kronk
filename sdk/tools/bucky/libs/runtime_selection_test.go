package libs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSelectRuntime(t *testing.T) {
	tests := []struct {
		name      string
		arch      string
		opSys     string
		preferred string
		cuda      bool
		vulkan    bool
		want      string
		wantOK    bool
	}{
		{"darwin arm64 metal", "arm64", "darwin", "metal", false, false, "metal", true},
		{"darwin arm64 cpu retained", "arm64", "darwin", "cpu", false, false, "cpu", true},
		{"linux amd64 cuda", "amd64", "linux", "cuda", true, false, "cuda", true},
		{"linux cuda to vulkan", "amd64", "linux", "cuda", false, true, "vulkan", true},
		{"linux cuda to cpu", "amd64", "linux", "cuda", false, false, "cpu", true},
		{"linux rocm to vulkan", "amd64", "linux", "rocm", false, true, "vulkan", true},
		{"linux rocm to cpu", "amd64", "linux", "rocm", false, false, "cpu", true},
		{"linux vulkan retained", "amd64", "linux", "vulkan", false, true, "vulkan", true},
		{"windows vulkan", "amd64", "windows", "vulkan", false, true, "cpu", true},
		{"windows cuda", "amd64", "windows", "cuda", true, false, "cuda", true},
		{"unsupported windows arm64", "arm64", "windows", "vulkan", false, false, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			probes := fakeRuntimeProbes(tt.cuda, tt.vulkan)
			got, _, ok := selectRuntime(context.Background(), tt.arch, tt.opSys, tt.preferred, probes)
			if got != tt.want {
				t.Errorf("processor: got %q, want %q", got, tt.want)
			}
			if ok != tt.wantOK {
				t.Errorf("supported: got %v, want %v", ok, tt.wantOK)
			}
		})
	}
}

func TestCompatibleCUDAOutput(t *testing.T) {
	tests := []struct {
		name       string
		devices    string
		version    string
		visible    string
		filtered   bool
		arch       string
		opSys      string
		compatible bool
	}{
		{"linux amd64 boundary", "0, GPU-a, 8.6", "CUDA Version: 12.9", "", false, "amd64", "linux", true},
		{"linux amd64 below capability", "0, GPU-a, 8.5", "CUDA Version: 12.9", "", false, "amd64", "linux", false},
		{"linux arm64 boundary", "0, GPU-a, 8.7", "CUDA Version: 12.9", "", false, "arm64", "linux", true},
		{"linux arm64 rejects 86", "0, GPU-a, 8.6", "CUDA Version: 12.9", "", false, "arm64", "linux", false},
		{"linux old driver", "0, GPU-a, 8.9", "CUDA Version: 12.8", "", false, "amd64", "linux", false},
		{"future two digit minor", "0, GPU-a, 8.10", "CUDA Version: 12.10", "", false, "amd64", "linux", true},
		{"windows boundary", "0, GPU-a, 5.0", "CUDA Version: 12.4", "", false, "amd64", "windows", true},
		{"windows below capability", "0, GPU-a, 4.9", "CUDA Version: 12.4", "", false, "amd64", "windows", false},
		{"windows old driver", "0, GPU-a, 8.6", "CUDA Version: 12.3", "", false, "amd64", "windows", false},
		{"visible supported device", "0, GPU-a, 8.6\n1, GPU-b, 6.1", "CUDA Version: 12.9", "0", true, "amd64", "linux", true},
		{"visible unsupported device", "0, GPU-a, 8.6\n1, GPU-b, 6.1", "CUDA Version: 12.9", "1", true, "amd64", "linux", false},
		{"empty visibility", "0, GPU-a, 8.6", "CUDA Version: 12.9", "", true, "amd64", "linux", false},
		{"trailing empty visibility token", "0, GPU-a, 8.6", "CUDA Version: 12.9", "0,", true, "amd64", "linux", false},
		{"leading empty visibility token", "0, GPU-a, 8.6", "CUDA Version: 12.9", ",0", true, "amd64", "linux", false},
		{"ambiguous uuid", "0, GPU-aaaa, 8.6\n1, GPU-aaab, 8.9", "CUDA Version: 12.9", "GPU-aa", true, "amd64", "linux", false},
		{"malformed row", "broken", "CUDA Version: 12.9", "", false, "amd64", "linux", false},
		{"missing uuid", "0, , 8.6", "CUDA Version: 12.9", "", false, "amd64", "linux", false},
		{"missing capability dot", "0, GPU-a, 86", "CUDA Version: 12.9", "", false, "amd64", "linux", false},
		{"signed capability", "0, GPU-a, +8.6", "CUDA Version: 12.9", "", false, "amd64", "linux", false},
		{"nan capability", "0, GPU-a, NaN", "CUDA Version: 12.9", "", false, "amd64", "linux", false},
		{"malformed version", "0, GPU-a, 8.6", "CUDA Version: unknown", "", false, "amd64", "linux", false},
		{"suffixed version", "0, GPU-a, 8.6", "CUDA Version: 12.9junk", "", false, "amd64", "linux", false},
		{"three component version", "0, GPU-a, 8.6", "CUDA Version: 12.9.1", "", false, "amd64", "linux", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compatibleCUDAOutput(tt.devices, tt.version, tt.visible, tt.filtered, tt.arch, tt.opSys, "cuda")
			if got != tt.compatible {
				t.Errorf("compatible: got %v, want %v", got, tt.compatible)
			}
		})
	}
}

func TestNewRuntimeSelection(t *testing.T) {
	t.Run("custom path is strict", func(t *testing.T) {
		scrubRuntimeEnv(t)
		custom := filepath.Join(t.TempDir(), "custom")
		lib, err := New(
			WithLibPath(custom),
			WithOS("linux"),
			WithArch("amd64"),
			WithProcessor("cuda"),
			withRuntimeProbes(fakeRuntimeProbes(false, true)),
		)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if got := lib.LibsPath(); got != custom {
			t.Errorf("path: got %q, want %q", got, custom)
		}
		if got := lib.Processor(); got != "cuda" {
			t.Errorf("processor: got %q, want %q", got, "cuda")
		}
	})

	t.Run("explicit processor is authoritative in any order", func(t *testing.T) {
		scrubRuntimeEnv(t)
		tests := [][]Option{
			{
				WithDetect(context.Background(), nil),
				withRuntimeProbes(fakeRuntimeProbes(false, true)),
				WithProcessor("cuda"),
			},
			{
				WithProcessor("cuda"),
				withRuntimeProbes(fakeRuntimeProbes(false, true)),
				WithDetect(context.Background(), nil),
			},
		}

		for i, opts := range tests {
			opts = append(opts, WithBasePath(t.TempDir()), WithOS("linux"), WithArch("amd64"))
			lib, err := New(opts...)
			if err != nil {
				t.Fatalf("New case %d: %v", i, err)
			}
			if got := lib.Processor(); got != "cuda" {
				t.Errorf("processor case %d: got %q, want %q", i, got, "cuda")
			}
		}
	})

	t.Run("environment rocm maps to vulkan", func(t *testing.T) {
		scrubRuntimeEnv(t)
		t.Setenv("KRONK_PROCESSOR", "rocm")
		lib := newTestLib(t, "amd64", "linux", fakeRuntimeProbes(false, true))
		if got := lib.Processor(); got != "vulkan" {
			t.Errorf("processor: got %q, want %q", got, "vulkan")
		}
		if !IsSupported(lib.Arch(), lib.OS(), lib.Processor()) {
			t.Errorf("selected unsupported combination: %s/%s/%s", lib.OS(), lib.Arch(), lib.Processor())
		}
	})

	t.Run("environment rocm maps to cpu", func(t *testing.T) {
		scrubRuntimeEnv(t)
		t.Setenv("KRONK_PROCESSOR", "rocm")
		lib := newTestLib(t, "amd64", "linux", fakeRuntimeProbes(false, false))
		if got := lib.Processor(); got != "cpu" {
			t.Errorf("processor: got %q, want %q", got, "cpu")
		}
	})

	t.Run("environment cpu remains cpu", func(t *testing.T) {
		scrubRuntimeEnv(t)
		t.Setenv("KRONK_PROCESSOR", "cpu")
		lib := newTestLib(t, "amd64", "linux", fakeRuntimeProbes(false, true))
		if got := lib.Processor(); got != "cpu" {
			t.Errorf("processor: got %q, want %q", got, "cpu")
		}
	})

	t.Run("environment windows vulkan maps to cpu", func(t *testing.T) {
		scrubRuntimeEnv(t)
		t.Setenv("KRONK_PROCESSOR", "vulkan")
		lib := newTestLib(t, "amd64", "windows", fakeRuntimeProbes(false, true))
		if got := lib.Processor(); got != "cpu" {
			t.Errorf("processor: got %q, want %q", got, "cpu")
		}
	})

	t.Run("installed incompatible cuda does not pin runtime", func(t *testing.T) {
		scrubRuntimeEnv(t)
		basePath := t.TempDir()
		root := filepath.Join(basePath, localFolder)
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatalf("mkdir root: %v", err)
		}
		if err := writeVersionFile(root, defaultVersion, "amd64", "linux", "cuda"); err != nil {
			t.Fatalf("write version file: %v", err)
		}

		lib, err := New(
			WithBasePath(basePath),
			WithArch("amd64"),
			WithOS("linux"),
			withRuntimeProbes(fakeRuntimeProbes(false, false)),
		)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if got := lib.Processor(); got != "cpu" {
			t.Errorf("processor: got %q, want %q", got, "cpu")
		}
		wantPath := filepath.Join(root, "linux", "amd64", "cpu")
		if got := lib.LibsPath(); got != wantPath {
			t.Errorf("path: got %q, want %q", got, wantPath)
		}
	})

	t.Run("unsupported automatic platform returns error", func(t *testing.T) {
		scrubRuntimeEnv(t)
		t.Setenv("KRONK_PROCESSOR", "vulkan")
		_, err := New(
			WithBasePath(t.TempDir()),
			WithArch("arm64"),
			WithOS("windows"),
			withRuntimeProbes(fakeRuntimeProbes(false, false)),
		)
		if err == nil {
			t.Fatal("New: got nil error, want unsupported automatic runtime error")
		}
	})
}

func TestHasVulkanGPU(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{"discrete", "deviceType = PHYSICAL_DEVICE_TYPE_DISCRETE_GPU", true},
		{"integrated", "deviceType = PHYSICAL_DEVICE_TYPE_INTEGRATED_GPU", true},
		{"virtual", "deviceType = PHYSICAL_DEVICE_TYPE_VIRTUAL_GPU", true},
		{"cpu", "deviceType = PHYSICAL_DEVICE_TYPE_CPU", false},
		{"header only", "Vulkan Instance Version: 1.3.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasVulkanGPU(tt.output); got != tt.want {
				t.Errorf("Vulkan GPU: got %v, want %v", got, tt.want)
			}
		})
	}
}

func fakeRuntimeProbes(cuda bool, vulkan bool) runtimeProbes {
	return runtimeProbes{
		cuda:   func(_ context.Context, _, _, processor string) bool { return cuda && processor != "cuda13" },
		vulkan: func(context.Context) bool { return vulkan },
	}
}

func newTestLib(t *testing.T, arch string, opSys string, probes runtimeProbes) *Libs {
	t.Helper()

	lib, err := New(
		WithBasePath(t.TempDir()),
		WithArch(arch),
		WithOS(opSys),
		withRuntimeProbes(probes),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return lib
}

func scrubRuntimeEnv(t *testing.T) {
	t.Helper()

	for _, key := range []string{
		"KRONK_ARCH",
		"KRONK_OS",
		"KRONK_PROCESSOR",
		"KRONK_BASE_PATH",
		"KRONK_BUCKY_LIB_PATH",
		"CUDA_VISIBLE_DEVICES",
	} {
		t.Setenv(key, "")
	}
}

func TestCUDA13Selection(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		for _, tt := range []struct {
			name      string
			preferred string
			cuda12    bool
			cuda13    bool
			want      string
		}{
			{"both runtimes", "cuda", true, true, "cuda13"},
			{"only CUDA 12", "cuda", true, false, "cuda"},
			{"only CUDA 13", "cuda", false, true, "cuda13"},
			{"neither runtime", "cuda", false, false, "cpu"},
			{"CUDA 12 preference", "cuda12", true, true, "cuda12"},
			{"CUDA 13 preference", "cuda13", true, true, "cuda13"},
			{"shared CUDA 12 spelling", "cuda-12", true, true, "cuda12"},
			{"shared CUDA 13 spelling", "cuda-13", true, true, "cuda13"},
		} {
			t.Run(arch+"/"+tt.name, func(t *testing.T) {
				scrubRuntimeEnv(t)
				t.Setenv("KRONK_PROCESSOR", tt.preferred)
				probes := fakeRuntimeProbes(false, false)
				probes.cuda = func(_ context.Context, _, _, processor string) bool {
					if processor == "cuda13" {
						return tt.cuda13
					}
					return tt.cuda12
				}
				lib := newTestLib(t, arch, "linux", probes)
				if got := lib.Processor(); got != tt.want {
					t.Fatalf("processor: got %q, want %q", got, tt.want)
				}
				wantPath := filepath.Join(lib.Root(), "linux", arch, tt.want)
				if lib.LibsPath() != wantPath {
					t.Fatalf("path: got %q, want %q", lib.LibsPath(), wantPath)
				}
			})
		}
	}
	if IsSupported("amd64", "windows", "cuda13") || IsSupported("arm64", "darwin", "cuda13") {
		t.Fatal("CUDA 13 must only be advertised on Linux")
	}
}

func TestCUDA13DriverCompatibility(t *testing.T) {
	for _, tt := range []struct {
		name, arch, devices, version, visible string
		filtered, want                        bool
	}{
		{"amd64 boundary", "amd64", "0, GPU-a, 8.6", "CUDA Version: 13.0", "", false, true},
		{"old driver", "amd64", "0, GPU-a, 8.9", "CUDA Version: 12.9", "", false, false},
		{"arm64 boundary", "arm64", "0, GPU-a, 8.7", "CUDA Version: 13.0", "", false, true},
		{"arm64 below capability", "arm64", "0, GPU-a, 8.6", "CUDA Version: 13.0", "", false, false},
		{"visible incompatible GPU", "amd64", "0, GPU-a, 8.6\n1, GPU-b, 6.1", "CUDA Version: 13.0", "1", true, false},
		{"visible compatible GPU", "amd64", "0, GPU-a, 8.6\n1, GPU-b, 6.1", "CUDA Version: 13.0", "GPU-a", true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := compatibleCUDAOutput(tt.devices, tt.version, tt.visible, tt.filtered, tt.arch, "linux", "cuda13"); got != tt.want {
				t.Fatalf("compatible: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCUDALibraries(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"libcudart.so.12", "libcublas.so.12", "libcudart.so.13"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if !hasCUDALibraries("", dir, "12") {
		t.Fatal("CUDA 12 pair in LD_LIBRARY_PATH was not found")
	}
	if hasCUDALibraries("", dir, "13") {
		t.Fatal("CUDA 13 must require matching cuBLAS, not CUDA 12 cuBLAS")
	}
	cublas := filepath.Join(dir, "libcublas.so.13")
	if err := os.WriteFile(cublas, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cache := "libcublas.so.13 (libc6,aarch64) => " + cublas
	if !hasCUDALibraries(cache, dir, "13") {
		t.Fatal("CUDA 13 pair split across loader cache and search path was not found")
	}
	if err := os.Remove(cublas); err != nil {
		t.Fatal(err)
	}
	if hasCUDALibraries(cache, dir, "13") {
		t.Fatal("stale loader cache must not count as an installed library")
	}
}
