package runner

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"

	"github.com/merefield/clai/internal/model"
)

type Runner interface {
	Run(context.Context, string, bool) model.CommandResult
}

type Bash struct {
	Stdout io.Writer
	Stderr io.Writer
}

func (r Bash) Run(ctx context.Context, command string, edited bool) model.CommandResult {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "bash", "-o", "errexit", "-o", "pipefail", "-c", command)
	cmd.Stdout = io.MultiWriter(writerOrDiscard(r.Stdout), &stdout)
	cmd.Stderr = io.MultiWriter(writerOrDiscard(r.Stderr), &stderr)
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			exitCode = exit.ExitCode()
		} else {
			exitCode = 1
			if stderr.Len() == 0 {
				stderr.WriteString(err.Error())
			}
		}
	}
	return model.CommandResult{Command: command, ExitCode: exitCode, Stdout: stdout.String(), Stderr: stderr.String(), Edited: edited}
}

func TailLines(value string, maximum int) string {
	if maximum < 1 || value == "" {
		return ""
	}
	value = strings.TrimSuffix(value, "\n")
	lines := strings.Split(value, "\n")
	if len(lines) <= maximum {
		return value
	}
	return "[truncated to last " + intString(maximum) + " lines]\n" + strings.Join(lines[len(lines)-maximum:], "\n")
}

func intString(value int) string {
	const digits = "0123456789"
	if value == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for value > 0 {
		i--
		b[i] = digits[value%10]
		value /= 10
	}
	return string(b[i:])
}

func writerOrDiscard(writer io.Writer) io.Writer {
	if writer == nil {
		return io.Discard
	}
	return writer
}
