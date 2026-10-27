package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/merefield/clai/internal/guidance"
)

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}
func skillText(name, description, body string) string {
	return fmt.Sprintf("---\nname: %s\ndescription: %q\n---\n%s", name, description, body)
}
func fixture(t *testing.T) guidance.Locations {
	t.Helper()
	base := t.TempDir()
	cwd := filepath.Join(base, "project", "nested")
	if err := os.MkdirAll(cwd, 0o700); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(base, "project", ".git"), "gitdir: fixture")
	if err := os.Mkdir(filepath.Join(base, "home"), 0o700); err != nil {
		t.Fatal(err)
	}
	locations, err := guidance.FindLocations(cwd, filepath.Join(base, "home"), filepath.Join(base, "config"))
	if err != nil {
		t.Fatal(err)
	}
	return locations
}
func TestFrontmatterStandardYAMLAndValidation(t *testing.T) {
	valid := "---\nname: example\ndescription: >-\n  Handle example documents\n  using portable workflows.\nlicense: MIT\ncompatibility: Requires example\nmetadata:\n  author: me\nallowed-tools: Bash(example:*)\nfuture-extension: [ignored, metadata]\n---\nbody"
	metadata, err := frontmatter(strings.NewReader(valid), "example")
	if err != nil || metadata.Description != "Handle example documents using portable workflows." {
		t.Fatalf("valid YAML: %+v, %v", metadata, err)
	}
	merged := "---\ndefaults: &defaults\n  name: example\n  description: Handle example documents\n<<: *defaults\n---\nbody"
	if metadata, err := frontmatter(strings.NewReader(merged), "example"); err != nil || metadata.Name != "example" {
		t.Fatalf("YAML merge/anchor support: %+v, %v", metadata, err)
	}
	optionalAliases := "---\nname: example\ndescription: Handle example documents\nbase-metadata: &base\n  author: &author Person\nmetadata:\n  <<: *base\n  maintainer: *author\nbase-tools: &tools Bash(example:*)\nallowed-tools: *tools\n---\nbody"
	if _, err := frontmatter(strings.NewReader(optionalAliases), "example"); err != nil {
		t.Fatalf("optional metadata YAML aliases: %v", err)
	}
	for _, header := range []string{
		"name: example\ndescription: hello\nmetadata: {1: string}",
		"name: Example\ndescription: hello", "name: -example\ndescription: hello", "name: example--name\ndescription: hello", "name: other\ndescription: hello",
		"name: example\ndescription: ''", "name: example\ndescription: 123", "name: example\ndescription: [bad", "name: example\nname: example\ndescription: hello",
		"name: example\ndescription: " + strings.Repeat("x", 1025), "name: example\ndescription: hello\nmetadata: [bad]", "name: example\ndescription: hello\nallowed-tools: [Bash]",
	} {
		if _, err := frontmatter(strings.NewReader("---\n"+header+"\n---\n"), "example"); err == nil {
			t.Errorf("accepted invalid frontmatter: %s", header)
		}
	}
	if _, err := frontmatter(strings.NewReader("---\nname: example\ndescription: "+strings.Repeat("x", MaxFrontmatterBytes)+"\n---\n"), "example"); err == nil {
		t.Fatal("accepted oversized header")
	}
}
func TestDiscoveryPrecedenceAndFreshMetadata(t *testing.T) {
	locations := fixture(t)
	for i, root := range []string{locations.Home, locations.Project, locations.CWD} {
		write(t, filepath.Join(root, ".agents", "skills", "example", "SKILL.md"), skillText("example", fmt.Sprintf("description %d", i), "private body"))
	}
	write(t, filepath.Join(locations.Project, "sibling", ".agents", "skills", "hidden", "SKILL.md"), skillText("hidden", "hidden description", "hidden body"))
	write(t, filepath.Join(locations.CWD, ".agents", "skills", "bad", "SKILL.md"), "not a skill")
	catalog := Discover(locations, true)
	if len(catalog.Skills) != 1 || catalog.Skills[0].Description != "description 2" || len(catalog.Warnings) != 1 {
		t.Fatalf("catalog: %+v", catalog)
	}
	shadowed := 0
	for _, entry := range catalog.Entries {
		if entry.Status == "shadowed" {
			shadowed++
		}
	}
	if shadowed != 2 {
		t.Fatalf("shadowed = %d", shadowed)
	}
	write(t, filepath.Join(locations.CWD, ".agents", "skills", "example", "SKILL.md"), "invalid nearest")
	if updated := Discover(locations, true); len(updated.Skills) != 1 || updated.Skills[0].Description != "description 1" {
		t.Fatalf("invalid nearest hid valid root: %+v", updated)
	}
	if disabled := Discover(locations, false); len(disabled.Skills) != 0 {
		t.Fatal("disabled discovery read skills")
	}
}
func TestMetadataDiscoveryDoesNotLoadBodyAndBoundsIndex(t *testing.T) {
	locations := fixture(t)
	path := filepath.Join(locations.Project, ".agents", "skills", "example", "SKILL.md")
	write(t, path, skillText("example", "Process example documents", strings.Repeat("private body", 10000)))
	catalog := Discover(locations, true)
	if len(catalog.Skills) != 1 {
		t.Fatalf("metadata should not require a small body: %+v", catalog)
	}
	index, _ := catalog.Index(8000)
	if strings.Contains(index, "private body") {
		t.Fatal("body leaked into index")
	}
	if index, warnings := catalog.Index(10); utf8.RuneCountInString(index) > 10 || len(warnings) == 0 {
		t.Fatalf("index budget: %q, %v", index, warnings)
	}
	if _, _, err := catalog.Select("", []string{"example"}, Options{Enabled: true, FileMax: 100}); err == nil {
		t.Fatal("selected oversized skill")
	}
	write(t, path, skillText("example", "Process example documents", string([]byte{0xff})))
	if _, err := Validate(path); err == nil {
		t.Fatal("explicit validation accepted a non-UTF-8 skill body")
	}
}
func TestRelevanceExplicitAmbiguityAndLimits(t *testing.T) {
	locations := fixture(t)
	description := "Use the official OpenAI REST API CLI to inspect models files responses audio and other API resources; activate for OpenAI CLI commands or API operations in the shell."
	write(t, filepath.Join(locations.Project, ".agents", "skills", "openai-cli", "SKILL.md"), skillText("openai-cli", description, "selected body"))
	catalog := Discover(locations, true)
	options := Options{Enabled: true, Auto: true, MaxActive: 2, FileMax: 24576}
	for _, query := range []string{"list the OpenAI API models I have access to", "OpenAI CLI commands"} {
		active, _, err := catalog.Select(query, nil, options)
		if err != nil || len(active) != 1 {
			t.Fatalf("expected match for %q: %v, %v", query, active, err)
		}
		active[0].Close()
	}
	for _, query := range []string{"list the files in this directory", "what is OpenAI", "OpenAI API"} {
		active, _, err := catalog.Select(query, nil, options)
		if err != nil || len(active) != 0 {
			t.Fatalf("unexpected match for %q: %v, %v", query, active, err)
		}
	}
	active, _, err := catalog.Select("unrelated", []string{"openai-cli"}, options)
	if err != nil || len(active) != 1 {
		t.Fatal("explicit selection failed")
	}
	active[0].Close()
	if _, _, err := catalog.Select("", []string{"unknown"}, options); err == nil || !strings.Contains(err.Error(), "openai-cli") {
		t.Fatalf("unknown selection: %v", err)
	}
	write(t, filepath.Join(locations.Project, ".agents", "skills", "openai-tools", "SKILL.md"), skillText("openai-tools", description, "other body"))
	catalog = Discover(locations, true)
	active, warnings, err := catalog.Select("list OpenAI models", nil, options)
	if err != nil || len(active) != 0 || len(warnings) == 0 {
		t.Fatalf("ambiguous selection: %+v,%v,%v", active, warnings, err)
	}
	options.MaxActive = 0
	active, _, _ = catalog.Select("openai-cli", nil, options)
	if len(active) != 0 {
		t.Fatal("automatic skill count limit ignored")
	}
}
func TestReferencesStayInsideActiveSkillAndCache(t *testing.T) {
	locations := fixture(t)
	directory := filepath.Join(locations.Project, ".agents", "skills", "example")
	write(t, filepath.Join(directory, "SKILL.md"), skillText("example", "Process example documents", "Use references/guide.md."))
	write(t, filepath.Join(directory, "references", "guide.md"), "first reference")
	write(t, filepath.Join(directory, "references", "large.md"), strings.Repeat("x", 101))
	write(t, filepath.Join(directory, "scripts", "setup.sh"), "touch never-executed")
	catalog := Discover(locations, true)
	active, _, err := catalog.Select("", []string{"example"}, Options{Enabled: true, FileMax: 24576})
	if err != nil {
		t.Fatal(err)
	}
	defer active[0].Close()
	reader := &ReferenceReader{Active: active, MaxBytes: 100}
	call := func(skill, path string) (any, error) {
		args, _ := json.Marshal(map[string]string{"skill": skill, "path": path})
		return reader.Execute(context.Background(), args)
	}
	first, err := call("example", "references/guide.md")
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(directory, "references", "guide.md"), "changed reference")
	second, err := call("example", "references/guide.md")
	if err != nil || fmt.Sprint(first) != fmt.Sprint(second) {
		t.Fatal("reference snapshot changed")
	}
	for _, path := range []string{"../outside.md", "/etc/passwd.md", `C:\secrets.md`, `\\host\share\secret.md`, "references/../SKILL.md", "scripts/setup.sh", "references/large.md"} {
		trial := &ReferenceReader{Active: active, MaxBytes: 100}
		args, _ := json.Marshal(map[string]string{"skill": "example", "path": path})
		if _, err := trial.Execute(context.Background(), args); err == nil {
			t.Errorf("accepted unsafe/oversized reference %q", path)
		}
	}
	inactive := &ReferenceReader{Active: active, MaxBytes: 100}
	if _, err := inactive.Execute(context.Background(), json.RawMessage(`{"skill":"inactive","path":"references/guide.md"}`)); err == nil {
		t.Fatal("read an inactive skill")
	}
}
func TestReferenceSymlinkEscapesAndDirectorySwap(t *testing.T) {
	locations := fixture(t)
	directory := filepath.Join(locations.Project, ".agents", "skills", "example")
	write(t, filepath.Join(directory, "SKILL.md"), skillText("example", "Process example documents", "core"))
	write(t, filepath.Join(directory, "references", "guide.md"), "inside")
	outside := filepath.Join(locations.Home, "outside")
	write(t, filepath.Join(outside, "references", "guide.md"), "outside secret")
	if err := os.Symlink(filepath.Join(outside, "references", "guide.md"), filepath.Join(directory, "references", "escape.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	catalog := Discover(locations, true)
	active, warnings, err := catalog.Select("", []string{"example"}, Options{Enabled: true, FileMax: 24576})
	if err != nil {
		t.Fatal(err)
	}
	defer active[0].Close()
	if contains(active[0].References, "references/escape.md") || len(warnings) == 0 {
		t.Fatal("symlink escape was exposed")
	}
	if err := os.Rename(directory, directory+"-original"); err != nil {
		t.Skipf("directory pinning test unavailable: %v", err)
	}
	if err := os.Symlink(outside, directory); err != nil {
		t.Fatal(err)
	}
	reader := &ReferenceReader{Active: active, MaxBytes: 100}
	args := json.RawMessage(`{"skill":"example","path":"references/guide.md"}`)
	value, err := reader.Execute(context.Background(), args)
	if err != nil || strings.Contains(fmt.Sprint(value), "outside secret") {
		t.Fatalf("directory swap escaped root: %v,%v", value, err)
	}
}

func TestAutomaticSkillCountAndReferenceCallLimits(t *testing.T) {
	locations := fixture(t)
	for name, description := range map[string]string{"alpha-one": "Alpha workflows", "beta": "Inspect tables backups", "gamma": "Inspect tables"} {
		write(t, filepath.Join(locations.Project, ".agents", "skills", name, "SKILL.md"), skillText(name, description, "core instructions"))
	}
	catalog := Discover(locations, true)
	options := Options{Enabled: true, Auto: true, MaxActive: 2, FileMax: 24576}
	active, _, err := catalog.Select("alpha-one beta tables backups gamma", nil, options)
	if err != nil || len(active) != 2 || active[0].Name != "alpha-one" || active[1].Name != "beta" {
		t.Fatalf("automatic count/ordering: %+v,%v", active, err)
	}
	for _, skill := range active {
		skill.Close()
	}
	directory := filepath.Join(locations.Project, ".agents", "skills", "alpha-one")
	write(t, filepath.Join(directory, "references", "guide.md"), "reference")
	catalog = Discover(locations, true)
	active, _, err = catalog.Select("", []string{"alpha-one"}, options)
	if err != nil {
		t.Fatal(err)
	}
	defer active[0].Close()
	reader := &ReferenceReader{Active: active, MaxBytes: 100}
	arguments := json.RawMessage(`{"skill":"alpha-one","path":"references/guide.md"}`)
	for i := 0; i < 8; i++ {
		if _, err := reader.Execute(context.Background(), arguments); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := reader.Execute(context.Background(), arguments); err == nil || !strings.Contains(err.Error(), "request limit") {
		t.Fatalf("reference call limit: %v", err)
	}
}

func TestDiscoveryDirectoryCountIsBounded(t *testing.T) {
	locations := fixture(t)
	for i := 0; i <= MaxCandidates; i++ {
		if err := os.MkdirAll(filepath.Join(locations.Project, ".agents", "skills", fmt.Sprintf("entry-%03d", i)), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	catalog := Discover(locations, true)
	if len(catalog.Skills) != 0 || len(catalog.Warnings) == 0 || !strings.Contains(strings.Join(catalog.Warnings, " "), "directory entries") {
		t.Fatalf("oversized directory was not bounded: %+v", catalog)
	}
}
