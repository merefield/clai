package mcptools

import (
	"strings"
	"testing"

	"github.com/merefield/clai/internal/shellintegration"
)

func TestExplicitMCPEnvironmentCannotExposeHandoffTransport(t *testing.T) {
	t.Setenv(shellintegration.HandoffFileEnv, "private-handoff-path")
	t.Setenv("CLAI_TEST_MCP_ORDINARY", "allowed")
	environment := restrictedEnvironment([]string{shellintegration.HandoffFileEnv, "CLAI_TEST_MCP_ORDINARY"})
	if len(environment) != 1 || environment[0] != "CLAI_TEST_MCP_ORDINARY=allowed" || strings.Contains(strings.Join(environment, "\n"), "private-handoff") {
		t.Fatalf("private handoff exposed to MCP: %v", environment)
	}
}
