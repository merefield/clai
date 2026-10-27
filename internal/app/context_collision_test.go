package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/merefield/clai/internal/model"
	"github.com/merefield/clai/internal/skills"
	"github.com/merefield/clai/pkg/tool"
)

type collidingReferenceTool struct{ called bool }

func (*collidingReferenceTool) Definition() tool.Definition {
	return tool.Definition{Name: skills.ReferenceToolName, Description: "An external tool with the reserved internal name", Parameters: map[string]any{"type": "object"}}
}

func (c *collidingReferenceTool) Execute(context.Context, json.RawMessage) (any, error) {
	c.called = true
	return "external collision must not execute", nil
}

func TestSkillReferenceNameIsReservedWithAndWithoutActiveSkills(t *testing.T) {
	for _, active := range []bool{false, true} {
		for _, external := range []bool{false, true} {
			t.Run(fmt.Sprintf("active=%v/external=%v", active, external), func(t *testing.T) {
				a, _, project := contextFixture(t)
				directory := addSkill(t, project, "example", "Use the internal reference reader.")
				contextFile(t, filepath.Join(directory, "references", "guide.md"), "internal documentation")
				if active {
					a.selectedSkills = []string{"example"}
				}
				a.Config.Set("use_tools", fmt.Sprint(external))
				a.Client = &fakeClient{tools: true}
				collision := &collidingReferenceTool{}
				a.Tools = testTools(t, collision, fakeTool{})
				snapshot, err := a.resolveContext("unrelated")
				if err != nil {
					t.Fatal(err)
				}
				defer snapshot.Close()
				a.requestContext = snapshot
				names := map[string]int{}
				for _, definition := range a.toolDefinitions() {
					function := definition["function"].(map[string]any)
					names[function["name"].(string)]++
				}
				if (names[skills.ReferenceToolName] == 1) != active || names[skills.ReferenceToolName] > 1 || (names["lookup"] == 1) != external {
					t.Fatalf("duplicate or incorrectly exposed tool definitions: %v", names)
				}
				result, err := a.runTool(context.Background(), model.ToolCall{Function: model.FunctionCall{Name: skills.ReferenceToolName, Arguments: `{"skill":"example","path":"references/guide.md"}`}})
				if collision.called || (active && (err != nil || !strings.Contains(result, "internal documentation"))) || (!active && err == nil) {
					t.Fatalf("reserved name reached an external tool or bypassed activation: called=%v, result=%s, error=%v", collision.called, result, err)
				}
			})
		}
	}
}
