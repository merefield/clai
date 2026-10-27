package shellintegration

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func requireShell(t *testing.T, shell string) {
	t.Helper()
	if runtime.GOOS == "windows" && (shell == "bash" || shell == "zsh") {
		t.Skip("automatic shell profile integration is Unix-only")
	}
	if _, err := exec.LookPath(shell); err != nil {
		t.Skip(shell + " is not installed")
	}
}

func TestAliasPassesLiteralArgumentsAndQuotesExecutable(t *testing.T) {
	for _, shell := range []string{"bash", "zsh"} {
		t.Run(shell, func(t *testing.T) {
			requireShell(t, shell)
			directory := t.TempDir()
			executable := filepath.Join(directory, "clai's preview $binary")
			if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf '<%s>\\n' \"$@\"\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			initialization, err := Init(shell, "computer", executable)
			if err != nil {
				t.Fatal(err)
			}
			args := []string{"-f", "-c"}
			if shell == "bash" {
				args = []string{"--noprofile", "--norc", "-O", "expand_aliases", "-c"}
			}
			script := initialization + "\ncomputer show * ? 'two words' --question\n"
			if shell == "zsh" {
				// zsh parses a whole -c script before running alias definitions.
				script = initialization + "\neval " + Quote("computer show * ? 'two words' --question")
			}
			command := exec.Command(shell, append(args, script)...)
			command.Dir = directory
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("alias failed: %v: %s", err, output)
			}
			if want := "<show>\n<*>\n<?>\n<two words>\n<--question>\n"; string(output) != want {
				t.Fatalf("output = %q, want %q", output, want)
			}
		})
	}
}

func TestBashAliasPreservesExitStatusAndGlobbing(t *testing.T) {
	requireShell(t, "bash")
	for _, noGlob := range []bool{false, true} {
		t.Run(map[bool]string{false: "globbing-enabled", true: "globbing-disabled"}[noGlob], func(t *testing.T) {
			executable := filepath.Join(t.TempDir(), "clai")
			if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 7\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			initialization, err := Init("bash", "computer", executable)
			if err != nil {
				t.Fatal(err)
			}
			script := initialization + "\nset -e\n"
			if noGlob {
				script += "set -f\ntrap 'case $- in *f*) ;; *) exit 99 ;; esac' EXIT\n"
			} else {
				script += "trap 'case $- in *f*) exit 99 ;; esac' EXIT\n"
			}
			command := exec.Command("bash", "--noprofile", "--norc", "-O", "expand_aliases", "-c", script+"computer fail\n")
			output, err := command.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 7 {
				t.Fatalf("exit = %v, output = %q; want status 7 with original globbing restored", err, output)
			}
		})
	}
}

func TestInitRejectsUnsafeNamesAndUnsupportedShells(t *testing.T) {
	for _, name := range []string{"", "Computer", "two words", "foo;touch pwned", "$(id)", "-x", "foo\nbar", "foo'"} {
		if _, err := Init("bash", name, "clai"); err == nil {
			t.Errorf("accepted name %q", name)
		}
	}
	if _, err := Init("fish", "computer", "clai"); err == nil || !strings.Contains(err.Error(), "unsupported shell") {
		t.Fatalf("error = %v", err)
	}
}
