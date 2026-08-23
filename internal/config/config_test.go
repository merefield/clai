package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLoadCreatesCompatibleDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".config", "clai.cfg")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.API != "https://api.openai.com/v1/chat/completions" {
		t.Fatalf("unexpected API: %s", cfg.API)
	}
	if cfg.Model != "gpt-4.1" || cfg.Tokens != 500 || cfg.MaxHistoryTurns != 10 {
		t.Fatalf("unexpected defaults: %#v", cfg)
	}
	if cfg.UseTools {
		t.Fatal("tools must be disabled by default")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "use_tools=false\n") {
		t.Fatalf("shipped config does not disable tools: %q", contents)
	}
}

func TestSetSavePreservesEqualsInValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clai.cfg")
	if err := os.WriteFile(path, []byte("key=a=b=c\nrisk_appetite=9\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Key != "a=b=c" || cfg.RiskAppetite != 0 {
		t.Fatalf("normalization failed: %#v", cfg)
	}
	cfg.Set("model", "gpt-test")
	cfg.Set("use_tools", "true")
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Key != "a=b=c" || reloaded.Model != "gpt-test" || !reloaded.UseTools {
		t.Fatalf("save mismatch: %#v", reloaded)
	}
}

func TestEnsureSecuresAnExistingRegularConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clai.cfg")
	if err := os.WriteFile(path, []byte("key=secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Ensure(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
}

func TestEnsureRejectsNonRegularConfigPaths(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clai.cfg")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Ensure(path); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("Ensure error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o755) {
		t.Fatalf("directory was modified: mode=%v", info.Mode())
	}
}

func TestEnsureRejectsConfigSymlinks(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.cfg")
	path := filepath.Join(dir, "clai.cfg")
	if err := os.WriteFile(target, []byte("key=secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if err := Ensure(path); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("Ensure error = %v", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o644 {
		t.Fatalf("symlink target mode changed to %o", info.Mode().Perm())
	}
}

func TestFloatValueRejectsNonFiniteValues(t *testing.T) {
	for _, value := range []string{"NaN", "+Inf", "-Inf", "Infinity"} {
		if got := floatValue(value, 0.1); got != 0.1 {
			t.Errorf("floatValue(%q) = %v, want fallback", value, got)
		}
	}
	if got := floatValue("0.25", 0.1); got != 0.25 {
		t.Fatalf("finite value = %v", got)
	}
}
