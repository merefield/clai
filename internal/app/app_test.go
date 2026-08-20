package app

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/merefield/clai/internal/config"
	"github.com/merefield/clai/internal/history"
	"github.com/merefield/clai/internal/model"
	"github.com/merefield/clai/internal/plugin"
	"github.com/merefield/clai/internal/provider"
	"github.com/merefield/clai/internal/ui"
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
}

func (f *fakeRunner) Run(_ context.Context, command string, edited bool) model.CommandResult {
	f.calls = append(f.calls, command)
	if len(f.results) == 0 {
		return model.CommandResult{Command: command, Edited: edited}
	}
	result := f.results[0]
	f.results = f.results[1:]
	return result
}

func TestProcessPreservesFreeFormQueryAndAutoRunsSafeCommand(t *testing.T) {
	var out, errOut bytes.Buffer
	client := &fakeClient{responses: []provider.Response{{Text: `{"cmd":"printf 42","info":"prints the result","risk":"none","variables":[]}`, FinishReason: "stop"}}}
	commandRunner := &fakeRunner{}
	application := &Application{
		Config:  &config.Config{Key: "test", Model: "test", API: "http://test", RiskAppetite: 1, MaxHistoryTurns: 10, ExposeCurrentDir: true},
		History: &history.Store{Path: filepath.Join(t.TempDir(), "history.json")},
		Plugins: &plugin.Manager{Tools: map[string]plugin.Tool{}},
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

func TestProcessQuestionDoesNotRunCommand(t *testing.T) {
	var out bytes.Buffer
	client := &fakeClient{responses: []provider.Response{{Text: `{"cmd":"","info":"approximately 9.4248","risk":"none","variables":[]}`, FinishReason: "stop"}}}
	commandRunner := &fakeRunner{}
	application := &Application{
		Config:  &config.Config{Key: "test", Model: "test", API: "http://test", MaxHistoryTurns: 10},
		History: &history.Store{Path: filepath.Join(t.TempDir(), "history.json")},
		Plugins: &plugin.Manager{Tools: map[string]plugin.Tool{}},
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

func TestProcessCompletesToolRoundTrip(t *testing.T) {
	dir := t.TempDir()
	script := `init() { printf '%s' '{"type":"function","function":{"name":"lookup","description":"test","parameters":{"type":"object","properties":{},"required":[]}}}'; }
execute() { printf 'tool-data'; }
`
	if err := os.WriteFile(filepath.Join(dir, "lookup.sh"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	plugins, warnings, err := plugin.Load(context.Background(), dir)
	if err != nil || len(warnings) != 0 {
		t.Fatalf("plugins err=%v warnings=%v", err, warnings)
	}
	client := &fakeClient{tools: true, responses: []provider.Response{
		{FinishReason: "tool_calls", ToolCalls: []model.ToolCall{{ID: "call-1", Type: "function", Function: model.FunctionCall{Name: "lookup", Arguments: `{"tool_reason":"verify"}`}}}},
		{FinishReason: "stop", Text: `{"cmd":"","info":"used tool-data","risk":"none","variables":[]}`},
	}}
	var out bytes.Buffer
	application := &Application{
		Config:    &config.Config{Key: "test", Model: "test", API: "http://test", MaxHistoryTurns: 10},
		History:   &history.Store{Path: filepath.Join(t.TempDir(), "history.json")},
		Plugins:   plugins,
		Client:    client,
		Runner:    &fakeRunner{},
		UI:        ui.New(strings.NewReader(""), &out, &out, false),
		toolsPath: dir,
	}
	if err := application.process(context.Background(), "look it up", ""); err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 2 {
		t.Fatalf("requests = %d", len(client.requests))
	}
	foundToolResult := false
	for _, message := range application.History.Messages {
		if message.Role == "tool" && message.ContentText() == "tool-data" {
			foundToolResult = true
		}
	}
	if !foundToolResult {
		t.Fatalf("history = %#v", application.History.Messages)
	}
}
