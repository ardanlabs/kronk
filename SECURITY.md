# Security Policy

## Reporting a vulnerability

Please do not open a public issue for a suspected vulnerability. Use this
repository's **Security** tab and GitHub private vulnerability reporting to
send the maintainers a report. Include affected versions, reproduction steps,
impact, and any suggested mitigation. The maintainers will coordinate
validation and disclosure through that private report. If private vulnerability
reporting is unavailable, report the issue privately to
[Bill Kennedy](mailto:bill@ardanlabs.com).

## Supported versions

Kronk is evolving and does not currently promise a fixed long-term-support
window. Security fixes are made on the default branch and, when practical, in
the latest released version. Users should run the newest available release and
check release notes when upgrading. Older releases may not receive backports.

## Trust boundaries

Kronk dynamically loads native llama.cpp and whisper.cpp libraries. The active
locations can be overridden with `KRONK_LIB_PATH` and
`KRONK_BUCKY_LIB_PATH`. A native bundle executes with the privileges of the
Kronk process. GGUF models, multimodal projections, Whisper models, and media
uploads are complex, externally supplied inputs processed in part by native
code. Obtain libraries and models from trusted sources, restrict who can
replace them, and avoid processing files supplied by untrusted users without
appropriate isolation.

Kronk's installers obtain native bundles associated with the upstream
[`ggml-org/llama.cpp`](https://github.com/ggml-org/llama.cpp) and
[`ggml-org/whisper.cpp`](https://github.com/ggml-org/whisper.cpp) projects
through the yzma and Bucky integrations. Upstream provenance and a successful
download are not substitutes for a project-specific security audit. Pin known
compatible versions and independently verify provenance and checksums where
your deployment requires it.

## Network deployment

Kronk listens on `0.0.0.0:11435` and serves plain HTTP with authentication
disabled by default. The debug service also binds to all interfaces on port
`11445` by default. Do not expose either listener directly to the public
internet or an untrusted network. Bind to loopback or a trusted private
interface, restrict access with host or cloud firewall rules, and terminate TLS
at a trusted reverse proxy.

Kronk supports JWT authentication for inference and administration. Enable
full authentication with `--auth-enabled`, or protect administration with
`--admin-auth-enabled`, before remote use. Restrict CORS origins, issue
short-lived least-privilege tokens, configure appropriate quotas and proxy
request limits, and restrict model-management operations. See
[Chapter 12](.manual/chapter-12-security-authentication.md) for the complete
hardening guidance.

Run Kronk as a dedicated least-privileged account with narrowly writable model,
library, configuration, and key directories. Protect `master.pem`,
`master.jwt`, application tokens, and other credentials. Do not enable
insecure prompt logging in production; logs and diagnostic profiles may contain
sensitive request data.
