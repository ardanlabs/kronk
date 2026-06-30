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
	Engine   Engine   `json:"engine" yaml:"engine"`
	Bench    Bench    `json:"bench" yaml:"bench"`
	Hints    []Hint   `json:"hints,omitempty" yaml:"hints,omitempty"`
}

// Hint is an actionable finding: a likely problem detected during collection
// along with a one-line remediation. Severity is "warn" or "fail".
type Hint struct {
	Severity string `json:"severity" yaml:"severity"`
	Message  string `json:"message" yaml:"message"`
	Remedy   string `json:"remedy,omitempty" yaml:"remedy,omitempty"`
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
// whether any llama.cpp library bundle is present; when false the binaries were
// not inspected (run with install enabled to download them). Every installed
// backend bundle (cpu, cuda, rocm, vulkan, metal) is probed independently so
// the report shows what each one actually sees — the running server may use a
// different backend than auto-detection would pick.
type Llama struct {
	Installed bool      `json:"installed" yaml:"installed"`
	Root      string    `json:"root" yaml:"root"`
	Backends  []Backend `json:"backends" yaml:"backends"`
}

// Backend holds the information reported by one installed llama.cpp library
// bundle (one processor/accelerator variant). Build and Devices are parsed
// from Commands.
type Backend struct {
	Processor string    `json:"processor" yaml:"processor"`
	Version   string    `json:"version" yaml:"version"`
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

// Engine reports whether Kronk can load the llama.cpp libraries in-process
// (the path the server uses via yzma), as opposed to merely running the
// standalone binaries as subprocesses. This is what catches failures that put
// the server in degraded mode (e.g. Windows System32 DLL shadowing) which the
// subprocess probes do not see. Probed is false when no engine probe was
// supplied or no libraries are installed; in that case Loaded/Error are unset.
type Engine struct {
	Probed    bool   `json:"probed" yaml:"probed"`
	Loaded    bool   `json:"loaded" yaml:"loaded"`
	Processor string `json:"processor" yaml:"processor"`
	LibPath   string `json:"libPath" yaml:"libPath"`
	Error     string `json:"error,omitempty" yaml:"error,omitempty"`
}

// Bench holds the llama-bench results for the selected model. Processor is the
// backend that was benchmarked.
type Bench struct {
	Processor string    `json:"processor" yaml:"processor"`
	Model     string    `json:"model" yaml:"model"`
	Commands  []Command `json:"commands,omitempty" yaml:"commands,omitempty"`
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

// EngineProbe attempts to load the inference engine in-process and returns the
// error (nil on success). It is injected by the caller — the same way the Kronk
// version is — so this package does not depend on the top-level kronk package.
type EngineProbe func() error

type options struct {
	kronkVersion string
	modelSource  string
	skipBench    bool
	install      bool
	engineProbe  EngineProbe
	processor    string
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

// WithEngineProbe supplies the in-process engine load check. When set (and
// libraries are installed) Collect runs it to report whether Kronk can actually
// load the llama.cpp libraries in-process — the real path the server uses — and
// emits a hint when it fails. The probe is injected so this package stays free
// of a dependency on the top-level kronk package.
func WithEngineProbe(probe EngineProbe) Option {
	return func(o *options) {
		o.engineProbe = probe
	}
}

// WithProcessor pins the processor (cpu, cuda, metal, vulkan) the BENCHMARK runs
// on. It does NOT affect the engine section, which always reflects the real
// server (ambient KRONK_PROCESSOR or auto-detection) so its health check is not
// distorted by a benchmark-only choice. An empty value falls back to that same
// ambient resolution. When the resolved processor is "cpu", the benchmark forces
// CPU-only execution even from a GPU library bundle, so it measures the path the
// user asked about.
func WithProcessor(processor string) Option {
	return func(o *options) {
		o.processor = processor
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

	// Validate an explicit benchmark processor override up front so a bad value
	// fails fast, even when the benchmark is skipped. An empty value is fine: it
	// resolves to the ambient KRONK_PROCESSOR or auto-detection later.
	if o.processor != "" {
		if _, err := defaults.Processor(o.processor); err != nil {
			return Report{}, fmt.Errorf("resolve processor: %w", err)
		}
	}

	r := Report{
		Versions: collectVersions(o.kronkVersion),
		System:   collectSystem(),
	}

	backends, root, err := resolveBackends(ctx, log, o.install)
	if err != nil {
		return Report{}, fmt.Errorf("resolve libraries: %w", err)
	}

	r.Llama = Llama{
		Installed: len(backends) > 0,
		Root:      root,
		Backends:  backends,
	}

	// When a GPU backend is installed but sees no device, look for a host
	// reason we can explain (e.g. render nodes not accessible).
	if gpuBackendMissingDevices(backends) {
		r.Hints = append(r.Hints, gpuAccessHints()...)
	}

	// Probe the real in-process engine load (the path the server uses). The
	// subprocess probes above only prove the standalone binaries run; this is
	// what catches the failures that put the server in degraded mode.
	if o.engineProbe != nil && len(backends) > 0 {
		r.Engine = collectEngine(o.engineProbe)
		if !r.Engine.Loaded {
			r.Hints = append(r.Hints, engineLoadHint(r.Engine.Error)...)
		}
	}

	if len(backends) > 0 && !o.skipBench {
		modelPath, ok, err := resolveModel(ctx, log, o.modelSource, o.install)
		if err != nil {
			return Report{}, fmt.Errorf("resolve model: %w", err)
		}
		if ok {
			// The benchmark honors the override; the engine above stays on the
			// real server processor, so a benchmark-only choice never distorts
			// the engine health check.
			proc, err := defaults.Processor(o.processor)
			if err != nil {
				return Report{}, fmt.Errorf("resolve processor: %w", err)
			}

			// A non-cpu request must match an installed bundle. cpu is always
			// allowed because any GPU bundle can run CPU-only via -ngl 0.
			// Without this guard benchBackend silently falls back to another
			// bundle and the report would claim a processor the user did not
			// ask for. Only fail when the user explicitly chose a processor.
			if o.processor != "" && proc.String() != "cpu" && !backendInstalled(backends, proc.String()) {
				return Report{}, fmt.Errorf("processor %q is not installed (available: %s)", proc.String(), strings.Join(installedProcessors(backends), ", "))
			}

			b := benchBackend(backends, proc.String())

			// Force CPU only when reusing a GPU bundle's binary for a cpu
			// request (e.g. cpu chosen but only a vulkan bundle is installed):
			// "-ngl 0" offloads zero layers so it runs on the CPU. When the
			// selected bundle is itself the cpu bundle, let it run its natural
			// default — important on Intel macOS, where the "cpu" bundle uses
			// Metal and forcing -ngl 0 would wrongly benchmark CPU-only.
			forceCPU := proc.String() == "cpu" && b.Processor != "cpu"

			// Report what actually ran: "cpu" when forced onto the CPU,
			// otherwise the bundle that was benchmarked.
			benchProc := b.Processor
			if forceCPU {
				benchProc = "cpu"
			}

			r.Bench = collectBench(benchProc, b.BinDir, modelPath, forceCPU)
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

func collectBench(processor, binDir, modelPath string, forceCPU bool) Bench {
	args := []string{"-m", modelPath}

	// Force CPU-only execution only when reusing a GPU bundle's binary for a
	// cpu request. llama-bench defaults to offloading to the GPU, so without
	// this a GPU bundle's binary would benchmark the GPU even though the user
	// asked for cpu. "-ngl 0" offloads zero layers, measuring the CPU path the
	// user actually runs. The cpu bundle itself is left at its natural default
	// (important on Intel macOS, where that bundle uses Metal).
	if forceCPU {
		args = append(args, "-ngl", "0")
	}

	return Bench{
		Processor: processor,
		Model:     modelPath,
		Commands: []Command{
			capture(commandSpec{bin(binDir, "llama-bench"), args}),
		},
	}
}

// collectEngine runs the injected in-process engine load probe and records the
// outcome. Processor and LibPath describe the real server backend (the ambient
// KRONK_PROCESSOR or auto-detection), so a failure here mirrors what the server
// hits at startup. It deliberately ignores any benchmark processor override so
// the health check always reflects the actual server.
func collectEngine(probe EngineProbe) Engine {
	e := Engine{
		Probed:  true,
		LibPath: libs.Path(""),
	}

	if p, err := defaults.Processor(""); err == nil {
		e.Processor = p.String()
	}

	if err := probe(); err != nil {
		e.Error = err.Error()
		return e
	}

	e.Loaded = true
	return e
}

// engineLoadHint explains an in-process engine load failure: the server can
// start but cannot load models or run inference (degraded mode). It reports
// only what is verified — the failure and the exact error — and offers no
// remedy, because at this point the libraries are installed and the underlying
// cause cannot be reliably classified from the error text. A confident fix is
// withheld rather than risk sending the user down the wrong path.
func engineLoadHint(errMsg string) []Hint {
	msg := "Kronk could not load its llama.cpp libraries in-process; the server " +
		"runs in degraded mode and cannot load models or run inference."
	if errMsg != "" {
		msg += " Error: " + errMsg
	}

	return []Hint{{
		Severity: "fail",
		Message:  msg,
	}}
}

// gpuBackendMissingDevices reports whether an installed GPU-capable backend
// (cuda, rocm, vulkan) found no device. That is the symptom worth explaining
// with a hint; a cpu or metal backend reporting no GPU is not noteworthy.
func gpuBackendMissingDevices(backends []Backend) bool {
	for _, b := range backends {
		switch b.Processor {
		case "cuda", "rocm", "vulkan":
			if len(b.Devices) == 0 {
				return true
			}
		}
	}
	return false
}

// backendInstalled reports whether an installed bundle matches the processor.
func backendInstalled(backends []Backend, processor string) bool {
	for _, b := range backends {
		if b.Processor == processor {
			return true
		}
	}
	return false
}

// installedProcessors lists the processors of every installed bundle, plus cpu
// (always benchmarkable via -ngl 0), for use in user-facing error messages.
func installedProcessors(backends []Backend) []string {
	seen := map[string]bool{"cpu": true}
	procs := []string{"cpu"}
	for _, b := range backends {
		if !seen[b.Processor] {
			seen[b.Processor] = true
			procs = append(procs, b.Processor)
		}
	}
	return procs
}

// benchBackend chooses which installed backend bundle to benchmark. It honors
// the resolved processor first so the benchmark obeys the same processor setting
// the rest of Kronk does. If no installed bundle matches that processor it falls
// back to a bundle that sees a device (a real GPU) — its binary can still run
// CPU-only when collectBench forces it — and finally the first installed bundle.
func benchBackend(backends []Backend, processor string) Backend {
	if processor != "" {
		for _, b := range backends {
			if b.Processor == processor {
				return b
			}
		}
	}

	for _, b := range backends {
		if len(b.Devices) > 0 {
			return b
		}
	}

	return backends[0]
}

// =============================================================================
// Installation

// resolveBackends discovers every installed llama.cpp library bundle for the
// running machine and probes each one. When install is true it first downloads
// the auto-detected backend if it is missing. It returns the probed backends
// and the libraries root. In inspect-only mode (install false) a missing
// install is not an error: the backend list is simply empty.
func resolveBackends(ctx context.Context, log applog.Logger, install bool) ([]Backend, string, error) {
	lib, err := libs.New(libs.WithVersion(defaults.LibVersion("")))
	if err != nil {
		return nil, "", err
	}

	if install {
		dctx, cancel := context.WithTimeout(ctx, installTimeout)
		defer cancel()

		if _, err := lib.Download(dctx, log); err != nil {
			return nil, "", fmt.Errorf("download llama.cpp: %w", err)
		}
	}

	root := lib.Root()

	tags, err := lib.List()
	if err != nil {
		return nil, root, fmt.Errorf("list installed libraries: %w", err)
	}

	var backends []Backend
	for _, tag := range tags {
		// Only probe bundles built for the running machine; binaries for
		// another OS/arch cannot be executed here.
		if tag.OS != runtime.GOOS || tag.Arch != runtime.GOARCH {
			continue
		}

		binDir := filepath.Join(root, tag.OS, tag.Arch, tag.Processor)
		cmds := llamaCommands(binDir)

		backends = append(backends, Backend{
			Processor: tag.Processor,
			Version:   tag.Version,
			BinDir:    binDir,
			Build:     parseLlamaBuild(commandOutput(cmds, "--version")),
			Devices:   parseDevices(commandOutput(cmds, "--list-devices")),
			Commands:  cmds,
		})
	}

	return backends, root, nil
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
	cmd.Output = expandTabs(string(out))
	if err != nil {
		cmd.Err = err.Error()
	}

	return cmd
}

// expandTabs replaces tab characters with spaces, honoring 8-column tab stops.
// Some commands (e.g. "sw_vers") separate their columns with tabs; the raw tabs
// render inconsistently depending on the viewer's tab width (terminal, browser
// <pre>, pasted bug report), which makes the section hard to read. Expanding the
// tabs here, at the single capture point, fixes the output for every consumer
// (CLI, BUI, and JSON/YAML) at once.
func expandTabs(s string) string {
	if !strings.ContainsRune(s, '\t') {
		return s
	}

	const tabStop = 8

	var b strings.Builder
	b.Grow(len(s) + len(s)/4)

	col := 0
	for _, r := range s {
		switch r {
		case '\t':
			n := tabStop - (col % tabStop)
			for range n {
				b.WriteByte(' ')
			}
			col += n
		case '\n':
			b.WriteByte('\n')
			col = 0
		default:
			b.WriteRune(r)
			col++
		}
	}

	return b.String()
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
	devices := []Device{}
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
