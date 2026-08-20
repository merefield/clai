package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/merefield/clai/internal/model"
)

type Tool struct {
	Name       string
	Path       string
	Definition model.ToolDefinition
}

type Manager struct {
	Tools map[string]Tool
}

func DefaultPath() (string, error) {
	if path := os.Getenv("CLAI_TOOLS_DIR"); path != "" {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".clai_tools"), nil
}

func Load(ctx context.Context, path string) (*Manager, []string, error) {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return nil, nil, err
	}
	files, err := filepath.Glob(filepath.Join(path, "*.sh"))
	if err != nil {
		return nil, nil, err
	}
	sort.Strings(files)
	manager := &Manager{Tools: map[string]Tool{}}
	var warnings []string
	for _, file := range files {
		definition, name, err := initialize(ctx, file)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", file, err))
			continue
		}
		if _, exists := manager.Tools[name]; exists {
			return nil, warnings, fmt.Errorf("%s tried to claim duplicate function name %q", file, name)
		}
		manager.Tools[name] = Tool{Name: name, Path: file, Definition: definition}
	}
	return manager, warnings, nil
}

func initialize(ctx context.Context, path string) (model.ToolDefinition, string, error) {
	command := exec.CommandContext(ctx, "bash", "-c", `source "$1"; init`, "clai-tool", path)
	output, err := command.Output()
	if err != nil {
		return nil, "", errorsFromCommand(err)
	}
	var definition model.ToolDefinition
	if err := json.Unmarshal(output, &definition); err != nil {
		return nil, "", fmt.Errorf("init returned invalid JSON: %w", err)
	}
	if contentString(definition["type"]) != "function" {
		return nil, "", fmt.Errorf("unknown tool type %q", contentString(definition["type"]))
	}
	function, ok := definition["function"].(map[string]any)
	if !ok {
		return nil, "", fmt.Errorf("missing function definition")
	}
	name := contentString(function["name"])
	if name == "" {
		return nil, "", fmt.Errorf("missing function name")
	}
	parameters, _ := function["parameters"].(map[string]any)
	if parameters == nil {
		parameters = map[string]any{"type": "object"}
		function["parameters"] = parameters
	}
	properties, _ := parameters["properties"].(map[string]any)
	if properties == nil {
		properties = map[string]any{}
		parameters["properties"] = properties
	}
	properties["tool_reason"] = map[string]any{"type": "string", "description": "Reason why this tool must be used."}
	required, _ := parameters["required"].([]any)
	found := false
	for _, item := range required {
		if item == "tool_reason" {
			found = true
		}
	}
	if !found {
		required = append(required, "tool_reason")
	}
	parameters["required"] = required
	return definition, name, nil
}

func (m *Manager) Definitions() []model.ToolDefinition {
	names := m.Names()
	definitions := make([]model.ToolDefinition, 0, len(names))
	for _, name := range names {
		definitions = append(definitions, m.Tools[name].Definition)
	}
	return definitions
}

func (m *Manager) Names() []string {
	names := make([]string, 0, len(m.Tools))
	for name := range m.Tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (m *Manager) Run(ctx context.Context, name, arguments string) (string, error) {
	tool, ok := m.Tools[name]
	if !ok {
		return "", fmt.Errorf("unknown tool %q", name)
	}
	command := exec.CommandContext(ctx, "bash", "-c", `source "$1"; execute "$2"`, "clai-tool", tool.Path, arguments)
	output, err := command.CombinedOutput()
	if err != nil {
		return string(output), errorsFromCommand(err)
	}
	text := string(output)
	if len(text) > 1000 {
		text = text[:1000]
	}
	return text, nil
}

func errorsFromCommand(err error) error {
	if exit, ok := err.(*exec.ExitError); ok {
		message := strings.TrimSpace(string(exit.Stderr))
		if message != "" {
			return fmt.Errorf("%s", message)
		}
	}
	return err
}

func contentString(value any) string {
	text, _ := value.(string)
	return text
}
