//go:build !unix

package agentprobe

import (
	"context"
	"os/exec"
)

// configureProcessGroup is a no-op on platforms without process groups;
// exec.CommandContext still kills the direct child on cancellation.
func configureProcessGroup(cmd *exec.Cmd) {
	_ = context.Canceled // keep the context import meaningful on all platforms
}
