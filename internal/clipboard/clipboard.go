package clipboard

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"golang.org/x/term"
)

// Copy sends text to a local clipboard helper or to the visible terminal.
// terminal reports whether we sent an OSC 52 request without an acknowledgement.
func Copy(ctx context.Context, text string, output io.Writer) (terminal bool, err error) {
	var localErr error
	if os.Getenv("SSH_CONNECTION") == "" {
		for _, helper := range localHelpers() {
			process := exec.CommandContext(ctx, helper[0], helper[1:]...)
			process.Stdin = strings.NewReader(text)
			if err := process.Run(); err != nil {
				localErr = fmt.Errorf("%s: %w", helper[0], err)
				continue
			}
			return false, nil
		}
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if file, ok := output.(*os.File); ok && term.IsTerminal(int(file.Fd())) && os.Getenv("TERM") != "dumb" {
		encoded := base64.StdEncoding.EncodeToString([]byte(text))
		_, err := io.WriteString(output, "\x1b]52;c;"+encoded+"\x1b\\")
		return true, err
	}
	if localErr != nil {
		return false, fmt.Errorf("clipboard unavailable: %w", localErr)
	}
	return false, fmt.Errorf("clipboard unavailable in this session")
}

const windowsClipboard = `$ErrorActionPreference = 'Stop'; $reader = [IO.StreamReader]::new([Console]::OpenStandardInput(), [Text.Encoding]::UTF8, $false); Set-Clipboard -Value ($reader.ReadToEnd())`

func localHelpers() [][]string {
	windows := []string{"powershell.exe", "-NoProfile", "-NonInteractive", "-Command", windowsClipboard}
	switch runtime.GOOS {
	case "darwin":
		return [][]string{{"pbcopy"}}
	case "windows":
		return [][]string{windows, {"pwsh.exe", "-NoProfile", "-NonInteractive", "-Command", windowsClipboard}}
	default:
		var helpers [][]string
		if os.Getenv("WSL_INTEROP") != "" || os.Getenv("WSL_DISTRO_NAME") != "" {
			helpers = append(helpers, windows)
		}
		if os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("WAYLAND_SOCKET") != "" {
			helpers = append(helpers, []string{"wl-copy"})
		}
		if os.Getenv("DISPLAY") != "" {
			helpers = append(helpers, []string{"xclip", "-selection", "clipboard", "-in"}, []string{"xsel", "--clipboard", "--input"})
		}
		return helpers
	}
}
