package history

import (
	"encoding/json"
	"strings"

	"github.com/merefield/clai/internal/model"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// RecordCommand attaches an example or edited command to the current reply.
func (s *Store) RecordCommand(command string) {
	if command != "" && len(s.Messages) > 0 && s.Messages[len(s.Messages)-1].Role == "assistant" {
		s.Messages[len(s.Messages)-1].CopyCommand = command
		s.dirty = true
	}
}

// LatestCommand uses only the latest recorded turn and prefers the command
// run or proposed in that turn over any examples in its prose.
func (s *Store) LatestCommand() string {
	var example string
	for i := len(s.Messages) - 1; i >= 0; i-- {
		message := s.Messages[i]
		if message.Role == "user" {
			break
		}
		if message.Role != "assistant" || len(message.ToolCalls) > 0 {
			continue
		}
		if message.CopyCommand != "" {
			return message.CopyCommand
		}
		var reply struct {
			model.Reply
			Result *model.CommandResult `json:"command_result"`
		}
		if json.Unmarshal([]byte(message.ContentText()), &reply) != nil {
			continue
		}
		if reply.Result != nil && reply.Result.Command != "" {
			return reply.Result.Command
		}
		if reply.Command != "" {
			return reply.Command
		}
		if example == "" {
			example = shellExample(reply.Info)
		}
	}
	return example
}

func shellExample(info string) string {
	if !strings.Contains(info, "```") && !strings.Contains(info, "~~~") && !strings.HasPrefix(info, "    ") {
		return ""
	}
	source := []byte(info)
	document := goldmark.DefaultParser().Parse(text.NewReader(source))
	var example string
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch block := node.(type) {
		case *ast.FencedCodeBlock:
			switch strings.ToLower(string(block.Language(source))) {
			case "bash", "sh", "shell", "zsh":
			default:
				return ast.WalkSkipChildren, nil
			}
		case *ast.CodeBlock:
			// Releases before copy support stored how-to commands this way.
			if node != document.FirstChild() || !strings.HasPrefix(info, "    ") {
				return ast.WalkSkipChildren, nil
			}
		default:
			return ast.WalkContinue, nil
		}
		var command strings.Builder
		lines := node.Lines()
		for i := 0; i < lines.Len(); i++ {
			line := lines.At(i)
			command.Write(line.Value(source))
		}
		if snippet := strings.TrimSuffix(command.String(), "\n"); strings.TrimSpace(snippet) != "" {
			example = snippet
		}
		return ast.WalkSkipChildren, nil
	})
	return example
}
