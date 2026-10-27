package guidance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func put(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLayeredDiscoveryAndBoundaries(t *testing.T) {
	for _, gitFile := range []bool{false, true} {
		t.Run(map[bool]string{false: "git-directory", true: "worktree-file"}[gitFile], func(t *testing.T) {
			base := t.TempDir()
			project := filepath.Join(base, "project")
			cwd := filepath.Join(project, "backend", "api")
			if err := os.MkdirAll(cwd, 0o700); err != nil {
				t.Fatal(err)
			}
			if gitFile {
				put(t, filepath.Join(project, ".git"), "gitdir: ../elsewhere\n")
			} else if err := os.Mkdir(filepath.Join(project, ".git"), 0o700); err != nil {
				t.Fatal(err)
			}
			locations, err := FindLocations(cwd, filepath.Join(base, "home"), filepath.Join(base, "config"))
			if err != nil {
				t.Fatal(err)
			}
			put(t, filepath.Join(locations.Config, "AGENTS.md"), "global preferences")
			put(t, filepath.Join(base, "AGENTS.md"), "outside")
			put(t, filepath.Join(project, "AGENTS.md"), "root preferences")
			put(t, filepath.Join(project, "backend", "AGENTS.override.md"), "backend override")
			put(t, filepath.Join(project, "backend", "AGENTS.md"), "ignored fallback")
			put(t, filepath.Join(project, "frontend", "AGENTS.md"), "sibling")
			put(t, filepath.Join(cwd, "AGENTS.override.md"), "\ufeff \n")
			put(t, filepath.Join(cwd, "AGENTS.md"), "closest preferences")
			result := Discover(locations, Options{true, 32768, 16384})
			if len(result.Documents) != 4 || len(result.Warnings) != 0 {
				t.Fatalf("result: %+v", result)
			}
			for i, want := range []string{"global preferences", "root preferences", "backend override", "closest preferences"} {
				if result.Documents[i].Content != want {
					t.Fatalf("layer %d: %+v", i, result.Documents[i])
				}
			}
			put(t, filepath.Join(cwd, "AGENTS.md"), "updated")
			refreshed := Discover(locations, Options{true, 32768, 16384})
			if refreshed.Documents[3].Content != "updated" {
				t.Fatal("stale guidance")
			}
			other, err := FindLocations(filepath.Join(project, "frontend"), locations.Home, filepath.Dir(locations.Config))
			if err != nil {
				t.Fatal(err)
			}
			if result := Discover(other, Options{true, 32768, 16384}); len(result.Documents) != 3 || result.Documents[2].Content != "sibling" {
				t.Fatalf("CWD did not change chain: %+v", result)
			}
		})
	}
}

func TestNonGitAndGlobalOnly(t *testing.T) {
	base := t.TempDir()
	cwd := filepath.Join(base, "work")
	if err := os.Mkdir(cwd, 0o700); err != nil {
		t.Fatal(err)
	}
	// Isolate marker discovery from the host's temporary-directory ancestors.
	locations, err := findLocations(cwd, filepath.Join(base, "home"), filepath.Join(base, "config"), func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(base, "AGENTS.md"), "unrelated parent")
	put(t, filepath.Join(locations.Config, "AGENTS.md"), "global")
	options := Options{true, 1000, 1000}
	if result := Discover(locations, options); len(result.Documents) != 1 || result.Documents[0].Scope != "global" {
		t.Fatalf("global: %+v", result)
	}
	put(t, filepath.Join(cwd, "AGENTS.md"), "cwd")
	if result := Discover(locations, options); len(result.Documents) != 2 || len(locations.Chain) != 1 {
		t.Fatalf("non-git: %+v", result)
	}
	if result := Discover(locations, Options{}); len(result.Documents) != 0 {
		t.Fatal("disabled guidance loaded files")
	}
}

func TestGuidanceBudgetsAndMalformedFiles(t *testing.T) {
	base := t.TempDir()
	cwd := filepath.Join(base, "work")
	if err := os.Mkdir(cwd, 0o700); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(cwd, ".git"), "gitdir: fixture")
	locations, _ := FindLocations(cwd, base, filepath.Join(base, "config"))
	put(t, filepath.Join(locations.Config, "AGENTS.md"), "global")
	put(t, filepath.Join(cwd, "AGENTS.override.md"), string([]byte{0xff, 0xfe}))
	put(t, filepath.Join(cwd, "AGENTS.md"), "nested")
	result := Discover(locations, Options{true, 7, 100})
	if len(result.Documents) != 1 || len(result.Warnings) != 2 {
		t.Fatalf("budgets/UTF-8: %+v", result)
	}
	put(t, filepath.Join(cwd, "AGENTS.override.md"), strings.Repeat("é", 51))
	result = Discover(locations, Options{true, 1000, 100})
	if len(result.Documents) != 2 || len(result.Warnings) != 1 {
		t.Fatalf("file budget fallback: %+v", result)
	}
	if err := os.Remove(filepath.Join(cwd, "AGENTS.override.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(cwd, "AGENTS.override.md"), 0o700); err != nil {
		t.Fatal(err)
	}
	result = Discover(locations, Options{true, 1000, 100})
	if len(result.Documents) != 2 || len(result.Warnings) != 1 {
		t.Fatalf("nonregular: %+v", result)
	}
}

func TestGuidanceSymlinkIsolationAndDeduplication(t *testing.T) {
	base := t.TempDir()
	project := filepath.Join(base, "project")
	cwd := filepath.Join(project, "nested")
	if err := os.MkdirAll(cwd, 0o700); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(project, ".git"), "gitdir: example")
	put(t, filepath.Join(project, "AGENTS.md"), "root")
	link := filepath.Join(cwd, "AGENTS.md")
	if err := os.Symlink(filepath.Join("..", "AGENTS.md"), link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	locations, _ := FindLocations(cwd, base, filepath.Join(base, "config"))
	if result := Discover(locations, Options{true, 1000, 1000}); len(result.Documents) != 1 {
		t.Fatalf("duplicate: %+v", result)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(base, "outside.md"), "secret outside")
	if err := os.Symlink(filepath.Join("..", "..", "outside.md"), link); err != nil {
		t.Fatal(err)
	}
	result := Discover(locations, Options{true, 1000, 1000})
	if len(result.Documents) != 1 || len(result.Warnings) != 1 {
		t.Fatalf("symlink escape: %+v", result)
	}
}
