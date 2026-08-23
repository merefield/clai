//go:build windows

package runner

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
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
		// Process.Kill only terminates bash itself on Windows. taskkill /T
		// follows the process tree so commands spawned by bash do not survive
		// context cancellation.
		if err := exec.Command("taskkill", "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F").Run(); err == nil {
			return nil
		}
		if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return err
		}
		return nil
	}
	cmd.WaitDelay = commandWaitDelay
}
