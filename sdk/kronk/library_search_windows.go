//go:build windows

package kronk

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

// preloadLibraries loads kronk's bundled top-level DLLs (and their transitive
// static dependencies) from libPath BEFORE yzma loads them, so they resolve to
// our copies instead of an older, same-named DLL that happens to live in the
// Windows system directory (System32).
//
// Why this is needed only on Windows: yzma loads the top-level libraries
// (llama.dll, mtmd.dll) by absolute path, but the OS resolves their transitive
// dependencies by bare file name using the standard search order, in which
// System32 is searched BEFORE the directories on PATH. A stale system-wide copy
// of a bundled dependency therefore shadows ours and the load fails with "The
// specified procedure could not be found" (a newer llama.cpp build imports a
// symbol the old system DLL does not export). Appending libPath to PATH cannot
// win that race. Linux (LD_LIBRARY_PATH) and macOS (install names / @rpath)
// already let the bundled directory take precedence, so they need no equivalent.
//
// Mechanism: each top-level DLL is loaded with LoadLibraryEx and
// LOAD_WITH_ALTERED_SEARCH_PATH, which makes the directory containing that DLL
// (libPath) the first directory searched for it AND its dependency tree —
// ggml.dll, ggml-base.dll, libomp140.x86_64.dll, and so on. This fixes the
// whole class of System32-shadowing for everything kronk ships, not just
// libomp. yzma's later plain LoadLibrary of the same absolute path simply
// reuses the already-resident module. The ggml-cpu-*.dll backends loaded later
// by GGMLBackendLoadAllFromPath are covered indirectly: their shared
// dependencies (ggml-base, libomp) are already resident, and loaded modules are
// the first thing the loader checks.
//
// This is deliberately scoped to kronk's own load. It does NOT alter the
// process-global DLL search order, so it cannot break or cross-contaminate a
// sibling backend (e.g. bucky/whisper) that loads its own libraries from a
// different directory in the same process.
//
// libomp140.x86_64.dll is preloaded explicitly first as a deterministic leaf
// guard, in case LOAD_WITH_ALTERED_SEARCH_PATH does not propagate to that deep
// a transitive dependency on some Windows versions; an absolute path ignores
// the search order entirely. Every load here is best-effort: handles are
// intentionally leaked for the process lifetime (these libraries are never
// unloaded), and the first error is returned for the caller to treat as
// non-fatal, leaving the PATH entry as the fallback.
func preloadLibraries(libPath string) error {
	names := []string{
		"libomp140.x86_64.dll", // leaf dependency: load first by absolute path
		"llama.dll",
		"mtmd.dll",
	}

	var firstErr error
	for _, name := range names {
		full := filepath.Join(libPath, name)

		if _, err := windows.LoadLibraryEx(full, 0, windows.LOAD_WITH_ALTERED_SEARCH_PATH); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	return firstErr
}
