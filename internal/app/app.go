package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/merefield/clai/internal/config"
	"github.com/merefield/clai/internal/history"
	"github.com/merefield/clai/internal/model"
	"github.com/merefield/clai/internal/plugin"
	"github.com/merefield/clai/internal/provider"
	"github.com/merefield/clai/internal/runner"
	"github.com/merefield/clai/internal/ui"
)

type Application struct {
	Config    *config.Config
	History   *history.Store
	Plugins   *plugin.Manager
	Client    provider.Client
	Runner    runner.Runner
	UI        *ui.Console
	toolsPath string
}

func New(ctx context.Context, in io.Reader, out, errOut io.Writer) (*Application, error) {
	configPath, err := config.DefaultPath()
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	historyPath, err := history.DefaultPath()
	if err != nil {
		return nil, err
	}
	historyStore, historyErr := history.Load(historyPath)
	if historyErr != nil {
		fmt.Fprintf(errOut, "WARNING: Could not parse history at %s; starting empty: %v\n", historyPath, historyErr)
		historyStore = &history.Store{Path: historyPath, Messages: []model.Message{}}
	}
	toolsPath, err := plugin.DefaultPath()
	if err != nil {
		return nil, err
	}
	plugins, warnings, err := plugin.Load(ctx, toolsPath)
	if err != nil {
		return nil, fmt.Errorf("load tools: %w", err)
	}
	for _, warning := range warnings {
		fmt.Fprintf(errOut, "WARNING: %s\n", warning)
	}
	console := ui.New(in, out, errOut, cfg.HighContrast)
	return &Application{Config: cfg, History: historyStore, Plugins: plugins, Client: provider.New(cfg, nil), Runner: runner.Bash{Stdout: out, Stderr: errOut}, UI: console, toolsPath: toolsPath}, nil
}

func (a *Application) Close() error {
	return a.History.Save(a.Config.MaxHistoryTurns)
}

func (a *Application) Run(ctx context.Context, args []string) error {
	if handled, err := a.handleBuiltIn(args); handled {
		return err
	}
	if a.Config.Key == "" {
		if err := a.setup(); err != nil {
			return err
		}
	}
	query := strings.TrimSpace(strings.Join(args, " "))
	if query != "" {
		if isClearRequest(query) {
			return a.clearHistory()
		}
		return a.process(ctx, query, "")
	}
	a.UI.Title(Version, a.Plugins.Names())
	a.UI.Info(`Hi! Ask a terminal question or give me a task. Type "exit" when done.`)
	for {
		query, err := a.UI.Prompt("CLAI> ")
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		if query == "exit" || errors.Is(err, io.EOF) {
			a.UI.Info("Bye!")
			return nil
		}
		if query == "" {
			continue
		}
		if isClearRequest(query) {
			if err := a.clearHistory(); err != nil {
				a.UI.Error(err.Error())
			}
			continue
		}
		if err := a.process(ctx, query, ""); err != nil {
			a.UI.Error(err.Error())
		}
	}
}

func (a *Application) handleBuiltIn(args []string) (bool, error) {
	if len(args) >= 1 && args[0] == "shell-init" {
		if len(args) != 2 {
			return true, fmt.Errorf("usage: clai shell-init bash|zsh")
		}
		initialization, err := ShellInit(args[1])
		if err != nil {
			return true, err
		}
		fmt.Fprintln(a.UI.Out, initialization)
		return true, nil
	}
	if len(args) == 1 && (args[0] == "setup" || args[0] == "--setup") {
		return true, a.setup()
	}
	if len(args) == 1 && args[0] == "--clear-history" {
		return true, a.clearHistory()
	}
	if len(args) >= 1 && args[0] == "--show-history" {
		if len(args) > 2 || (len(args) == 2 && args[1] != "--verbose") {
			return true, fmt.Errorf("--show-history only supports --verbose")
		}
		fmt.Fprint(a.UI.Out, a.History.Render(len(args) == 2))
		return true, nil
	}
	if len(args) == 1 && args[0] == "--show-results-sharing" {
		state := "disabled"
		if a.Config.ShareCommandResults {
			state = "enabled"
		}
		fmt.Fprintf(a.UI.Out, "Command result sharing is %s.\n", state)
		return true, nil
	}
	if len(args) == 1 && args[0] == "--toggle-results-sharing" {
		a.Config.Set("share_command_results", fmt.Sprintf("%t", !a.Config.ShareCommandResults))
		if err := a.Config.Save(); err != nil {
			return true, err
		}
		if a.Config.ShareCommandResults {
			fmt.Fprintln(a.UI.Err, "WARNING: Shared command results may contain sensitive stdout/stderr and may be sent to the model.")
		}
		state := "disabled"
		if a.Config.ShareCommandResults {
			state = "enabled"
		}
		fmt.Fprintf(a.UI.Out, "Command result sharing is now %s.\n", state)
		return true, nil
	}
	return false, nil
}

func (a *Application) setup() error {
	key, endpoint, selectedModel, risk, err := a.UI.Setup(a.Config.Key, a.Config.API, a.Config.Model, a.Config.RiskAppetite)
	if err != nil {
		return err
	}
	a.Config.Set("key", key)
	a.Config.Set("api", endpoint)
	a.Config.Set("model", selectedModel)
	a.Config.Set("risk_appetite", fmt.Sprintf("%d", risk))
	if err := a.Config.Save(); err != nil {
		return err
	}
	a.Client = provider.New(a.Config, nil)
	fmt.Fprintln(a.UI.Out, "CLAI configuration updated.")
	return nil
}

func (a *Application) clearHistory() error {
	if err := a.History.Clear(); err != nil {
		return fmt.Errorf("failed to clear CLAI history: %w", err)
	}
	a.UI.Info("Cleared CLAI history.")
	return nil
}

func (a *Application) process(ctx context.Context, query, requestedKind string) error {
	kind := requestedKind
	if kind == "" {
		kind = "execute"
		if isQuestion(query) {
			kind = "question"
		}
	}
	a.History.AppendText("user", query)
	for round := 0; round < 9; round++ {
		messages := a.messages(kind)
		stop := a.UI.Spinner(ctx, "Thinking...")
		response, err := a.Client.Complete(ctx, provider.Request{Messages: messages, Tools: a.toolDefinitions()})
		stop()
		if err != nil {
			return err
		}
		if len(response.ToolCalls) > 0 || response.FinishReason == "tool_calls" {
			if len(response.ToolCalls) == 0 {
				return fmt.Errorf("API requested tools without supplying tool calls")
			}
			a.History.AppendToolCalls(response.ToolCalls)
			for _, call := range response.ToolCalls {
				output, toolErr := a.runTool(ctx, call)
				if toolErr != nil {
					output = "tool error: " + toolErr.Error()
				}
				a.History.AppendToolResult(call.ID, output)
			}
			continue
		}
		if strings.TrimSpace(response.Text) == "" {
			return fmt.Errorf("API returned an empty assistant message")
		}
		reply, err := ParseReply(response.Text)
		if err != nil {
			reply = model.Reply{Info: strings.TrimSpace(response.Text), Risk: model.RiskNone, Variables: []model.Variable{}}
		}
		if err := a.resolveVariables(&reply); err != nil {
			a.UI.Cancel()
			return nil
		}
		if HasPlaceholders(reply.Command) || HasPlaceholders(reply.Info) {
			reply = model.Reply{Info: "CLAI returned unresolved placeholders. Rephrase the request or specify missing values.", Risk: model.RiskNone, Variables: []model.Variable{}}
		}
		if err := a.History.AppendReply(reply); err != nil {
			return err
		}
		a.UI.Reply(reply)
		if reply.Command == "" {
			return nil
		}
		return a.confirmAndRun(ctx, reply)
	}
	return fmt.Errorf("tool-call limit exceeded")
}

func (a *Application) messages(kind string) []model.Message {
	capability := "No local CLAI tools are available."
	if len(a.Plugins.Tools) > 0 && a.Client.SupportsTools() {
		capability = "Local CLAI tools are available and may be called when needed."
	}
	if len(a.Plugins.Tools) > 0 && !a.Client.SupportsTools() {
		capability = "Local tools are installed but unavailable through this provider."
	}
	system := systemPrompt(a.Config.Path, a.History.Path, a.toolsPath, capability) + " " + queryGuidance(kind, configuredQuery(a.Config, kind))
	messages := templateMessages(kind, system)
	if a.Config.ExposeCurrentDir {
		if cwd, err := os.Getwd(); err == nil {
			messages = append(messages, model.TextMessage("system", "User is working from directory \""+cwd+"\"."))
		}
	}
	messages = append(messages, a.History.Messages...)
	return messages
}

func configuredQuery(cfg *config.Config, kind string) string {
	switch kind {
	case "question":
		return cfg.QuestionQuery
	case "error":
		return cfg.ErrorQuery
	default:
		return cfg.ExecQuery
	}
}

func (a *Application) toolDefinitions() []model.ToolDefinition {
	if !a.Client.SupportsTools() {
		return nil
	}
	return a.Plugins.Definitions()
}

func (a *Application) runTool(ctx context.Context, call model.ToolCall) (string, error) {
	var args map[string]any
	_ = json.Unmarshal([]byte(call.Function.Arguments), &args)
	reason, _ := args["tool_reason"].(string)
	if reason != "" {
		a.UI.Info(reason)
	}
	a.UI.Info("Using tool \"" + call.Function.Name + "\"")
	return a.Plugins.Run(ctx, call.Function.Name, call.Function.Arguments)
}

func (a *Application) resolveVariables(reply *model.Reply) error {
	for _, variable := range reply.Variables {
		value, err := a.UI.Prompt(variable.Prompt + ": ")
		if err != nil {
			return err
		}
		if value == "" {
			return errors.New("variable collection cancelled")
		}
		reply.Command, reply.Info = ResolveVariable(reply.Command, reply.Info, variable.Name, value)
	}
	reply.Variables = []model.Variable{}
	return nil
}

func (a *Application) confirmAndRun(ctx context.Context, reply model.Reply) error {
	command := reply.Command
	edited := false
	if RequiresConfirmation(reply.Risk, a.Config.RiskAppetite) {
		choice, err := a.UI.Choice("execute command? [y/e/N]: ")
		if err != nil {
			return err
		}
		switch choice {
		case "e":
			value, err := a.UI.Prompt("edit command [" + command + "]: ")
			if err != nil {
				return err
			}
			if value != "" {
				command = value
			}
			edited = true
		case "y":
			if reply.Risk == model.RiskDanger && a.Config.ConfirmDangerousCommands {
				confirm, err := a.UI.Choice("danger zone command, are you sure? [y/N]: ")
				if err != nil {
					return err
				}
				if confirm != "y" {
					a.UI.Cancel()
					return nil
				}
			}
		default:
			a.UI.Cancel()
			return nil
		}
	}
	result := a.Runner.Run(ctx, command, edited)
	if a.Config.ShareCommandResults {
		stored := result
		stored.Stdout = runner.TailLines(stored.Stdout, a.Config.ResultLines)
		stored.Stderr = runner.TailLines(stored.Stderr, a.Config.ResultLines)
		if err := a.History.AppendCommandResult(stored); err != nil {
			return err
		}
	}
	if result.ExitCode == 0 {
		a.UI.OK("[ok]")
		return nil
	}
	a.UI.Error("[error]")
	if strings.TrimSpace(result.Stderr) == "" {
		return nil
	}
	if a.UI.Interactive() {
		choice, err := a.UI.Choice("examine error? [y/N]: ")
		if err != nil {
			return err
		}
		if choice == "y" {
			query := fmt.Sprintf("You executed %q. Which returned error %q.", command, strings.TrimSpace(result.Stderr))
			return a.process(ctx, query, "error")
		}
	}
	return nil
}
