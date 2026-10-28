package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"charm.land/huh/v2"
	"github.com/ardanlabs/kronk/sdk/tools/devices"
	"github.com/ardanlabs/kronk/sdk/tools/modelconfig"
)

// The start form picks the config file and the machine the config is for.
// Its answers are kept in start, so going back rebuilds the form with them.

const (
	fileOther = "other"

	targetThis    = "this"
	targetAnother = "another"

	whatModel  = "model"
	whatServer = "server"
)

// Start form field keys, used to find the focused field when going back.
const (
	keyFile     = "start/file"
	keyFilePath = "start/file-path"
	keyWhat     = "start/what"
	keyTarget   = "start/target"
	keyGPUs     = "start/gpus"
	keyGPUType  = "start/gpu-type"
	keyVRAM     = "start/vram"
	keyRAM      = "start/ram"
)

// gpuTypes are the GPU backends a target machine can have. name is the
// llama.cpp device name prefix, used to build CUDA0, CUDA1, ...
var gpuTypes = []struct {
	label, value, name string
}{
	{"NVIDIA (CUDA)", "cuda", "CUDA"},
	{"AMD (ROCm)", "rocm", "ROCm"},
	{"Vulkan", "vulkan", "Vulkan"},
	{"Apple Silicon (Metal)", "metal", "MTL"},
}

// start holds the start form answers.
type start struct {
	file     string
	filePath string
	what     string
	target   string
	gpus     string
	gpuType  string
	vram     string
	ram      string
}

func newStart(serverFile string) *start {
	return &start{file: serverFile, what: whatModel, target: targetThis, gpuType: "cuda"}
}

func (s *start) server() bool { return s.what == whatServer }

// another reports whether a model config is for another machine. Server
// settings don't depend on the hardware, so they skip those questions.
func (s *start) another() bool { return !s.server() && s.target == targetAnother }

func (s *start) gpuCount() int {
	n, _ := strconv.Atoi(strings.TrimSpace(s.gpus))
	return n
}

// path is the config file picked.
func (s *start) path() string {
	if s.file == fileOther {
		p, _ := filepath.Abs(expandHome(s.filePath))
		return p
	}
	return s.file
}

// machine describes the target machine from the answers. It returns false for
// this machine, whose devices are detected.
func (s *start) machine() (machine, bool) {
	if !s.another() {
		return machine{}, false
	}

	m := machine{gpus: s.gpuCount(), ram: gb(s.ram)}
	if m.gpus > 0 {
		m.gpuType = s.gpuType
		m.vram = gb(s.vram)
	}
	return m, true
}

// =============================================================================

// machine is a target machine described by the user.
type machine struct {
	gpus    int
	gpuType string
	vram    uint64 // per GPU
	ram     uint64
}

// devices builds the device list the analysis reads, as llama.cpp would
// report it on that machine.
func (m machine) devices() devices.Devices {
	devs := devices.Devices{
		Devices:        []devices.DeviceInfo{{Name: "CPU", Type: "cpu", HardwareType: "cpu"}},
		SystemRAMBytes: m.ram,
		MMapSupported:  true,
		UnifiedMemory:  m.gpuType == "metal",
	}

	i := slices.IndexFunc(gpuTypes, func(t struct{ label, value, name string }) bool { return t.value == m.gpuType })
	for n := range m.gpus {
		name := fmt.Sprintf("GPU%d", n)
		if i >= 0 {
			name = fmt.Sprintf("%s%d", gpuTypes[i].name, n)
		}
		devs.Devices = append(devs.Devices, devices.DeviceInfo{
			Index:        n + 1,
			Name:         name,
			Type:         "gpu_" + m.gpuType,
			HardwareType: "gpu",
			Backend:      m.gpuType,
			FreeBytes:    m.vram,
			TotalBytes:   m.vram,
		})
		devs.GPUCount++
		devs.GPUTotalBytes += m.vram
		devs.SupportsGPUOffload = true
	}

	return devs
}

func (m machine) String() string {
	if m.gpus == 0 {
		return fmt.Sprintf("another machine: CPU only, %s RAM", formatBytes(m.ram))
	}

	name := m.gpuType
	for _, t := range gpuTypes {
		if t.value == m.gpuType {
			name = t.label
		}
	}
	return fmt.Sprintf("another machine: %d × %s, %s each, %s RAM", m.gpus, name, formatBytes(m.vram), formatBytes(m.ram))
}

func gb(s string) uint64 {
	n, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return uint64(n * (1 << 30))
}

// =============================================================================

// configFiles lists the config files to offer: the one the server reads,
// then model_config*.yaml files in the current directory. Other YAML files
// can load as a config too (modelconfig.Load ignores unknown keys), so the
// name is what selects them.
func configFiles(serverFile string) []string {
	files := []string{serverFile}

	cwd, err := os.Getwd()
	if err != nil {
		return files
	}

	var found []string
	for _, pattern := range []string{"model_config*.yaml", "model_config*.yml"} {
		m, _ := filepath.Glob(filepath.Join(cwd, pattern))
		found = append(found, m...)
	}
	slices.Sort(found)

	for _, f := range found {
		if f == serverFile || slices.Contains(files, f) {
			continue
		}
		if _, err := modelconfig.Load(f); err != nil {
			continue
		}
		files = append(files, f)
	}

	return files
}

func (a *app) newStartForm() *huh.Form {
	s := a.start

	var fileOpts []huh.Option[string]
	for _, f := range a.files {
		label := shortPath(f)
		if f == a.serverFile {
			label += "  " + a.st.muted.Render("the server reads this one")
			if _, err := os.Stat(f); errors.Is(err, os.ErrNotExist) {
				label += a.st.muted.Render(", new")
			}
		}
		fileOpts = append(fileOpts, huh.NewOption(label, f))
	}
	fileOpts = append(fileOpts, huh.NewOption("Another file…", fileOther))

	another := func() bool { return s.another() }
	withGPUs := func() bool { return s.another() && s.gpuCount() > 0 }

	gpuOpts := make([]huh.Option[string], 0, len(gpuTypes))
	for _, t := range gpuTypes {
		gpuOpts = append(gpuOpts, huh.NewOption(t.label, t.value))
	}

	return a.sized(huh.NewForm(
		huh.NewGroup(huh.NewSelect[string]().
			Key(keyFile).
			Title("Which config file?").
			Description("Kronk reads one model_config.yaml: server settings and one entry per model.").
			Options(fileOpts...).
			Value(&s.file)),

		huh.NewGroup(huh.NewInput().
			Key(keyFilePath).
			Title("File path").
			Description("An existing file, or a new one to create.").
			Placeholder("./model_config.yaml").
			Validate(validFile).
			Value(&s.filePath)).
			WithHideFunc(func() bool { return s.file != fileOther }),

		huh.NewGroup(huh.NewSelect[string]().
			Key(keyWhat).
			Title("What do you want to configure?").
			Options(
				huh.NewOption("A model", whatModel),
				huh.NewOption("Server settings", whatServer),
			).
			Value(&s.what)),

		huh.NewGroup(huh.NewSelect[string]().
			Key(keyTarget).
			Title("Is this config for this machine or another one?").
			Description("For another machine, describe its hardware. It's used for the questions and the memory check.").
			Options(
				huh.NewOption("This machine", targetThis),
				huh.NewOption("Another machine", targetAnother),
			).
			Value(&s.target)).
			WithHideFunc(s.server),

		huh.NewGroup(huh.NewInput().
			Key(keyGPUs).
			Title("How many GPUs does the target machine have?").
			Description("0 for CPU only.").
			Validate(intAtLeast(0)).
			Value(&s.gpus)).
			WithHideFunc(func() bool { return !another() }),

		huh.NewGroup(huh.NewSelect[string]().
			Key(keyGPUType).
			Title("What type of GPUs does the target machine have?").
			Options(gpuOpts...).
			Value(&s.gpuType)).
			WithHideFunc(func() bool { return !withGPUs() }),

		huh.NewGroup(huh.NewInput().
			Key(keyVRAM).
			Title("How much memory does each GPU have (GB)?").
			Validate(positiveNumber).
			Value(&s.vram)).
			WithHideFunc(func() bool { return !withGPUs() }),

		huh.NewGroup(huh.NewInput().
			Key(keyRAM).
			Title("How much system RAM does the target machine have (GB)?").
			Validate(positiveNumber).
			Value(&s.ram)).
			WithHideFunc(func() bool { return !another() }),
	).WithTheme(a.theme).WithKeyMap(backKeyMap()).WithShowHelp(true))
}

// validFile accepts a path to an existing config file that loads, or to a
// file that doesn't exist yet in an existing or creatable directory.
func validFile(s string) error {
	path := expandHome(s)
	if path == "" {
		return errors.New("enter a path")
	}

	info, err := os.Stat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil
	case err != nil:
		return err
	case info.IsDir():
		return errors.New("that is a directory, enter a file path")
	}

	if _, err := modelconfig.Load(path); err != nil {
		return fmt.Errorf("not a model config file: %w", err)
	}
	return nil
}

func intAtLeast(lo int) func(string) error {
	return func(s string) error {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			return errors.New("enter a whole number")
		}
		if n < lo {
			return fmt.Errorf("must be %d or more", lo)
		}
		return nil
	}
}

func positiveNumber(s string) error {
	n, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return errors.New("enter a number")
	}
	if n <= 0 {
		return errors.New("must be more than 0")
	}
	return nil
}
