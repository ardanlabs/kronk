//go:build !linux

package source

// numaNodes returns 1 on platforms where NUMA nodes are not detected.
func numaNodes() int {
	return 1
}
