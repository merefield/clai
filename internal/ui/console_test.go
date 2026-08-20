package ui

import (
	"bytes"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

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

func TestLongExplanationWrapsInsideSectionMargins(t *testing.T) {
	var output bytes.Buffer
	console := New(strings.NewReader(""), &output, &output, true)
	console.width = 36

	console.Reply(model.Reply{
		Info: "Memory pressure is low and CPU load remains comfortably within the machine's available capacity.",
	})

	rendered := strings.TrimSuffix(strings.TrimPrefix(output.String(), "\n"), "\n")
	lines := strings.Split(rendered, "\n")
	if len(lines) < 3 {
		t.Fatalf("explanation did not wrap: %q", output.String())
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, strings.Repeat(" ", sectionLeftMargin)) {
			t.Fatalf("wrapped line lost left margin: %q in %q", line, output.String())
		}
		if width := lipgloss.Width(line); width > console.width-sectionRightMargin {
			t.Fatalf("wrapped line width = %d, want <= %d: %q", width, console.width-sectionRightMargin, line)
		}
	}
}
