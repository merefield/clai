package shellintegration

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"
)

const HandoffFileEnv = "CLAI_HANDOFF_FILE"
const MaxHandoffBytes = 64 * 1024

// WriteHandoff creates a new file in the wrapper's private per-invocation
// directory. Existing files (including symlinks) are never opened or replaced.
// The wrapper treats its contents as data, never as shell initialization code.
func WriteHandoff(path, command string) error {
	if err := ValidateHandoff(command); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := file.WriteString(command)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		os.Remove(path)
		return fmt.Errorf("write terminal handoff: %w", err)
	}
	return nil
}

// ValidateHandoff keeps multiline input and controls out of editing buffers.
func ValidateHandoff(command string) error {
	if strings.TrimSpace(command) == "" || len(command) > MaxHandoffBytes || !utf8.ValidString(command) {
		return fmt.Errorf("command is empty, invalid UTF-8, or exceeds the terminal handoff size limit")
	}
	for _, character := range command {
		if unicode.IsControl(character) {
			return fmt.Errorf("multiline commands and control characters require manual copying")
		}
	}
	return nil
}
