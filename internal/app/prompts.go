package app

import (
	"fmt"
	"os"
	"os/user"
	"runtime"
	"strings"

	"github.com/merefield/clai/internal/model"
)

const Version = "1.1.0-go"

const defaultExecQuery = "Return only a single compact JSON object containing 'cmd', 'info', 'risk' and 'variables' fields. 'cmd' must contain one or more shell commands that perform the task, or be empty only as a last resort. 'info' must be a single-line explanation. 'risk' must be exactly 'none', 'reversible change', or 'danger zone'. Use 'none' only for read-only inspection. Use 'reversible change' for changes that are normally undoable. Use 'danger zone' for deletion, overwrite, reset, force, or hard-to-reverse changes. 'variables' must be an array. Represent missing user values as {{variable_name}} in cmd and info and include matching objects with name and prompt. Do not quote placeholders in cmd; CLAI shell-escapes substitutions."

const defaultQuestionQuery = "Return only a single compact JSON object with cmd, info, risk and variables. For questions, cmd must be empty, risk must be 'none', variables must be empty, and info must be a concise terminal-related answer."

const defaultErrorQuery = "Return only a single compact JSON object with cmd, info, risk and variables. Explain what the error means and why the suggested repair may work. Classify the repair risk using none, reversible change, or danger zone."

func systemPrompt(configPath, statePath, toolsPath, toolCapability string) string {
	currentUser := os.Getenv("USER")
	if currentUser == "" {
		if u, err := user.Current(); err == nil {
			currentUser = u.Username
		}
	}
	home, _ := os.UserHomeDir()
	return fmt.Sprintf("You are CLAI (clai) v%s, an advanced terminal assistant. Give precise curt answers without sign-offs or platitudes. Always assume the query is terminal and shell related. Respond with one JSON object containing cmd, info, risk, and variables. The user runs %s/%s as %s with home %s. LANG=%s and LC_TIME=%s. CLAI config: %s. State: %s. Tools: %s. %s", Version, runtime.GOOS, runtime.GOARCH, currentUser, home, os.Getenv("LANG"), os.Getenv("LC_TIME"), configPath, statePath, toolsPath, toolCapability)
}

func ShellInit(shell string) (string, error) {
	switch shell {
	case "bash":
		return `_clai_noglob() {
  local _clai_status _clai_restore_glob=1
  case "${_CLAI_SHELL_FLAGS:-}" in *f*) _clai_restore_glob=0 ;; esac
  command clai "$@"
  _clai_status=$?
  if [ "$_clai_restore_glob" -eq 1 ]; then set +f; fi
  unset _CLAI_SHELL_FLAGS
  return "$_clai_status"
}
alias clai='_CLAI_SHELL_FLAGS=$-; set -f; _clai_noglob'`, nil
	case "zsh":
		return `alias clai='noglob clai'`, nil
	default:
		return "", fmt.Errorf("unsupported shell %q; expected bash or zsh", shell)
	}
}

func queryGuidance(kind, configured string) string {
	if configured != "" {
		return configured
	}
	switch kind {
	case "question":
		return defaultQuestionQuery
	case "error":
		return defaultErrorQuery
	default:
		return defaultExecQuery
	}
}

func templateMessages(kind, system string) []model.Message {
	messages := []model.Message{model.TextMessage("system", system)}
	add := func(user, assistant string) {
		messages = append(messages, model.TextMessage("user", user), model.TextMessage("assistant", assistant))
	}
	switch kind {
	case "question":
		add("how do I list all files?", `{ "cmd": "", "info": "Use the ls command with the -a flag to list all files, including hidden ones.", "risk": "none", "variables": [] }`)
		add("how do I autocomplete commands?", `{ "cmd": "", "info": "Press Tab to autocomplete commands, file names, and directories.", "risk": "none", "variables": [] }`)
	case "error":
		add(`You executed "start avidemux". Which returned error "avidemux: command not found".`, `{ "cmd": "sudo install avidemux", "info": "The application was not found; installing it may resolve the error.", "risk": "reversible change", "variables": [] }`)
	default:
		add("list all files", `{ "cmd": "ls -a", "info": "lists all files, including hidden ones", "risk": "none", "variables": [] }`)
		add("remove the hello world folder", `{ "cmd": "rm -r \"hello world\"", "info": "recursively removes the folder and its contents", "risk": "danger zone", "variables": [] }`)
		add("checkout a new branch", `{ "cmd": "git checkout -b {{branch_name}}", "info": "creates and switches to {{branch_name}}", "risk": "reversible change", "variables": [{"name":"branch_name","prompt":"new branch name"}] }`)
	}
	return messages
}

func isQuestion(query string) bool {
	return strings.Contains(query, "?")
}

func isClearRequest(query string) bool {
	query = strings.TrimSpace(strings.ToLower(query))
	query = strings.TrimRight(query, ".!?;:")
	switch query {
	case "clear history", "clear your history", "clear our history", "forget history", "forget your history", "forget our history", "reset history", "flush history":
		return true
	default:
		return false
	}
}
