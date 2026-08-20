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

func TestProviderTextCannotEmitTerminalControlSequences(t *testing.T) {
	var output bytes.Buffer
	console := New(strings.NewReader(""), &output, &output, true)

	console.Reply(model.Reply{
		Command: "printf safe\x1b[2J",
		Info:    "ordinary\x1b]52;c;Y2xpcGJvYXJkBw==\a text\r\x1b[31mred\x1b[0m",
	})

	got := output.String()
	if strings.Contains(got, "\x1b[2J") || strings.Contains(got, "\x1b]") || strings.ContainsAny(got, "\a\r") {
		t.Fatalf("unsafe terminal controls in output: %q", got)
	}
	if !strings.Contains(got, "printf safe") || !strings.Contains(got, "ordinary textred") {
		t.Fatalf("safe text was lost: %q", got)
	}
}

func TestDangerExplanationWrapsInsideSectionMargins(t *testing.T) {
	var output bytes.Buffer
	console := New(strings.NewReader(""), &output, &output, true)
	console.width = 38

	console.Reply(model.Reply{
		Command: "remove an example",
		Info:    "This permanently removes the selected example and cannot be reversed without a backup.",
		Risk:    model.RiskDanger,
	})

	for _, line := range strings.Split(strings.Trim(output.String(), "\n"), "\n") {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, strings.Repeat(" ", sectionLeftMargin)) {
			t.Fatalf("danger line lost left margin: %q in %q", line, output.String())
		}
		if width := lipgloss.Width(line); width > console.width-sectionRightMargin {
			t.Fatalf("danger line width = %d, want <= %d: %q", width, console.width-sectionRightMargin, line)
		}
	}
}
