package shellintegration

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWriteHandoffCreatesPrivateTextAndNeverOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "command")
	command := "bash -c 'tmux attach -t work'"
	if err := WriteHandoff(path, command); err != nil {
		t.Fatal(err)
	}
	if err := WriteHandoff(path, "different command"); err == nil {
		t.Fatal("overwrote an existing handoff")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != command {
		t.Fatalf("handoff changed: %q, %v", data, err)
	}
	info, err := os.Stat(path)
	if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o600) {
		t.Fatalf("handoff permissions: %v, %v", info, err)
	}
	target := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(path, target); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := WriteHandoff(target, "changed through a link"); err == nil {
		t.Fatal("followed an existing symlink")
	}
	data, _ = os.ReadFile(path)
	if string(data) != command {
		t.Fatal("symlink target was changed")
	}
}

func TestHandoffRejectsControlsMultilineAndOversizedInput(t *testing.T) {
	for _, command := range []string{"", "  ", "echo hi\nrm file", "echo hi\rcommand", "echo \x1b[31m", "echo \x00", strings.Repeat("x", MaxHandoffBytes+1), string([]byte{0xff})} {
		path := filepath.Join(t.TempDir(), "command")
		if err := WriteHandoff(path, command); err == nil {
			t.Fatalf("accepted unsafe editing buffer: %q", command)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("invalid command created a handoff file: %v", err)
		}
	}
}
