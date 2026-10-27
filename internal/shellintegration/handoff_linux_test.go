package shellintegration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// A real controlling terminal exercises interactive-only shell integration.
func runHandoffShell(t *testing.T, shell, script, temporary string) string {
	t.Helper()
	fd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	master := os.NewFile(uintptr(fd), "handoff-pty")
	defer master.Close()
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		t.Fatal(err)
	}
	number, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", number), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer slave.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	args := []string{"-f", "-i", "-c", script}
	if shell == "bash" {
		args = []string{"--noprofile", "--norc", "-i", "-c", script}
	}
	command := exec.CommandContext(ctx, shell, args...)
	command.Env = append(os.Environ(), "TMPDIR="+temporary, "HISTFILE=/dev/null")
	command.Stdin, command.Stdout, command.Stderr = slave, slave, slave
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	slave.Close()
	output, readErr := io.ReadAll(master)
	if readErr != nil && !errors.Is(readErr, syscall.EIO) {
		t.Fatal(readErr)
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("handoff shell failed: %v: %s", err, output)
	}
	return string(output)
}

func TestInteractiveShellHandoffQueuesTextWithoutExecuting(t *testing.T) {
	for _, shell := range []string{"bash", "zsh"} {
		for _, test := range []struct {
			name, file         string
			status, wantStatus int
			queued             bool
		}{
			{"ordinary-success", "data", 0, 0, false},
			{"failure", "data", 7, 7, false},
			{"explicit-handoff", "data", HandoffExitCode, 0, true},
			{"missing-file", "missing", HandoffExitCode, 1, false},
			{"empty-file", "empty", HandoffExitCode, 1, false},
			{"symlink", "symlink", HandoffExitCode, 1, false},
		} {
			t.Run(shell+"/"+test.name, func(t *testing.T) {
				requireShell(t, shell)
				directory := t.TempDir()
				marker := filepath.Join(directory, "must-not-execute")
				pending := "touch " + Quote(marker) + ` && printf '%s' 'literal $HOME \backslash'`
				fake := filepath.Join(directory, "clai")
				// A command that would create a file if the wrapper evaluated it.
				program := "#!/bin/sh\n"
				switch test.file {
				case "data":
					program += "printf '%s' " + Quote(pending) + " > \"$CLAI_HANDOFF_FILE\"\n"
				case "empty":
					program += ": > \"$CLAI_HANDOFF_FILE\"\n"
				case "symlink":
					target := filepath.Join(directory, "source")
					if err := os.WriteFile(target, []byte(pending), 0o600); err != nil {
						t.Fatal(err)
					}
					program += "ln -s " + Quote(target) + " \"$CLAI_HANDOFF_FILE\"\n"
				}
				program += "exit " + fmt.Sprint(test.status) + "\n"
				if err := os.WriteFile(fake, []byte(program), 0o700); err != nil {
					t.Fatal(err)
				}
				initialization, err := Init(shell, "computer", fake)
				if err != nil {
					t.Fatal(err)
				}
				inspect := "history 1"
				if shell == "zsh" {
					inspect = `IFS= read -rz _pending; print -r -- "$_pending"`
				}
				// eval is used only to parse the known alias after defining it.
				// The handed-off text is never evaluated by this test or wrapper.
				script := initialization + "\neval 'computer request'\nprintf 'CLAI_STATUS=%s\\n' \"$?\"\n" + inspect + "\nprintf 'SHELL_RETURNED\\n'\n"
				output := runHandoffShell(t, shell, script, directory)
				if strings.Contains(output, pending) != test.queued || !strings.Contains(output, "SHELL_RETURNED") || !strings.Contains(output, fmt.Sprintf("CLAI_STATUS=%d", test.wantStatus)) {
					t.Fatalf("queued text changed or shell did not return: %q", output)
				}
				if _, err := os.Stat(marker); !os.IsNotExist(err) {
					t.Fatalf("pending command was executed: %v", err)
				}
				entries, err := os.ReadDir(directory)
				wantFiles := 1
				if test.file == "symlink" {
					wantFiles++
				}
				if err != nil || len(entries) != wantFiles || entries[0].Name() != "clai" {
					t.Fatalf("temporary handoff files not cleaned up: %v, %v", entries, err)
				}
			})
		}
	}
}
