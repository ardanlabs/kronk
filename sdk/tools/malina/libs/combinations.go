package libs

var supportedCombinations = []Combination{
	{Arch: "arm64", OS: "darwin", Processor: "cpu"}, {Arch: "arm64", OS: "darwin", Processor: "metal"},
	{Arch: "amd64", OS: "linux", Processor: "cpu"}, {Arch: "amd64", OS: "linux", Processor: "vulkan"}, {Arch: "amd64", OS: "linux", Processor: "rocm"},
	{Arch: "amd64", OS: "windows", Processor: "cpu"}, {Arch: "amd64", OS: "windows", Processor: "cuda"}, {Arch: "amd64", OS: "windows", Processor: "vulkan"}, {Arch: "amd64", OS: "windows", Processor: "rocm"},
}

// SupportedCombinations returns the published native artifact matrix.
func SupportedCombinations() []Combination {
	return append([]Combination(nil), supportedCombinations...)
}

// IsSupported reports whether a native artifact is published for the triple.
func IsSupported(arch, opSys, processor string) bool {
	for _, c := range supportedCombinations {
		if c.Arch == arch && c.OS == opSys && c.Processor == processor {
			return true
		}
	}
	return false
}
