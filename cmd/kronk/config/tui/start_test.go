package tui

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestMachineDevices(t *testing.T) {
	s := newStart("/srv/model_config.yaml")
	s.target = targetAnother
	s.gpus = "2"
	s.gpuType = "cuda"
	s.vram = "24"
	s.ram = "128"

	m, ok := s.machine()
	if !ok {
		t.Fatal("another machine must describe its hardware")
	}

	devs := m.Devices()
	if devs.GPUCount != 2 || !devs.SupportsGPUOffload {
		t.Fatalf("gpus = %d, offload = %t", devs.GPUCount, devs.SupportsGPUOffload)
	}
	if devs.GPUTotalBytes != 48<<30 || devs.SystemRAMBytes != 128<<30 {
		t.Fatalf("vram = %d, ram = %d", devs.GPUTotalBytes, devs.SystemRAMBytes)
	}

	var names []string
	for _, d := range devs.Devices {
		names = append(names, d.Name)
	}
	if want := []string{"CPU", "CUDA0", "CUDA1"}; len(names) != 3 || names[1] != want[1] || names[2] != want[2] {
		t.Fatalf("device names = %v, want %v", names, want)
	}
}

func TestMachineCPUOnly(t *testing.T) {
	s := newStart("/srv/model_config.yaml")
	s.target = targetAnother
	s.gpus = "0"
	s.vram = "24" // ignored without GPUs
	s.ram = "64"

	m, _ := s.machine()
	devs := m.Devices()
	if devs.GPUCount != 0 || devs.SupportsGPUOffload || devs.GPUTotalBytes != 0 {
		t.Fatalf("CPU-only machine has GPUs: %+v", devs)
	}
}

func TestThisMachineIsDetected(t *testing.T) {
	if _, ok := newStart("/srv/model_config.yaml").machine(); ok {
		t.Fatal("this machine must be detected, not described")
	}
}

func TestValidFile(t *testing.T) {
	dir := t.TempDir()

	good := filepath.Join(dir, "good.yaml")
	if err := os.WriteFile(good, []byte("version: 2\nmodels: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(bad, []byte("version: 99\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		path string
		ok   bool
	}{
		{good, true},
		{filepath.Join(dir, "new.yaml"), true},
		{bad, false},
		{dir, false},
		{"", false},
	} {
		if err := validFile(tt.path); (err == nil) != tt.ok {
			t.Errorf("validFile(%q) = %v, want ok %t", tt.path, err, tt.ok)
		}
	}

	if err := validPick(bad); err == nil {
		t.Error("a picked file that doesn't load must stop at the file question")
	}
	if err := validPick(fileOther); err != nil {
		t.Errorf("another file is checked when its path is entered: %v", err)
	}
}

func TestAnotherMachineGetsItsOwnFile(t *testing.T) {
	a := newApp(context.Background(), nil, true)
	a.serverFile = "/srv/model_config.yaml"
	cwdFile := filepath.Join(t.TempDir(), "model_config.prod.yaml")
	a.files = []string{a.serverFile, cwdFile}
	a.start = newStart(a.serverFile)

	if got := a.start.path(); got != a.serverFile {
		t.Fatalf("this machine: path = %q, want the server's file", got)
	}

	a.start.target = targetAnother
	if got := a.start.path(); got == a.serverFile {
		t.Fatalf("another machine: path = %q, want a file other than the server's", got)
	}

	for _, o := range a.anotherFileOptions() {
		if o.Value == a.serverFile {
			t.Fatal("another machine: the server's file is offered")
		}
	}

	a.start.fileAnother = cwdFile
	if got := a.start.path(); got != cwdFile {
		t.Fatalf("another machine: path = %q, want %q", got, cwdFile)
	}
}

func TestStartRowsFollowTheAnswers(t *testing.T) {
	a := newApp(context.Background(), nil, true)
	a.start = newStart("/srv/model_config.yaml")
	a.step = stepStart

	keys := func() []string {
		var out []string
		for _, r := range a.answerRows() {
			out = append(out, r.key)
		}
		return out
	}

	if got, want := keys(), []string{keyTarget, keyFile, keyWhat}; !slices.Equal(got, want) {
		t.Fatalf("this machine: rows = %v, want %v", got, want)
	}

	a.start.target, a.start.gpus = targetAnother, "2"
	want := []string{keyTarget, keyFileAnother, keyWhat, keyGPUs, keyGPUType, keyVRAM, keyRAM}
	if got := keys(); !slices.Equal(got, want) {
		t.Fatalf("another machine: rows = %v, want %v", got, want)
	}

	a.start.what = whatServer
	if got, want := keys(), []string{keyTarget, keyFileAnother, keyWhat}; !slices.Equal(got, want) {
		t.Fatalf("server settings: rows = %v, want %v; no hardware", got, want)
	}
}
