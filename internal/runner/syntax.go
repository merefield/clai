package runner

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// CheckSyntax returns a Bash diagnostic for rejected syntax and an error if
// the check itself could not complete. It never uses the runner's terminal.
func (Bash) CheckSyntax(ctx context.Context, command string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// Privileged mode ignores startup hooks (including BASH_ENV) and
	// imported shell functions and options. No inherited input is needed.
	cmd := exec.CommandContext(ctx, "bash", "--noprofile", "--norc", "-p", "-n", "-c", command)
	cmd.WaitDelay = commandWaitDelay
	stderr := newTailWriter(4096)
	cmd.Stderr = stderr
	err := cmd.Run()
	if err == nil {
		return "", nil
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	var status *exec.ExitError
	if errors.As(err, &status) && status.ExitCode() == 2 {
		if diagnostic := strings.TrimSpace(stderr.String()); diagnostic != "" {
			return diagnostic, nil
		}
		return "Bash rejected the command syntax.", nil
	}
	return "", fmt.Errorf("start or run Bash parser: %w", err)
}
