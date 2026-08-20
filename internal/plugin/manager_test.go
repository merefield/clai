package plugin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellToolCompatibilityAdapter(t *testing.T) {
	dir := t.TempDir()
	script := `init() { printf '%s' '{"type":"function","function":{"name":"echo_tool","description":"test","parameters":{"type":"object","properties":{},"required":[]}}}'; }
execute() { printf 'received:%s' "$1"; }
`
	path := filepath.Join(dir, "echo.sh")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	manager, warnings, err := Load(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 || len(manager.Tools) != 1 {
		t.Fatalf("warnings=%v tools=%v", warnings, manager.Tools)
	}
	definition := manager.Tools["echo_tool"].Definition
	function := definition["function"].(map[string]any)
	parameters := function["parameters"].(map[string]any)
	properties := parameters["properties"].(map[string]any)
	if _, ok := properties["tool_reason"]; !ok {
		t.Fatal("tool_reason not injected")
	}
	output, err := manager.Run(context.Background(), "echo_tool", `{"value":"hello"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, `received:{"value":"hello"}`) {
		t.Fatalf("output = %q", output)
	}
}
