package app

import (
	"context"
	"fmt"

	"github.com/merefield/clai/internal/clipboard"
)

func (a *Application) copyLastCommand(ctx context.Context) error {
	command := a.History.LatestCommand()
	if command == "" {
		return fmt.Errorf("the latest CLAI request has no command to copy")
	}
	return a.copyCommand(ctx, command)
}

func (a *Application) copyCommand(ctx context.Context, command string) error {
	copyText := a.copyClipboard
	if copyText == nil {
		copyText = clipboard.Copy
	}
	terminal, err := copyText(ctx, command, a.UI.Out)
	if err != nil {
		return err
	}
	if terminal {
		a.UI.Info("Sent the command to the terminal clipboard.")
	} else {
		a.UI.OK("Copied command to clipboard.")
	}
	return nil
}

func (a *Application) commandChoice(ctx context.Context, prompt, command string) (string, error) {
	for {
		choice, err := a.UI.Choice(prompt)
		if err != nil || choice != "c" {
			return choice, err
		}
		if err := a.copyCommand(ctx, command); err != nil {
			a.UI.Error(err.Error())
		}
	}
}
