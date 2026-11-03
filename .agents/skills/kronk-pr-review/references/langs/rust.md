# Rust idioms

**Tools:** `rust-analyzer` via LSP if available, else `rg`; `cargo tree -i <crate>` for dependency questions. **Checks:** `cargo-fmt`, `cargo-audit`, `cargo-deny`; for a maintainer PR also `cargo-clippy` (`-D warnings`; `cargo-check` when clippy is missing) and `cargo-test` — compiling runs build scripts and proc macros, so they are skipped otherwise. Cargo runs with `--locked`: a failure saying the lock file needs updating is a real finding (`Cargo.lock` out of date).

## Correctness
Blocking in async (`std::fs`, `thread::sleep`, sync `Mutex` held across `.await`); `RefCell` borrow panics; `as` casts that truncate or change sign; integer overflow (wraps in release); `clone()` added only to silence the borrow checker on hot paths; `Send`/`Sync` assumptions; feature-flag and `#[cfg]` combinations that no longer compile; iterator invalidation via indices.

## Failure paths
`unwrap`/`expect`/`panic!`/indexing on library or request paths; `?` losing context (add `.context()` / typed errors); errors dropped with `let _ =` or `.ok()`; `#[must_use]` results ignored; `Drop` can't report errors — flush/sync files explicitly.

## Contracts
Public API and SemVer (removed/renamed `pub` items, changed signatures, new required trait bounds; `cargo-semver-checks` if installed, else Open check); `#[non_exhaustive]` on public enums/structs; newtypes and validating constructors (`TryFrom`) for invariants; `serde` attributes (`rename`, `default`, `deny_unknown_fields`, `untagged`) keep the wire format; feature-gated API.

## Tests
Unit tests in `#[cfg(test)]`, integration tests in `tests/`, doctests compile; error variants asserted with `matches!`; async tests (`#[tokio::test]`); property tests for parsers; `#[ignore]`/`#[should_panic]` hiding a real failure.

## Security
`unsafe` without a `// SAFETY:` invariant or with an unsound one; FFI boundaries; untrusted deserialization without size limits; `Command` via a shell; path traversal; non-constant-time secret comparison; RUSTSEC advisories (`cargo-audit`); licenses and sources (`cargo-deny`).

## Clarity
`///` docs on `pub` items with `# Errors`, `# Panics`, `# Safety` where they apply; `?` over nested `match`; iterator chains only while readable.
