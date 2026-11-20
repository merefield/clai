package app

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/merefield/clai/internal/model"
	"github.com/merefield/clai/internal/provider"
	"github.com/merefield/clai/internal/systemone"
	"github.com/merefield/clai/internal/ui"
)

func TestSyntaxRepairChecksAndAuditsRevisedCommandBeforeApproval(t *testing.T) {
	a, out, _ := contextFixture(t)
	a.UI = ui.New(strings.NewReader("y\n"), out, out, true)
	client := &fakeClient{tools: true, responses: []provider.Response{
		{Text: `{"cmd":"if true; then printf first","info":"bad proposal","risk":"none","variables":[]}`},
		{Text: `{"cmd":"printf repaired","info":"fixed proposal","risk":"none","variables":[]}`},
	}}
	a.Client = client
	audit := &fakeSystemOne{intent: systemone.IntentDecision{Intent: systemone.IntentExecute}, risk: systemone.RiskDecision{Risk: "none", Confidence: 1}}
	a.SystemOne = audit
	var checked []string
	r := &fakeRunner{check: func(command string) (string, error) {
		if strings.Contains(out.String(), command) {
			t.Fatalf("command displayed before syntax check: %s", out)
		}
		checked = append(checked, command)
		if strings.HasPrefix(command, "if ") {
			return "syntax error: unexpected end of file", nil
		}
		return "", nil
	}}
	a.Runner = r
	if err := a.process(t.Context(), "print the value", ""); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(checked, []string{"if true; then printf first", "printf repaired"}) || !slices.Equal(r.calls, []string{"printf repaired"}) {
		t.Fatalf("checked %q and ran %q", checked, r.calls)
	}
	if len(audit.riskRequests) != 1 || audit.riskRequests[0].Command != "printf repaired" || !strings.Contains(out.String(), "execute command?") {
		t.Fatalf("repaired command bypassed review: audit=%v output=%s", audit.riskRequests, out)
	}
	if len(client.requests) != 2 || len(client.requests[1].Tools) != 0 {
		t.Fatalf("expected one repair request without tools: %v", client.requests)
	}
	message := requestText(client.requests[1])
	for _, want := range []string{"print the value", "if true; then printf first", "unexpected end of file", "has not run"} {
		if !strings.Contains(message, want) {
			t.Fatalf("repair request omitted %q", want)
		}
	}
	if strings.Contains(out.String(), "bad proposal") || strings.Contains(a.History.Render(true), "unexpected end of file") {
		t.Fatalf("rejected proposal or transient diagnostic was retained: %s", out)
	}
}

func TestSyntaxRepairStopsAfterOneAttemptAndKeepsLocalValuesOut(t *testing.T) {
	a, out, _ := contextFixture(t)
	const value = "entered locally"
	a.UI = ui.New(strings.NewReader(value+"\n"), out, out, true)
	a.Config.Set("risk_appetite", "2")
	client := &fakeClient{responses: []provider.Response{
		{Text: `{"cmd":"printf {{value}} |","info":"first proposal","risk":"none","variables":[{"name":"value","prompt":"Value"}]}`},
		{Text: `{"cmd":"printf still |","info":"invalid repair","risk":"none","variables":[]}`},
	}}
	a.Client = client
	r := &fakeRunner{check: func(command string) (string, error) { return "unexpected pipe in " + command, nil }}
	a.Runner = r
	if err := a.process(t.Context(), "print the value", ""); err == nil || !strings.Contains(err.Error(), "after one repair attempt") {
		t.Fatalf("expected a bounded repair failure, got %v", err)
	}
	if len(client.requests) != 2 || len(r.calls) != 0 || strings.Contains(out.String(), "execute command?") {
		t.Fatalf("attempts=%d runs=%v output=%s", len(client.requests), r.calls, out)
	}
	message := requestText(client.requests[1])
	if strings.Contains(message, value) || !strings.Contains(message, "unexpected pipe in printf {{value}} |") {
		t.Fatalf("repair request must use the template diagnostic: %s", message)
	}
}

func TestInvalidEditStopsBeforeHandoffOrExecution(t *testing.T) {
	a, out, _ := contextFixture(t)
	a.UI = ui.New(strings.NewReader("e\ntmux attach |\n"), out, out, true)
	r := &fakeRunner{check: func(command string) (string, error) {
		if command != "tmux attach |" {
			return "", errors.New("checked the wrong command")
		}
		return "unexpected end of file", nil
	}}
	a.Runner = r
	err := a.confirmAndRun(context.Background(), "view status", model.Reply{Command: "printf status", Risk: model.RiskNone}, false)
	if err == nil || !strings.Contains(err.Error(), "edited command has invalid Bash syntax") || len(r.calls) != 0 || a.exitToShell || strings.Contains(out.String(), "return to your shell with this command?") {
		t.Fatalf("invalid edit advanced: err=%v runs=%v output=%s", err, r.calls, out)
	}
}
