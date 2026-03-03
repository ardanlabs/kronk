// Package devices provides the devices command for listing available compute devices.
package devices

import (
	"fmt"
	"os"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "devices",
	Short: "List available compute devices",
	Long: `List all available compute devices that can be used for model inference.

Device names shown here can be used with --devices flag in the run command
or the "devices" field in model configuration.

EXAMPLES

  # List all devices
  kronk devices`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := run(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	},
}

func run() error {
	if err := kronk.Init(); err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}

	count := llama.GGMLBackendDeviceCount()
	if count == 0 {
		fmt.Println("No compute devices found.")
		return nil
	}

	fmt.Printf("Available compute devices (%d):\n\n", count)
	fmt.Printf("  %-8s  %-20s  %s\n", "INDEX", "NAME", "TYPE")
	fmt.Printf("  %-8s  %-20s  %s\n", "-----", "----", "----")

	for i := range count {
		dev := llama.GGMLBackendDeviceGet(i)
		name := llama.GGMLBackendDeviceName(dev)
		fmt.Printf("  %-8d  %-20s  %s\n", i, name, deviceType(dev))
	}

	fmt.Printf("\nGPU offload supported: %t\n", llama.SupportsGpuOffload())

	return nil
}

func deviceType(dev llama.GGMLBackendDevice) string {
	name := llama.GGMLBackendDeviceName(dev)
	switch {
	case name == "CPU":
		return "CPU"
	case len(name) >= 4 && name[:4] == "CUDA":
		return "GPU (CUDA)"
	case name == "Metal":
		return "GPU (Metal)"
	case len(name) >= 3 && name[:3] == "HIP":
		return "GPU (ROCm)"
	case len(name) >= 6 && name[:6] == "Vulkan":
		return "GPU (Vulkan)"
	default:
		return "Unknown"
	}
}
