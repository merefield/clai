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
	if !cfg.AgentsEnabled || cfg.AgentsMaxBytes != 32768 || cfg.AgentsFileMaxBytes != 16384 || !cfg.SkillsEnabled || !cfg.SkillsAutoActivate || cfg.SkillsMaxActive != 2 || cfg.SkillsMaxIndexChars != 8000 || cfg.SkillsFileMaxBytes != 24576 || cfg.SkillsReferenceMaxBytes != 16384 || cfg.SkillsQuiet {
		t.Fatalf("unexpected local context defaults: %+v", cfg)
	}
	if cfg.SystemOneKey != "" || cfg.SystemOneAPI != "https://api.typesafe.ai/v1/systemone" || cfg.SystemOneModel != "jev-latest" {
		t.Fatalf("unexpected system one defaults: %#v", cfg)
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

func TestLocalContextSettingsAreOptionalAndBounded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.cfg")
	if err := os.WriteFile(path, []byte("key=test\nuse_tools=false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.AgentsEnabled || !cfg.SkillsEnabled || cfg.UseTools {
		t.Fatal("old configuration needs a migration or enables external tools")
	}
	for _, key := range []string{"agents_max_bytes", "agents_file_max_bytes", "skills_max_index_chars", "skills_max_active", "skills_file_max_bytes", "skills_reference_max_bytes"} {
		cfg.Set(key, "999999999")
	}
	if cfg.AgentsMaxBytes != 4<<20 || cfg.AgentsFileMaxBytes != 1<<20 || cfg.SkillsMaxIndexChars != 65536 || cfg.SkillsMaxActive != 8 || cfg.SkillsFileMaxBytes != 1<<20 || cfg.SkillsReferenceMaxBytes != 16384 {
		t.Fatal("hard context limits were not applied")
	}
	cfg.Set("agents_enabled", "false")
	cfg.Set("skills_enabled", "false")
	cfg.Set("skills_max_active", "0")
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Load(path)
	if err != nil || reloaded.AgentsEnabled || reloaded.SkillsEnabled || reloaded.SkillsMaxActive != 0 {
		t.Fatalf("local context settings did not persist: %+v, %v", reloaded, err)
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
