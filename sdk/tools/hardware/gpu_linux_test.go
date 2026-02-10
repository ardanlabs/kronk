//go:build linux

package hardware

import "testing"

func TestParseNvidiaSmi(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		wantCount int
		wantFirst GPUDevice
	}{
		{
			name:      "single GPU",
			output:    "0, NVIDIA GeForce RTX 4090, 24564\n",
			wantCount: 1,
			wantFirst: GPUDevice{
				ID:        "0",
				Name:      "NVIDIA GeForce RTX 4090",
				Backend:   BackendCUDA,
				VRAMBytes: 24564 * mibToBytes,
			},
		},
		{
			name:      "dual GPUs",
			output:    "0, NVIDIA A100, 81920\n1, NVIDIA A100, 81920\n",
			wantCount: 2,
			wantFirst: GPUDevice{
				ID:        "0",
				Name:      "NVIDIA A100",
				Backend:   BackendCUDA,
				VRAMBytes: 81920 * mibToBytes,
			},
		},
		{
			name:      "empty output",
			output:    "",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			devices, err := parseNvidiaSmi(tt.output)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(devices) != tt.wantCount {
				t.Fatalf("expected %d devices, got %d", tt.wantCount, len(devices))
			}

			if tt.wantCount == 0 {
				return
			}

			got := devices[0]
			if got.ID != tt.wantFirst.ID {
				t.Errorf("ID: got %q, want %q", got.ID, tt.wantFirst.ID)
			}
			if got.Name != tt.wantFirst.Name {
				t.Errorf("Name: got %q, want %q", got.Name, tt.wantFirst.Name)
			}
			if got.Backend != tt.wantFirst.Backend {
				t.Errorf("Backend: got %q, want %q", got.Backend, tt.wantFirst.Backend)
			}
			if got.VRAMBytes != tt.wantFirst.VRAMBytes {
				t.Errorf("VRAMBytes: got %d, want %d", got.VRAMBytes, tt.wantFirst.VRAMBytes)
			}
		})
	}
}

func TestParseROCmSmiJSON(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCount int
	}{
		{
			name: "single GPU",
			input: `{
				"card0": {
					"VRAM Total Memory (B)": "17163091968",
					"VRAM Total Used Memory (B)": "9502720"
				}
			}`,
			wantCount: 1,
		},
		{
			name: "dual GPUs",
			input: `{
				"card0": {
					"VRAM Total Memory (B)": "17163091968",
					"VRAM Total Used Memory (B)": "9502720"
				},
				"card1": {
					"VRAM Total Memory (B)": "17163091968",
					"VRAM Total Used Memory (B)": "0"
				}
			}`,
			wantCount: 2,
		},
		{
			name: "numeric float value",
			input: `{
				"card0": {
					"VRAM Total Memory (B)": 17163091968
				}
			}`,
			wantCount: 1,
		},
		{
			name:      "empty",
			input:     `{}`,
			wantCount: 0,
		},
		{
			name:      "invalid json",
			input:     `not json`,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			devices := parseROCmSmiJSON([]byte(tt.input))
			if len(devices) != tt.wantCount {
				t.Fatalf("expected %d devices, got %d", tt.wantCount, len(devices))
			}

			if tt.wantCount > 0 {
				for _, d := range devices {
					if d.Backend != BackendROCm {
						t.Errorf("expected backend %q, got %q", BackendROCm, d.Backend)
					}
					if d.VRAMBytes == 0 {
						t.Error("expected non-zero VRAMBytes")
					}
				}
			}
		})
	}
}

func TestParseAMDSmiJSON(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCount int
	}{
		{
			name: "single GPU",
			input: `[{
				"vram": {
					"type": "HBM",
					"vendor": "N/A",
					"size": {
						"value": 196592,
						"unit": "MB"
					}
				}
			}]`,
			wantCount: 1,
		},
		{
			name: "dual GPUs",
			input: `[
				{"vram": {"size": {"value": 49140, "unit": "MB"}}},
				{"vram": {"size": {"value": 49140, "unit": "MB"}}}
			]`,
			wantCount: 2,
		},
		{
			name:      "empty array",
			input:     `[]`,
			wantCount: 0,
		},
		{
			name:      "no vram field",
			input:     `[{"gpu": 0}]`,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			devices := parseAMDSmiJSON([]byte(tt.input))
			if len(devices) != tt.wantCount {
				t.Fatalf("expected %d devices, got %d", tt.wantCount, len(devices))
			}

			if tt.wantCount > 0 {
				for _, d := range devices {
					if d.Backend != BackendROCm {
						t.Errorf("expected backend %q, got %q", BackendROCm, d.Backend)
					}
					if d.VRAMBytes == 0 {
						t.Error("expected non-zero VRAMBytes")
					}
				}
			}
		})
	}
}
