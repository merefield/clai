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
