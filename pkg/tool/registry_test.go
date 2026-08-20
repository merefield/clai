package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type stubTool struct {
	name   string
	result any
}

func (s stubTool) Definition() Definition {
	return Definition{Name: s.name, Description: "A test tool.", Parameters: map[string]any{"type": "object"}}
}

func (s stubTool) Execute(_ context.Context, _ json.RawMessage) (any, error) {
	return s.result, nil
}

func TestRegistryBuildsSortedProviderDefinitionsAndExecutes(t *testing.T) {
	registry, err := NewRegistry(stubTool{name: "zeta", result: map[string]string{"answer": "last"}}, stubTool{name: "alpha", result: map[string]string{"answer": "first"}})
	if err != nil {
		t.Fatal(err)
	}
	definitions := registry.Definitions()
	first := definitions[0]["function"].(map[string]any)
	if first["name"] != "alpha" {
		t.Fatalf("first definition = %#v", first)
	}
	result, err := registry.Execute(context.Background(), "alpha", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if result != `{"answer":"first"}` {
		t.Fatalf("result = %q", result)
	}
}

func TestRegistryRejectsDuplicateNamesAndInvalidArguments(t *testing.T) {
	if _, err := NewRegistry(stubTool{name: "same"}, stubTool{name: "same"}); err == nil {
		t.Fatal("expected duplicate-name error")
	}
	registry, err := NewRegistry(stubTool{name: "valid"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Execute(context.Background(), "valid", `not-json`); err == nil {
		t.Fatal("expected invalid-arguments error")
	}
}

func TestRegistryRejectsInvalidDefinitions(t *testing.T) {
	invalidName := stubTool{name: "spaces are invalid"}
	if _, err := NewRegistry(invalidName); err == nil {
		t.Fatal("expected invalid-name error")
	}
	invalidSchema := stubTool{name: "bad_schema"}
	registryDefinition := invalidSchema.Definition()
	registryDefinition.Parameters = map[string]any{"type": "array"}
	if _, err := NewRegistry(definitionTool{definition: registryDefinition}); err == nil {
		t.Fatal("expected invalid-schema error")
	}
}

type definitionTool struct {
	definition Definition
}

func (d definitionTool) Definition() Definition { return d.definition }

func (d definitionTool) Execute(context.Context, json.RawMessage) (any, error) { return nil, nil }

func TestRegistryRejectsOversizedResults(t *testing.T) {
	registry, err := NewRegistry(stubTool{name: "large", result: strings.Repeat("x", MaxResultBytes)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Execute(context.Background(), "large", `{}`); err == nil {
		t.Fatal("expected result-size error")
	}
}
