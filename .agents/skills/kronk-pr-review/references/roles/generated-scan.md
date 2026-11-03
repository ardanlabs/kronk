# Generated-code secret scan (opt-in)

Generated, vendored, and lockfile changes are not reviewed unless the user opted in (`generated=included` in `scope.txt` — then this agent doesn't run). This agent checks one thing in them — a secret baked into build output, bundles, embedded config, or vendored fixtures — and runs separately so those large files never enter anyone else's context.

**Inputs:** `<run>/context/generated.patch`, `skipped.tsv`, `tools.tsv`. Write only `<run>/lenses/generated-scan.md`.

## Method — never read the patch whole

1. **Scanner, if installed.** `gitleaks stdin -v --no-banner --redact < <run>/context/generated.patch` (older gitleaks: `gitleaks detect -v --no-git --no-banner --redact --source <that file>`), else `trufflehog filesystem --no-update <skipped paths that exist>`.
2. **Otherwise, a pattern sweep** of added lines (`^\+`) with `rg -n --max-columns 300`:
   - private keys `-----BEGIN [A-Z ]*PRIVATE KEY-----`
   - cloud keys `AKIA[0-9A-Z]{16}`, `AIza[0-9A-Za-z_-]{35}`
   - tokens `gh[pousr]_[0-9A-Za-z]{36}`, `xox[abpr]-`, `sk-[A-Za-z0-9]{20,}`, JWTs `eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.`
   - credentials in URLs `[a-z]+://[^/\s:@]+:[^/\s@]+@`
   - quoted literals assigned to `password|secret|token|api[_-]?key`
3. **Confirm each hit** from a few lines around it (`rg -n -C 3`). Drop placeholders, obviously fake fixtures, and public client identifiers, with a one-line reason.

## Report

Use the contract's report format with `Lens: Security Sentinel (generated code)`. A real credential in a committed artifact is **G4**; Patch direction: rotate it, remove it, and fix the build input that injected it. Under Open checks, record the method used and anything the scan couldn't cover (binary files).

## Reply

```
generated-scan: G<n> — G4:<a> G3:<b> G2:<c> G1:<d> — open:<e> — <run>/lenses/generated-scan.md
```
