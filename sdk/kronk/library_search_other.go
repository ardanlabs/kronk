//go:build !windows

package kronk

// preloadLibraries is a no-op on non-Windows platforms. Linux uses
// LD_LIBRARY_PATH and macOS uses dylib install names / @rpath, both of which
// already let the bundled library directory take precedence over system
// locations, so no DLL-search-order fixup is required. See the Windows build
// of this function for the full rationale.
func preloadLibraries(libPath string) error {
	return nil
}
