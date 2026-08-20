package runner

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"unicode/utf8"

	"github.com/merefield/clai/internal/model"
)

const (
	// MaxCapturedStreamBytes bounds the memory retained for each command stream.
	// Command output is still streamed to the terminal without this limit.
	MaxCapturedStreamBytes = 256 * 1024
	// MaxSharedStreamBytes bounds each stream sent back to an LLM.
	MaxSharedStreamBytes = 64 * 1024
)

const truncationMarker = "[earlier output truncated]\n"

type Runner interface {
	Run(context.Context, string, bool) model.CommandResult
}

type Bash struct {
	Stdout io.Writer
	Stderr io.Writer
}

func (r Bash) Run(ctx context.Context, command string, edited bool) model.CommandResult {
	stdout := newTailWriter(MaxCapturedStreamBytes)
	stderr := newTailWriter(MaxCapturedStreamBytes)
	cmd := exec.CommandContext(ctx, "bash", "-o", "errexit", "-o", "pipefail", "-c", command)
	configureCommand(cmd)
	cmd.Stdout = io.MultiWriter(writerOrDiscard(r.Stdout), stdout)
	cmd.Stderr = io.MultiWriter(writerOrDiscard(r.Stderr), stderr)
	err := cmd.Run()
	// A successful shell can leave a background descendant holding its output
	// pipes open. WaitDelay closes those pipes and reports ErrWaitDelay; the
	// command itself still succeeded in that case.
	if errors.Is(err, exec.ErrWaitDelay) && cmd.ProcessState != nil && cmd.ProcessState.Success() {
		err = nil
	}
	exitCode := 0
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			exitCode = exit.ExitCode()
		} else {
			exitCode = 1
			if stderr.Len() == 0 {
				_, _ = stderr.Write([]byte(err.Error()))
			}
		}
	}
	return model.CommandResult{Command: command, ExitCode: exitCode, Stdout: stdout.String(), Stderr: stderr.String(), Edited: edited}
}

func TailLines(value string, maximum int) string {
	return TailOutput(value, maximum, 0)
}

// TailOutput keeps the newest complete lines and bytes from a captured stream.
// It preserves a single truncation notice even when the runner already bounded
// the captured value.
func TailOutput(value string, maximumLines, maximumBytes int) string {
	if maximumLines < 1 || value == "" {
		return ""
	}
	truncated := strings.HasPrefix(value, truncationMarker)
	value = strings.TrimPrefix(value, truncationMarker)
	value = strings.TrimSuffix(value, "\n")
	lines := strings.Split(value, "\n")
	if len(lines) > maximumLines {
		truncated = true
		value = strings.Join(lines[len(lines)-maximumLines:], "\n")
	}
	if maximumBytes > 0 {
		value = string(bytes.ToValidUTF8([]byte(value), []byte("\uFFFD")))
		allowance := maximumBytes
		if truncated {
			allowance -= len(truncationMarker)
		}
		if allowance < 0 {
			allowance = 0
		}
		if len(value) > allowance {
			truncated = true
			allowance = maximumBytes - len(truncationMarker)
			if allowance < 0 {
				allowance = 0
			}
			value = validUTF8Tail(value, allowance)
		}
	}
	if truncated {
		return truncationMarker + value
	}
	return value
}

type tailWriter struct {
	limit     int
	data      []byte
	truncated bool
}

func newTailWriter(limit int) *tailWriter {
	return &tailWriter{limit: limit}
}

func (w *tailWriter) Write(p []byte) (int, error) {
	written := len(p)
	if w.limit <= 0 {
		w.truncated = w.truncated || written > 0
		return written, nil
	}
	if len(p) >= w.limit {
		w.data = append(w.data[:0], p[len(p)-w.limit:]...)
		w.truncated = true
		return written, nil
	}
	if excess := len(w.data) + len(p) - w.limit; excess > 0 {
		copy(w.data, w.data[excess:])
		w.data = w.data[:len(w.data)-excess]
		w.truncated = true
	}
	w.data = append(w.data, p...)
	return written, nil
}

func (w *tailWriter) Len() int {
	return len(w.data)
}

func (w *tailWriter) String() string {
	value := validUTF8Tail(string(w.data), w.limit)
	if w.truncated {
		return truncationMarker + value
	}
	return value
}

func validUTF8Tail(value string, maximum int) string {
	if maximum <= 0 {
		return ""
	}
	valid := bytes.ToValidUTF8([]byte(value), []byte("\uFFFD"))
	if len(valid) <= maximum {
		return string(valid)
	}
	start := len(valid) - maximum
	for start < len(valid) && !utf8.RuneStart(valid[start]) {
		start++
	}
	return string(valid[start:])
}

func writerOrDiscard(writer io.Writer) io.Writer {
	if writer == nil {
		return io.Discard
	}
	return writer
}
