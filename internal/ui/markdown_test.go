package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/merefield/clai/internal/model"
)

func markdownConsole(t *testing.T, width int) (*Console, *bytes.Buffer) {
	t.Helper()
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("NO_COLOR", "")
	var output bytes.Buffer
	c := New(strings.NewReader(""), &output, &output, true)
	c.width = width
	return c, &output
}

func TestResponseMarkdownFormatsProseAndPreservesCode(t *testing.T) {
	c, output := markdownConsole(t, 72)
	code := `printf '%s\\n' '**literal**'`
	c.Reply(model.Reply{Info: "# Summary\n\n**Bold** and *italic* with `a_b`.\n\n- First\n- Second\n\n> Quoted\n\n- [ ] Pending\n- [x] Done\n\n```bash\n" + code + "\n```\n\n[Guide](https://example.com/guide)"})
	plain := ansi.Strip(output.String())
	for _, want := range []string{"◆ Summary", "Bold and italic", "a_b", "First", "Second", "│ Quoted", "☐ Pending", "☑ Done", code, "Guide", "https://example.com/guide"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("missing %q in rendered prose:\n%s", want, plain)
		}
	}
	for _, marker := range []string{"# Summary", "**Bold**", "*italic*", "`a_b`", "```"} {
		if strings.Contains(plain, marker) {
			t.Fatalf("unrendered markup %q:\n%s", marker, plain)
		}
	}
	if !strings.Contains(output.String(), "\x1b[") || strings.Contains(output.String(), "\x1b]") {
		t.Fatalf("expected styling with visible link destinations, got %q", output.String())
	}
}

func TestResponseProseDimmingPreservesMarkdownStyles(t *testing.T) {
	c, output := markdownConsole(t, 72)
	reply := model.Reply{Info: "Before **bold** and *italic* then `code` after."}
	c.Reply(reply)
	normal := output.String()
	output.Reset()
	c.highContrast = false
	c.Reply(reply)
	dimmed := output.String()
	faint := ansi.Style{}.Faint().String()
	for _, span := range []string{
		faint + "Before ",
		ansi.Style{}.Bold().Styled("bold") + faint + " and ",
		ansi.Style{}.Italic(true).Styled("italic") + faint + " then ",
		ansi.Style{}.Reverse(true).Styled("code") + faint + " after.",
	} {
		if !strings.Contains(dimmed, span) {
			t.Fatalf("missing styled span %q in %q", span, dimmed)
		}
	}
	if !strings.HasSuffix(dimmed, ansi.ResetStyle+"\n") {
		t.Fatalf("dimmed style leaked beyond the response: %q", dimmed)
	}
	if strings.Contains(normal, faint) || ansi.Strip(normal) != ansi.Strip(dimmed) {
		t.Fatalf("expected identical text with normal intensity in high contrast mode: %q, %q", normal, dimmed)
	}
}

func TestResponseMarkdownTablesRetainValuesAtNarrowWidths(t *testing.T) {
	for _, width := range []int{72, 24} {
		c, output := markdownConsole(t, width)
		c.Reply(model.Reply{Info: "Before.\n\n| City | Total |\n|:---|---:|\n| **界** | `3` |\n| London | 100 |\n\nAfter."})
		plain := ansi.Strip(output.String())
		for _, want := range []string{"Before.", "City", "Total", "界", "3", "London", "100", "After."} {
			if !strings.Contains(plain, want) {
				t.Fatalf("width %d lost %q:\n%s", width, want, plain)
			}
		}
		for _, line := range strings.Split(plain, "\n") {
			if ansi.StringWidth(line) > width-sectionRightMargin {
				t.Fatalf("width %d overflow: %q", width, line)
			}
		}
	}
}

func TestResponseMarkdownLeavesCommandsAndWarningsLiteral(t *testing.T) {
	c, output := markdownConsole(t, 80)
	command := `printf '%s\n' '**literal**'`
	c.Reply(model.Reply{Command: command, Info: "# Warning\n\n**Review** this first.", Risk: model.RiskDanger})
	plain := ansi.Strip(output.String())
	if !strings.Contains(plain, command) || !strings.Contains(plain, "DANGER ZONE:") || !strings.Contains(plain, "◆ Warning") || !strings.Contains(plain, "Review this first.") {
		t.Fatalf("command, warning or explanation lost:\n%s", plain)
	}
	if strings.Index(plain, "DANGER ZONE:") > strings.Index(plain, "◆ Warning") {
		t.Fatal("provider prose appeared before the danger warning")
	}
}

func TestResponseMarkdownFallsBackToOriginalText(t *testing.T) {
	for _, scenario := range []string{"redirected", "disabled", "dumb", "tiny", "oversized", "large-table"} {
		t.Run(scenario, func(t *testing.T) {
			c, output := markdownConsole(t, 72)
			c.highContrast = false
			source := "# Summary\n\n**Literal Markdown**"
			switch scenario {
			case "redirected":
				c.width = 0
			case "disabled":
				c.Markdown = false
			case "dumb":
				t.Setenv("TERM", "dumb")
			case "tiny":
				c.width = 10
			case "oversized":
				source += strings.Repeat("x", 32*1024)
			case "large-table":
				source += "\n\n|" + strings.Repeat("x|", 65)
			}
			c.Reply(model.Reply{Info: source})
			plain := ansi.Strip(output.String())
			if (scenario == "redirected" || scenario == "dumb") && output.String() != plain {
				t.Fatalf("non-styling output contains ANSI: %q", output.String())
			}
			compact := strings.Join(strings.Fields(plain), "")
			if !strings.Contains(compact, "#Summary") || !strings.Contains(compact, "**LiteralMarkdown**") {
				t.Fatalf("fallback reformatted the source: %q", plain)
			}
			if scenario == "redirected" && plain != "\n  # Summary\n  \n  **Literal Markdown**\n" {
				t.Fatalf("redirected output changed: %q", plain)
			}
		})
	}
}

func TestResponseMarkdownNoColorKeepsReadableFormatting(t *testing.T) {
	c, output := markdownConsole(t, 60)
	c.highContrast = false
	t.Setenv("NO_COLOR", "1")
	c.Reply(model.Reply{Info: "## Result\n\n**Success** with `code`."})
	got := output.String()
	if strings.Contains(got, "\x1b") || !strings.Contains(got, "▌ Result") || !strings.Contains(got, "Success with code.") {
		t.Fatalf("NO_COLOR response = %q", got)
	}
}

func TestResponseMarkdownFiltersTerminalControlsAfterRendering(t *testing.T) {
	c, output := markdownConsole(t, 72)
	c.Reply(model.Reply{Info: "**Safe**\x1b[2J\x1b]52;c;clipboard\a\r\n\n[Guide](https://example.com)\n\n&#27;]52;c;encoded&#7;"})
	got := output.String()
	if strings.Contains(got, "\x1b]") || strings.Contains(got, "\x1b[2J") || strings.ContainsAny(got, "\a\r") {
		t.Fatalf("provider controls survived Markdown: %q", got)
	}
	if !strings.Contains(ansi.Strip(got), "Safe") || !strings.Contains(ansi.Strip(got), "https://example.com") {
		t.Fatalf("lost safe prose: %q", got)
	}
}
