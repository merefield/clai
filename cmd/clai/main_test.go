package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/merefield/clai/internal/app"
)

func TestRootCommandVersion(t *testing.T) {
	command := newRootCommand(context.Background())
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"--version"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "clai version "+app.Version+"\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestRootCommandShellInit(t *testing.T) {
	command := newRootCommand(context.Background())
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"shell-init", "bash"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "alias clai=") || !strings.Contains(output.String(), "set -f") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestRootCommandRejectsInvalidShellInit(t *testing.T) {
	command := newRootCommand(context.Background())
	command.SetArgs([]string{"shell-init"})
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "usage: clai shell-init") {
		t.Fatalf("error = %v", err)
	}
}
