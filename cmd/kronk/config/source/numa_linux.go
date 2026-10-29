//go:build linux

package source

import (
	"path/filepath"
	"strconv"
	"strings"
)

// numaNodes returns the number of NUMA nodes on Linux, from sysfs, or 1 when
// sysfs doesn't list any. The SDK doesn't report it.
func numaNodes() int {
	return countNUMANodes("/sys/devices/system/node")
}

// countNUMANodes counts the node<N> entries in dir.
func countNUMANodes(dir string) int {
	entries, _ := filepath.Glob(filepath.Join(dir, "node[0-9]*"))

	var n int
	for _, e := range entries {
		if _, err := strconv.Atoi(strings.TrimPrefix(filepath.Base(e), "node")); err == nil {
			n++
		}
	}

	return max(n, 1)
}
