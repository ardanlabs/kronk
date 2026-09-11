package vram

import "testing"

func TestCalculateDefaultsToOneContext(t *testing.T) {
	input := Input{ModelSizeBytes: 4_300_000_000}

	got := Calculate(input)

	if got.Input != input {
		t.Errorf("Input: got %+v, want %+v", got.Input, input)
	}
	if got.ModelBytes != input.ModelSizeBytes {
		t.Errorf("ModelBytes: got %d, want %d", got.ModelBytes, input.ModelSizeBytes)
	}
	if got.RuntimeBytes != RuntimeOverhead {
		t.Errorf("RuntimeBytes: got %d, want %d", got.RuntimeBytes, RuntimeOverhead)
	}
	if want := input.ModelSizeBytes + RuntimeOverhead; got.TotalVRAM != want {
		t.Errorf("TotalVRAM: got %d, want %d", got.TotalVRAM, want)
	}
}

func TestCalculateChargesEveryContext(t *testing.T) {
	input := Input{
		ModelSizeBytes: 5_335_000_000,
		Contexts:       3,
	}

	got := Calculate(input)

	if want := input.ModelSizeBytes * input.Contexts; got.ModelBytes != want {
		t.Errorf("ModelBytes: got %d, want %d", got.ModelBytes, want)
	}
	if want := RuntimeOverhead * input.Contexts; got.RuntimeBytes != want {
		t.Errorf("RuntimeBytes: got %d, want %d", got.RuntimeBytes, want)
	}
	if want := (input.ModelSizeBytes + RuntimeOverhead) * input.Contexts; got.TotalVRAM != want {
		t.Errorf("TotalVRAM: got %d, want %d", got.TotalVRAM, want)
	}
}
