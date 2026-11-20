package runner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckSyntaxDoesNotRunCommandOrReadStartupHook(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("Bash is unavailable")
	}
	t.Setenv("LC_ALL", "C")
	t.Chdir(t.TempDir())
	if err := os.WriteFile("bash-env", []byte("printf startup > startup-ran\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Forward slashes in the path also work with Git Bash on Windows.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASH_ENV", filepath.ToSlash(filepath.Join(cwd, "bash-env")))
	bash := Bash{}
	command := `if true; then printf '%s' "$(printf nested > nested-ran)" > command-ran; fi`
	if diagnostic, err := bash.CheckSyntax(t.Context(), command); err != nil || diagnostic != "" {
		t.Fatalf("valid syntax: %q, %v", diagnostic, err)
	}
	if diagnostic, err := bash.CheckSyntax(t.Context(), "printf incomplete > command-ran; if true; then"); err != nil || !strings.Contains(diagnostic, "syntax error") {
		t.Fatalf("invalid syntax: %q, %v", diagnostic, err)
	}
	for _, path := range []string{"startup-ran", "nested-ran", "command-ran"} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%s: expected no side effect, got %v", path, err)
		}
	}
}
