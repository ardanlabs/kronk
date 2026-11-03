---
name: use-modern-go
description: Apply modern Go syntax guidelines based on the project's Go version. Use whenever writing, editing, or reviewing Go (.go) code - not only when explicitly asked for guidelines.
---

# Modern Go

Before writing Go, run on the file/dir being edited (`<skill>` = this file's dir):

- sh: `sh <skill>/scripts/guidelines.sh <path>`
- PowerShell: `pwsh -NoProfile -ExecutionPolicy Bypass -File <skill>/scripts/guidelines.ps1 <path>` (no `pwsh`: use `powershell`, Windows PowerShell 5.1)

It prints `go 1.XX` + the guidelines for that version. Read all of it, unfiltered. Never detect the version yourself; re-run per module.

- `go 1.XX`: say only "This project is using Go X.XX, so I'll use modern Go features up to that version. Let me know to target a different one."
- Otherwise: say "Could not detect Go version.", ask "Which Go version should I target?" [1.26] / [1.27], re-run with `--go X.Y` (PowerShell: `-Go X.Y`).

Apply every guideline; never use newer features. "details X" = section X of `<skill>/references/details.md`.

Modernizing existing code: if the installed toolchain (`go version`) is 1.26+, run `go fix` on its packages before reading their files (details "go fix"); otherwise apply the guidelines by hand.
