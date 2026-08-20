package ui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/term"

	"github.com/merefield/clai/internal/model"
)

type Console struct {
	In           io.Reader
	Out          io.Writer
	Err          io.Writer
	reader       *bufio.Reader
	interactive  bool
	highContrast bool
	width        int
	mu           sync.Mutex
}

const (
	sectionLeftMargin  = 2
	sectionRightMargin = 2
)

func New(in io.Reader, out, errOut io.Writer, highContrast bool) *Console {
	interactive := false
	if f, ok := in.(*os.File); ok {
		interactive = term.IsTerminal(int(f.Fd()))
	}
	return &Console{In: in, Out: out, Err: errOut, reader: bufio.NewReader(in), interactive: interactive, highContrast: highContrast}
}

func (c *Console) Interactive() bool { return c.interactive }

func (c *Console) Info(text string) {
	if text == "" {
		return
	}
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Italic(true)
	if c.highContrast {
		style = lipgloss.NewStyle()
	}
	c.section(style.Render(text))
}

func (c *Console) Error(text string) {
	if text == "" {
		return
	}
	c.section(lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(text))
}

func (c *Console) OK(text string) {
	c.section(lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render(text))
}

func (c *Console) Cancel() {
	c.section(lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render("[cancel]"))
}

// BlankLine separates CLAI's explanation or confirmation from live command
// output without applying section styling.
func (c *Console) BlankLine() {
	c.mu.Lock()
	defer c.mu.Unlock()
	fmt.Fprintln(c.Out)
}

func (c *Console) Title(version string, tools []string) {
	style := lipgloss.NewStyle().Bold(true)
	c.section("🤖 " + style.Render("CLAI v"+version))
	if len(tools) > 0 {
		fmt.Fprintln(c.Out, "\n🔧 "+style.Render("Activated Tools"))
		for _, name := range tools {
			fmt.Fprintf(c.Out, "  %s\n", style.Render(name))
		}
	}
}

func (c *Console) Reply(reply model.Reply) {
	if reply.Command == "" {
		c.Info(reply.Info)
		return
	}
	color := "10"
	if reply.Risk == model.RiskReversible {
		color = "11"
	}
	if reply.Risk == model.RiskDanger {
		color = "9"
	}
	commandStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Background(lipgloss.Color("236"))
	c.section(commandStyle.Render(" " + reply.Command + " "))
	if reply.Risk == model.RiskDanger {
		label := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true).Render("DANGER ZONE: ")
		fmt.Fprintf(c.Out, "  %s%s\n", label, reply.Info)
	} else {
		c.Info(reply.Info)
	}
}

func (c *Console) Prompt(label string) (string, error) {
	fmt.Fprintf(c.Out, "\n  %s", label)
	line, err := c.reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func (c *Console) Secret(label string) (string, error) {
	fmt.Fprint(c.Out, label)
	if f, ok := c.In.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		value, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(c.Out)
		return strings.TrimSpace(string(value)), err
	}
	value, err := c.reader.ReadString('\n')
	return strings.TrimSpace(value), err
}

func (c *Console) Choice(label string) (string, error) {
	value, err := c.Prompt(label)
	if value == "" {
		return "", err
	}
	return strings.ToLower(value[:1]), err
}

func (c *Console) Spinner(ctx context.Context, title string) func() {
	if !c.interactive {
		fmt.Fprintf(c.Out, "\n  %s", title)
		return func() { fmt.Fprintln(c.Out) }
	}
	spinnerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		frames := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		index := 0
		for {
			select {
			case <-spinnerCtx.Done():
				c.mu.Lock()
				fmt.Fprint(c.Out, "\r\033[2K")
				c.mu.Unlock()
				return
			case <-ticker.C:
				c.mu.Lock()
				fmt.Fprintf(c.Out, "\r  %c %s", frames[index%len(frames)], title)
				c.mu.Unlock()
				index++
			}
		}
	}()
	return func() { cancel(); <-done }
}

func (c *Console) Setup(currentKey, currentAPI, currentModel string, currentRisk int) (key, api, selectedModel string, risk int, err error) {
	fmt.Fprintln(c.Out, "CLAI setup")
	if currentKey != "" {
		fmt.Fprintln(c.Out, "Press Enter on API key to keep the current value.")
	}
	key, err = c.Secret("API key: ")
	if err != nil && err != io.EOF {
		return
	}
	if key == "" {
		key = currentKey
	}
	if key == "" {
		err = fmt.Errorf("no API key provided; CLAI is not configured")
		return
	}
	api, err = c.promptDefault("API base URL", currentAPI)
	if err != nil {
		return
	}
	selectedModel, err = c.promptDefault("Model", currentModel)
	if err != nil {
		return
	}
	riskText, promptErr := c.promptDefault("Risk appetite (0=always prompt, 1=auto-run green, 2=auto-run amber)", strconv.Itoa(currentRisk))
	if promptErr != nil {
		err = promptErr
		return
	}
	risk, err = strconv.Atoi(riskText)
	if err != nil || risk < 0 || risk > 2 {
		risk = 0
		err = nil
	}
	return
}

func (c *Console) promptDefault(label, current string) (string, error) {
	value, err := c.Prompt(fmt.Sprintf("%s [%s]: ", label, current))
	if value == "" {
		value = current
	}
	return value, err
}

func (c *Console) section(text string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if width := c.terminalWidth(); width > sectionLeftMargin+sectionRightMargin {
		text = ansi.Wrap(text, width-sectionLeftMargin-sectionRightMargin, " ")
	}
	margin := strings.Repeat(" ", sectionLeftMargin)
	fmt.Fprintf(c.Out, "\n%s%s\n", margin, strings.ReplaceAll(text, "\n", "\n"+margin))
}

func (c *Console) terminalWidth() int {
	if c.width > 0 {
		return c.width
	}
	file, ok := c.Out.(*os.File)
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return 0
	}
	width, _, err := term.GetSize(int(file.Fd()))
	if err != nil {
		return 0
	}
	return width
}
