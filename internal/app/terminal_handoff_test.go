package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/merefield/clai/internal/config"
	"github.com/merefield/clai/internal/history"
	"github.com/merefield/clai/internal/model"
	"github.com/merefield/clai/internal/provider"
	"github.com/merefield/clai/internal/runner"
	"github.com/merefield/clai/internal/shellintegration"
	"github.com/merefield/clai/internal/ui"
)

func handoffFixture(t *testing.T, input string) (*Application, *fakeRunner, *bytes.Buffer, string) {
	t.Helper()
	var output bytes.Buffer
	path := filepath.Join(t.TempDir(), "command")
	t.Setenv(shellintegration.HandoffFileEnv, path)
	runner := &fakeRunner{}
	app := &Application{
		Config:  &config.Config{Key: "test", MaxHistoryTurns: 10},
		History: &history.Store{Path: filepath.Join(t.TempDir(), "history.json")},
		Tools:   testTools(t), Client: &fakeClient{}, Runner: runner,
		UI: ui.New(strings.NewReader(input), &output, &output, true),
	}
	return app, runner, &output, path
}

func TestKnownInteractiveCommandNeverAutoRuns(t *testing.T) {
	app, runner, output, path := handoffFixture(t, "t\n")
	app.Config.RiskAppetite = 2
	if err := app.confirmAndRun(context.Background(), "attach", model.Reply{Command: "tmux attach", Risk: model.RiskNone}, false); !errors.Is(err, ErrShellHandoff) {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "bash -o errexit -o pipefail -c 'tmux attach'" || !app.exitToShell || len(runner.calls) != 0 {
		t.Fatalf("handoff: %q, %v, calls=%v, exit=%v", data, err, runner.calls, app.exitToShell)
	}
	if !strings.Contains(output.String(), "full terminal") || len(app.History.Messages) != 0 {
		t.Fatal("missing notice or fabricated execution history")
	}
}

func TestManualHandoffAndEditedInteractiveCommand(t *testing.T) {
	for _, test := range []struct{ input, command, want string }{
		{"t\n", "custom-tool", "custom-tool"},
		{"e\nvim README.md\nt\n", "cat README.md", "vim README.md"},
	} {
		app, runner, _, path := handoffFixture(t, test.input)
		if err := app.confirmAndRun(context.Background(), "request", model.Reply{Command: test.command, Risk: model.RiskNone}, false); !errors.Is(err, ErrShellHandoff) {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil || !strings.HasSuffix(string(data), shellintegration.Quote(test.want)) || len(runner.calls) != 0 || !app.exitToShell {
			t.Fatalf("manual handoff: %q, %v, calls=%v", data, err, runner.calls)
		}
	}
}

func TestTerminalErrorOffersHandoffWithoutRetryOrInterpretation(t *testing.T) {
	app, runner, output, _ := handoffFixture(t, "t\n")
	app.Config.RiskAppetite = 1
	app.Config.ShareCommandResults = true
	app.Config.ResultLines = 20
	runner.results = []model.CommandResult{{Command: "custom-tool", ExitCode: 1, Stderr: "open terminal failed: not a terminal"}}
	if err := app.confirmAndRun(context.Background(), "request", model.Reply{Command: "custom-tool", Risk: model.RiskNone}, false); !errors.Is(err, ErrShellHandoff) {
		t.Fatal(err)
	}
	if len(runner.calls) != 1 || !app.exitToShell || len(app.Client.(*fakeClient).requests) != 0 || !strings.Contains(output.String(), "partially completed") || strings.Contains(output.String(), "examine error?") {
		t.Fatalf("unexpected repair or retry: calls=%v, output=%s", runner.calls, output)
	}
}

func TestHandoffCancellationAndMissingIntegration(t *testing.T) {
	app, runner, _, path := handoffFixture(t, "n\n")
	if err := app.confirmAndRun(context.Background(), "request", model.Reply{Command: "tmux", Risk: model.RiskNone}, false); err != nil {
		t.Fatal(err)
	}
	if app.exitToShell || len(runner.calls) > 0 {
		t.Fatal("cancelled handoff executed or exited")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("cancelled handoff created a file: %v", err)
	}
	t.Setenv(shellintegration.HandoffFileEnv, "")
	if err := app.handoffCommand("tmux attach"); err != nil || !app.exitToShell {
		t.Fatalf("copy fallback failed: %v", err)
	}
}

func TestInteractiveSessionExitsAfterHandoff(t *testing.T) {
	app, runner, _, path := handoffFixture(t, "attach my session\nt\nthis must not reach a provider\n")
	app.Client = &fakeClient{responses: []provider.Response{{Text: `{"cmd":"tmux attach","info":"attach session","risk":"none","variables":[]}`, FinishReason: "stop"}}}
	if err := app.Run(context.Background(), nil); !errors.Is(err, ErrShellHandoff) {
		t.Fatal(err)
	}
	if len(app.Client.(*fakeClient).requests) != 1 || len(runner.calls) != 0 {
		t.Fatal("interactive session continued after handoff")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestHandoffEOFDisplaysCommandWithoutQueuing(t *testing.T) {
	app, runner, output, path := handoffFixture(t, "")
	if err := app.offerTerminal("tmux attach -t work", true); err != nil {
		t.Fatal(err)
	}
	if !app.exitToShell || len(runner.calls) != 0 || !strings.Contains(output.String(), "\ntmux attach -t work\n") {
		t.Fatalf("EOF lost the copy fallback: %s", output)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("EOF prepared a handoff without selection")
	}
}

func TestHandoffChoiceWithoutTrailingNewline(t *testing.T) {
	app, _, _, path := handoffFixture(t, "t")
	if err := app.offerTerminal("tmux attach", false); !errors.Is(err, ErrShellHandoff) {
		t.Fatalf("explicit choice at EOF was ignored: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestExistingHandoffFileNeverSignalsSuccess(t *testing.T) {
	app, runner, output, path := handoffFixture(t, "t\n")
	if err := os.WriteFile(path, []byte("stale command"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := app.offerTerminal("tmux attach", false); err != nil {
		t.Fatalf("failed exclusive write must use copy fallback, not a handoff status: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "stale command" || len(runner.calls) != 0 || !app.exitToShell || !strings.Contains(output.String(), "\ntmux attach\n") {
		t.Fatalf("stale file was overwritten or consumed: %q, %v; %s", data, err, output)
	}
}

func TestOrdinaryCommandCannotForgeHandoffWithFileAndExitCode(t *testing.T) {
	app, _, _, path := handoffFixture(t, "")
	app.Config.RiskAppetite = 2
	app.Runner = runner.Bash{}
	command := "printf stale > " + shellintegration.Quote(filepath.ToSlash(path)) + "; exit 85"
	reply, err := json.Marshal(model.Reply{Command: command, Info: "write a test file", Risk: model.RiskReversible, Variables: []model.Variable{}})
	if err != nil {
		t.Fatal(err)
	}
	app.Client = &fakeClient{responses: []provider.Response{{Text: string(reply), FinishReason: "stop"}}}
	if err := app.Run(context.Background(), []string{"ordinary", "request"}); err != nil || app.exitToShell {
		t.Fatalf("ordinary child forged the parent handoff status: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "stale" {
		t.Fatalf("test command did not plant its file: %q, %v", data, err)
	}
	if err := app.handoffCommand("tmux attach"); err != nil {
		t.Fatalf("existing file unexpectedly signaled a handoff: %v", err)
	}
}
