package app

import (
	"context"
	"fmt"
	"os"

	"github.com/merefield/clai/internal/shellintegration"
)

func (a *Application) setupCommandAlias(ctx context.Context) {
	profile, err := shellintegration.DetectProfile(ctx)
	if err != nil {
		a.UI.Info("Command alias setup skipped: " + err.Error())
		return
	}
	current, err := profile.CurrentName()
	if err != nil {
		a.UI.Error("Command alias setup skipped: " + err.Error())
		return
	}
	a.UI.Info("For voice input, use a familiar word such as computer to call CLAI.")
	label := "Command alias (suggested: computer; Enter to skip): "
	if current != "" {
		label = fmt.Sprintf("Command alias (current: %s; Enter to keep; - to remove): ", current)
	}
	for {
		name, err := a.UI.Prompt(label)
		if err != nil {
			a.UI.Error("Command alias setup skipped: " + err.Error())
			return
		}
		if name == "" {
			return
		}
		executable := ""
		if name == "-" && current != "" {
			name = ""
			a.UI.Info(fmt.Sprintf("Remove CLAI's command alias from %s.", profile.Path))
		} else {
			if err := profile.CheckConflict(ctx, name); err != nil {
				a.UI.Error(err.Error())
				if ctx.Err() != nil {
					return
				}
				continue
			}
			executable, err = os.Executable()
			if err != nil {
				a.UI.Error("Command alias setup skipped: " + err.Error())
				return
			}
			a.UI.Info(fmt.Sprintf("Add %s as a command for CLAI in %s, with literal * and ? handling.", name, profile.Path))
		}
		choice, err := a.UI.Choice("Save this shell profile change? [y/N]: ")
		if err != nil || choice != "y" {
			return
		}
		if err := profile.Save(name, executable); err != nil {
			a.UI.Error("CLAI configuration was saved, but the command alias was not updated: " + err.Error())
			return
		}
		a.UI.OK("Shell profile updated.")
		if current != "" {
			a.UI.Info("Start a new shell to clear the previous alias and load the updated profile.")
		} else {
			a.UI.Info("Start a new shell or run: source " + shellintegration.Quote(profile.Path))
		}
		if name == "" {
			a.UI.Info("The clai command is still available.")
		} else {
			a.UI.Info("Try: " + name + " show the ten largest files")
		}
		return
	}
}
