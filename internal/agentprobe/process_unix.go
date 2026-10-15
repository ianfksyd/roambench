//go:build unix

package agentprobe

import (
	"context"
	"os/exec"
	"syscall"
)

// configureProcessGroup puts the version-command child in its own process
// group so that a context timeout can kill the whole tree (some stubs/CLIs
// spawn sleep-like children that would otherwise hold the output pipes open
// past the deadline).
func configureProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			// Negative PID = every process in the child's group.
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return context.Canceled
	}
}
