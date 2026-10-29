package tui

import (
	"os"
	"path/filepath"
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
}
