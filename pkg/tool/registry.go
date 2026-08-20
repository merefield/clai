package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/merefield/clai/internal/model"
)

const MaxResultBytes = 32 << 10

var namePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Registry validates, exposes, and dispatches a snapshot of available tools.
type Registry struct {
	tools map[string]Tool
}

func NewRegistry(registered ...Tool) (*Registry, error) {
	registry := &Registry{tools: make(map[string]Tool, len(registered))}
	for _, implementation := range registered {
		if implementation == nil {
			return nil, fmt.Errorf("register tool: nil implementation")
		}
		definition := implementation.Definition()
		if !namePattern.MatchString(definition.Name) {
			return nil, fmt.Errorf("register tool: invalid name %q", definition.Name)
		}
		if strings.TrimSpace(definition.Description) == "" {
			return nil, fmt.Errorf("register tool %q: empty description", definition.Name)
		}
		if definition.Parameters == nil {
			return nil, fmt.Errorf("register tool %q: missing parameter schema", definition.Name)
		}
		if parameterType, _ := definition.Parameters["type"].(string); parameterType != "object" {
			return nil, fmt.Errorf("register tool %q: parameter schema type must be object", definition.Name)
		}
		if _, err := json.Marshal(definition.Parameters); err != nil {
			return nil, fmt.Errorf("register tool %q: invalid parameter schema: %w", definition.Name, err)
		}
		if _, exists := registry.tools[definition.Name]; exists {
			return nil, fmt.Errorf("register tool: duplicate name %q", definition.Name)
		}
		registry.tools[definition.Name] = implementation
	}
	return registry, nil
}

func (r *Registry) Len() int {
	if r == nil {
		return 0
	}
	return len(r.tools)
}

func (r *Registry) Names() []string {
	if r == nil {
		return nil
	}
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (r *Registry) Definitions() []model.ToolDefinition {
	if r == nil {
		return nil
	}
	definitions := make([]model.ToolDefinition, 0, r.Len())
	for _, name := range r.Names() {
		definition := r.tools[name].Definition()
		definitions = append(definitions, model.ToolDefinition{
			"type": "function",
			"function": map[string]any{
				"name":        definition.Name,
				"description": definition.Description,
				"parameters":  definition.Parameters,
			},
		})
	}
	return definitions
}

func (r *Registry) Execute(ctx context.Context, name, arguments string) (string, error) {
	if r == nil {
		return "", fmt.Errorf("unknown tool %q", name)
	}
	implementation, exists := r.tools[name]
	if !exists {
		return "", fmt.Errorf("unknown tool %q", name)
	}
	if strings.TrimSpace(arguments) == "" {
		arguments = "{}"
	}
	if !json.Valid([]byte(arguments)) {
		return "", fmt.Errorf("tool %q received invalid JSON arguments", name)
	}
	result, err := implementation.Execute(ctx, json.RawMessage(arguments))
	if err != nil {
		return "", fmt.Errorf("tool %q: %w", name, err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("tool %q returned an invalid result: %w", name, err)
	}
	if len(encoded) > MaxResultBytes {
		return "", fmt.Errorf("tool %q result exceeds %d bytes", name, MaxResultBytes)
	}
	return string(encoded), nil
}

// InvocationSummary describes a completed call without exposing raw arguments.
// Details are included only when the implementation explicitly marks them safe.
func (r *Registry) InvocationSummary(name, arguments string) string {
	if r == nil {
		return "Used an unknown tool."
	}
	implementation, exists := r.tools[name]
	if !exists {
		return fmt.Sprintf("Used tool %q.", name)
	}
	definition := implementation.Definition()
	label := strings.Join(strings.Fields(definition.DisplayName), " ")
	if label == "" {
		label = definition.Name
	}
	message := "Used the " + label + " tool"
	if summarizer, ok := implementation.(InvocationSummarizer); ok && json.Valid([]byte(arguments)) {
		detail := strings.Join(strings.Fields(summarizer.InvocationSummary(json.RawMessage(arguments))), " ")
		if detail != "" {
			message += " " + detail
		}
	}
	return message + "."
}
