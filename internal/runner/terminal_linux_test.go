package runner

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/merefield/clai/internal/ui"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

func TestTerminalHelper(t *testing.T) {
	mode := os.Getenv("CLAI_TERMINAL_TEST")
	if mode == "" {
		return
	}
	console := ui.New(os.Stdin, os.Stdout, os.Stderr, false)
	choice, err := console.Choice("CONFIRM:")
	if err != nil || choice != "y" {
		t.Fatalf("confirmation %q: %v", choice, err)
	}
	before, err := term.GetState(0)
	if err != nil {
		t.Fatal(err)
	}
	runner := Bash{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	command := `printf 'CHILD_READY\n'; IFS= read -r answer; printf 'ANSWER:%s\n' "$answer"; stty -echo`
	if mode == "cancel" {
		ctx, cancel = context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		command = `stty -echo; printf 'CHILD_READY\n'; IFS= read -r answer`
	}
	if mode == "interrupt" {
		command = `printf 'CHILD_READY\n'; sleep 30`
	}
	if mode == "eof" {
		command = `printf 'CHILD_READY\n'; if IFS= read -r answer; then exit 2; fi`
	}
	if mode == "start-failure" {
		t.Setenv("PATH", "")
	}
	result := runner.Run(ctx, command, false)
	if (mode == "cancel" || mode == "interrupt" || mode == "start-failure") && result.ExitCode == 0 {
		t.Fatal("cancelled command succeeded")
	}
	if (mode == "reply" || mode == "eof") && result.ExitCode != 0 {
		t.Fatalf("command failed: %#v", result)
	}
	group, err := unix.IoctlGetInt(0, unix.TIOCGPGRP)
	if err != nil || group != syscall.Getpgrp() {
		t.Fatalf("foreground not restored: %d %v", group, err)
	}
	after, err := term.GetState(0)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("terminal state not restored: %v", err)
	}
	answer, err := console.Prompt("RETURNED:")
	if err != nil || answer != "next" {
		t.Fatalf("next prompt %q: %v", answer, err)
	}
	fmt.Println("HANDOFF_OK")
}

func TestTerminalHandoff(t *testing.T) {
	for _, mode := range []string{"reply", "cancel", "interrupt", "eof", "start-failure"} {
		t.Run(mode, func(t *testing.T) {
			fd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
			if err != nil {
				t.Fatal(err)
			}
			master := os.NewFile(uintptr(fd), "pty-master")
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
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestTerminalHelper$", "-test.timeout=8s")
			cmd.Env = append(os.Environ(), "CLAI_TERMINAL_TEST="+mode)
			cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
			cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			slave.Close()
			// Send both lines together to catch prompt read-ahead stealing input.
			input := "y\n"
			if mode == "reply" {
				input += "yes\n"
			}
			if _, err := master.WriteString(input); err != nil {
				t.Fatal(err)
			}
			done := make(chan string, 1)
			go func() {
				reader := bufio.NewReader(master)
				var output strings.Builder
				ready, returned := false, false
				for {
					b, err := reader.ReadByte()
					if err != nil {
						break
					}
					output.WriteByte(b)
					if !ready && strings.Contains(output.String(), "CHILD_READY\r\n") {
						ready = true
						if mode == "interrupt" {
							master.Write([]byte{3})
						}
						if mode == "eof" {
							master.Write([]byte{4})
						}
					}
					if !returned && strings.Contains(output.String(), "RETURNED:") {
						returned = true
						master.WriteString("next\n")
					}
				}
				done <- output.String()
			}()
			err = cmd.Wait()
			master.Close()
			output := <-done
			if err != nil || !strings.Contains(output, "HANDOFF_OK") {
				t.Fatalf("helper: %v\n%s", err, output)
			}
			if mode == "reply" && !strings.Contains(output, "ANSWER:yes") {
				t.Fatalf("lost child input: %s", output)
			}
		})
	}
}
