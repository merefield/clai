package history

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/merefield/clai/internal/model"
)

func TestSaveTrimsByUserTurnsAndDropsSystem(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "history.json")
	store := &Store{Path: path}
	store.AppendText("system", "transient")
	store.AppendText("user", "first")
	store.AppendText("assistant", "one")
	store.AppendText("user", "second")
	store.AppendText("assistant", "two")
	if err := store.Save(1); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Messages) != 2 || loaded.Messages[0].ContentText() != "second" {
		t.Fatalf("messages = %#v", loaded.Messages)
	}
}

func TestRenderCommandResult(t *testing.T) {
	store := &Store{}
	if err := store.AppendCommandResult(model.CommandResult{Command: "false", ExitCode: 1, Stderr: "bad"}); err != nil {
		t.Fatal(err)
	}
	rendered := store.Render(false)
	if !strings.Contains(rendered, "command result") || !strings.Contains(rendered, "exit_code: 1") {
		t.Fatalf("rendered = %s", rendered)
	}
}
