package app

import (
	"strings"
	"testing"

	"github.com/merefield/clai/internal/provider"
	"github.com/merefield/clai/internal/systemone"
	"github.com/merefield/clai/internal/ui"
)

func TestActionWithoutCommandRetriesBeforeNormalReview(t *testing.T) {
	const query = "resolve LoadError Discourse requires libvips for image processing"
	const diagnostic = "pkg-config --modversion vips"
	for _, mode := range []string{"routed", "explicit"} {
		t.Run(mode, func(t *testing.T) {
			a, out, _ := contextFixture(t)
			a.UI = ui.New(strings.NewReader("n\n"), out, out, true)
			router := &fakeSystemOne{intent: systemone.IntentDecision{Intent: systemone.IntentExecute, Confidence: 1}, risk: systemone.RiskDecision{Risk: "none", Confidence: 1}}
			a.SystemOne = router
			args := []string{query}
			if mode == "explicit" {
				router.intent.Intent = systemone.IntentQuestion
				args = []string{"--execute", query}
			}
			client := &fakeClient{responses: []provider.Response{
				{Text: `{"cmd":"","info":"Installs the libvips development package.","risk":"none","variables":[]}`},
				{Text: `{"cmd":"pkg-config --modversion vips","info":"Checks whether pkg-config can find libvips.","risk":"none","variables":[]}`},
			}}
			a.Client = client
			r := &fakeRunner{}
			var checked string
			r.check = func(command string) (string, error) { checked = command; return "", nil }
			a.Runner = r
			if err := a.Run(t.Context(), args); err != nil {
				t.Fatal(err)
			}
			if (len(router.intentRequests) == 0) != (mode == "explicit") || len(router.riskRequests) != 1 || router.riskRequests[0].Command != diagnostic {
				t.Fatalf("intent=%v audit=%v", router.intentRequests, router.riskRequests)
			}
			if len(client.requests) != 2 || len(client.requests[1].Tools) != 0 || !strings.Contains(requestText(client.requests[1]), "did not provide a usable command") {
				t.Fatalf("unexpected retry: %+v", client.requests)
			}
			if checked != diagnostic || len(r.calls) != 0 || !strings.Contains(out.String(), diagnostic) || !strings.Contains(out.String(), "execute command?") || strings.Contains(out.String(), "Installs the libvips") {
				t.Fatalf("expected a checked proposal at the approval prompt: checked=%q ran=%v output=%s", checked, r.calls, out)
			}
		})
	}
}

func TestActionStillWithoutCommandShowsBlockerAndStops(t *testing.T) {
	a, out, _ := contextFixture(t)
	client := &fakeClient{responses: []provider.Response{
		{Text: "Installs the dependency."},
		{Text: `{"cmd":"","info":"Which environment needs the dependency?","risk":"none","variables":[]}`},
	}}
	a.Client = client
	if err := a.Run(t.Context(), []string{"--execute", "fix the dependency"}); err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 2 || len(a.Runner.(*fakeRunner).calls) != 0 || !strings.Contains(out.String(), "No command was proposed.") || !strings.Contains(out.String(), "Which environment needs the dependency?") {
		t.Fatalf("expected one retry and a visible blocker: %s", out)
	}
}

func TestHowToRetryDisplaysCommandExampleWithoutApprovingOrRunningIt(t *testing.T) {
	a, out, _ := contextFixture(t)
	a.Config.Set("risk_appetite", "2")
	a.Config.Set("share_command_results", "true")
	const query = "how to uninstall this?"
	const command = "sudo apt remove imagemagick"
	a.History.AppendText("user", "I installed ImageMagick using apt.")
	router := &fakeSystemOne{intent: systemone.IntentDecision{Intent: systemone.IntentHowTo, Confidence: 1}}
	a.SystemOne = router
	client := &fakeClient{responses: []provider.Response{
		{Text: `{"cmd":"","info":"Uninstalls ImageMagick and removes unused dependencies.","risk":"none","variables":[]}`},
		{Text: `{"cmd":"sudo apt remove imagemagick","info":"Removes the package. Review apt's proposed changes before confirming.","risk":"reversible change","variables":[]}`},
	}}
	a.Client = client
	r := &fakeRunner{check: func(command string) (string, error) {
		t.Fatalf("how-to example entered the syntax check: %s", command)
		return "", nil
	}}
	a.Runner = r
	if err := a.Run(t.Context(), []string{query}); err != nil {
		t.Fatal(err)
	}
	if len(router.intentRequests) != 1 || len(router.riskRequests) != 0 || len(r.calls) != 0 || strings.Contains(out.String(), "execute command?") {
		t.Fatalf("how-to example entered the execution flow: %v / %s", r.calls, out)
	}
	if len(client.requests) != 2 || !strings.Contains(requestText(client.requests[1]), "did not provide a command example") || !strings.Contains(requestText(client.requests[1]), "installed ImageMagick using apt") {
		t.Fatalf("how-to retry lost the reason or conversation context: %+v", client.requests)
	}
	if !strings.Contains(out.String(), command) || strings.Contains(out.String(), "removes unused dependencies") {
		t.Fatalf("missing example or stale explanation: %s", out)
	}
	saved, err := ParseReply(a.History.Messages[len(a.History.Messages)-1].ContentText())
	if err != nil || saved.Command != "" || !strings.Contains(saved.Info, "    "+command) {
		t.Fatalf("example was not saved as inert instructions: %+v, %v", saved, err)
	}
}

func TestExecuteOptionRequiresRequestAndConflictsWithQuestion(t *testing.T) {
	for _, args := range [][]string{{"--execute"}, {"--question", "--execute", "test"}, {"--execute", "--question", "test"}} {
		if err := (&Application{}).Run(t.Context(), args); err == nil {
			t.Errorf("accepted invalid options: %v", args)
		}
	}
}
