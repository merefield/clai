package history

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/merefield/clai/internal/model"
)

type Store struct {
	Path     string
	Messages []model.Message
	dirty    bool
}

func DefaultPath() (string, error) {
	if path := os.Getenv("CLAI_HISTORY"); path != "" {
		return path, nil
	}
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		state = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(state, "clai", "history_com.json"), nil
}

func Load(path string) (*Store, error) {
	s := &Store{Path: path, Messages: []model.Message{}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &s.Messages); err != nil {
		return s, fmt.Errorf("parse history: %w", err)
	}
	s.dropSystemMessages()
	return s, nil
}

func (s *Store) dropSystemMessages() {
	filtered := s.Messages[:0]
	for _, message := range s.Messages {
		if message.Role != "system" {
			filtered = append(filtered, message)
		}
	}
	s.Messages = filtered
}

func (s *Store) Append(message model.Message) {
	s.Messages = append(s.Messages, message)
	s.dirty = true
}

func (s *Store) AppendText(role, content string) {
	s.Append(model.TextMessage(role, content))
}

func (s *Store) AppendReply(reply model.Reply) error {
	b, err := json.Marshal(reply)
	if err != nil {
		return err
	}
	s.Append(model.TextMessage("assistant", string(b)))
	return nil
}

func (s *Store) AppendToolCall(call model.ToolCall) {
	s.AppendToolCalls([]model.ToolCall{call})
}

func (s *Store) AppendToolCalls(calls []model.ToolCall) {
	m := model.NullMessage("assistant")
	m.ToolCalls = append([]model.ToolCall(nil), calls...)
	s.Append(m)
}

func (s *Store) AppendToolResult(id, output string) {
	m := model.TextMessage("tool", output)
	m.ToolCallID = id
	s.Append(m)
}

func (s *Store) AppendCommandResult(result model.CommandResult) error {
	b, err := json.Marshal(map[string]any{"command_result": result})
	if err != nil {
		return err
	}
	s.Append(model.TextMessage("assistant", string(b)))
	return nil
}

func (s *Store) Save(maxTurns int) error {
	if !s.dirty {
		return nil
	}
	if maxTurns < 1 {
		maxTurns = 1
	}
	s.dropSystemMessages()
	start := 0
	var userIndexes []int
	for i, message := range s.Messages {
		if message.Role == "user" {
			userIndexes = append(userIndexes, i)
		}
	}
	if len(userIndexes) > maxTurns {
		start = userIndexes[len(userIndexes)-maxTurns]
	} else if len(userIndexes) == 0 && len(s.Messages) > maxTurns {
		start = len(s.Messages) - maxTurns
	}
	s.Messages = append([]model.Message(nil), s.Messages[start:]...)
	b, err := json.Marshal(s.Messages)
	if err != nil {
		return err
	}
	if err := atomicWrite(s.Path, append(b, '\n')); err != nil {
		return err
	}
	s.dirty = false
	return nil
}

func (s *Store) Clear() error {
	s.Messages = []model.Message{}
	s.dirty = false
	if err := os.Remove(s.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *Store) Render(verbose bool) string {
	if len(s.Messages) == 0 {
		return "No CLAI history.\n"
	}
	var b strings.Builder
	for i, message := range s.Messages {
		content := message.ContentText()
		switch {
		case message.Role == "user":
			fmt.Fprintf(&b, "[%d] user\n%s\n", i+1, indent(content, 2))
		case message.Role == "tool":
			fmt.Fprintf(&b, "[%d] tool %s\n%s\n", i+1, message.ToolCallID, indent(content, 2))
		case len(message.ToolCalls) > 0:
			fmt.Fprintf(&b, "[%d] assistant tool call\n", i+1)
			for _, call := range message.ToolCalls {
				fmt.Fprintf(&b, "  name: %s\n  arguments:\n%s\n", call.Function.Name, indent(call.Function.Arguments, 4))
			}
		case message.Role == "assistant":
			if renderStructured(&b, i+1, content, verbose) {
				break
			}
			fmt.Fprintf(&b, "[%d] assistant\n%s\n", i+1, indent(content, 2))
		default:
			fmt.Fprintf(&b, "[%d] %s\n%s\n", i+1, message.Role, indent(content, 2))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func renderStructured(b *strings.Builder, index int, content string, verbose bool) bool {
	var resultEnvelope struct {
		CommandResult *model.CommandResult `json:"command_result"`
	}
	if json.Unmarshal([]byte(content), &resultEnvelope) == nil && resultEnvelope.CommandResult != nil {
		r := resultEnvelope.CommandResult
		fmt.Fprintf(b, "[%d] command result\n  command: %s\n  exit_code: %d\n  edited: %t\n", index, r.Command, r.ExitCode, r.Edited)
		if verbose {
			if r.Stdout != "" {
				fmt.Fprintf(b, "  stdout:\n%s\n", indent(r.Stdout, 4))
			}
			if r.Stderr != "" {
				fmt.Fprintf(b, "  stderr:\n%s\n", indent(r.Stderr, 4))
			}
		} else {
			fmt.Fprintf(b, "%s\n%s\n", preview("stdout", r.Stdout), preview("stderr", r.Stderr))
		}
		return true
	}
	var reply model.Reply
	if json.Unmarshal([]byte(content), &reply) == nil && (reply.Info != "" || reply.Command != "") {
		fmt.Fprintf(b, "[%d] assistant\n  info: %s\n  risk: %s\n  cmd: %s\n", index, reply.Info, reply.Risk, reply.Command)
		return true
	}
	return false
}

func preview(name, value string) string {
	if value == "" {
		return "  " + name + ": empty"
	}
	lines := strings.Split(value, "\n")
	truncated := ""
	if len(lines) > 3 {
		lines = lines[:3]
		truncated = "\n    [truncated after first 3 lines]"
	}
	return "  " + name + ":\n" + indent(strings.Join(lines, "\n"), 4) + truncated
}

func indent(value string, spaces int) string {
	prefix := strings.Repeat(" ", spaces)
	return prefix + strings.ReplaceAll(value, "\n", "\n"+prefix)
}

func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp.*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
