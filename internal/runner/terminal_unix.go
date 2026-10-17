//go:build unix

package runner

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

func prepareTerminal(cmd *exec.Cmd, input *os.File) (func() error, error) {
	noop := func() error { return nil }
	if input == nil || !term.IsTerminal(int(input.Fd())) {
		return noop, nil
	}
	fd := int(input.Fd())
	group, err := unix.IoctlGetInt(fd, unix.TIOCGPGRP)
	if err != nil {
		return nil, fmt.Errorf("read terminal foreground group: %w", err)
	}
	if group != syscall.Getpgrp() {
		return nil, fmt.Errorf("CLAI must own the foreground terminal before running an interactive command")
	}
	state, err := term.GetState(fd)
	if err != nil {
		return nil, fmt.Errorf("read terminal state: %w", err)
	}
	// The child sets its foreground group before exec, avoiding a race where
	// its first terminal read could receive SIGTTIN.
	cmd.SysProcAttr.Foreground = true
	// Foreground uses a parent fd before stdin remapping, unlike Setctty.
	cmd.SysProcAttr.Ctty = fd
	return func() error {
		// Reclaiming the terminal from a background group can raise SIGTTOU.
		// Ignore it at the OS level: a handler is insufficient for an orphaned
		// background group. CLAI does not otherwise subscribe to SIGTTOU.
		ignored := signal.Ignored(syscall.SIGTTOU)
		signal.Ignore(syscall.SIGTTOU)
		if !ignored {
			defer signal.Reset(syscall.SIGTTOU)
		}
		groupErr := unix.IoctlSetPointerInt(fd, unix.TIOCSPGRP, group)
		stateErr := term.Restore(fd, state)
		if err := errors.Join(groupErr, stateErr); err != nil {
			return fmt.Errorf("restore terminal: %w", err)
		}
		return nil
	}, nil
}
