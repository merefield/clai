package app

import (
	"errors"
	"io"
	"os"

	"github.com/merefield/clai/internal/shellintegration"
)

func (a *Application) offerTerminal(command string, failed bool) error {
	a.UI.Info("This command appears to need a full terminal. CLAI captures output through pipes and cannot provide that interface.")
	if failed {
		a.UI.Info("The command may have partially completed. Review it before running it again; CLAI will not retry it automatically.")
	}
	choice, err := a.UI.Choice("return to your shell with this command? [t/N]: ")
	if errors.Is(err, io.EOF) {
		a.UI.Info("Run the displayed command directly in Bash if it needs a full terminal.")
		return nil
	}
	if err != nil {
		return err
	}
	if choice != "t" {
		a.UI.Cancel()
		return nil
	}
	return a.handoffCommand(command)
}

func (a *Application) handoffCommand(command string) error {
	if path := os.Getenv(shellintegration.HandoffFileEnv); path != "" {
		// Preserve Bash semantics when the parent shell is zsh. The wrapper
		// inserts this text for review and never evaluates it itself.
		pending := "bash -o errexit -o pipefail -c " + shellintegration.Quote(command)
		if err := shellintegration.WriteHandoff(path, pending); err == nil {
			a.UI.Info("Returning to your shell. The command has not been executed by this handoff; review it before pressing Enter.")
			a.exitToShell = true
			return nil
		} else {
			a.UI.Error("Could not prepare shell handoff: " + err.Error())
		}
	}
	a.UI.Info("Without Bash/zsh handoff integration, copy this command and run it directly in Bash after CLAI exits.")
	a.UI.Plain(command + "\n")
	a.exitToShell = true
	return nil
}
