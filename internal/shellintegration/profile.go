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
	startup []string
}

func DetectProfile(ctx context.Context) (Profile, error) {
	shell := os.Getenv("CLAI_SHELL")
	if shell == "" {
		// SHELL describes the login shell, even when CLAI is launched from
		// PowerShell on macOS/Linux. Prefer a PowerShell parent when present.
		parent := parentExecutable(ctx)
		name := strings.TrimSuffix(strings.ToLower(filepath.Base(parent)), ".exe")
		if name == "pwsh" || name == "powershell" {
			shell = parent
		} else {
			shell = os.Getenv("SHELL")
		}
		if shell == "" && runtime.GOOS == "windows" {
			if path, err := exec.LookPath("pwsh"); err == nil {
				shell = path
			} else {
				shell = "powershell"
			}
		}
	}
	name := strings.TrimSuffix(strings.ToLower(filepath.Base(shell)), ".exe")
	if name == "pwsh" || name == "powershell" {
		return detectPowerShellProfile(ctx, shell)
	}
	if runtime.GOOS == "windows" {
		return Profile{}, fmt.Errorf("automatic command alias setup on Windows supports PowerShell; run setup in PowerShell or set CLAI_SHELL=pwsh")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Profile{}, err
	}
	profile := Profile{Shell: name, command: shell}
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
		return Profile{}, fmt.Errorf("automatic command alias setup supports Bash, zsh and PowerShell; shell is %q", shell)
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
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	shell := p.command
	if shell == "" {
		shell = p.Shell
	}
	flags := "-ic"
	if p.login {
		flags = "-lic"
	}
	args := []string{flags, `if command -v "$1" >/dev/null 2>&1; then exit 10; fi`, "clai-alias-check", name}
	if p.isPowerShell() {
		// A native alias accepts every argument exactly as a direct invocation.
		// Reserved words are still unsuitable command names in PowerShell.
		script := p.powerShellStartup() + fmt.Sprintf("if ((Get-Command -Name %s -ErrorAction SilentlyContinue) -or ([System.Management.Automation.PSParser]::Tokenize(%s, [ref]$null) | Where-Object { $_.Type -eq 'Keyword' })) { exit 10 }", PowerShellQuote(name), PowerShellQuote(name))
		args = powerShellArgs(script)
	}
	command := exec.CommandContext(ctx, shell, args...)
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
	content, _, err := decodeText(data)
	if err != nil {
		return "", err
	}
	start, end, err := blockRange(content)
	if err != nil || start == end {
		return "", err
	}
	for _, line := range strings.Split(content[start:end], "\n") {
		if name, ok := strings.CutPrefix(strings.TrimSuffix(line, "\r"), "# Command: "); ok {
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
		guard := fmt.Sprintf("if [ \"${%s:-}\" != 1 ] && ! command -v %s >/dev/null 2>&1; then\n%s\nfi", probeEnv, Quote(name), initialization)
		if p.isPowerShell() {
			guard = fmt.Sprintf("if ($env:%s -ne '1' -and -not (Get-Command -Name %s -ErrorAction SilentlyContinue)) {\n%s\n}", probeEnv, PowerShellQuote(name), initialization)
		}
		block = fmt.Sprintf("%s\n# Command: %s\n%s\n%s\n", blockStart, name, guard, blockEnd)
	}
	data, path, mode, err := p.read()
	if err != nil {
		return err
	}
	content, encoding, err := decodeText(data)
	if err != nil {
		return err
	}
	newline := "\n"
	if strings.Contains(content, "\r\n") || (p.isPowerShell() && content == "" && runtime.GOOS == "windows") {
		newline = "\r\n"
		block = strings.ReplaceAll(block, "\n", newline)
	}
	if p.isPowerShell() && encoding == utf8Text {
		// A BOM makes non-ASCII executable paths work in Windows PowerShell 5.1.
		encoding = utf8BOM
	}
	start, end, err := blockRange(content)
	if err != nil {
		return err
	}
	if start == end && block != "" && content != "" && !strings.HasSuffix(content, "\n") {
		content += newline
		start, end = len(content), len(content)
	}
	updated := content[:start] + block + content[end:]
	if updated == content {
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
	if _, err := file.Write(encoding.encode(updated)); err != nil {
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
	startLineEnd := start + len(blockStart)
	if start >= 0 && startLineEnd < len(content) && content[startLineEnd] == '\r' {
		startLineEnd++
	}
	if start < 0 || end < start || strings.Count(content, blockStart) != 1 || strings.Count(content, blockEnd) != 1 ||
		(start > 0 && content[start-1] != '\n') || (end > 0 && content[end-1] != '\n') ||
		(startLineEnd < len(content) && content[startLineEnd] != '\n') {
		return 0, 0, fmt.Errorf("shell profile has an incomplete or duplicate CLAI command alias block; repair it before running setup again")
	}
	end += len(blockEnd)
	if end < len(content) && content[end] == '\r' {
		end++
	}
	if end < len(content) {
		if content[end] != '\n' {
			return 0, 0, fmt.Errorf("unexpected text after CLAI command alias block marker")
		}
		end++
	}
	return start, end, nil
}
