package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/merefield/clai/internal/app"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	root := &cobra.Command{
		Use:                "clai [request...]",
		Short:              "AI-powered terminal assistant",
		Version:            app.Version,
		DisableFlagParsing: true,
		SilenceErrors:      true,
		SilenceUsage:       true,
		Args:               cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
				return command.Help()
			}
			if len(args) == 1 && (args[0] == "--version" || args[0] == "-v") {
				fmt.Fprintf(command.OutOrStdout(), "clai version %s\n", app.Version)
				return nil
			}
			if len(args) >= 1 && args[0] == "shell-init" {
				if len(args) != 2 {
					return fmt.Errorf("usage: clai shell-init bash|zsh")
				}
				initialization, err := app.ShellInit(args[1])
				if err != nil {
					return err
				}
				fmt.Fprintln(command.OutOrStdout(), initialization)
				return nil
			}
			application, err := app.New(ctx, os.Stdin, os.Stdout, os.Stderr)
			if err != nil {
				return err
			}
			defer func() {
				if closeErr := application.Close(); closeErr != nil {
					fmt.Fprintln(os.Stderr, "WARNING:", closeErr)
				}
			}()
			return application.Run(ctx, args)
		},
	}
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}
