//go:build windows

package runner

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

const commandWaitDelay = 250 * time.Millisecond

func configureCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		return cmd.Process.Kill()
	}
	cmd.WaitDelay = commandWaitDelay
}
