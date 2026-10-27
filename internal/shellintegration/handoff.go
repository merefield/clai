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

// HandoffExitCode is reserved for a selected, successfully written handoff.
// Ordinary command exit codes never propagate to the executable's exit status.
// Parent wrappers consume the file only with this status, then return success.
const HandoffExitCode = 85

// WithoutHandoffEnvironment keeps the private parent/CLAI transport out of
// executed commands and tool servers, including explicit MCP environment lists.
func WithoutHandoffEnvironment(environment []string) []string {
	filtered := make([]string, 0, len(environment))
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		if !strings.EqualFold(name, HandoffFileEnv) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

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
