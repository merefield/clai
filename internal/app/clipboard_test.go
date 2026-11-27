package app

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/merefield/clai/internal/history"
	"github.com/merefield/clai/internal/model"
	"github.com/merefield/clai/internal/provider"
	"github.com/merefield/clai/internal/systemone"
	"github.com/merefield/clai/internal/ui"
)

func TestCopyAtCommandPrompts(t *testing.T) {
	for _, tc := range []struct {
		name, command, prompt, input string
		copyError                    error
		wantRuns                     int
	}{
		{name: "execute", command: "printf 'café'\nprintf '%s' '$HOME'", prompt: "[y/e/c/t/N]", input: "c\ny\n", wantRuns: 1},
		{name: "handoff", command: "tmux attach", prompt: "[c/t/N]", input: "c\nn\n"},
		{name: "failed copy", command: "printf hello", prompt: "[y/e/c/t/N]", input: "c\nn\n", copyError: errors.New("clipboard unavailable")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, runner, out, handoff := handoffFixture(t, tc.input)
			var copied []string
			a.copyClipboard = func(_ context.Context, text string, _ io.Writer) (bool, error) {
				if len(runner.calls) != 0 {
					t.Fatal("copy ran the command before approval")
				}
				copied = append(copied, text)
				return false, tc.copyError
			}
			if err := a.confirmAndRun(t.Context(), "request", model.Reply{Command: tc.command, Risk: model.RiskNone}, false); err != nil {
				t.Fatal(err)
			}
			if len(copied) != 1 || copied[0] != tc.command || len(runner.calls) != tc.wantRuns || strings.Count(out.String(), tc.prompt) != 2 {
				t.Fatalf("copied %q, ran %q, output %q", copied, runner.calls, out)
			}
			if _, err := os.Stat(handoff); !os.IsNotExist(err) {
				t.Fatalf("copy created a handoff: %v", err)
			}
		})
	}
}

func TestCopyFromLatestSavedRequest(t *testing.T) {
	const shellAdvice = `{"cmd":"","info":"List files:\n\n~~~bash\nls -a\n~~~","risk":"none","variables":[]}`
	for _, tc := range []struct {
		name, input, response, want string
		args                        []string
		howTo                       bool
		wantRuns                    int
	}{
		{
			name: "executed edit with result sharing off", input: "e\nprintf 'new command'\n",
			response: `{"cmd":"printf old","info":"Prints.","risk":"none","variables":[]}`,
			args:     []string{"print something"}, want: "printf 'new command'", wantRuns: 1,
		},
		{
			name: "declined proposal", input: "n\n",
			response: `{"cmd":"printf proposed","info":"Prints.","risk":"none","variables":[]}`,
			args:     []string{"print something"}, want: "printf proposed",
		},
		{
			name: "structured how-to example", howTo: true,
			response: `{"cmd":"tmux attach-session","info":"Attaches to a session.","risk":"none","variables":[]}`,
			args:     []string{"what about last session?"}, want: "tmux attach-session",
		},
		{
			name: "Markdown advice", response: shellAdvice,
			args: []string{"--question", "how to list files?"}, want: "ls -a",
		},
		{
			name:     "prose only does not pick an older command",
			response: `{"cmd":"","info":"The -t option specifies the target session.","risk":"none","variables":[]}`,
			args:     []string{"what is -t for?"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, out, _ := contextFixture(t)
			a.UI = ui.New(strings.NewReader(tc.input), out, out, true)
			a.History.AppendText("user", "previous request")
			if err := a.History.AppendReply(model.Reply{Command: "printf stale"}); err != nil {
				t.Fatal(err)
			}
			if tc.howTo {
				a.SystemOne = &fakeSystemOne{intent: systemone.IntentDecision{Intent: systemone.IntentHowTo, Confidence: 1}}
			}
			client := &fakeClient{responses: []provider.Response{{Text: tc.response}}}
			a.Client = client
			if err := a.Run(t.Context(), tc.args); err != nil {
				t.Fatal(err)
			}
			if err := a.Close(); err != nil {
				t.Fatal(err)
			}
			reloaded, err := history.Load(a.History.Path)
			if err != nil {
				t.Fatal(err)
			}
			a.History = reloaded
			a.Config.Set("key", "")
			var copied []string
			a.copyClipboard = func(_ context.Context, text string, _ io.Writer) (bool, error) {
				copied = append(copied, text)
				return false, nil
			}
			err = a.Run(t.Context(), []string{"copy"})
			if tc.want == "" {
				if err == nil || !strings.Contains(err.Error(), "no command to copy") || len(copied) != 0 {
					t.Fatalf("copied %q with error %v", copied, err)
				}
			} else if err != nil || len(copied) != 1 || copied[0] != tc.want {
				t.Fatalf("copied %q, want %q, error %v", copied, tc.want, err)
			}
			if len(a.Runner.(*fakeRunner).calls) != tc.wantRuns || len(client.requests) != 1 {
				t.Fatal("copy triggered execution or made a model request")
			}
		})
	}
}
