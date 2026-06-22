// Package diagnose gathers host, accelerator, library, and benchmark
// information that helps diagnose problems on a user's machine. It is the
// backend for the "kronk diagnose" CLI command and contains NO output or
// formatting code: every function returns structured data so the same report
// can be rendered by the CLI, an HTTP handler, or the Browser UI.
package diagnose

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/sdk/applog"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// defaultModelSource is the small model benchmarked when the caller does not
// specify one. It just proves the machine can run a model.
const defaultModelSource = "unsloth/Qwen3-0.6B-Q8_0"

// installTimeout bounds the library/model download steps.
const installTimeout = 15 * time.Minute

// resolveTimeout bounds the inspect-only model lookup (no download).
const resolveTimeout = 10 * time.Second

// =============================================================================
// Report model

// Report is the full diagnostic payload. All fields are plain data so it can be
// marshalled to JSON or YAML, or rendered however a frontend wants.
type Report struct {
	Versions Versions `json:"versions" yaml:"versions"`
	System   System   `json:"system" yaml:"system"`
	Llama    Llama    `json:"llama" yaml:"llama"`
	Bench    Bench    `json:"bench" yaml:"bench"`
}

// Versions holds the relevant component versions.
type Versions struct {
	Kronk string `json:"kronk" yaml:"kronk"`
	Yzma  string `json:"yzma" yaml:"yzma"`
}

// System holds host/device information gathered from OS tools. The parsed
// fields lead; Commands holds the raw command output behind them.
type System struct {
	OS       string    `json:"os" yaml:"os"`
	Arch     string    `json:"arch" yaml:"arch"`
	NumCPU   int       `json:"numCPU" yaml:"numCPU"`
	CPUModel string    `json:"cpuModel" yaml:"cpuModel"`
	RAMBytes uint64    `json:"ramBytes" yaml:"ramBytes"`
	Commands []Command `json:"commands,omitempty" yaml:"commands,omitempty"`
}

// Llama holds information reported by the llama.cpp binaries. Installed reports
// whether the llama.cpp libraries are present; when false the binaries were not
// inspected (run with install enabled to download them). Build and Devices are
// parsed from Commands.
type Llama struct {
	Installed bool      `json:"installed" yaml:"installed"`
	BinDir    string    `json:"binDir" yaml:"binDir"`
	Build     string    `json:"build" yaml:"build"`
	Devices   []Device  `json:"devices" yaml:"devices"`
	Commands  []Command `json:"commands,omitempty" yaml:"commands,omitempty"`
}

// Device is a compute device reported by "llama-bench --list-devices". VRAM
// values are in MiB, as reported by llama.cpp.
type Device struct {
	ID           string `json:"id" yaml:"id"`
	Name         string `json:"name" yaml:"name"`
	VRAMTotalMiB uint64 `json:"vramTotalMiB" yaml:"vramTotalMiB"`
	VRAMFreeMiB  uint64 `json:"vramFreeMiB" yaml:"vramFreeMiB"`
}

// Bench holds the llama-bench results for the selected model.
type Bench struct {
	Model    string    `json:"model" yaml:"model"`
	Commands []Command `json:"commands,omitempty" yaml:"commands,omitempty"`
}

// Command is the captured output of a single diagnostic command.
type Command struct {
	Cmd    string `json:"cmd" yaml:"cmd"`
	Output string `json:"output" yaml:"output"`
	Err    string `json:"err,omitempty" yaml:"err,omitempty"`
}

// =============================================================================
// Options

// Option configures a Collect call.
type Option func(*options)

type options struct {
	kronkVersion string
	modelSource  string
	skipBench    bool
	install      bool
}

func defaultOptions() options {
	return options{
		modelSource: defaultModelSource,
	}
}

// WithKronkVersion sets the Kronk version reported in the result. It is passed
// in by the caller so this package does not need to depend on the top-level
// kronk package.
func WithKronkVersion(version string) Option {
	return func(o *options) {
		o.kronkVersion = version
	}
}

// WithModelSource sets the model to benchmark. It may be a model source (e.g.
// "unsloth/Qwen3-8B-Q8_0") or a path to a local .gguf file. An empty value
// keeps the small default model.
func WithModelSource(source string) Option {
	return func(o *options) {
		if source != "" {
			o.modelSource = source
		}
	}
}

// WithSkipBench disables the llama-bench step (the slowest part).
func WithSkipBench(skip bool) Option {
	return func(o *options) {
		o.skipBench = skip
	}
}

// WithInstall allows Collect to download missing llama.cpp libraries and the
// benchmark model. When false (the default) Collect is inspect-only: it uses
// what is already installed and never downloads anything.
func WithInstall(install bool) Option {
	return func(o *options) {
		o.install = install
	}
}

// =============================================================================
// Collect

// Collect gathers a diagnostic Report. By default it is inspect-only: it uses
// the llama.cpp libraries and model already installed and never downloads. When
// the install option is enabled it downloads anything missing. It captures
// versions, system information, llama.cpp device information, and (unless
// skipped, or unavailable) a benchmark. Progress is reported through log; this
// package writes no output itself.
func Collect(ctx context.Context, log applog.Logger, opts ...Option) (Report, error) {
	o := defaultOptions()
	for _, opt := range opts {
		opt(&o)
	}

	r := Report{
		Versions: collectVersions(o.kronkVersion),
		System:   collectSystem(),
	}

	binDir, installed, err := resolveLibs(ctx, log, o.install)
	if err != nil {
		return Report{}, fmt.Errorf("resolve libraries: %w", err)
	}

	r.Llama = Llama{Installed: installed, BinDir: binDir}
	if installed {
		cmds := llamaCommands(binDir)
		r.Llama.Commands = cmds
		r.Llama.Build = parseLlamaBuild(commandOutput(cmds, "--version"))
		r.Llama.Devices = parseDevices(commandOutput(cmds, "--list-devices"))
	}

	if installed && !o.skipBench {
		modelPath, ok, err := resolveModel(ctx, log, o.modelSource, o.install)
		if err != nil {
			return Report{}, fmt.Errorf("resolve model: %w", err)
		}
		if ok {
			r.Bench = collectBench(binDir, modelPath)
		}
	}

	return r, nil
}

// =============================================================================
// Collectors

func collectVersions(kronkVersion string) Versions {
	return Versions{
		Kronk: kronkVersion,
		Yzma:  yzmaVersion(),
	}
}

func collectSystem() System {
	s := System{
		OS:     runtime.GOOS,
		Arch:   runtime.GOARCH,
		NumCPU: runtime.NumCPU(),
	}

	for _, spec := range systemCommandSpecs() {
		s.Commands = append(s.Commands, capture(spec))
	}

	s.CPUModel, s.RAMBytes = parseSystem(s.Commands)

	return s
}

func llamaCommands(binDir string) []Command {
	return []Command{
		capture(commandSpec{bin(binDir, "llama-cli"), []string{"--version"}}),
		capture(commandSpec{bin(binDir, "llama-bench"), []string{"--list-devices"}}),
	}
}

func collectBench(binDir, modelPath string) Bench {
	return Bench{
		Model: modelPath,
		Commands: []Command{
			capture(commandSpec{bin(binDir, "llama-bench"), []string{"-m", modelPath}}),
		},
	}
}

// =============================================================================
// Installation

// resolveLibs locates the installed llama.cpp binaries. When install is true it
// downloads them first. It returns the binary directory, whether the libraries
// are installed, and any error. In inspect-only mode (install false) a missing
// install is not an error: installed is simply false.
func resolveLibs(ctx context.Context, log applog.Logger, install bool) (string, bool, error) {
	lib, err := libs.New(libs.WithVersion(defaults.LibVersion("")))
	if err != nil {
		return "", false, err
	}

	if install {
		dctx, cancel := context.WithTimeout(ctx, installTimeout)
		defer cancel()

		if _, err := lib.Download(dctx, log); err != nil {
			return "", false, fmt.Errorf("download llama.cpp: %w", err)
		}
	}

	// A readable version file means the libraries are installed.
	if _, err := lib.InstalledVersion(); err != nil {
		return "", false, nil
	}

	return lib.LibsPath(), true, nil
}

// resolveModel resolves the model to benchmark. A path to an existing .gguf
// file is used directly. When install is true the source is downloaded if
// missing. In inspect-only mode the model is used only if it is already present
// on disk; nothing is downloaded. The bool reports whether a usable model was
// found.
func resolveModel(ctx context.Context, log applog.Logger, source string, install bool) (string, bool, error) {
	// A local file path: use it as-is, no download.
	if filepath.Ext(source) == ".gguf" {
		if _, err := os.Stat(source); err == nil {
			return source, true, nil
		}
	}

	mdls, err := models.New()
	if err != nil {
		return "", false, err
	}

	if install {
		dctx, cancel := context.WithTimeout(ctx, installTimeout)
		defer cancel()

		mp, err := mdls.Download(dctx, log, source)
		if err != nil {
			return "", false, fmt.Errorf("download model: %w", err)
		}
		if len(mp.ModelFiles) == 0 {
			return "", false, fmt.Errorf("no model files for %q", source)
		}
		return mp.ModelFiles[0], true, nil
	}

	// Inspect-only: use the model only if it is already on disk.
	rctx, cancel := context.WithTimeout(ctx, resolveTimeout)
	defer cancel()

	res, err := mdls.ResolveSource(rctx, source)
	if err != nil || len(res.LocalPaths) == 0 || res.VerifyLocal() != nil {
		return "", false, nil
	}

	return res.LocalPaths[0], true, nil
}

// =============================================================================
// Helpers

// commandSpec describes a command to run.
type commandSpec struct {
	name string
	args []string
}

// capture runs a command and returns its combined output as a Command.
func capture(spec commandSpec) Command {
	cmd := Command{Cmd: strings.TrimSpace(spec.name + " " + strings.Join(spec.args, " "))}

	out, err := exec.Command(spec.name, spec.args...).CombinedOutput()
	cmd.Output = string(out)
	if err != nil {
		cmd.Err = err.Error()
	}

	return cmd
}

// bin returns the path to a llama binary, adding .exe on Windows.
func bin(binDir, name string) string {
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(binDir, name)
}

// commandOutput returns the output of the first captured Command whose command
// line contains match. It returns "" when no command matches.
func commandOutput(cmds []Command, match string) string {
	for _, c := range cmds {
		if strings.Contains(c.Cmd, match) {
			return c.Output
		}
	}
	return ""
}

// parseLlamaBuild extracts the build identifier from "llama-cli --version"
// output, e.g. "9748 (bfa321917)" from "version: 9748 (bfa321917)".
func parseLlamaBuild(out string) string {
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, "version:"); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// deviceLine matches a device row from "llama-bench --list-devices", e.g.
// "  MTL0: Apple M3 Max (110100 MiB, 110100 MiB free)".
var deviceLine = regexp.MustCompile(`^\s*(\S+):\s+(.+?)\s+\((\d+)\s*MiB,\s*(\d+)\s*MiB free\)`)

// parseDevices extracts the device list from "llama-bench --list-devices"
// output. VRAM values are reported in MiB.
func parseDevices(out string) []Device {
	var devices []Device
	for line := range strings.SplitSeq(out, "\n") {
		m := deviceLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}

		total, _ := strconv.ParseUint(m[3], 10, 64)
		free, _ := strconv.ParseUint(m[4], 10, 64)
		devices = append(devices, Device{
			ID:           m[1],
			Name:         strings.TrimSpace(m[2]),
			VRAMTotalMiB: total,
			VRAMFreeMiB:  free,
		})
	}
	return devices
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
