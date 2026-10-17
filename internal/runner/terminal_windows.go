package runner

import (
	"os"
	"os/exec"

	"golang.org/x/term"
)

func prepareTerminal(_ *exec.Cmd, input *os.File) (func() error, error) {
	if input == nil || !term.IsTerminal(int(input.Fd())) {
		return func() error { return nil }, nil
	}
	fd := int(input.Fd())
	state, err := term.GetState(fd)
	if err != nil {
		return nil, err
	}
	return func() error { return term.Restore(fd, state) }, nil
}
