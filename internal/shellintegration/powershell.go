package shellintegration

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"
)

func (p Profile) isPowerShell() bool {
	return p.Shell == "pwsh" || p.Shell == "powershell"
}

func powerShellArgs(script string) []string {
	// EncodedCommand avoids Windows command-line quoting differences between
	// Windows PowerShell 5.1 and PowerShell 7.
	units := utf16.Encode([]rune(script))
	data := make([]byte, len(units)*2)
	for index, unit := range units {
		binary.LittleEndian.PutUint16(data[index*2:], unit)
	}
	return []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(data)}
}

func detectPowerShellProfile(ctx context.Context, shell string) (Profile, error) {
	executable, err := exec.LookPath(shell)
	if err != nil {
		return Profile{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	script := `[Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false)
@{Path = $PROFILE.CurrentUserAllHosts; Profiles = @($PROFILE.AllUsersAllHosts, $PROFILE.AllUsersCurrentHost, $PROFILE.CurrentUserAllHosts, $PROFILE.CurrentUserCurrentHost)} | ConvertTo-Json -Compress`
	command := exec.CommandContext(ctx, executable, powerShellArgs(script)...)
	command.WaitDelay = time.Second
	output, err := command.Output()
	if err != nil {
		return Profile{}, fmt.Errorf("could not find the PowerShell profile: %w", err)
	}
	var paths struct {
		Path     string
		Profiles []string
	}
	if err := json.Unmarshal(output, &paths); err != nil {
		return Profile{}, fmt.Errorf("could not read the PowerShell profile path: %w", err)
	}
	if !filepath.IsAbs(paths.Path) {
		return Profile{}, fmt.Errorf("PowerShell did not report an absolute profile path")
	}
	return Profile{Shell: strings.TrimSuffix(strings.ToLower(filepath.Base(executable)), ".exe"), Path: paths.Path, command: executable, startup: paths.Profiles}, nil
}

func (p Profile) powerShellStartup() string {
	paths := p.startup
	if len(paths) == 0 {
		paths = []string{p.Path}
	}
	quoted := make([]string, len(paths))
	for index, path := range paths {
		quoted[index] = PowerShellQuote(path)
	}
	return "$ErrorActionPreference = 'Stop'\nforeach ($_claiProfile in @(" + strings.Join(quoted, ",") + ")) { if (Test-Path -LiteralPath $_claiProfile) { . $_claiProfile } }\n"
}

func (p Profile) ActivationCommand() string {
	if p.isPowerShell() {
		return ". " + PowerShellQuote(p.Path)
	}
	return "source " + Quote(p.Path)
}
