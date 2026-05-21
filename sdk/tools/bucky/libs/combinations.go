package libs

// supportedCombinations lists every (os, arch, processor) triple that
// the bucky download package can resolve to a precompiled whisper.cpp
// release artifact. Entries here mirror the switch tables in
// github.com/ardanlabs/bucky/pkg/download/download.go and must be
// kept in sync when that package adds or drops a build target.
var supportedCombinations = []Combination{
	// macOS (the xcframework is universal arm64 + x86_64 and includes
	// Metal; bucky exposes the metal slice as both metal and cpu).
	{Arch: "arm64", OS: "darwin", Processor: "metal"},
	{Arch: "arm64", OS: "darwin", Processor: "cpu"},
	{Arch: "amd64", OS: "darwin", Processor: "cpu"},
	{Arch: "amd64", OS: "darwin", Processor: "metal"},

	// Linux (assets produced by ardanlabs/bucky-builder).
	{Arch: "amd64", OS: "linux", Processor: "cpu"},
	{Arch: "amd64", OS: "linux", Processor: "cuda"},
	{Arch: "amd64", OS: "linux", Processor: "vulkan"},
	{Arch: "arm64", OS: "linux", Processor: "cpu"},
	{Arch: "arm64", OS: "linux", Processor: "cuda"},
	{Arch: "arm64", OS: "linux", Processor: "vulkan"},

	// Windows (whisper.cpp upstream releases, AMD64 only in v1).
	{Arch: "amd64", OS: "windows", Processor: "cpu"},
	{Arch: "amd64", OS: "windows", Processor: "cuda"},
}

// SupportedCombinations returns every (architecture, operating
// system, processor) triple that the upstream whisper.cpp build
// matrix publishes. The returned slice is a copy and may be safely
// modified by the caller.
func SupportedCombinations() []Combination {
	out := make([]Combination, len(supportedCombinations))
	copy(out, supportedCombinations)
	return out
}

// IsSupported reports whether the supplied (arch, os, processor)
// triple is part of the upstream build matrix returned by
// SupportedCombinations. It is intended for validating user input
// before invoking install operations.
func IsSupported(arch string, opSys string, processor string) bool {
	for _, c := range supportedCombinations {
		if c.Arch == arch && c.OS == opSys && c.Processor == processor {
			return true
		}
	}
	return false
}
