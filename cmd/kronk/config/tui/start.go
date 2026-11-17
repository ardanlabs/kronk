package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"charm.land/huh/v2"
	"github.com/ardanlabs/kronk/cmd/kronk/config/source"
	"github.com/ardanlabs/kronk/sdk/tools/modelconfig"
)

// The start form asks for the machine the config is for, then the config
// file, then whether to configure a model or the server. The machine comes
// first because it decides the files offered: a config for another machine
// is not written to the file this machine's server reads. Its answers are kept
// in start, so going back rebuilds the form with them.

const (
	fileOther = "other"

	targetThis    = "this"
	targetAnother = "another"

	whatModel  = "model"
	whatServer = "server"
)

// Start form field keys, used to find the focused field when going back.
const (
	keyFile        = "start/file"
	keyFileAnother = "start/file-another"
	keyFilePath    = "start/file-path"
	keyWhat        = "start/what"
	keyTarget      = "start/target"
	keyGPUs        = "start/gpus"
	keyGPUType     = "start/gpu-type"
	keyVRAM        = "start/vram"
	keyRAM         = "start/ram"
)

// start holds the start form answers.
type start struct {
	file        string // for this machine
	fileAnother string // for another machine
	filePath    string // when the file picked is fileOther
	what        string
	target      string
	gpus        string
	gpuType     string
	vram        string
	ram         string
}

func newStart(serverFile string) *start {
	return &start{
		file:        serverFile,
		fileAnother: newAnotherFile(serverFile),
		what:        whatModel,
		target:      targetThis,
		gpuType:     "cuda",
	}
}

// newAnotherFile is the file first offered for another machine:
// model_config.yaml in the current directory, unless it is the file this
// machine's server reads.
func newAnotherFile(serverFile string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return fileOther
	}
	if f := filepath.Join(cwd, "model_config.yaml"); f != serverFile {
		return f
	}
	return fileOther
}

// forAnother reports whether the config is for another machine.
func (s *start) forAnother() bool { return s.target == targetAnother }

func (s *start) server() bool { return s.what == whatServer }

// another reports whether a model config is for another machine. Server
// settings don't depend on the hardware, so they skip its questions.
func (s *start) another() bool { return !s.server() && s.forAnother() }

// picked is the file picked in the list for the machine.
func (s *start) picked() string {
	if s.forAnother() {
		return s.fileAnother
	}
	return s.file
}

func (s *start) gpuCount() int {
	n, _ := strconv.Atoi(strings.TrimSpace(s.gpus))
	return n
}

// path is the config file picked.
func (s *start) path() string {
	if f := s.picked(); f != fileOther {
		return f
	}
	p, _ := filepath.Abs(expandHome(s.filePath))
	return p
}

// machine describes the target machine from the answers. It returns false for
// this machine, whose devices are detected.
func (s *start) machine() (source.Machine, bool) {
	if !s.another() {
		return source.Machine{}, false
	}

	m := source.Machine{GPUs: s.gpuCount(), RAM: gb(s.ram)}
	if m.GPUs > 0 {
		m.GPUType = s.gpuType
		m.VRAM = gb(s.vram)
	}
	return m, true
}

func gb(s string) uint64 {
	n, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return uint64(n * (1 << 30))
}

func (a *app) newStartForm() *huh.Form {
	s := a.start

	// Every field lists ctrl+o in its key help, as the questions do.
	group := func(f huh.Field) *huh.Group { return huh.NewGroup(withAnswersKey(f)) }

	another := func() bool { return s.another() }
	withGPUs := func() bool { return s.another() && s.gpuCount() > 0 }

	gpuOpts := make([]huh.Option[string], 0, len(source.GPUTypes))
	for _, t := range source.GPUTypes {
		gpuOpts = append(gpuOpts, huh.NewOption(t.Label, t.Value))
	}

	return a.newForm(
		group(huh.NewSelect[string]().
			Key(keyTarget).
			Title("Is this config for this machine or another one?").
			Description("For another machine, the config is written to a file to copy there, and the model questions use the hardware you describe.").
			Options(
				huh.NewOption("This machine", targetThis),
				huh.NewOption("Another machine", targetAnother),
			).
			Value(&s.target)),

		group(huh.NewSelect[string]().
			Key(keyFile).
			Title("Which config file?").
			Description("Kronk reads one model_config.yaml: server settings and one entry per model.").
			Options(a.thisFileOptions()...).
			Validate(validPick).
			Value(&s.file)).
			WithHideFunc(s.forAnother),

		group(huh.NewSelect[string]().
			Key(keyFileAnother).
			Title("Which config file?").
			Description("A file to copy to that machine. The file this machine's server reads is not offered.").
			Options(a.anotherFileOptions()...).
			Validate(validPick).
			Value(&s.fileAnother)).
			WithHideFunc(func() bool { return !s.forAnother() }),

		group(huh.NewInput().
			Key(keyFilePath).
			Title("File path").
			Description("An existing file, or a new one to create.").
			Placeholder("./model_config.yaml").
			Validate(validFile).
			Value(&s.filePath)).
			WithHideFunc(func() bool { return s.picked() != fileOther }),

		group(huh.NewSelect[string]().
			Key(keyWhat).
			Title("What do you want to configure?").
			Description("A model's entry, or the server's own settings (kms:).").
			Options(
				huh.NewOption("A model", whatModel),
				huh.NewOption("Server settings", whatServer),
			).
			Value(&s.what)),

		group(huh.NewInput().
			Key(keyGPUs).
			Title("How many GPUs does the target machine have?").
			Description("0 for CPU only.").
			Validate(intAtLeast(0)).
			Value(&s.gpus)).
			WithHideFunc(func() bool { return !another() }),

		group(huh.NewSelect[string]().
			Key(keyGPUType).
			Title("What type of GPUs does the target machine have?").
			Description("It names the GPUs for the questions and sets how memory is counted.").
			Options(gpuOpts...).
			Value(&s.gpuType)).
			WithHideFunc(func() bool { return !withGPUs() }),

		group(huh.NewInput().
			Key(keyVRAM).
			Title("How much memory does each GPU have (GB)?").
			Description("The memory of one GPU, in GB.").
			Validate(positiveNumber).
			Value(&s.vram)).
			WithHideFunc(func() bool { return !withGPUs() }),

		group(huh.NewInput().
			Key(keyRAM).
			Title("How much system RAM does the target machine have (GB)?").
			Description("For the layers and experts that run on the CPU, in GB.").
			Validate(positiveNumber).
			Value(&s.ram)).
			WithHideFunc(func() bool { return !another() }),
	)
}

// startRows are the start form's answers for the answers list, in the order
// asked. Only the questions asked for these answers are listed.
func (a *app) startRows() []answerRow {
	s := a.start
	if s == nil {
		return nil
	}
	row := func(key, label, value string) answerRow {
		return answerRow{group: "Setup", key: key, label: label, value: value}
	}
	pending := func(v, unit string) string {
		if v = strings.TrimSpace(v); v == "" {
			return "…"
		}
		return v + unit
	}

	machine, fileKey := "This machine", keyFile
	if s.forAnother() {
		machine, fileKey = "Another machine", keyFileAnother
	}
	rows := []answerRow{row(keyTarget, "machine", machine)}

	if f := s.picked(); f != fileOther {
		rows = append(rows, row(fileKey, "config file", shortPath(f)))
	} else {
		rows = append(rows, row(fileKey, "config file", "another file"), row(keyFilePath, "file path", pending(s.filePath, "")))
	}

	what := "A model"
	if s.server() {
		what = "Server settings"
	}
	rows = append(rows, row(keyWhat, "configure", what))

	if !s.another() {
		return rows
	}
	rows = append(rows, row(keyGPUs, "gpus", pending(s.gpus, "")))
	if s.gpuCount() > 0 {
		gpuType := s.gpuType
		for _, t := range source.GPUTypes {
			if t.Value == s.gpuType {
				gpuType = t.Label
			}
		}
		rows = append(rows, row(keyGPUType, "gpu type", gpuType), row(keyVRAM, "gpu memory", pending(s.vram, " GB each")))
	}
	return append(rows, row(keyRAM, "ram", pending(s.ram, " GB")))
}

// thisFileOptions are the files offered for this machine: the one its server
// reads first, then the model_config*.yaml files in the current directory.
func (a *app) thisFileOptions() []huh.Option[string] {
	var opts []huh.Option[string]
	for _, f := range a.files {
		label := shortPath(f)
		if f == a.serverFile {
			label += "  " + a.st.muted.Render("the server reads this one")
		}
		opts = append(opts, huh.NewOption(label+a.newMark(f), f))
	}
	return append(opts, huh.NewOption("Another file…", fileOther))
}

// anotherFileOptions are the files offered for another machine: the
// model_config*.yaml files in the current directory, and model_config.yaml
// there when it doesn't exist yet. This machine's server file is left out.
func (a *app) anotherFileOptions() []huh.Option[string] {
	var opts []huh.Option[string]
	def := a.start.fileAnother
	if def != fileOther && !slices.Contains(a.files, def) {
		opts = append(opts, huh.NewOption(shortPath(def)+a.newMark(def), def))
	}
	for _, f := range a.files {
		if f != a.serverFile {
			opts = append(opts, huh.NewOption(shortPath(f), f))
		}
	}
	return append(opts, huh.NewOption("Another file…", fileOther))
}

// newMark marks a file that doesn't exist yet.
func (a *app) newMark(f string) string {
	if _, err := os.Stat(f); errors.Is(err, os.ErrNotExist) {
		return "  " + a.st.muted.Render("new")
	}
	return ""
}

// validPick checks the file picked from the list, before any other question.
// Another file is checked when its path is entered.
func validPick(s string) error {
	if s == fileOther {
		return nil
	}
	return validFile(s)
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
