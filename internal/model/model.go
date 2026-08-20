package model

import "encoding/json"

const (
	RiskNone       = "none"
	RiskReversible = "reversible change"
	RiskDanger     = "danger zone"
)

type Variable struct {
	Name   string `json:"name"`
	Prompt string `json:"prompt"`
}

type Reply struct {
	Command   string     `json:"cmd"`
	Info      string     `json:"info"`
	Risk      string     `json:"risk"`
	Variables []Variable `json:"variables"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type Message struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall      `json:"tool_calls,omitempty"`
}

func TextMessage(role, content string) Message {
	b, _ := json.Marshal(content)
	return Message{Role: role, Content: b}
}

func NullMessage(role string) Message {
	return Message{Role: role, Content: json.RawMessage("null")}
}

func (m Message) ContentText() string {
	if len(m.Content) == 0 || string(m.Content) == "null" {
		return ""
	}
	var value string
	if json.Unmarshal(m.Content, &value) == nil {
		return value
	}
	return string(m.Content)
}

type CommandResult struct {
	Command  string `json:"command"`
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Edited   bool   `json:"edited"`
}

type ToolDefinition map[string]any
