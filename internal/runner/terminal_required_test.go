package runner

import (
	"testing"

	"github.com/merefield/clai/internal/model"
)

func TestTerminalCommand(t *testing.T) {
	for _, command := range []string{
		"tmux", "tmux attach", "tmux attach-session -t work", "tmux -L demo attach", "tmux new-session -s work", "tmux attach -d",
		"screen", "screen -r work", "vim README.md", "sudo vim file", "sudo -u root vi file", "command /usr/bin/vi file", "exec nano file", "nvim file", "emacs file", "less file", "more file", "top", "htop", "btop", "watch date",
	} {
		if !TerminalCommand(command) {
			t.Errorf("interactive invocation missed: %s", command)
		}
	}
	for _, command := range []string{
		"", "echo tmux attach", "printf 'vim file'", "tmux ls", "tmux list-sessions", "tmux -V", "tmux new-session -d -s work", "tmux new -Ad -s work", "screen -ls", "screen -dmS work", "screen -S work -X quit", "top -b -n 1", "top -bn1", "vim --version", "nvim --headless file", "vim -es file", "emacs --batch file", "less --help", "ls -la", "echo ready && tmux attach", "custom-tool --interactive",
	} {
		if TerminalCommand(command) {
			t.Errorf("ordinary or unknown invocation diverted: %s", command)
		}
	}
}

func TestTerminalFailure(t *testing.T) {
	for _, diagnostic := range []string{
		"open terminal failed: not a terminal", "the input device is not a TTY", "stty: stdin: Inappropriate ioctl for device", "Error opening terminal: unknown.", "cannot open /dev/tty: No such device", "tool requires an interactive terminal",
	} {
		if !TerminalFailure(model.CommandResult{ExitCode: 1, Stderr: diagnostic}) || !TerminalFailure(model.CommandResult{ExitCode: 1, Stdout: diagnostic}) {
			t.Errorf("diagnostic missed: %s", diagnostic)
		}
		if TerminalFailure(model.CommandResult{ExitCode: 0, Stdout: diagnostic}) {
			t.Errorf("successful output treated as failure: %s", diagnostic)
		}
	}
	for _, diagnostic := range []string{"permission denied", "command not found", "no sessions", "cannot find session: work", "terminal guide not found", "file not found"} {
		if TerminalFailure(model.CommandResult{ExitCode: 1, Stderr: diagnostic}) {
			t.Errorf("ordinary failure diverted: %s", diagnostic)
		}
	}
}
