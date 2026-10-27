package shellintegration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	blockStart = "# >>> clai command alias >>>"
	blockEnd   = "# <<< clai command alias <<<"
	probeEnv   = "_CLAI_CHECK_COMMAND_ALIAS"
)

type Profile struct {
	Shell   string
	Path    string
	login   bool
	command string
}

func DetectProfile(ctx context.Context) (Profile, error) {
	if runtime.GOOS == "windows" {
		return Profile{}, fmt.Errorf("automatic command alias setup supports Bash and zsh on Linux and macOS")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Profile{}, err
	}
	profile := Profile{Shell: filepath.Base(os.Getenv("SHELL")), command: os.Getenv("SHELL")}
	switch profile.Shell {
	case "bash":
		profile.Path = filepath.Join(home, ".bashrc")
		if runtime.GOOS == "darwin" {
			profile.login = true
			profile.Path = filepath.Join(home, ".bash_profile")
			for _, name := range []string{".bash_profile", ".bash_login", ".profile"} {
				path := filepath.Join(home, name)
				if _, err := os.Stat(path); err == nil {
					profile.Path = path
					break
				} else if !errors.Is(err, os.ErrNotExist) {
					return Profile{}, err
				}
			}
		}
		return profile, nil
	case "zsh":
		// ZDOTDIR may be set in .zshenv without being exported to CLAI.
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, profile.command, "-c", `printf '\000%s\000' "${ZDOTDIR:-$HOME}"`)
		command.WaitDelay = time.Second
		output, err := command.Output()
		if err != nil {
			return Profile{}, fmt.Errorf("could not find zsh's profile directory: %w", err)
		}
		parts := strings.Split(string(output), "\x00")
		if len(parts) < 3 || !filepath.IsAbs(parts[len(parts)-2]) {
			return Profile{}, fmt.Errorf("could not find an absolute zsh profile directory")
		}
		profile.Path = filepath.Join(parts[len(parts)-2], ".zshrc")
		return profile, nil
	default:
		return Profile{}, fmt.Errorf("automatic command alias setup supports Bash and zsh; SHELL is %q", os.Getenv("SHELL"))
	}
}

// CheckConflict starts an interactive shell to include aliases, functions,
// builtins, keywords and PATH commands. Our managed block skips itself during
// this check, allowing setup to update its own alias on subsequent runs.
func (p Profile) CheckConflict(ctx context.Context, name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	if name == "clai" {
		return fmt.Errorf("clai is already the primary command; choose another name")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	shell := p.command
	if shell == "" {
		shell = p.Shell
	}
	flags := "-ic"
	if p.login {
		flags = "-lic"
	}
	command := exec.CommandContext(ctx, shell, flags, `if command -v "$1" >/dev/null 2>&1; then exit 10; fi`, "clai-alias-check", name)
	command.Env = append(os.Environ(), probeEnv+"=1")
	command.Stdout, command.Stderr = io.Discard, io.Discard
	command.WaitDelay = time.Second
	err := command.Run()
	if ctx.Err() != nil {
		return fmt.Errorf("could not check existing shell commands: %w", ctx.Err())
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 10 {
		return fmt.Errorf("%q is already a command, alias, function or shell keyword; choose another name", name)
	}
	if err != nil {
		return fmt.Errorf("could not check existing shell commands: %w", err)
	}
	return nil
}

func (p Profile) CurrentName() (string, error) {
	data, _, _, err := p.read()
	if err != nil {
		return "", err
	}
	start, end, err := blockRange(string(data))
	if err != nil || start == end {
		return "", err
	}
	for _, line := range strings.Split(string(data[start:end]), "\n") {
		if name, ok := strings.CutPrefix(line, "# Command: "); ok {
			return name, ValidateName(name)
		}
	}
	return "", fmt.Errorf("CLAI command alias block in %s has no command name", p.Path)
}

// Save replaces only the managed block. An empty name removes the block.
// Existing symlinks and file permissions are preserved.
func (p Profile) Save(name, executable string) error {
	block := ""
	if name != "" {
		initialization, err := Init(p.Shell, name, executable)
		if err != nil {
			return err
		}
		block = fmt.Sprintf("%s\n# Command: %s\nif [ \"${%s:-}\" != 1 ] && ! command -v %s >/dev/null 2>&1; then\n%s\nfi\n%s\n", blockStart, name, probeEnv, Quote(name), initialization, blockEnd)
	}
	data, path, mode, err := p.read()
	if err != nil {
		return err
	}
	content := string(data)
	start, end, err := blockRange(content)
	if err != nil {
		return err
	}
	if start == end && block != "" && content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
		start, end = len(content), len(content)
	}
	updated := content[:start] + block + content[end:]
	if updated == string(data) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".clai-profile-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := file.Chmod(mode); err != nil {
		file.Close()
		return err
	}
	if _, err := file.WriteString(updated); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func (p Profile) read() ([]byte, string, os.FileMode, error) {
	if _, err := os.Lstat(p.Path); errors.Is(err, os.ErrNotExist) {
		return nil, p.Path, 0o600, nil
	} else if err != nil {
		return nil, "", 0, err
	}
	path, err := filepath.EvalSymlinks(p.Path)
	if err != nil {
		return nil, "", 0, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, "", 0, err
	}
	if !info.Mode().IsRegular() {
		return nil, "", 0, fmt.Errorf("shell profile must be a regular file: %s", p.Path)
	}
	data, err := os.ReadFile(path)
	return data, path, info.Mode().Perm(), err
}

func blockRange(content string) (int, int, error) {
	start := strings.Index(content, blockStart)
	if start < 0 && !strings.Contains(content, blockEnd) {
		return len(content), len(content), nil
	}
	end := strings.Index(content, blockEnd)
	if start < 0 || end < start || strings.Count(content, blockStart) != 1 || strings.Count(content, blockEnd) != 1 ||
		(start > 0 && content[start-1] != '\n') || (end > 0 && content[end-1] != '\n') ||
		(start+len(blockStart) < len(content) && content[start+len(blockStart)] != '\n') {
		return 0, 0, fmt.Errorf("shell profile has an incomplete or duplicate CLAI command alias block; repair it before running setup again")
	}
	end += len(blockEnd)
	if end < len(content) {
		if content[end] != '\n' {
			return 0, 0, fmt.Errorf("unexpected text after CLAI command alias block marker")
		}
		end++
	}
	return start, end, nil
}
