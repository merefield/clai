package shellintegration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestProfileUpdatesOnlyManagedBlock(t *testing.T) {
	profile := Profile{Shell: "bash", Path: filepath.Join(t.TempDir(), ".bashrc")}
	prefix := "# my settings\nexport EDITOR=vim\n"
	suffix := "# later settings\nexport PAGER=less\n"
	if err := os.WriteFile(profile.Path, []byte(prefix), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := profile.Save("computer", "/opt/clai"); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(profile.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := profile.Save("computer", "/opt/clai"); err != nil {
		t.Fatal(err)
	}
	repeated, err := os.ReadFile(profile.Path)
	if err != nil || string(repeated) != string(first) {
		t.Fatalf("repeated setup changed the profile: %v", err)
	}
	if err := os.WriteFile(profile.Path, append(first, []byte(suffix)...), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := profile.Save("helper", "/opt/new-clai"); err != nil {
		t.Fatal(err)
	}
	updated, err := os.ReadFile(profile.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(updated), prefix) || !strings.HasSuffix(string(updated), suffix) || strings.Contains(string(updated), "computer") || strings.Count(string(updated), blockStart) != 1 {
		t.Fatalf("unexpected profile after rename: %s", updated)
	}
	if name, err := profile.CurrentName(); err != nil || name != "helper" {
		t.Fatalf("current name = %q, %v", name, err)
	}
	if info, err := os.Stat(profile.Path); err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o640) {
		t.Fatalf("profile permissions were not preserved: %v", err)
	}
	if err := profile.Save("", ""); err != nil {
		t.Fatal(err)
	}
	removed, err := os.ReadFile(profile.Path)
	if err != nil || string(removed) != prefix+suffix {
		t.Fatalf("removing alias changed other settings: %q, %v", removed, err)
	}
}

func TestProfileAppendsAfterMissingNewline(t *testing.T) {
	profile := Profile{Shell: "bash", Path: filepath.Join(t.TempDir(), ".bashrc")}
	if err := os.WriteFile(profile.Path, []byte("# my settings"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := profile.Save("computer", "/opt/clai"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(profile.Path)
	if err != nil || !strings.HasPrefix(string(data), "# my settings\n"+blockStart) {
		t.Fatalf("alias was not added on a new line: %q, %v", data, err)
	}
}

func TestProfilePreservesSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks may require privileges on Windows")
	}
	directory := t.TempDir()
	target := filepath.Join(directory, "dotfiles", "bashrc")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("# linked profile\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	profile := Profile{Shell: "bash", Path: filepath.Join(directory, ".bashrc")}
	if err := os.Symlink(target, profile.Path); err != nil {
		t.Fatal(err)
	}
	if err := profile.Save("computer", "/opt/clai"); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(profile.Path); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("profile symlink was replaced: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil || !strings.Contains(string(data), "alias computer=") {
		t.Fatalf("symlink target was not updated: %q, %v", data, err)
	}
}

func TestProfileRefusesMalformedManagedBlocks(t *testing.T) {
	for _, content := range []string{
		blockStart + "\n",
		blockEnd + "\n",
		blockEnd + "\n" + blockStart + "\n",
		blockStart + "\n" + blockEnd + "\n" + blockStart + "\n" + blockEnd + "\n",
		"# other text " + blockStart + "\n" + blockEnd + "\n",
		blockStart + "\n" + blockEnd + " other text\n",
	} {
		profile := Profile{Shell: "bash", Path: filepath.Join(t.TempDir(), ".bashrc")}
		if err := os.WriteFile(profile.Path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := profile.Save("computer", "/opt/clai"); err == nil {
			t.Fatalf("accepted malformed block: %q", content)
		}
		data, err := os.ReadFile(profile.Path)
		if err != nil || string(data) != content {
			t.Fatalf("malformed profile was changed: %q, %v", data, err)
		}
	}
}

func TestConflictChecksIncludeShellDefinitionsAndIgnoreManagedAlias(t *testing.T) {
	for _, shell := range []string{"bash", "zsh"} {
		t.Run(shell, func(t *testing.T) {
			requireShell(t, shell)
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("ZDOTDIR", home)
			profile := Profile{Shell: shell, Path: filepath.Join(home, "."+shell+"rc")}
			for _, definition := range []string{"alias computer='echo other'\n", "computer() { :; }\n", ""} {
				if err := os.WriteFile(profile.Path, []byte(definition), 0o600); err != nil {
					t.Fatal(err)
				}
				err := profile.CheckConflict(context.Background(), "computer")
				if (err != nil) != (definition != "") {
					t.Fatalf("definition %q: error = %v", definition, err)
				}
			}
			if err := profile.Save("computer", "/opt/clai"); err != nil {
				t.Fatal(err)
			}
			if err := profile.CheckConflict(context.Background(), "computer"); err != nil {
				t.Fatalf("managed alias blocked repeated setup: %v", err)
			}
			for _, name := range []string{"cd", "if", "clai"} {
				if err := profile.CheckConflict(context.Background(), name); err == nil {
					t.Errorf("accepted existing builtin, keyword or canonical command %q", name)
				}
			}
			bin := t.TempDir()
			if err := os.WriteFile(filepath.Join(bin, "computer"), []byte("#!/bin/sh\necho existing-command\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			if err := profile.CheckConflict(context.Background(), "computer"); err == nil {
				t.Fatal("accepted conflicting executable")
			}
			command := exec.Command(shell, "-ic", "computer")
			output, err := command.Output()
			if err != nil || !strings.HasSuffix(string(output), "existing-command\n") {
				t.Fatalf("managed alias shadowed a new conflict: %q, %v", output, err)
			}
		})
	}
}

func TestDetectProfile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Setenv("CLAI_SHELL", "fish")
		if _, err := DetectProfile(context.Background()); err == nil {
			t.Fatal("automatic profile setup should reject unsupported Windows shells")
		}
		return
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/bash")
	t.Setenv("CLAI_SHELL", "/bin/bash")
	file := ".bashrc"
	if runtime.GOOS == "darwin" {
		file = ".bash_profile"
	}
	if profile, err := DetectProfile(context.Background()); err != nil || profile.Path != filepath.Join(home, file) {
		t.Fatalf("Bash profile = %+v, %v", profile, err)
	}
	t.Setenv("SHELL", "/bin/fish")
	t.Setenv("CLAI_SHELL", "/bin/fish")
	if _, err := DetectProfile(context.Background()); err == nil {
		t.Fatal("automatic profile setup should be unavailable for fish")
	}
}

func TestDetectZshProfileIncludesZshenv(t *testing.T) {
	requireShell(t, "zsh")
	shell, err := exec.LookPath("zsh")
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	directory := filepath.Join(home, "custom zsh")
	t.Setenv("HOME", home)
	t.Setenv("SHELL", shell)
	t.Setenv("CLAI_SHELL", shell)
	t.Setenv("ZDOTDIR", "")
	if err := os.Unsetenv("ZDOTDIR"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".zshenv"), []byte("ZDOTDIR="+Quote(directory)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if profile, err := DetectProfile(context.Background()); err != nil || profile.Path != filepath.Join(directory, ".zshrc") {
		t.Fatalf("zsh profile = %+v, %v", profile, err)
	}
}
