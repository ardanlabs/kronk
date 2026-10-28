package config

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

	devs := m.devices()
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
	devs := m.devices()
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

func TestConfigFilesByName(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	for name, body := range map[string]string{
		".goreleaser.yaml":      "version: 2\nbuilds: []\n",
		"model_config.yaml":     "version: 2\nmodels: {}\n",
		"model_config.gpu.yaml": "version: 2\nmodels: {}\n",
		"model_config.bad.yaml": "version: 99\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got := configFiles("/srv/model_config.yaml")
	want := []string{"/srv/model_config.yaml", filepath.Join(dir, "model_config.gpu.yaml"), filepath.Join(dir, "model_config.yaml")}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
