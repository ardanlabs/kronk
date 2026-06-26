package toolapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
)

// diagnose gathers the host diagnostic report used by the BUI Info screen by
// re-executing this same binary as "kronk diagnose --format json" and returning
// its JSON output unchanged.
//
// It shells out (instead of calling the diagnose package in-process) so the
// in-process engine load probe runs in a throwaway child process. That
// reproduces the server's real library-load path — catching failures that put
// the server in degraded mode — without ever re-initializing or mutating the
// running server. The child is the same compiled binary, located via
// os.Executable, so this works even when "kronk" is not installed on PATH (for
// example when the server is started with "go run").
//
// By default the benchmark step is skipped ("--no-bench") because it loads a
// model and would make this endpoint slow; it runs only when requested with
// "?bench=true". An optional "?model=" selects which installed model to
// benchmark.
func (a *app) diagnose(ctx context.Context, r *http.Request) web.Encoder {
	bench := r.URL.Query().Get("bench") == "true"
	model := r.URL.Query().Get("model")

	exe, err := os.Executable()
	if err != nil {
		return errs.New(errs.Internal, fmt.Errorf("locate executable: %w", err))
	}

	args := []string{"diagnose", "--format", "json"}
	if !bench {
		args = append(args, "--no-bench")
	}
	if model != "" {
		args = append(args, "--model", model)
	}

	// stdout carries the JSON report; stderr carries progress and is only used
	// to enrich an error if the command fails.
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return errs.New(errs.Internal, fmt.Errorf("run diagnose: %w: %s", err, stderr.String()))
	}

	// The child writes the JSON report to stdout. Validate it so a corrupted
	// stream (e.g. a library logging to stdout) yields a clear server error
	// rather than an opaque parse failure in the browser.
	if !json.Valid(stdout.Bytes()) {
		return errs.New(errs.Internal, fmt.Errorf("diagnose produced invalid JSON: %s", stderr.String()))
	}

	return diagnoseRaw(stdout.Bytes())
}

// diagnoseRaw is the JSON report produced by the diagnose subprocess, returned
// to the client unchanged.
type diagnoseRaw []byte

// Encode implements the web.Encoder interface.
func (d diagnoseRaw) Encode() ([]byte, string, error) {
	return d, "application/json", nil
}
