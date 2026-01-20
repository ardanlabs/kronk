//go:build !windows

package start

import (
	"os/exec"
	"syscall"
)

// setDetachAttrs configures the process to run independently of the parent.
// On Unix, Setsid creates a new session so the process survives terminal close.
func setDetachAttrs(proc *exec.Cmd) {
	proc.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
}
