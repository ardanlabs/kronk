package gomlx

import (
	"strings"
	"testing"

	"github.com/gomlx/compute"
)

func TestInspectComputeRejectsAnUnavailableConfiguredBackend(t *testing.T) {
	t.Setenv(compute.ConfigEnvVar, "not-installed:test")

	_, err := InspectCompute()
	if err == nil || !strings.Contains(err.Error(), "not-installed") {
		t.Fatalf("error = %v, want unavailable configured backend", err)
	}
}
