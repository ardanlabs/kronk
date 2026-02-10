package defaults

import (
	"context"
	"path"
	"time"

	"github.com/ardanlabs/kronk/sdk/tools/hardware"
)

// HardwareInfo performs hardware detection with default options.
func HardwareInfo(ctx context.Context, libDir string) (hardware.HardwareInfo, error) {
	llamaBenchPath := path.Join(libDir, "libraries/llama-bench")

	opt := hardware.Options{
		LlamaBenchPath: llamaBenchPath,
		Timeout:        5 * time.Second,
	}

	return hardware.Detect(ctx, opt)
}
