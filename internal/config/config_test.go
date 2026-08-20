package config

import (
	"os"
	"path/filepath"
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
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
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
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Key != "a=b=c" || reloaded.Model != "gpt-test" {
		t.Fatalf("save mismatch: %#v", reloaded)
	}
}
