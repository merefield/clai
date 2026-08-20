package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/merefield/clai/internal/model"
)

func TestMultilineExplanationKeepsSectionMargin(t *testing.T) {
	var output bytes.Buffer
	console := New(strings.NewReader(""), &output, &output, true)

	console.Reply(model.Reply{
		Info: "Memory pressure is low.\nCPU load is within available capacity.\nThe machine is not overwhelmed.",
	})

	want := "\n  Memory pressure is low.\n  CPU load is within available capacity.\n  The machine is not overwhelmed.\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}
