package history

import (
	"encoding/json"
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

func TestLatestCommandFromExistingHistory(t *testing.T) {
	store := &Store{}
	store.AppendText("user", "previous")
	if err := store.AppendReply(model.Reply{Info: "    tmux attach\n\nAttaches to a session."}); err != nil {
		t.Fatal(err)
	}
	if command := store.LatestCommand(); command != "tmux attach" {
		t.Fatalf("previous how-to example = %q", command)
	}
	store.AppendText("user", "execute")
	if err := store.AppendReply(model.Reply{Command: "printf original"}); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendCommandResult(model.CommandResult{Command: "printf edited", Edited: true}); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendReply(model.Reply{Info: "Shows the output."}); err != nil {
		t.Fatal(err)
	}
	if command := store.LatestCommand(); command != "printf edited" {
		t.Fatalf("previous command result = %q", command)
	}
}

func TestCopyCommandIsExcludedFromProviderMessages(t *testing.T) {
	store := &Store{}
	store.AppendText("assistant", "{}")
	store.RecordCommand("local edit")
	payload, err := json.Marshal(store.Messages)
	if err != nil || strings.Contains(string(payload), "local edit") {
		t.Fatalf("provider messages include local clipboard metadata: %s, %v", payload, err)
	}
}
