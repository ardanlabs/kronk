// Package devices provides the devices command for listing available compute devices.
package devices

import (
	"fmt"
	"os"

	"github.com/ardanlabs/kronk/sdk/kronk"
	sdkdevices "github.com/ardanlabs/kronk/sdk/tools/devices"
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

	d := sdkdevices.List()
	if len(d.Devices) == 0 {
		fmt.Println("No compute devices found.")
		return nil
	}

	fmt.Printf("Available compute devices (%d):\n\n", len(d.Devices))
	fmt.Printf("  %-8s  %-20s  %s\n", "INDEX", "NAME", "TYPE")
	fmt.Printf("  %-8s  %-20s  %s\n", "-----", "----", "----")

	for _, dev := range d.Devices {
		fmt.Printf("  %-8d  %-20s  %s\n", dev.Index, dev.Name, deviceType(dev))
	}

	fmt.Printf("\nGPU offload supported: %t\n", d.SupportsGPUOffload)

	return nil
}

func deviceType(dev sdkdevices.DeviceInfo) string {
	switch dev.Type {
	case "cpu":
		return "CPU"
	case "gpu_cuda":
		return "GPU (CUDA)"
	case "gpu_metal":
		return "GPU (Metal)"
	case "gpu_rocm":
		return "GPU (ROCm)"
	case "gpu_vulkan":
		return "GPU (Vulkan)"
	default:
		return "Unknown"
	}
}
