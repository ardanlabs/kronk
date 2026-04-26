package libs

// Combination represents a single supported (architecture, operating system,
// processor) triple for a precompiled llama.cpp library bundle. Values match
// the strings accepted by the upstream download package and by the
// KRONK_ARCH, KRONK_OS, and KRONK_PROCESSOR environment variables.
type Combination struct {
	Arch      string `json:"arch"`
	OS        string `json:"os"`
	Processor string `json:"processor"`
}

// supportedCombinations lists every (os, arch, processor) triple that the
// upstream hybridgroup/yzma download package can resolve to a precompiled
// llama.cpp release artifact. Entries here mirror the switch tables in
// yzma/pkg/download/download.go and must be kept in sync when that package
// adds or drops a build target.
var supportedCombinations = []Combination{
	// Linux (Ubuntu-based releases).
	{Arch: "amd64", OS: "linux", Processor: "cpu"},
	{Arch: "arm64", OS: "linux", Processor: "cpu"},
	{Arch: "amd64", OS: "linux", Processor: "cuda"},
	{Arch: "arm64", OS: "linux", Processor: "cuda"},
	{Arch: "amd64", OS: "linux", Processor: "vulkan"},
	{Arch: "arm64", OS: "linux", Processor: "vulkan"},
	{Arch: "amd64", OS: "linux", Processor: "rocm"},

	// Debian Bookworm (ARM64-only build matrix).
	{Arch: "arm64", OS: "bookworm", Processor: "cpu"},
	{Arch: "arm64", OS: "bookworm", Processor: "cuda"},
	{Arch: "arm64", OS: "bookworm", Processor: "vulkan"},

	// Debian Trixie.
	{Arch: "amd64", OS: "trixie", Processor: "cpu"},
	{Arch: "arm64", OS: "trixie", Processor: "cpu"},
	{Arch: "amd64", OS: "trixie", Processor: "cuda"},
	{Arch: "amd64", OS: "trixie", Processor: "vulkan"},
	{Arch: "arm64", OS: "trixie", Processor: "vulkan"},

	// macOS (Apple Silicon plus an x86_64 CPU-only build).
	{Arch: "arm64", OS: "darwin", Processor: "metal"},
	{Arch: "arm64", OS: "darwin", Processor: "cpu"},
	{Arch: "amd64", OS: "darwin", Processor: "cpu"},

	// Windows.
	{Arch: "amd64", OS: "windows", Processor: "cpu"},
	{Arch: "arm64", OS: "windows", Processor: "cpu"},
	{Arch: "amd64", OS: "windows", Processor: "cuda"},
	{Arch: "amd64", OS: "windows", Processor: "vulkan"},
	{Arch: "amd64", OS: "windows", Processor: "rocm"},
}

// SupportedCombinations returns every (architecture, operating system,
// processor) triple that the upstream llama.cpp build matrix publishes. The
// returned slice is a copy and may be safely modified by the caller. UI
// selectors can use it to populate dropdowns and prevent users from
// requesting unbuildable combinations.
func SupportedCombinations() []Combination {
	out := make([]Combination, len(supportedCombinations))
	copy(out, supportedCombinations)
	return out
}

// IsSupported reports whether the supplied (arch, os, processor) triple is
// part of the upstream build matrix returned by SupportedCombinations. It is
// intended for validating user input before invoking install operations.
func IsSupported(arch string, opSys string, processor string) bool {
	for _, c := range supportedCombinations {
		if c.Arch == arch && c.OS == opSys && c.Processor == processor {
			return true
		}
	}
	return false
}
