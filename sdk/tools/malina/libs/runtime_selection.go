package libs

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const hostProbeTimeout = 2 * time.Second

type detectOptions struct {
	ctx    context.Context
	log    Logger
	probes *runtimeProbes
}

type runtimeProbes struct {
	cuda   func(context.Context, string, string) bool
	vulkan func(context.Context) bool
}

// WithDetect supplies the context and logger used by automatic host runtime
// detection.
func WithDetect(ctx context.Context, log Logger) Option {
	return func(o *Options) {
		if o.detect == nil {
			o.detect = &detectOptions{}
		}
		o.detect.ctx = ctx
		o.detect.log = log
	}
}

func defaultRuntimeProbes() runtimeProbes {
	return runtimeProbes{cuda: hasCompatibleCUDA, vulkan: hasVulkanHostSupport}
}

func selectRuntime(ctx context.Context, arch string, opSys string, preferred string, probes runtimeProbes) (string, string, bool) {
	switch preferred {
	case "cpu":
		if IsSupported(arch, opSys, preferred) {
			return preferred, "preferred CPU runtime retained", true
		}

	case "metal":
		if IsSupported(arch, opSys, preferred) {
			return preferred, "preferred Metal runtime retained", true
		}

	case "cuda":
		if IsSupported(arch, opSys, preferred) && probes.cuda(ctx, arch, opSys) {
			return preferred, "CUDA host and driver are compatible", true
		}

	case "rocm":
		if IsSupported(arch, opSys, preferred) {
			return preferred, "preferred ROCm runtime retained", true
		}

	case "vulkan":
		if IsSupported(arch, opSys, preferred) && probes.vulkan(ctx) {
			return preferred, "preferred Vulkan runtime is usable", true
		}
	}

	if opSys == "linux" && IsSupported(arch, opSys, "vulkan") && probes.vulkan(ctx) {
		return "vulkan", "preferred runtime is incompatible; Vulkan GPU is usable", true
	}

	if IsSupported(arch, opSys, "cpu") {
		return "cpu", "preferred runtime is incompatible; using CPU", true
	}

	return "", "no compatible Malina bundle is published for this platform", false
}

type majorMinor struct {
	major int
	minor int
}

func (v majorMinor) atLeast(minimum majorMinor) bool {
	return v.major > minimum.major || v.major == minimum.major && v.minor >= minimum.minor
}

type cudaDevice struct {
	index      string
	uuid       string
	capability majorMinor
}

var cudaVersionRE = regexp.MustCompile(`CUDA Version:\s*([^[:space:]|]+)`)

func hasCompatibleCUDA(ctx context.Context, arch string, opSys string) bool {
	pctx, cancel := context.WithTimeout(ctx, hostProbeTimeout)
	defer cancel()

	devicesOut, err := exec.CommandContext(pctx, "nvidia-smi", "--query-gpu=index,uuid,compute_cap", "--format=csv,noheader,nounits").Output()
	if err != nil {
		return false
	}
	versionOut, err := exec.CommandContext(pctx, "nvidia-smi").Output()
	if err != nil {
		return false
	}

	visible, filtered := os.LookupEnv("CUDA_VISIBLE_DEVICES")
	return compatibleCUDAOutput(string(devicesOut), string(versionOut), visible, filtered, arch, opSys)
}

func compatibleCUDAOutput(devicesOutput string, versionOutput string, visible string, filtered bool, arch string, opSys string) bool {
	match := cudaVersionRE.FindStringSubmatch(versionOutput)
	if len(match) != 2 {
		return false
	}
	version, err := parseMajorMinor(match[1])
	if err != nil {
		return false
	}
	minimumVersion := majorMinor{major: 12, minor: 9}
	minimumCapability := majorMinor{major: 8, minor: 6}
	if opSys == "windows" {
		minimumVersion = majorMinor{major: 12, minor: 4}
		minimumCapability = majorMinor{major: 5, minor: 0}
	} else if arch == "arm64" {
		minimumCapability = majorMinor{major: 8, minor: 7}
	}
	if !version.atLeast(minimumVersion) {
		return false
	}

	devices, ok := parseCUDADevices(devicesOutput)
	if !ok {
		return false
	}
	if filtered {
		devices = visibleCUDADevices(devices, visible)
	}
	if len(devices) == 0 {
		return false
	}
	for _, device := range devices {
		if !device.capability.atLeast(minimumCapability) {
			return false
		}
	}
	return true
}

func parseMajorMinor(value string) (majorMinor, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 || !isASCIIDigits(parts[0]) || !isASCIIDigits(parts[1]) {
		return majorMinor{}, fmt.Errorf("invalid major.minor value %q", value)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return majorMinor{}, fmt.Errorf("invalid major value %q", parts[0])
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return majorMinor{}, fmt.Errorf("invalid minor value %q", parts[1])
	}

	return majorMinor{major: major, minor: minor}, nil
}

func isASCIIDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func parseCUDADevices(output string) ([]cudaDevice, bool) {
	var devices []cudaDevice
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, ",")
		if len(fields) != 3 {
			return nil, false
		}
		index := strings.TrimSpace(fields[0])
		uuid := strings.TrimSpace(fields[1])
		if index == "" || uuid == "" {
			return nil, false
		}
		capability, err := parseMajorMinor(strings.TrimSpace(fields[2]))
		if err != nil {
			return nil, false
		}
		devices = append(devices, cudaDevice{index: index, uuid: uuid, capability: capability})
	}
	return devices, len(devices) > 0
}

func visibleCUDADevices(devices []cudaDevice, visible string) []cudaDevice {
	if visible == "all" {
		return devices
	}
	if visible == "" || strings.Contains(visible, "MIG-") {
		return nil
	}
	var selected []cudaDevice
	for token := range strings.SplitSeq(visible, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			return nil
		}
		var matches []cudaDevice
		for _, device := range devices {
			if token == device.index || strings.HasPrefix(device.uuid, token) {
				matches = append(matches, device)
			}
		}
		if len(matches) != 1 {
			return nil
		}
		selected = append(selected, matches[0])
	}
	return selected
}

func hasVulkanHostSupport(ctx context.Context) bool {
	pctx, cancel := context.WithTimeout(ctx, hostProbeTimeout)
	defer cancel()
	out, err := exec.CommandContext(pctx, "vulkaninfo", "--summary").CombinedOutput()
	return err == nil && hasVulkanGPU(string(out))
}

func hasVulkanGPU(output string) bool {
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.Join(strings.Fields(line), " ")
		if strings.Contains(line, "PHYSICAL_DEVICE_TYPE_DISCRETE_GPU") ||
			strings.Contains(line, "PHYSICAL_DEVICE_TYPE_INTEGRATED_GPU") ||
			strings.Contains(line, "PHYSICAL_DEVICE_TYPE_VIRTUAL_GPU") ||
			strings.Contains(line, "PHYSICAL_DEVICE_TYPE_OTHER") {
			return true
		}
	}
	return false
}
