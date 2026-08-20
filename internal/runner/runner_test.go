package runner

import (
	"bytes"
	"context"
	"strings"
	"testing"
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
