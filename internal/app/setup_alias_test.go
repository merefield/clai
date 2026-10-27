package app

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/merefield/clai/internal/config"
	"github.com/merefield/clai/internal/ui"
)

func TestSetupOffersOptionalCommandAlias(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("automatic shell profile setup is Unix-only")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("Bash is not installed")
	}
	for _, test := range []struct {
		name  string
		input string
		alias string
	}{
		{name: "skip", input: "\n"},
		{name: "end-of-input"},
		{name: "decline", input: "computer\nn\n"},
		{name: "install", input: "computer\ny\n", alias: "computer"},
		{name: "custom", input: "assistant\ny\n", alias: "assistant"},
		{name: "invalid-then-valid", input: "Computer\ncomputer\ny\n", alias: "computer"},
		{name: "conflict-then-valid", input: "cd\ncomputer\ny\n", alias: "computer"},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("SHELL", "/bin/bash")
			cfg, err := config.Load(filepath.Join(home, "clai.cfg"))
			if err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			application := &Application{Config: cfg, UI: ui.New(strings.NewReader("test-key\n\n\n\n"+test.input), &output, &output, true)}
			if err := application.Run(context.Background(), []string{"setup"}); err != nil {
				t.Fatal(err)
			}
			if saved, err := config.Load(cfg.Path); err != nil || saved.Key != "test-key" {
				t.Fatalf("provider settings were not saved: %v", err)
			}
			profile := ".bashrc"
			if runtime.GOOS == "darwin" {
				profile = ".bash_profile"
			}
			data, err := os.ReadFile(filepath.Join(home, profile))
			if test.alias == "" {
				if !os.IsNotExist(err) {
					t.Fatalf("skipped or declined setup changed the profile: %q, %v", data, err)
				}
			} else if err != nil || !strings.Contains(string(data), "alias "+test.alias+"=") || !strings.Contains(output.String(), "source ") {
				t.Fatalf("alias was not installed with activation instructions: %q, %v; output = %s", data, err, output.String())
			}
		})
	}
}

func TestSetupKeepsReplacesAndRemovesManagedAlias(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("automatic shell profile setup is Unix-only")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("Bash is not installed")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/bash")
	var output bytes.Buffer
	for _, input := range []string{"computer\ny\n", "\n", "computer\ny\n", "assistant\ny\n", "-\ny\n"} {
		application := &Application{UI: ui.New(strings.NewReader(input), &output, &output, true)}
		application.setupCommandAlias(context.Background())
		profile := ".bashrc"
		if runtime.GOOS == "darwin" {
			profile = ".bash_profile"
		}
		data, err := os.ReadFile(filepath.Join(home, profile))
		if err != nil {
			t.Fatal(err)
		}
		if input == "-\ny\n" {
			if len(data) != 0 {
				t.Fatalf("alias was not removed: %s", data)
			}
		} else if strings.Count(string(data), "# >>> clai command alias >>>") != 1 {
			t.Fatalf("expected one managed block: %s", data)
		}
		if input == "assistant\ny\n" && (strings.Contains(string(data), "computer") || !strings.Contains(string(data), "alias assistant=")) {
			t.Fatalf("alias was not replaced: %s", data)
		}
	}
}
