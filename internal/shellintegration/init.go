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

// Init disables glob expansion before the shell parses request arguments.
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
  local _clai_status _clai_restore_glob=1
  case "${_CLAI_SHELL_FLAGS:-}" in *f*) _clai_restore_glob=0 ;; esac
  if command %s "$@"; then
    _clai_status=0
  else
    _clai_status=$?
  fi
  if [ "$_clai_restore_glob" -eq 1 ]; then set +f; fi
  unset _CLAI_SHELL_FLAGS
  return "$_clai_status"
}
alias %s=%s`, helper, Quote(executable), name, Quote("_CLAI_SHELL_FLAGS=$-; set -f; "+helper)), nil
	case "zsh":
		return fmt.Sprintf("alias %s=%s", name, Quote("noglob "+Quote(executable))), nil
	default:
		return "", fmt.Errorf("unsupported shell %q; expected bash or zsh", shell)
	}
}

func Quote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
