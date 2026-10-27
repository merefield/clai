package shellintegration

import (
	"fmt"
	"regexp"
	"strings"
)

var commandName = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

func ValidateName(name string) error {
	if !commandName.MatchString(name) {
		return fmt.Errorf("use a lowercase command name beginning with a letter, followed by letters, numbers, hyphens or underscores")
	}
	return nil
}

// Init preserves prompt arguments and adds terminal handoff where supported.
// executable may be an absolute path so setup also works with renamed binaries
// and installation directories outside PATH.
func Init(shell, name, executable string) (string, error) {
	if err := ValidateName(name); err != nil {
		return "", err
	}
	switch shell {
	case "bash":
		helper := "_clai_noglob"
		if name != "clai" {
			helper = "_clai_alias_noglob"
		}
		return fmt.Sprintf(`%s() {
  local _clai_status _clai_restore_glob=1 _clai_handoff_dir= _clai_pending=
  case "${_CLAI_SHELL_FLAGS:-}" in *f*) _clai_restore_glob=0 ;; esac
  case $- in *i*)
    if [ -t 0 ] && [ -t 1 ]; then
      _clai_handoff_dir=$(mktemp -d "${TMPDIR:-/tmp}/clai-handoff.XXXXXXXX") || _clai_handoff_dir=
    fi ;;
  esac
  if CLAI_HANDOFF_FILE="${_clai_handoff_dir:+$_clai_handoff_dir/command}" command %s "$@"; then
    _clai_status=0
  else
    _clai_status=$?
  fi
  if [ "$_clai_restore_glob" -eq 1 ]; then set +f; fi
  unset _CLAI_SHELL_FLAGS
  if [ -n "$_clai_handoff_dir" ]; then
    if [ "$_clai_status" -eq %d ]; then
      _clai_status=1
      if [ -f "$_clai_handoff_dir/command" ] && [ ! -L "$_clai_handoff_dir/command" ]; then
        IFS= read -r _clai_pending < "$_clai_handoff_dir/command" || :
        if [ -n "$_clai_pending" ]; then
          if history -s -- "$_clai_pending"; then
            printf '\nCLAI: press Up to review the command, then Enter to run it.\n'
          else
            printf '\nCLAI: copy this command for review:\n%%s\n' "$_clai_pending"
          fi
          _clai_status=0
        fi
      fi
      if [ "$_clai_status" -ne 0 ]; then
        printf '\nCLAI: no valid handoff command was returned; copy the proposed command manually.\n' >&2
      fi
    fi
    command rm -f -- "$_clai_handoff_dir/command" || :
    command rmdir -- "$_clai_handoff_dir" || :
  fi
  return "$_clai_status"
}
alias %s=%s`, helper, Quote(executable), HandoffExitCode, name, Quote("_CLAI_SHELL_FLAGS=$-; set -f; "+helper)), nil
	case "zsh":
		helper := "_clai_terminal"
		if name != "clai" {
			helper = "_clai_alias_terminal"
		}
		return fmt.Sprintf(`%s() {
  local _clai_status _clai_handoff_dir= _clai_pending=
  if [[ -o interactive && -t 0 && -t 1 ]]; then
    _clai_handoff_dir=$(mktemp -d "${TMPDIR:-/tmp}/clai-handoff.XXXXXXXX") || _clai_handoff_dir=
  fi
  if CLAI_HANDOFF_FILE="${_clai_handoff_dir:+$_clai_handoff_dir/command}" command %s "$@"; then
    _clai_status=0
  else
    _clai_status=$?
  fi
  if [[ -n "$_clai_handoff_dir" ]]; then
    if [[ $_clai_status -eq %d ]]; then
      _clai_status=1
      if [[ -f "$_clai_handoff_dir/command" && ! -L "$_clai_handoff_dir/command" ]]; then
        IFS= read -r _clai_pending < "$_clai_handoff_dir/command" || :
        if [[ -n "$_clai_pending" ]]; then
          print -rz -- "$_clai_pending"
          print -r -- 'CLAI: review the command at your next prompt; Enter runs it.'
          _clai_status=0
        fi
      fi
      if [[ $_clai_status -ne 0 ]]; then
        print -ru2 -- 'CLAI: no valid handoff command was returned; copy the proposed command manually.'
      fi
    fi
    command rm -f -- "$_clai_handoff_dir/command" || :
    command rmdir -- "$_clai_handoff_dir" || :
  fi
  return "$_clai_status"
}
alias %s=%s`, helper, Quote(executable), HandoffExitCode, name, Quote("noglob "+helper)), nil
	case "pwsh", "powershell":
		return fmt.Sprintf("Set-Alias -Name %s -Value %s -Scope Global", PowerShellQuote(name), PowerShellQuote(executable)), nil
	default:
		return "", fmt.Errorf("unsupported shell %q; expected bash, zsh, pwsh or powershell", shell)
	}
}

func Quote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func PowerShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
