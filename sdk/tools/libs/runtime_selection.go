package libs

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/hybridgroup/yzma/pkg/download"
)

const (
	runtimeProbeTimeout = 10 * time.Second
	hostProbeTimeout    = 2 * time.Second

	minimumCUDA13ComputeCapability = 7.5
)

// RuntimeSelection describes an installed-runtime selection decision.
type RuntimeSelection struct {
	PreferredProcessor string
	SelectedProcessor  string
	Reason             string
}

type detectOptions struct {
	ctx       context.Context
	log       Logger
	libPath   string
	arch      string
	opSys     string
	processor string
}

// WithDetect supplies the context and logger used by automatic host runtime
// detection.
func WithDetect(ctx context.Context, log Logger) Option {
	return WithDetectOverrides(ctx, log, "", "", "", "")
}

// WithDetectOverrides supplies the context and logger used by automatic host
// runtime detection plus optional library path, architecture, operating
// system, and processor overrides.
// WithLibPath, WithArch, WithOS, and WithProcessor always take precedence,
// regardless of option order.
func WithDetectOverrides(ctx context.Context, log Logger, libPath string, arch string, opSys string, processor string) Option {
	return func(o *Options) {
		o.detect = &detectOptions{
			ctx:       ctx,
			log:       log,
			libPath:   libPath,
			arch:      arch,
			opSys:     opSys,
			processor: processor,
		}
	}
}

func applyDetectOverrides(options *Options) error {
	if options.detect == nil {
		return nil
	}

	if options.LibPath == "" && options.detect.libPath != "" {
		options.LibPath = options.detect.libPath
	}

	if options.Arch.String() == "" && options.detect.arch != "" {
		arch, err := download.ParseArch(options.detect.arch)
		if err != nil {
			return fmt.Errorf("detect: parse architecture: %w", err)
		}
		options.Arch = arch
	}

	if options.OS.String() == "" && options.detect.opSys != "" {
		opSys, err := download.ParseOS(options.detect.opSys)
		if err != nil {
			return fmt.Errorf("detect: parse operating system: %w", err)
		}
		options.OS = opSys
	}

	if options.Processor.String() == "" && options.detect.processor != "" {
		processor, err := download.ParseProcessor(options.detect.processor)
		if err != nil {
			return fmt.Errorf("detect: parse processor: %w", err)
		}
		options.Processor = processor
	}

	return nil
}

type runtimeProbeState uint8

const (
	runtimeProbeUnknown runtimeProbeState = iota
	runtimeProbeNone
	runtimeProbeDevices
)

type runtimeCandidate struct {
	processor download.Processor
	path      string
	version   string
}

type runtimeProbe struct {
	candidate runtimeCandidate
	state     runtimeProbeState
	devices   []string
	output    string
	err       error
}

type hostCUDAState uint8

const (
	hostCUDAUnknown hostCUDAState = iota
	hostCUDASupported
	hostCUDAUnsupported
	hostCUDAUnavailable
)

type hostCUDAProbe struct {
	state        hostCUDAState
	capabilities []string
}

type hostCUDADevice struct {
	index      string
	uuid       string
	capability string
}

func (lib *Libs) selectHostRuntime(
	ctx context.Context,
	log Logger,
	cudaProbeFn func(context.Context) hostCUDAProbe,
	fallbackFn func(context.Context) download.Processor,
) (*Libs, RuntimeSelection) {
	selection := RuntimeSelection{
		PreferredProcessor: lib.Processor(),
		SelectedProcessor:  lib.Processor(),
		Reason:             "preferred host runtime retained",
	}

	if lib.Processor() != "cuda" || lib.readOnly {
		return lib, selection
	}

	probe := cudaProbeFn(ctx)
	if probe.state != hostCUDAUnsupported && probe.state != hostCUDAUnavailable {
		if probe.state == hostCUDASupported {
			selection.Reason = "NVIDIA hardware supports the CUDA 13 libraries"
		} else {
			selection.Reason = "NVIDIA hardware detected; CUDA 13 compatibility could not be verified"
		}
		return lib, selection
	}

	processor := fallbackFn(ctx)
	selected := *lib
	selected.processor = processor
	selected.path = installPathFor(lib.root, lib.arch, lib.os, processor)
	selected.readOnly = false

	selection.SelectedProcessor = processor.String()
	status := "unsupported"
	if probe.state == hostCUDAUnavailable {
		status = "unavailable"
		selection.Reason = fmt.Sprintf("NVIDIA hardware detected but no CUDA devices are visible; selected %s", processor)
	} else {
		selection.Reason = fmt.Sprintf(
			"NVIDIA hardware compute capability %s is unsupported by the CUDA 13 libraries (requires 7.5 or newer); selected %s",
			strings.Join(probe.capabilities, ","),
			processor,
		)
	}

	if log != nil {
		log(
			ctx,
			"host accelerator support",
			"hardware", "nvidia",
			"processor", "cuda",
			"status", status,
			"computeCapabilities", strings.Join(probe.capabilities, ","),
			"minimumComputeCapability", minimumCUDA13ComputeCapability,
			"selected", processor,
		)
	}

	return &selected, selection
}

func probeCUDA13Host(ctx context.Context) hostCUDAProbe {
	pctx, cancel := context.WithTimeout(ctx, hostProbeTimeout)
	defer cancel()

	out, err := exec.CommandContext(
		pctx,
		"nvidia-smi",
		"--query-gpu=index,uuid,compute_cap",
		"--format=csv,noheader,nounits",
	).Output()
	if err != nil {
		return hostCUDAProbe{}
	}

	visible, filter := os.LookupEnv("CUDA_VISIBLE_DEVICES")
	return parseCUDA13Host(string(out), visible, filter)
}

func parseCUDA13Host(output string, visible string, filter bool) hostCUDAProbe {
	if filter && strings.Contains(visible, "MIG-") {
		return hostCUDAProbe{}
	}

	var devices []hostCUDADevice
	invalid := false
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Split(line, ",")
		if len(fields) != 3 {
			invalid = true
			continue
		}
		for i := range fields {
			fields[i] = strings.TrimSpace(fields[i])
		}
		devices = append(devices, hostCUDADevice{
			index:      fields[0],
			uuid:       fields[1],
			capability: fields[2],
		})
	}

	if filter {
		devices = visibleCUDADevices(devices, visible)
	}

	probe := hostCUDAProbe{state: hostCUDASupported}
	for _, device := range devices {
		probe.capabilities = append(probe.capabilities, device.capability)
		capability, err := strconv.ParseFloat(device.capability, 64)
		if err != nil {
			invalid = true
			continue
		}

		if capability < minimumCUDA13ComputeCapability {
			probe.state = hostCUDAUnsupported
		}
	}

	if probe.state == hostCUDAUnsupported {
		return probe
	}
	if invalid {
		probe.state = hostCUDAUnknown
	} else if len(probe.capabilities) == 0 {
		if filter {
			probe.state = hostCUDAUnavailable
		} else {
			probe.state = hostCUDAUnknown
		}
	}

	return probe
}

func visibleCUDADevices(devices []hostCUDADevice, visible string) []hostCUDADevice {
	if visible == "all" {
		return devices
	}

	var selected []hostCUDADevice
	for token := range strings.SplitSeq(visible, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			break
		}

		var matches []hostCUDADevice
		for _, device := range devices {
			if token == device.index || strings.HasPrefix(device.uuid, token) {
				matches = append(matches, device)
			}
		}
		if len(matches) != 1 {
			break
		}
		selected = append(selected, matches[0])
	}

	return selected
}

func (lib *Libs) detectHostFallback(ctx context.Context) download.Processor {
	if IsSupported(lib.Arch(), lib.OS(), "rocm") && hasROCmHostSupport(ctx) {
		return download.ROCm
	}

	if IsSupported(lib.Arch(), lib.OS(), "vulkan") && hasVulkanHostSupport(ctx) {
		return download.Vulkan
	}

	return download.CPU
}

func hasROCmHostSupport(ctx context.Context) bool {
	pctx, cancel := context.WithTimeout(ctx, hostProbeTimeout)
	defer cancel()

	out, err := exec.CommandContext(pctx, "rocminfo").CombinedOutput()
	return err == nil && hasROCmGPU(string(out))
}

func hasVulkanHostSupport(ctx context.Context) bool {
	pctx, cancel := context.WithTimeout(ctx, hostProbeTimeout)
	defer cancel()

	out, err := exec.CommandContext(pctx, "vulkaninfo", "--summary").CombinedOutput()
	return err == nil && hasVulkanGPU(string(out))
}

func hasROCmGPU(output string) bool {
	for block := range strings.SplitSeq(output, "Agent ") {
		gpu := false
		dispatch := false
		for line := range strings.SplitSeq(block, "\n") {
			line = strings.Join(strings.Fields(line), " ")
			gpu = gpu || line == "Device Type: GPU"
			dispatch = dispatch ||
				(strings.HasPrefix(line, "Feature:") || strings.HasPrefix(line, "Features:")) &&
					strings.Contains(line, "KERNEL_DISPATCH")
		}
		if gpu && dispatch {
			return true
		}
	}

	return false
}

func hasVulkanGPU(output string) bool {
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.Join(strings.Fields(line), " ")
		if strings.HasSuffix(line, "PHYSICAL_DEVICE_TYPE_DISCRETE_GPU") ||
			strings.HasSuffix(line, "PHYSICAL_DEVICE_TYPE_INTEGRATED_GPU") ||
			strings.HasSuffix(line, "PHYSICAL_DEVICE_TYPE_VIRTUAL_GPU") {
			return true
		}
	}

	return false
}

// SelectInstalledRuntime selects an installed accelerator bundle before any
// native libraries are loaded into the process. The receiver remains selected
// when its probe detects a device or is inconclusive. An alternative is chosen
// only when the receiver explicitly reports no devices and a same-version
// installed bundle positively reports at least one device.
//
// Selection never downloads libraries and never mutates the receiver.
func (lib *Libs) SelectInstalledRuntime(ctx context.Context, log Logger) (*Libs, RuntimeSelection, error) {
	return lib.selectInstalledRuntime(ctx, log, probeRuntime)
}

func (lib *Libs) selectInstalledRuntime(ctx context.Context, log Logger, probeFn func(context.Context, runtimeCandidate) runtimeProbe) (*Libs, RuntimeSelection, error) {
	selection := RuntimeSelection{
		PreferredProcessor: lib.Processor(),
		SelectedProcessor:  lib.Processor(),
		Reason:             "preferred runtime retained",
	}

	if lib.hostDemoted {
		selection.Reason = "host compatibility fallback retained"
		return lib, selection, nil
	}

	if !isAcceleratorProcessor(lib.Processor()) || lib.readOnly {
		return lib, selection, nil
	}

	installed, err := lib.InstalledVersion()
	if err != nil {
		selection.Reason = "preferred runtime is not installed"
		return lib, selection, nil
	}

	preferred := runtimeCandidate{
		processor: lib.processor,
		path:      lib.path,
		version:   installed.Version,
	}
	preferredProbe := probeFn(ctx, preferred)
	logRuntimeProbe(ctx, log, preferredProbe)

	switch preferredProbe.state {
	case runtimeProbeDevices:
		selection.Reason = "preferred runtime detected an accelerator"
		return lib, selection, nil
	case runtimeProbeUnknown:
		selection.Reason = "preferred runtime probe was inconclusive"
		return lib, selection, nil
	}

	candidates, err := lib.runtimeCandidates(installed.Version)
	if err != nil {
		return nil, RuntimeSelection{}, fmt.Errorf("select-installed-runtime: list candidates: %w", err)
	}

	probes := make([]runtimeProbe, 0, len(candidates))
	for _, candidate := range candidates {
		probe := probeFn(ctx, candidate)
		logRuntimeProbe(ctx, log, probe)
		probes = append(probes, probe)
	}

	selected, ok := chooseRuntime(probes)
	if !ok {
		selection.Reason = "preferred runtime detected no accelerator and no usable installed alternative was found"
		return lib, selection, nil
	}

	selectedLib := *lib
	selectedLib.path = selected.path
	selectedLib.processor = selected.processor
	selectedLib.readOnly = false

	selection.SelectedProcessor = selected.processor.String()
	selection.Reason = fmt.Sprintf("preferred %s runtime detected no accelerator; selected installed %s runtime", lib.Processor(), selected.processor.String())

	return &selectedLib, selection, nil
}

func (lib *Libs) runtimeCandidates(version string) ([]runtimeCandidate, error) {
	tags, err := lib.List()
	if err != nil {
		return nil, err
	}

	var candidates []runtimeCandidate
	for _, tag := range tags {
		if tag.Arch != lib.Arch() || tag.OS != lib.OS() || tag.Version != version || tag.Processor == lib.Processor() {
			continue
		}
		if !isAcceleratorProcessor(tag.Processor) {
			continue
		}

		processor, err := download.ParseProcessor(tag.Processor)
		if err != nil {
			continue
		}

		candidates = append(candidates, runtimeCandidate{
			processor: processor,
			path:      installPathFor(lib.root, lib.arch, lib.os, processor),
			version:   tag.Version,
		})
	}

	slices.SortFunc(candidates, func(a, b runtimeCandidate) int {
		return runtimeProcessorRank(a.processor.String()) - runtimeProcessorRank(b.processor.String())
	})

	return candidates, nil
}

func probeRuntime(ctx context.Context, candidate runtimeCandidate) runtimeProbe {
	probe := runtimeProbe{candidate: candidate}

	pctx, cancel := context.WithTimeout(ctx, runtimeProbeTimeout)
	defer cancel()

	name := "llama-bench"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}

	cmd := exec.CommandContext(pctx, filepath.Join(candidate.path, name), "--list-devices")
	cmd.Env = runtimeProbeEnv(candidate.path)

	out, err := cmd.CombinedOutput()
	probe.output = string(out)
	if err != nil {
		probe.err = err
		return probe
	}

	probe.state, probe.devices = parseRuntimeDevices(probe.output)
	return probe
}

var runtimeDeviceLine = regexp.MustCompile(`^\s*(\S+):\s+.+\s+\(\d+\s*MiB,\s*\d+\s*MiB free\)\s*$`)

func parseRuntimeDevices(output string) (runtimeProbeState, []string) {
	const header = "Available devices:"

	body, ok := runtimeDeviceBody(output, header)
	if !ok {
		return runtimeProbeUnknown, nil
	}

	var devices []string
	var lines int
	for line := range strings.SplitSeq(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines++
		if line == "(none)" && lines == 1 && strings.TrimSpace(body) == "(none)" {
			return runtimeProbeNone, nil
		}

		match := runtimeDeviceLine.FindStringSubmatch(line)
		if match == nil {
			return runtimeProbeUnknown, nil
		}
		devices = append(devices, match[1])
	}

	if len(devices) == 0 {
		return runtimeProbeUnknown, nil
	}
	return runtimeProbeDevices, devices
}

func runtimeDeviceBody(output string, header string) (string, bool) {
	lines := strings.Split(output, "\n")
	headerLine := -1
	for i, line := range lines {
		if strings.TrimSuffix(line, "\r") == header {
			headerLine = i
		}
	}
	if headerLine < 0 {
		return "", false
	}
	return strings.Join(lines[headerLine+1:], "\n"), true
}

func chooseRuntime(probes []runtimeProbe) (runtimeCandidate, bool) {
	for _, probe := range probes {
		if probe.state == runtimeProbeDevices {
			return probe.candidate, true
		}
	}
	return runtimeCandidate{}, false
}

func runtimeProbeEnv(path string) []string {
	env := prependRuntimePath(os.Environ(), "PATH", path)

	switch runtime.GOOS {
	case "darwin":
		return prependRuntimePath(env, "DYLD_LIBRARY_PATH", path)
	case "linux":
		return prependRuntimePath(env, "LD_LIBRARY_PATH", path)
	default:
		return env
	}
}

func prependRuntimePath(env []string, key string, path string) []string {
	prefix := key + "="
	for i, entry := range env {
		if value, ok := strings.CutPrefix(entry, prefix); ok {
			env[i] = prefix + path + string(os.PathListSeparator) + value
			return env
		}
	}
	return append(env, prefix+path)
}

func logRuntimeProbe(ctx context.Context, log Logger, probe runtimeProbe) {
	if log == nil {
		return
	}

	state := "unknown"
	switch probe.state {
	case runtimeProbeNone:
		state = "none"
	case runtimeProbeDevices:
		state = "devices"
	}

	args := []any{"processor", probe.candidate.processor, "state", state}
	if len(probe.devices) > 0 {
		args = append(args, "devices", strings.Join(probe.devices, ","))
	}
	if probe.err != nil {
		args = append(args, "error", probe.err)
	}
	log(ctx, "probe installed runtime", args...)
}

func isAcceleratorProcessor(processor string) bool {
	switch processor {
	case "cuda", "metal", "rocm", "vulkan":
		return true
	default:
		return false
	}
}

func runtimeProcessorRank(processor string) int {
	switch processor {
	case "cuda":
		return 0
	case "rocm":
		return 1
	case "metal":
		return 2
	case "vulkan":
		return 3
	default:
		return 4
	}
}
