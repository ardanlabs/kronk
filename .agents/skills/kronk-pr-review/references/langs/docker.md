# Dockerfile / Compose idioms

**Tools:** `rg`. **Checks:** `hadolint` (Dockerfiles), `iac-scan`. No image is built or run by the checks.

## Correctness
Base image pinned by tag and digest; multi-stage builds copying the right artifacts; layer order vs build cache; `.dockerignore` keeping secrets and build output out of the context; exec-form `ENTRYPOINT`/`CMD` (signals reach PID 1); `apt-get update` and `install` in one `RUN` with `--no-install-recommends` and cleanup; `ARG` vs `ENV` scope. Compose: `depends_on` with health conditions, restart policies, volume and port mappings, env files.

## Failure paths
`SHELL ["/bin/sh", "-o", "pipefail", "-c"]` (or bash) when a `RUN` pipes; `HEALTHCHECK`; entrypoint scripts with `set -eu` and real exit codes.

## Contracts
Kronk images (`zarf/docker`) agree with the `.github/workflows/docker.yml` variant matrix and the entrypoint, with the native libraries each variant needs. Exposed ports, environment variable names, volume paths, entrypoint arguments, and the image name/tag scheme are the image's API for deployments and Compose files.

## Tests
Container structure tests or a CI build; otherwise name the manual check (build, run, hit the health endpoint).

## Security
`USER` non-root; no secrets in `ARG`/`ENV`/`COPY` or layers (use build secrets); `ADD` from URLs; `curl | sh`; `latest` tags; minimal base; setuid binaries; Compose `privileged`, host network, or Docker socket mounts.

## Clarity
Comments explain non-obvious build steps; labels (`org.opencontainers.image.*`) are accurate.
