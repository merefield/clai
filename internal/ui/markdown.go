package ui

import (
	"os"
	"strings"

	"charm.land/glamour/v2"
	glamouransi "charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	"github.com/charmbracelet/x/ansi"
)

func (c *Console) explanation(text string) {
	if text == "" {
		return
	}
	terminalWidth := c.terminalWidth()
	prose := safeTerminalText(text)
	if terminalWidth == 0 || os.Getenv("TERM") == "dumb" {
		c.section(prose)
		return
	}
	width := terminalWidth - sectionLeftMargin - sectionRightMargin
	if c.Markdown && width >= 12 && boundedMarkdown(text) {
		prose = renderMarkdown(prose, width)
	}
	if os.Getenv("NO_COLOR") != "" {
		prose = ansi.Strip(prose)
	} else if !c.highContrast {
		faint := ansi.Style{}.Faint()
		// Markdown spans reset their styling after bold, italics, and code.
		// Restore the prose intensity after each span and reset it at the end.
		prose = faint.Styled(strings.ReplaceAll(prose, ansi.ResetStyle, ansi.ResetStyle+faint.String()))
	}
	c.section(prose)
}

func renderMarkdown(text string, width int) string {
	// Use the terminal's own foreground/background so prose works with both
	// light and dark themes, including high-contrast terminals. Do not query
	// the terminal background: those replies could consume confirmation input.
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(markdownStyle()),
		glamour.WithWordWrap(width),
		glamour.WithPreservedNewLines(),
		glamour.WithInlineTableLinks(true),
		// Preserve literal backslashes in code blocks. Glamour's unhighlighted
		// fallback applies prose escape handling; the noop formatter does not.
		glamour.WithChromaFormatter("noop"),
	)
	if err != nil {
		return text
	}
	rendered, err := renderer.Render(text)
	if err != nil || strings.TrimSpace(ansi.Strip(rendered)) == "" {
		return text
	}
	return strings.Trim(safeMarkdownOutput(rendered), "\n")
}

func markdownStyle() glamouransi.StyleConfig {
	style := styles.NoTTYStyleConfig
	style.Document.Margin = new(uint(0))
	style.Heading.Bold = new(true)
	style.H1.Prefix = "◆ "
	style.H2.Prefix = "▌ "
	style.H3.Prefix = "▎ "
	style.H4.Prefix = "▏ "
	style.H5.Prefix = "• "
	style.H6.Prefix = "· "
	style.Emph = glamouransi.StylePrimitive{Italic: new(true)}
	style.Strong = glamouransi.StylePrimitive{Bold: new(true)}
	style.Strikethrough = glamouransi.StylePrimitive{CrossedOut: new(true)}
	style.Code.StylePrimitive = glamouransi.StylePrimitive{Inverse: new(true)}
	style.CodeBlock.Theme = "github"
	style.CodeBlock.Margin = new(uint(0))
	style.CodeBlock.Indent = new(uint(2))
	style.BlockQuote.IndentToken = new("│ ")
	style.Link.Underline = new(true)
	style.Task.Ticked = "☑ "
	style.Task.Unticked = "☐ "
	style.Table.ColumnSeparator = new("│")
	style.Table.RowSeparator = new("─")
	style.Table.CenterSeparator = new("┼")
	return style
}

// Bound parsing and the rectangular expansion of ragged Markdown tables.
// Responses beyond these limits retain their original, sanitized text.
func boundedMarkdown(text string) bool {
	if len(text) > 32*1024 {
		return false
	}
	rows, columns := 0, 0
	for line := range strings.SplitSeq(text, "\n") {
		if strings.TrimSpace(line) == "" {
			rows, columns = 0, 0
			continue
		}
		rows++
		columns = max(columns, strings.Count(line, "|")+1)
		if columns > 64 || rows*columns > 4096 {
			return false
		}
	}
	return true
}

// Keep renderer-generated SGR styling only. In particular, remove OSC links
// and any terminal controls decoded from Markdown entities or destinations.
// Destinations remain visible as text, and nothing here opens or fetches them.
func safeMarkdownOutput(text string) string {
	var out strings.Builder
	var state byte
	parser := ansi.NewParser()
	for len(text) > 0 {
		sequence, _, size, next := ansi.DecodeSequence(text, state, parser)
		if size == 0 {
			break
		}
		if strings.HasPrefix(sequence, "\x1b[") && parser.Command() == 'm' {
			out.WriteString(sequence)
		} else {
			out.WriteString(safeTerminalText(sequence))
		}
		text, state = text[size:], next
	}
	return out.String()
}
