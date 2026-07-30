# Contributing to Kronk

Thank you for helping improve Kronk. Keep pull requests focused, explain the
behavioral reason for a change, and include tests where behavior changes.

## Setup

1. Fork and clone the repository.
2. Install the exact Go version recorded in `.go-version`; that file is the
   authoritative toolchain source.
3. Download the root and example modules:

   ```sh
   go mod download
   (cd examples && go mod download)
   ```
4. Install the development tools when needed with `make install-gotooling`.

The [developer guide](.manual/chapter-19-developer-guide.md) describes the
repository ownership boundaries and focused verification commands. Applicable
`AGENTS.md` files contain additional instructions for coding agents.

Pure unit tests do not require models, but inference and server integration
tests require native libraries and specific model files. Install the supported
llama.cpp and whisper.cpp bundles with:

```sh
make install-kronk
kronk libs --local
kronk bucky libs --local
```

The test model sets are maintained in `.make/install.mk` and
`.github/test-models.txt`. `make install-test-models` installs the local test
set; `make install-test-gh-models` installs the smaller CI set. These assets are
large and are not committed.

## Validate changes

Run the narrowest tests and static checks that exercise the changed package.
Go tests require these environment variables:

```sh
export RUN_IN_PARALLEL=yes
export GITHUB_WORKSPACE="$(pwd -P)"

go fix ./path/to/changed/package
gofmt -s -w path/to/changed.go
go vet ./path/to/changed/package
staticcheck ./path/to/changed/package
go build ./path/to/changed/package
go test -count=1 ./path/to/changed/package
```

For maintainers running broad integration checks, `make test` installs the
latest native bundles and local test models before running the project checks.
`make test-gh` uses the well-known native versions and smaller model set used by
CI. Both commands download large assets and can take substantial time. Do not
hide diagnostics or commit downloaded models, native bundles, logs, profiles,
or build outputs.

For BUI changes, use the committed npm lockfile:

```sh
cd cmd/server/api/frontends/bui
npm ci
npm run build
npm run dev  # local development server
```

When manual or generated SDK documentation changes, run `make kronk-docs` and
commit the authored source together with its generated BUI output.

## Pull requests and releases

- Prefer one coherent change over broad refactoring.
- Preserve model/context ownership, sequence isolation, cancellation, bounded
  admission, and unload guarantees.
- Preserve shared RAM/VRAM accounting when changing model pools or loaders.
- Call out Metal, CUDA, ROCm, Vulkan, CPU, and native ABI effects explicitly.
- Update tests and user documentation together with changed behavior.
- Regenerate derived documentation or BUI assets when their source changes.
- Avoid unrelated dependency or formatting churn.

Maintainers assign versions, create tags, and publish releases. Contributors
should describe compatibility or breaking-change implications, but should not
bump release versions or regenerate release artifacts unless a maintainer asks
for it. Native ABI updates should identify the compatible llama.cpp/yzma or
whisper.cpp/Bucky versions and any platform, provenance, or checksum
implications.
