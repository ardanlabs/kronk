package gonja

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"path"

	"github.com/nikolalohinski/gonja/v2/builtins"
	"github.com/nikolalohinski/gonja/v2/config"
	"github.com/nikolalohinski/gonja/v2/exec"
	"github.com/nikolalohinski/gonja/v2/loaders"
	"github.com/nikolalohinski/gonja/v2/logging"
	"github.com/sirupsen/logrus"
)

var (
	DefaultLoader      = loaders.MustNewFileSystemLoader("")
	DefaultConfig      = config.New()
	DefaultContext     = exec.EmptyContext().Update(builtins.GlobalFunctions).Update(builtins.GlobalVariables)
	DefaultEnvironment = &exec.Environment{
		Context:           DefaultContext,
		Filters:           builtins.Filters,
		Tests:             builtins.Tests,
		ControlStructures: builtins.ControlStructures,
		Methods:           builtins.Methods,
	}
)

func SetLoggerOutput(out io.Writer) {
	if out == io.Discard {
		logging.SetEnabled(false)
	} else {
		logging.SetEnabled(true)
	}
	logrus.SetOutput(out)
}

func SetLoggerLevel(level logrus.Level) {
	logrus.SetLevel(level)
}

func FromString(source string) (*exec.Template, error) {
	return FromBytes([]byte(source))
}

func FromBytes(source []byte) (*exec.Template, error) {
	rootID := fmt.Sprintf("root-%s", string(sha256.New().Sum(source)))

	loader, err := loaders.NewFileSystemLoader("")
	if err != nil {
		return nil, err
	}
	shiftedLoader, err := loaders.NewShiftedLoader(rootID, bytes.NewReader(source), loader)
	if err != nil {
		return nil, err
	}

	return exec.NewTemplate(rootID, DefaultConfig, shiftedLoader, DefaultEnvironment)
}

func FromFile(filepath string) (*exec.Template, error) {
	loader, err := loaders.NewFileSystemLoader(path.Dir(filepath))
	if err != nil {
		return nil, err
	}

	return exec.NewTemplate(path.Base(filepath), DefaultConfig, loader, DefaultEnvironment)
}
