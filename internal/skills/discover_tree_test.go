package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecursiveDiscoveryFindsCollectionsAndStopsAtBundles(t *testing.T) {
	locations := fixture(t)
	root := filepath.Join(locations.Project, ".agents", "skills")
	grouped := filepath.Join(root, "api-tools", "openai-cli")
	write(t, filepath.Join(grouped, "SKILL.md"), skillText("openai-cli", "Inspect OpenAI models", "grouped instructions"))
	write(t, filepath.Join(grouped, "references", "commands.md"), "grouped documentation")
	write(t, filepath.Join(root, "direct", "SKILL.md"), skillText("direct", "Direct workflow", "direct instructions"))
	for _, relative := range []string{"direct/references/templates/template-skill", "direct/scripts/script-skill", ".hidden/hidden-skill", "node_modules/dependency-skill"} {
		path := filepath.Join(root, filepath.FromSlash(relative))
		write(t, filepath.Join(path, "SKILL.md"), skillText(filepath.Base(path), "Must stay undiscovered", "private bundled example"))
	}
	catalog := Discover(locations, true)
	if len(catalog.Skills) != 2 || len(catalog.Warnings) != 0 {
		t.Fatalf("collection discovery: %+v", catalog)
	}
	active, _, err := catalog.Select("", []string{"openai-cli"}, Options{Enabled: true, FileMax: 24576})
	if err != nil {
		t.Fatal(err)
	}
	defer active[0].Close()
	if active[0].Body != "grouped instructions" || !contains(active[0].References, "references/commands.md") {
		t.Fatalf("grouped skill did not activate as a normal bundle: %+v", active[0])
	}
}

func TestRecursiveDiscoveryHasDeterministicPrecedence(t *testing.T) {
	locations := fixture(t)
	for _, source := range []struct{ root, group, name, description string }{
		{locations.Home, "", "example", "user"},
		{locations.Project, "collections", "example", "project"},
		{locations.CWD, "team", "example", "nearest project"},
		{locations.Project, "", "local", "direct child"},
		{locations.Project, "00-imported", "local", "grouped child"},
		{locations.Project, "alpha", "shared", "first group"},
		{locations.Project, "beta", "shared", "second group"},
	} {
		write(t, filepath.Join(source.root, ".agents", "skills", source.group, source.name, "SKILL.md"), skillText(source.name, source.description, "core"))
	}
	catalog := Discover(locations, true)
	for name, want := range map[string]string{"example": "nearest project", "local": "direct child", "shared": "first group"} {
		skill, err := catalog.Find(name)
		if err != nil || skill.Description != want {
			t.Fatalf("%s: %+v, %v", name, skill, err)
		}
	}
	shadowed := 0
	for _, entry := range catalog.Entries {
		if entry.Status == "shadowed" {
			shadowed++
		}
	}
	if shadowed != 4 {
		t.Fatalf("shadowed entries = %d; want 4", shadowed)
	}
}

func TestRecursiveDiscoveryDepthAndDirectoryBudgets(t *testing.T) {
	locations := fixture(t)
	root := filepath.Join(locations.Project, ".agents", "skills")
	groups := []string{root}
	for i := 0; i < MaxDiscoveryDepth-1; i++ {
		groups = append(groups, "group")
	}
	allowed := filepath.Join(append(append([]string(nil), groups...), "allowed")...)
	deep := filepath.Join(append(append([]string(nil), groups...), "another-group", "too-deep")...)
	write(t, filepath.Join(allowed, "SKILL.md"), skillText("allowed", "At the depth limit", "core"))
	write(t, filepath.Join(deep, "SKILL.md"), skillText("too-deep", "Below the depth limit", "core"))
	catalog := Discover(locations, true)
	if len(catalog.Skills) != 1 || catalog.Skills[0].Name != "allowed" || !strings.Contains(strings.Join(catalog.Warnings, " "), "depth limit") {
		t.Fatalf("depth budget: %+v", catalog)
	}
	budget := discoveryBudget{directories: MaxDiscoveryDirectories - 1}
	limited := Catalog{}
	winners := map[string]Skill{}
	scanSource(source{root, locations.Project, "project"}, &budget, &limited, winners)
	if budget.directories != MaxDiscoveryDirectories || len(winners) != 0 || !strings.Contains(strings.Join(limited.Warnings, " "), "directory limit") {
		t.Fatalf("directory budget: %+v, %+v", budget, limited)
	}
}

func TestRecursiveDiscoveryRejectsCollectionEscapesAndCycles(t *testing.T) {
	locations := fixture(t)
	root := filepath.Join(locations.Project, ".agents", "skills")
	write(t, filepath.Join(root, "example", "SKILL.md"), skillText("example", "Actual skill", "core"))
	// This target is inside the project, but outside the skills collection.
	outside := filepath.Join(locations.Project, ".agents", "outside")
	write(t, filepath.Join(outside, "SKILL.md"), skillText("escape", "Must remain outside the collection", "private"))
	if err := os.Symlink(".", filepath.Join(root, "cycle")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(filepath.Join("..", "outside"), filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	catalog := Discover(locations, true)
	if len(catalog.Skills) != 1 || catalog.Skills[0].Name != "example" || len(catalog.Warnings) < 2 {
		t.Fatalf("symlink traversal: %+v", catalog)
	}
}
