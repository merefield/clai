package runner

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/merefield/clai/internal/shellintegration"
	"github.com/merefield/clai/internal/ui"
)

func TestBashCapturesAndStreamsOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	result := (Bash{Stdout: &stdout, Stderr: &stderr}).Run(context.Background(), `printf out; printf err >&2; exit 7`, false)
	if result.ExitCode != 7 || result.Stdout != "out" || result.Stderr != "err" {
		t.Fatalf("result = %#v", result)
	}
	if stdout.String() != "out" || stderr.String() != "err" {
		t.Fatalf("streamed stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestBashSharesRedirectedInputWithoutPromptReadAhead(t *testing.T) {
	input, err := os.CreateTemp(t.TempDir(), "input")
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	if _, err := input.WriteString("y\ny\nyes\nnext\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := input.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	console := ui.New(input, &output, &output, false)
	for i := 0; i < 2; i++ {
		choice, err := console.Choice("confirm?")
		if err != nil || choice != "y" {
			t.Fatalf("confirmation %q: %v", choice, err)
		}
	}
	result := (Bash{Stdin: input}).Run(context.Background(), `IFS= read -r answer; printf '%s' "$answer"`, false)
	if result.ExitCode != 0 || result.Stdout != "yes" {
		t.Fatalf("child input lost: %#v", result)
	}
	answer, err := console.Prompt("next request")
	if err != nil || answer != "next" {
		t.Fatalf("parent input lost: %q %v", answer, err)
	}
}

func TestBashWithoutInputReceivesEOF(t *testing.T) {
	result := (Bash{}).Run(context.Background(), `if IFS= read -r answer; then exit 2; else printf eof; fi`, false)
	if result.ExitCode != 0 || result.Stdout != "eof" {
		t.Fatalf("result: %#v", result)
	}
}

func TestBashDoesNotInheritPrivateHandoffPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private-command")
	t.Setenv(shellintegration.HandoffFileEnv, path)
	t.Setenv("CLAI_TEST_NORMAL_ENV", "preserved")
	result := (Bash{}).Run(context.Background(), `printf '%s:%s' "${CLAI_HANDOFF_FILE-unset}" "$CLAI_TEST_NORMAL_ENV"`, false)
	if result.ExitCode != 0 || result.Stdout != "unset:preserved" || os.Getenv(shellintegration.HandoffFileEnv) != path {
		t.Fatalf("private transport leaked or parent environment changed: %+v", result)
	}
}

func TestTailLines(t *testing.T) {
	got := TailLines("one\ntwo\nthree\n", 2)
	if !strings.Contains(got, "truncated") || !strings.HasSuffix(got, "two\nthree") {
		t.Fatalf("got %q", got)
	}
}

func TestBashBoundsCapturedOutputWithoutLimitingStream(t *testing.T) {
	var streamed bytes.Buffer
	result := (Bash{Stdout: &streamed}).Run(context.Background(), `head -c 400000 /dev/zero | tr '\0' x`, false)
	if result.ExitCode != 0 {
		t.Fatalf("result = %#v", result)
	}
	if streamed.Len() != 400000 {
		t.Fatalf("streamed %d bytes", streamed.Len())
	}
	if len(result.Stdout) > MaxCapturedStreamBytes+len(truncationMarker) {
		t.Fatalf("captured %d bytes", len(result.Stdout))
	}
	if !strings.HasPrefix(result.Stdout, truncationMarker) || !strings.HasSuffix(result.Stdout, strings.Repeat("x", 100)) {
		t.Fatalf("capture was not a marked tail: prefix=%q suffix=%q", result.Stdout[:30], result.Stdout[len(result.Stdout)-100:])
	}
}

func TestTailOutputBoundsASingleLargeLine(t *testing.T) {
	got := TailOutput(strings.Repeat("x", MaxSharedStreamBytes*2), 20, MaxSharedStreamBytes)
	if len(got) > MaxSharedStreamBytes {
		t.Fatalf("got %d bytes", len(got))
	}
	if !strings.HasPrefix(got, truncationMarker) {
		t.Fatalf("missing truncation marker: %q", got[:40])
	}
}

func TestTailOutputBoundsAndRepairsInvalidUTF8(t *testing.T) {
	value := string(bytes.Repeat([]byte{0xff}, MaxSharedStreamBytes))
	got := TailOutput(value, 20, MaxSharedStreamBytes)
	if len(got) > MaxSharedStreamBytes {
		t.Fatalf("got %d bytes", len(got))
	}
	if !strings.ContainsRune(got, rune(0xfffd)) {
		t.Fatal("invalid UTF-8 was not replaced")
	}
}

func TestBashDoesNotWaitIndefinitelyForBackgroundOutputPipes(t *testing.T) {
	started := time.Now()
	result := (Bash{}).Run(context.Background(), `sleep 2 &`, false)
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("background command took %s", elapsed)
	}
	if result.ExitCode != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestBashCancellationKillsTheCommandGroup(t *testing.T) {
	marker := filepath.ToSlash(filepath.Join(t.TempDir(), "orphaned-child"))
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	command := fmt.Sprintf(`(sleep 1; printf orphaned > %q) & wait`, marker)
	result := (Bash{}).Run(ctx, command, false)
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("cancelled command took %s", elapsed)
	}
	if result.ExitCode == 0 {
		t.Fatalf("result = %#v", result)
	}
	time.Sleep(1200 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("cancelled child survived and wrote %s", marker)
	}
}
