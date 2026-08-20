package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/merefield/clai/internal/config"
	"github.com/merefield/clai/internal/history"
	"github.com/merefield/clai/internal/mcptools"
	"github.com/merefield/clai/internal/model"
	"github.com/merefield/clai/internal/provider"
	"github.com/merefield/clai/internal/ui"
	"github.com/merefield/clai/pkg/tool"
)

type fakeClient struct {
	responses []provider.Response
	requests  []provider.Request
	tools     bool
}

func (f *fakeClient) Complete(_ context.Context, request provider.Request) (provider.Response, error) {
	f.requests = append(f.requests, request)
	response := f.responses[0]
	f.responses = f.responses[1:]
	return response, nil
}

func (f *fakeClient) SupportsTools() bool { return f.tools }

type fakeRunner struct {
	results []model.CommandResult
	calls   []string
	output  *bytes.Buffer
}

type fakeTool struct{}

func (fakeTool) Definition() tool.Definition {
	return tool.Definition{Name: "lookup", Description: "Return test data.", Parameters: map[string]any{"type": "object"}}
}

func (fakeTool) Execute(_ context.Context, _ json.RawMessage) (any, error) {
	return map[string]string{"value": "tool-data"}, nil
}

func testTools(t *testing.T, registered ...tool.Tool) *tool.Registry {
	t.Helper()
	registry, err := tool.NewRegistry(registered...)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func (f *fakeRunner) Run(_ context.Context, command string, edited bool) model.CommandResult {
	f.calls = append(f.calls, command)
	if f.output != nil {
		f.output.WriteString("command result\n")
	}
	if len(f.results) == 0 {
		return model.CommandResult{Command: command, Edited: edited}
	}
	result := f.results[0]
	f.results = f.results[1:]
	return result
}

func TestCommandOutputHasBlankLineAfterExplanation(t *testing.T) {
	var out bytes.Buffer
	client := &fakeClient{responses: []provider.Response{{Text: `{"cmd":"printf result","info":"collects the requested information","risk":"none","variables":[]}`, FinishReason: "stop"}}}
	application := &Application{
		Config:  &config.Config{Key: "test", RiskAppetite: 1, MaxHistoryTurns: 10},
		History: &history.Store{Path: filepath.Join(t.TempDir(), "history.json")},
		Tools:   testTools(t),
		Client:  client,
		Runner:  &fakeRunner{output: &out},
		UI:      ui.New(strings.NewReader(""), &out, &out, true),
	}

	if err := application.process(context.Background(), "collect information", ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "  collects the requested information\n\ncommand result\n") {
		t.Fatalf("command output was not separated from explanation: %q", out.String())
	}
}

func TestProcessPreservesFreeFormQueryAndAutoRunsSafeCommand(t *testing.T) {
	var out, errOut bytes.Buffer
	client := &fakeClient{responses: []provider.Response{{Text: `{"cmd":"printf 42","info":"prints the result","risk":"none","variables":[]}`, FinishReason: "stop"}}}
	commandRunner := &fakeRunner{}
	application := &Application{
		Config:  &config.Config{Key: "test", Model: "test", API: "http://test", RiskAppetite: 1, MaxHistoryTurns: 10, ExposeCurrentDir: true},
		History: &history.Store{Path: filepath.Join(t.TempDir(), "history.json")},
		Tools:   testTools(t),
		Client:  client,
		Runner:  commandRunner,
		UI:      ui.New(strings.NewReader(""), &out, &errOut, false),
	}
	query := "how much is 3 * pi"
	if err := application.process(context.Background(), query, ""); err != nil {
		t.Fatal(err)
	}
	if len(commandRunner.calls) != 1 || commandRunner.calls[0] != "printf 42" {
		t.Fatalf("runner calls = %#v", commandRunner.calls)
	}
	if len(client.requests) != 1 {
		t.Fatalf("requests = %d", len(client.requests))
	}
	messages := client.requests[0].Messages
	if got := messages[len(messages)-1].ContentText(); got != query {
		t.Fatalf("last message = %q", got)
	}
	if !strings.Contains(out.String(), "prints the result") || !strings.Contains(out.String(), "[ok]") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestSharedCommandResultsAreInterpretedImmediately(t *testing.T) {
	const query = "evaluate whether this machine is overwhelmed"
	client := &fakeClient{responses: []provider.Response{
		{Text: `{"cmd":"uptime && free -h","info":"collects load and memory data","risk":"none","variables":[]}`, FinishReason: "stop"},
		{Text: `{"cmd":"rm -rf /tmp/example","info":"Load is modest and available memory is healthy; the machine is not overwhelmed.","risk":"danger zone","variables":[{"name":"unused","prompt":"unused"}]}`, FinishReason: "stop"},
	}}
	commandRunner := &fakeRunner{results: []model.CommandResult{
		{Command: "uptime && free -h", Stdout: "discarded first line\nload average: 0.20\navailable memory: 12 GiB\n"},
	}}
	var out bytes.Buffer
	application := &Application{
		Config:  &config.Config{Key: "test", Model: "test", API: "http://test", RiskAppetite: 1, MaxHistoryTurns: 10, ShareCommandResults: true, ResultLines: 2},
		History: &history.Store{Path: filepath.Join(t.TempDir(), "history.json")},
		Tools:   testTools(t),
		Client:  client,
		Runner:  commandRunner,
		UI:      ui.New(strings.NewReader(""), &out, &out, false),
	}

	if err := application.process(context.Background(), query, ""); err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want initial request and interpretation", len(client.requests))
	}
	if len(commandRunner.calls) != 1 || commandRunner.calls[0] != "uptime && free -h" {
		t.Fatalf("runner calls = %#v", commandRunner.calls)
	}
	interpretation := client.requests[1]
	if len(interpretation.Tools) != 0 {
		t.Fatalf("tools exposed during interpretation: %#v", interpretation.Tools)
	}
	last := interpretation.Messages[len(interpretation.Messages)-1]
	content := last.ContentText()
	if last.Role != "user" || !strings.Contains(content, query) || !strings.Contains(content, "load average: 0.20") || !strings.Contains(content, "available memory: 12 GiB") {
		t.Fatalf("interpretation input = %#v", last)
	}
	if strings.Contains(content, "discarded first line") || !strings.Contains(content, "earlier output truncated") {
		t.Fatalf("result_lines was not applied: %q", content)
	}
	if !strings.Contains(out.String(), "machine is not overwhelmed") || !strings.Contains(out.String(), "[ok]") {
		t.Fatalf("output = %q", out.String())
	}
	if !strings.HasSuffix(out.String(), "\n\n") {
		t.Fatalf("result interpretation did not leave a blank line: %q", out.String())
	}
	if len(application.History.Messages) != 4 {
		t.Fatalf("history messages = %#v", application.History.Messages)
	}
	var conclusion model.Reply
	if err := json.Unmarshal([]byte(application.History.Messages[3].ContentText()), &conclusion); err != nil {
		t.Fatal(err)
	}
	if conclusion.Command != "" || conclusion.Risk != model.RiskNone || len(conclusion.Variables) != 0 {
		t.Fatalf("unsafe interpretation was not normalized: %#v", conclusion)
	}
}

func TestFailedSharedCommandResultsAreAlsoInterpreted(t *testing.T) {
	client := &fakeClient{responses: []provider.Response{
		{Text: `{"cmd":"systemctl status example","info":"checks the service","risk":"none","variables":[]}`, FinishReason: "stop"},
		{Text: `{"cmd":"","info":"The service is failing because its configuration is invalid.","risk":"none","variables":[]}`, FinishReason: "stop"},
	}}
	commandRunner := &fakeRunner{results: []model.CommandResult{
		{Command: "systemctl status example", ExitCode: 3, Stderr: "invalid configuration"},
	}}
	var out bytes.Buffer
	application := &Application{
		Config:  &config.Config{Key: "test", RiskAppetite: 1, MaxHistoryTurns: 10, ShareCommandResults: true, ResultLines: 20},
		History: &history.Store{Path: filepath.Join(t.TempDir(), "history.json")},
		Tools:   testTools(t),
		Client:  client,
		Runner:  commandRunner,
		UI:      ui.New(strings.NewReader(""), &out, &out, false),
	}

	if err := application.process(context.Background(), "find out why the example service is down", ""); err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 2 || !strings.Contains(client.requests[1].Messages[len(client.requests[1].Messages)-1].ContentText(), "invalid configuration") {
		t.Fatalf("failed result was not interpreted: %#v", client.requests)
	}
	if !strings.Contains(out.String(), "[error]") || !strings.Contains(out.String(), "configuration is invalid") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestProcessQuestionDoesNotRunCommand(t *testing.T) {
	var out bytes.Buffer
	client := &fakeClient{responses: []provider.Response{{Text: `{"cmd":"rm -rf /tmp/question-mode","info":"approximately 9.4248","risk":"danger zone","variables":[{"name":"target","prompt":"target"}]}`, FinishReason: "stop"}}}
	commandRunner := &fakeRunner{}
	application := &Application{
		Config:  &config.Config{Key: "test", Model: "test", API: "http://test", MaxHistoryTurns: 10},
		History: &history.Store{Path: filepath.Join(t.TempDir(), "history.json")},
		Tools:   testTools(t),
		Client:  client,
		Runner:  commandRunner,
		UI:      ui.New(strings.NewReader(""), &out, &out, false),
	}
	if err := application.process(context.Background(), "how much is 3 times pi?", ""); err != nil {
		t.Fatal(err)
	}
	if len(commandRunner.calls) != 0 {
		t.Fatalf("unexpected command: %#v", commandRunner.calls)
	}
	if !strings.Contains(out.String(), "approximately 9.4248") {
		t.Fatalf("output = %q", out.String())
	}
	var reply model.Reply
	if err := json.Unmarshal([]byte(application.History.Messages[len(application.History.Messages)-1].ContentText()), &reply); err != nil {
		t.Fatal(err)
	}
	if reply.Command != "" || reply.Risk != model.RiskNone || len(reply.Variables) != 0 {
		t.Fatalf("question reply was not normalized: %#v", reply)
	}
}

func TestEditedDangerousCommandStillRequiresDangerConfirmation(t *testing.T) {
	var out bytes.Buffer
	commandRunner := &fakeRunner{}
	application := &Application{
		Config: &config.Config{RiskAppetite: 0, ConfirmDangerousCommands: true},
		Runner: commandRunner,
		UI:     ui.New(strings.NewReader("e\n\nn\n"), &out, &out, true),
	}
	reply := model.Reply{Command: "rm -rf /tmp/example", Info: "removes the example", Risk: model.RiskDanger}

	if err := application.confirmAndRun(context.Background(), "remove it", reply); err != nil {
		t.Fatal(err)
	}
	if len(commandRunner.calls) != 0 {
		t.Fatalf("dangerous command ran without second confirmation: %#v", commandRunner.calls)
	}
	if !strings.Contains(out.String(), "danger zone command, are you sure?") || !strings.Contains(out.String(), "[cancel]") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestShowHistorySanitizesStoredTerminalControls(t *testing.T) {
	var out bytes.Buffer
	application := &Application{
		Config:  &config.Config{},
		History: &history.Store{Messages: []model.Message{model.TextMessage("user", "safe\x1b]52;c;Y2xpcGJvYXJk\a\x1b[2J text")}},
		Tools:   testTools(t),
		UI:      ui.New(strings.NewReader(""), &out, &out, true),
	}

	if err := application.Run(context.Background(), []string{"--show-history"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "\x1b]") || strings.Contains(out.String(), "\x1b[2J") || strings.Contains(out.String(), "\a") {
		t.Fatalf("history emitted terminal controls: %q", out.String())
	}
	if !strings.Contains(out.String(), "safe text") {
		t.Fatalf("history text was lost: %q", out.String())
	}
}

func TestBashShellInitPreventsGlobExpansion(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "clai")
	if err := os.WriteFile(fake, []byte("#!/bin/bash\nprintf '<%s>\\n' \"$@\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a-file"), []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b-file"), []byte("b"), 0o600); err != nil {
		t.Fatal(err)
	}
	initialization, err := ShellInit("bash")
	if err != nil {
		t.Fatal(err)
	}
	script := initialization + "\ncd \"$1\"\nclai how much is 3 * pi"
	command := exec.Command("bash", "-O", "expand_aliases", "-c", script, "shell-test", dir)
	command.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"))
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("shell integration failed: %v: %s", err, output)
	}
	want := "<how>\n<much>\n<is>\n<3>\n<*>\n<pi>\n"
	if string(output) != want {
		t.Fatalf("output = %q, want %q", output, want)
	}
}

func TestBashShellInitRestoresGlobbingUnderErrexit(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "clai")
	if err := os.WriteFile(fake, []byte("#!/bin/bash\nexit 7\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	initialization, err := ShellInit("bash")
	if err != nil {
		t.Fatal(err)
	}
	script := initialization + `
set -e
trap 'case "$-" in *f*) exit 99 ;; esac' EXIT
clai fail`
	command := exec.Command("bash", "-O", "expand_aliases", "-c", script)
	command.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"))
	output, err := command.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 7 {
		t.Fatalf("exit = %v, output = %q; want status 7 with globbing restored", err, output)
	}
}

func TestErrorTemplateUsesAValidRepairCommand(t *testing.T) {
	messages := templateMessages("error", "system")
	repair := messages[len(messages)-1].ContentText()
	if !strings.Contains(repair, `"cmd": "echo hello"`) || strings.Contains(repair, "sudo install") {
		t.Fatalf("error repair example = %q", repair)
	}
}

func TestProcessCompletesToolRoundTrip(t *testing.T) {
	client := &fakeClient{tools: true, responses: []provider.Response{
		{FinishReason: "tool_calls", ToolCalls: []model.ToolCall{{ID: "call-1", Type: "function", Function: model.FunctionCall{Name: "lookup", Arguments: `{}`}}}},
		{FinishReason: "stop", Text: `{"cmd":"","info":"used tool-data","risk":"none","variables":[]}`},
	}}
	var out bytes.Buffer
	application := &Application{
		Config:  &config.Config{Key: "test", Model: "test", API: "http://test", MaxHistoryTurns: 10, UseTools: true},
		History: &history.Store{Path: filepath.Join(t.TempDir(), "history.json")},
		Tools:   testTools(t, fakeTool{}),
		Client:  client,
		Runner:  &fakeRunner{},
		UI:      ui.New(strings.NewReader(""), &out, &out, false),
	}
	if err := application.process(context.Background(), "look it up", ""); err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 2 {
		t.Fatalf("requests = %d", len(client.requests))
	}
	foundToolResult := false
	for _, message := range application.History.Messages {
		if message.Role == "tool" && message.ContentText() == `{"value":"tool-data"}` {
			foundToolResult = true
		}
	}
	if !foundToolResult {
		t.Fatalf("history = %#v", application.History.Messages)
	}
	if !strings.Contains(out.String(), "Used the lookup tool.") {
		t.Fatalf("tool invocation was not reported: %q", out.String())
	}
}

func TestToolsListDoesNotRequireProviderSetup(t *testing.T) {
	var out bytes.Buffer
	application := &Application{
		Config:  &config.Config{},
		History: &history.Store{Path: filepath.Join(t.TempDir(), "history.json")},
		Tools:   testTools(t, fakeTool{}),
		Client:  &fakeClient{},
		Runner:  &fakeRunner{},
		UI:      ui.New(strings.NewReader(""), &out, &out, false),
	}
	if err := application.Run(context.Background(), []string{"tools", "list"}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "lookup" {
		t.Fatalf("output = %q", out.String())
	}
}

func TestDisabledToolsDoNotStartExternalServer(t *testing.T) {
	directory := t.TempDir()
	marker := filepath.Join(directory, "started")
	command := filepath.Join(directory, "server")
	if err := os.WriteFile(command, []byte("#!/bin/sh\nprintf started > \"$1\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest, err := json.Marshal(map[string]any{
		"id":      "unused",
		"command": command,
		"args":    []string{marker},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "unused.json"), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	manager, err := mcptools.New(directory, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	client := &fakeClient{tools: true, responses: []provider.Response{{Text: `{"cmd":"","info":"no tools needed","risk":"none","variables":[]}`, FinishReason: "stop"}}}
	var out bytes.Buffer
	application := &Application{
		Config:      &config.Config{Key: "test", MaxHistoryTurns: 10},
		History:     &history.Store{Path: filepath.Join(directory, "history.json")},
		Tools:       manager.Registry(),
		ToolManager: manager,
		Client:      client,
		Runner:      &fakeRunner{},
		UI:          ui.New(strings.NewReader(""), &out, &out, false),
	}
	if err := application.process(context.Background(), "answer without tools", "question"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("external server started while use_tools=false: %v", err)
	}
	if len(client.requests) != 1 || len(client.requests[0].Tools) != 0 {
		t.Fatalf("disabled tool definitions sent to provider: %#v", client.requests)
	}
}

func TestRunToolRejectsCallsWhenToolsAreDisabled(t *testing.T) {
	application := &Application{Config: &config.Config{}, Tools: testTools(t, fakeTool{})}
	_, err := application.runTool(context.Background(), model.ToolCall{Function: model.FunctionCall{Name: "lookup", Arguments: `{}`}})
	if err == nil || !strings.Contains(err.Error(), "use_tools=false") {
		t.Fatalf("disabled tool call error = %v", err)
	}
}
