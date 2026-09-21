# Native Windows Tasks

The scripts in this directory are native PowerShell equivalents of targets in
`.make/`. They require neither GNU Make nor a Unix shell.

Run a target from the repository root using its script path:

```powershell
.\.power\install\install-kronk.ps1
.\.power\server\bui-install.ps1
.\.power\server\kronk-build.ps1
```

Each subdirectory corresponds to the similarly named `.make/*.mk` file, and
each script has the same name as the Make target it implements. Scripts with a
leading underscore are shared implementation details, not targets.
