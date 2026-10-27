package runner

import (
	"path/filepath"
	"strings"

	"github.com/merefield/clai/internal/model"
)

// TerminalCommand recognizes clear, simple interactive invocations. It is a
// convenience heuristic, not a shell parser or a security decision. Unknown
// commands and compound scripts use error detection after execution.
func TerminalCommand(command string) bool {
	if strings.ContainsAny(command, ";&|`$<>()\n\r") {
		return false
	}
	words := strings.Fields(command)
prefixes:
	for len(words) > 0 {
		switch words[0] {
		case "exec", "command":
			words = words[1:]
		case "sudo":
			words = words[1:]
			for len(words) > 0 && strings.HasPrefix(words[0], "-") {
				switch words[0] {
				case "-u", "-g":
					if len(words) < 2 {
						return false
					}
					words = words[2:]
				case "-H", "-n", "-E", "--":
					words = words[1:]
				default:
					return false
				}
			}
		default:
			break prefixes
		}
	}
	if len(words) == 0 {
		return false
	}
	name := filepath.Base(strings.Trim(words[0], "\"'"))
	args := words[1:]
	if hasArgument(args, "--help") || hasArgument(args, "--version") {
		return false
	}
	switch name {
	case "tmux":
		for len(args) > 0 && strings.HasPrefix(args[0], "-") {
			switch args[0] {
			case "-L", "-S", "-f":
				if len(args) < 2 {
					return false
				}
				args = args[2:]
			case "-u", "-2", "-l", "-v":
				args = args[1:]
			default:
				return false
			}
		}
		if len(args) == 0 {
			return true
		}
		switch args[0] {
		case "attach", "attach-session", "a", "at", "switch-client", "choose-tree", "choose-client":
			return true
		case "new", "new-session":
			return tmuxNewNeedsTerminal(args[1:])
		}
	case "screen":
		for _, argument := range args {
			if argument == "-ls" || argument == "-list" || argument == "-v" || argument == "-wipe" || argument == "-Q" || argument == "-X" || (strings.HasPrefix(argument, "-") && strings.Contains(argument, "m")) {
				return false
			}
		}
		return true
	case "vi", "vim", "nvim", "nano", "emacs":
		return !hasArgument(args, "-h") && !hasArgument(args, "--headless") && !hasArgument(args, "--batch") && !hasArgument(args, "-es") && !hasArgument(args, "-Es") && !hasArgument(args, "-e") && !hasArgument(args, "-s")
	case "less", "more", "htop", "btop", "watch":
		return !hasArgument(args, "-h")
	case "top":
		for _, argument := range args {
			if strings.HasPrefix(argument, "-") && strings.Contains(argument, "b") {
				return false
			}
		}
		return !hasArgument(args, "-h")
	}
	return false
}

// tmux new-session permits bundled short flags. Options taking a value consume
// the remainder of that word (or the next word), so a 'd' in a session name or
// format must not be mistaken for the detached flag. Stop at the shell command.
func tmuxNewNeedsTerminal(args []string) bool {
	detached := false
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if argument == "--" || argument == "-" || !strings.HasPrefix(argument, "-") {
			break
		}
		for position := 1; position < len(argument); position++ {
			flag := rune(argument[position])
			if flag == 'd' {
				detached = true
			} else if strings.ContainsRune("cefFnstxy", flag) {
				if position == len(argument)-1 {
					index++
				}
				break
			} else if !strings.ContainsRune("ADEPX", flag) {
				// Unrecognized syntax uses runtime error detection instead.
				return false
			}
		}
	}
	return !detached
}

func hasArgument(args []string, value string) bool {
	for _, argument := range args {
		if argument == value {
			return true
		}
	}
	return false
}

// TerminalFailure requires a failed command and a specific terminal diagnostic.
func TerminalFailure(result model.CommandResult) bool {
	if result.ExitCode == 0 {
		return false
	}
	output := strings.ToLower(result.Stderr + "\n" + result.Stdout)
	for _, diagnostic := range []string{
		"not a terminal", "not a tty", "isn't a terminal", "is not a tty",
		"requires a terminal", "requires an interactive terminal", "requires a tty",
		"must be run from a terminal", "must be run in a terminal",
		"must be connected to a terminal", "can only be run in a terminal",
		"requires a real terminal", "output is not to a terminal",
		"must be run from a tty", "must be run in a tty",
		"input device is not a tty", "inappropriate ioctl for device",
		"cannot open /dev/tty", "can't open /dev/tty", "failed to open /dev/tty",
		"cannot open terminal", "could not open terminal", "error opening terminal",
		"no controlling terminal", "couldn't open /dev/tty", "not connected to a terminal",
	} {
		if strings.Contains(output, diagnostic) {
			return true
		}
	}
	return false
}
