//go:build windows

package start

import (
	"os/exec"
	"syscall"
)

const (
	createNewProcessGroup = 0x00000200
	detachedProcess       = 0x00000008
)

// setDetachAttrs configures the process to run independently of the parent.
// On Windows, DETACHED_PROCESS detaches from the console, and
// CREATE_NEW_PROCESS_GROUP gives it its own process group.
func setDetachAttrs(proc *exec.Cmd) {
	proc.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNewProcessGroup | detachedProcess,
	}
}
