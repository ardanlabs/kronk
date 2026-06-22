// This example is a diagnostic ("hack") tool. The goal right now is just to
// discover and PRINT all the information we can get access to that might help
// us debug a problem on a user's machine. Once we can see everything, we'll
// figure out the engineering (what to keep, how to structure it, etc.).
//
// The first time you run this program the system will download and install
// the llama.cpp libraries and a small benchmark model.
//
// Run the example like this from the root of the project:
// $ go run ./examples/diagnose
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// modelSource is a small model used for the llama-bench run.
const modelSource = "unsloth/Qwen3-0.6B-Q8_0"

func main() {
	if err := run(); err != nil {
		fmt.Printf("\nERROR: %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Make sure llama.cpp is installed and find where the binaries live.
	binDir, err := installLibraries()
	if err != nil {
		return fmt.Errorf("unable to install libraries: %w", err)
	}

	// Figure out which model to benchmark. The real diagnostic value is
	// benchmarking the user's ACTUAL model (their problem is usually their
	// specific large model + context), so allow passing a model source or a
	// path to a .gguf file. Falls back to a tiny model that just proves the
	// machine works.
	//
	//   go run ./examples/diagnose                          # tiny default model
	//   go run ./examples/diagnose unsloth/Qwen3-8B-Q8_0    # pull + bench a model
	//   go run ./examples/diagnose /path/to/model.gguf      # bench a local file
	source := modelSource
	if len(os.Args) > 1 {
		source = os.Args[1]
	}

	modelPath, err := installModel(source)
	if err != nil {
		return fmt.Errorf("unable to install model: %w", err)
	}

	versionInfo()
	systemInfo()
	llamaInfo(binDir)
	benchInfo(binDir, modelPath)

	return nil
}

// installLibraries makes sure llama.cpp is installed and returns the directory
// that holds the installed binaries (llama-bench, llama-cli, etc.).
func installLibraries() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	lib, err := libs.New(libs.WithVersion(defaults.LibVersion("")))
	if err != nil {
		return "", err
	}

	if _, err := lib.Download(ctx, kronk.FmtLogger); err != nil {
		return "", fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	return lib.LibsPath(), nil
}

// installModel resolves the model to benchmark. If source is a path to an
// existing .gguf file it's used directly; otherwise it's treated as a model
// source to download. Returns the model file path.
func installModel(source string) (string, error) {
	// A local file path: use it as-is, no download.
	if filepath.Ext(source) == ".gguf" {
		if _, err := os.Stat(source); err == nil {
			return source, nil
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	mdls, err := models.New()
	if err != nil {
		return "", err
	}

	mp, err := mdls.Download(ctx, kronk.FmtLogger, source)
	if err != nil {
		return "", fmt.Errorf("unable to download model: %w", err)
	}

	if len(mp.ModelFiles) == 0 {
		return "", fmt.Errorf("no model files for %q", source)
	}

	return mp.ModelFiles[0], nil
}

// =============================================================================

// systemInfo prints "About This Mac"-style information about the host. The
// goal is to capture EVERYTHING we can about the machine and its devices, so
// we can later decide what's actually useful. The actual commands depend on
// the OS, so we dispatch by GOOS.
func systemInfo() {
	fmt.Println("\n========== SYSTEM INFO ==========")

	fmt.Println("- goOS    :", runtime.GOOS)
	fmt.Println("- goArch  :", runtime.GOARCH)
	fmt.Println("- numCPU  :", runtime.NumCPU())

	switch runtime.GOOS {
	case "darwin":
		systemInfoDarwin()
	case "linux":
		systemInfoLinux()
	case "windows":
		systemInfoWindows()
	default:
		fmt.Println("(no system info collector for", runtime.GOOS, ")")
	}
}

// systemInfoDarwin captures host/device info on macOS.
func systemInfoDarwin() {
	// OS version + kernel.
	dump("sw_vers")
	dump("uname", "-a")

	// CPU / memory / hardware from the kernel.
	dump("sysctl", "-n", "machdep.cpu.brand_string", "hw.memsize", "hw.logicalcpu", "hw.physicalcpu")

	// Memory pressure / usage right now. Swap usage and free disk are the two
	// most common real problems: a model bigger than RAM swaps (and feels like
	// a Kronk bug), and a full disk makes model downloads fail.
	dump("vm_stat")
	dump("sysctl", "vm.swapusage")
	dump("df", "-h")

	// Power / thermal state (no sudo needed). These reveal whether the
	// machine is throttled, on battery, or in Low Power Mode -- common
	// reasons a machine "feels slow" that have nothing to do with Kronk.
	dump("pmset", "-g", "therm") // CPU_Speed_Limit < 100 => throttling
	dump("pmset", "-g", "batt")  // AC vs battery, charge %
	dump("pmset", "-g")          // power settings incl. lowpowermode
}

// systemInfoLinux captures host/device info on Linux, including discrete GPUs.
func systemInfoLinux() {
	// OS version + kernel.
	dump("uname", "-a")
	dump("cat", "/etc/os-release")

	// CPU / memory.
	dump("lscpu")
	dump("cat", "/proc/meminfo")
	dump("free", "-h")

	// Swap + free disk (same "model bigger than RAM / disk full" failures).
	dump("swapon", "--show")
	dump("df", "-h")

	// GPU cards. These only exist when the matching drivers are installed;
	// dump() just records an error otherwise. This is the important one for
	// the "Linux with GPU cards" target.
	dump("nvidia-smi") // NVIDIA
	dump("rocm-smi")   // AMD
}

// systemInfoWindows captures host/device info on Windows via PowerShell.
func systemInfoWindows() {
	// OS + CPU + memory overview.
	dump("systeminfo")

	// Structured CPU / OS / RAM / GPU via CIM (PowerShell).
	ps := func(expr string) { dump("powershell", "-NoProfile", "-Command", expr) }
	ps("Get-CimInstance Win32_Processor | Format-List Name,NumberOfCores,NumberOfLogicalProcessors")
	ps("Get-CimInstance Win32_OperatingSystem | Format-List Caption,Version,BuildNumber,TotalVisibleMemorySize,FreePhysicalMemory")
	ps("Get-CimInstance Win32_ComputerSystem | Format-List Manufacturer,Model,TotalPhysicalMemory")
	ps("Get-CimInstance Win32_VideoController | Format-List Name,AdapterRAM,DriverVersion")

	// GPU cards (NVIDIA), if drivers/tooling are present.
	dump("nvidia-smi")
}

// versionInfo prints the Kronk and yzma versions.
func versionInfo() {
	fmt.Println("\n========== VERSIONS ==========")

	fmt.Println("- kronk   :", kronk.Version)
	fmt.Println("- yzma    :", yzmaVersion())
}

// yzmaVersion reads the yzma dependency version from the build info.
func yzmaVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}

	for _, dep := range info.Deps {
		if dep.Path == "github.com/hybridgroup/yzma" {
			return dep.Version
		}
	}

	return "unknown"
}

// llamaInfo prints version and device information from the llama.cpp binaries.
func llamaInfo(binDir string) {
	fmt.Println("\n========== LLAMA.CPP INFO ==========")
	fmt.Println("- binDir  :", binDir)

	dump(bin(binDir, "llama-cli"), "--version")
	dump(bin(binDir, "llama-bench"), "--list-devices")
}

// benchInfo runs llama-bench against the model to capture real single-stream
// throughput (prompt-processing and token-generation tok/s) on this machine.
func benchInfo(binDir, modelPath string) {
	fmt.Println("\n========== LLAMA-BENCH ==========")
	fmt.Println("- model   :", modelPath)

	dump(bin(binDir, "llama-bench"), "-m", modelPath)
}

// =============================================================================

// bin returns the path to a llama binary, adding .exe on Windows.
func bin(binDir, name string) string {
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(binDir, name)
}

// dump runs a command and prints whatever it writes to stdout and stderr.
func dump(name string, args ...string) {
	fmt.Printf("\n$ %s %v\n", name, args)

	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout
	if err := cmd.Run(); err != nil {
		fmt.Printf("(error running %s: %s)\n", name, err)
	}
}
