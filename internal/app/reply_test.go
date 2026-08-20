package app

import (
	"testing"

	"github.com/merefield/clai/internal/model"
)

func TestParseReplyNormalizesKeysRiskAndVariables(t *testing.T) {
	reply, err := ParseReply(`{" cmd ":"echo {{message}}"," info ":" print {{message}} "," risk ":"reversible change","variables":[{"name":"message","prompt":"text"},{"name":"bad-name","prompt":"bad"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if reply.Command != "echo {{message}}" || reply.Risk != model.RiskReversible {
		t.Fatalf("unexpected reply: %#v", reply)
	}
	if len(reply.Variables) != 1 || reply.Variables[0].Name != "message" {
		t.Fatalf("unexpected variables: %#v", reply.Variables)
	}
}

func TestParseReplyAcceptsCodeFence(t *testing.T) {
	reply, err := ParseReply("```json\n{\"cmd\":\"\",\"info\":\"answer\",\"risk\":\"none\",\"variables\":[]}\n```")
	if err != nil {
		t.Fatal(err)
	}
	if reply.Info != "answer" {
		t.Fatalf("info = %q", reply.Info)
	}
}

func TestUnknownRiskFailsClosedForCommands(t *testing.T) {
	if got := NormalizeRisk("safe", "rm file"); got != model.RiskDanger {
		t.Fatalf("risk = %q", got)
	}
	if got := NormalizeRisk("safe", ""); got != model.RiskNone {
		t.Fatalf("empty command risk = %q", got)
	}
}

func TestResolveVariableShellQuotes(t *testing.T) {
	command, info, err := ResolveVariable(`printf %s "{{message}}"`, `print {{message}}`, "message", `it's complicated; rm -rf /`)
	if err != nil {
		t.Fatal(err)
	}
	if command != `printf %s 'it'\''s complicated; rm -rf /'` {
		t.Fatalf("command = %q", command)
	}
	if info != `print it's complicated; rm -rf /` {
		t.Fatalf("info = %q", info)
	}
}

func TestResolveVariableRejectsUnsafeShellQuoteContexts(t *testing.T) {
	commands := []string{
		`echo "prefix {{value}}"`,
		`echo 'prefix {{value}}'`,
		"echo `printf {{value}}`",
		"printf '%s\\n' first\nprintf '%s' {{value}}",
		`echo \{{value}}`,
		`echo ${{value}}`,
	}
	for _, command := range commands {
		if _, _, err := ResolveVariable(command, "show {{value}}", "value", `"; touch /tmp/injected; #`); err == nil {
			t.Errorf("ResolveVariable(%q) accepted an unsafe context", command)
		}
	}
}

func TestResolveVariableAllowsUnquotedWordComposition(t *testing.T) {
	command, _, err := ResolveVariable(`example --name={{value}}`, "", "value", `a b; touch /tmp/injected`)
	if err != nil {
		t.Fatal(err)
	}
	if command != `example --name='a b; touch /tmp/injected'` {
		t.Fatalf("command = %q", command)
	}
}

func TestConfirmationPolicy(t *testing.T) {
	cases := []struct {
		risk     string
		appetite int
		want     bool
	}{
		{model.RiskNone, 0, true}, {model.RiskNone, 1, false},
		{model.RiskReversible, 1, true}, {model.RiskReversible, 2, false},
		{model.RiskDanger, 2, true},
	}
	for _, tc := range cases {
		if got := RequiresConfirmation(tc.risk, tc.appetite); got != tc.want {
			t.Errorf("(%q,%d) = %t", tc.risk, tc.appetite, got)
		}
	}
}
