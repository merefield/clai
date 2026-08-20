package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/merefield/clai/internal/config"
	"github.com/merefield/clai/internal/model"
)

type Kind string

const (
	OpenAI    Kind = "openai"
	Generic   Kind = "generic"
	Anthropic Kind = "anthropic"
	Gemini    Kind = "gemini"
)

type Request struct {
	Messages []model.Message
	Tools    []model.ToolDefinition
}

type Response struct {
	Text         string
	FinishReason string
	ToolCalls    []model.ToolCall
}

type Client interface {
	Complete(context.Context, Request) (Response, error)
	SupportsTools() bool
}

type HTTPClient struct {
	config *config.Config
	kind   Kind
	http   *http.Client
}

func New(cfg *config.Config, client *http.Client) *HTTPClient {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	return &HTTPClient{config: cfg, kind: Detect(cfg.API), http: client}
}

func Detect(endpoint string) Kind {
	switch {
	case strings.Contains(endpoint, "api.openai.com"):
		return OpenAI
	case strings.Contains(endpoint, "anthropic.com"):
		return Anthropic
	case strings.Contains(endpoint, "generativelanguage.googleapis.com"):
		return Gemini
	default:
		return Generic
	}
}

func (c *HTTPClient) SupportsTools() bool {
	return c.kind == OpenAI || c.kind == Generic
}

func (c *HTTPClient) Complete(ctx context.Context, input Request) (Response, error) {
	payload, endpoint, headers, err := c.build(input)
	if err != nil {
		return Response{}, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Response{}, fmt.Errorf("encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return Response{}, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return Response{}, fmt.Errorf("read response: %w", err)
	}
	if !json.Valid(responseBody) {
		return Response{}, fmt.Errorf("API returned non-JSON response (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := extractError(responseBody)
		if message == "" {
			message = strings.TrimSpace(string(responseBody))
		}
		return Response{}, fmt.Errorf("API request failed (HTTP %d): %s", resp.StatusCode, message)
	}
	return c.parse(responseBody)
}

func (c *HTTPClient) build(input Request) (map[string]any, string, map[string]string, error) {
	headers := map[string]string{"Content-Type": "application/json"}
	endpoint := c.config.API
	switch c.kind {
	case Anthropic:
		headers["x-api-key"] = c.config.Key
		headers["anthropic-version"] = "2023-06-01"
		return c.anthropicPayload(input), endpoint, headers, nil
	case Gemini:
		headers["x-goog-api-key"] = c.config.Key
		endpoint = geminiEndpoint(endpoint, c.config.Model)
		return c.geminiPayload(input), endpoint, headers, nil
	default:
		headers["Authorization"] = "Bearer " + c.config.Key
		return c.openAIPayload(input), endpoint, headers, nil
	}
}

func (c *HTTPClient) openAIPayload(input Request) map[string]any {
	payload := map[string]any{"model": c.config.Model, "messages": input.Messages}
	ollama := isOllama(c.config.API)
	if ollama {
		payload["stream"] = false
		payload["options"] = map[string]any{"num_predict": c.config.Tokens, "temperature": c.config.Temperature}
		if c.config.JSONMode || c.config.Reasoning != "" {
			payload["format"] = "json"
		}
		if c.config.Reasoning != "" {
			payload["think"] = true
		}
	} else {
		payload["temperature"] = c.config.Temperature
		payload["max_completion_tokens"] = c.config.Tokens
		if c.config.JSONMode {
			if c.kind == OpenAI {
				payload["response_format"] = map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "clai_response", "strict": true, "schema": responseSchema()}}
			} else {
				payload["response_format"] = map[string]any{"type": "json_object"}
			}
		}
		if c.config.Reasoning != "" && isCompletions(c.config.API) && (c.kind != OpenAI || isReasoningModel(c.config.Model)) {
			payload["reasoning_effort"] = c.config.Reasoning
		}
	}
	if len(input.Tools) > 0 {
		payload["tools"] = input.Tools
		payload["tool_choice"] = "auto"
	}
	return payload
}

func (c *HTTPClient) anthropicPayload(input Request) map[string]any {
	system, messages := splitSystem(input.Messages, "assistant")
	payload := map[string]any{"model": c.config.Model, "max_tokens": c.config.Tokens, "temperature": c.config.Temperature, "messages": messages}
	if system != "" {
		payload["system"] = system
	}
	if c.config.JSONMode {
		payload["output_config"] = map[string]any{"format": map[string]any{"type": "json_schema", "name": "clai_response", "schema": responseSchema()}}
	}
	return payload
}

func (c *HTTPClient) geminiPayload(input Request) map[string]any {
	system, messages := splitSystem(input.Messages, "model")
	contents := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		contents = append(contents, map[string]any{"role": message.Role, "parts": []map[string]string{{"text": message.ContentText()}}})
	}
	generation := map[string]any{"maxOutputTokens": c.config.Tokens, "temperature": c.config.Temperature}
	if c.config.JSONMode {
		generation["responseMimeType"] = "application/json"
		generation["responseJsonSchema"] = responseSchema()
	}
	payload := map[string]any{"contents": contents, "generationConfig": generation}
	if system != "" {
		payload["systemInstruction"] = map[string]any{"parts": []map[string]string{{"text": system}}}
	}
	return payload
}

func splitSystem(messages []model.Message, assistantRole string) (string, []model.Message) {
	var systems []string
	filtered := make([]model.Message, 0, len(messages))
	for _, message := range messages {
		if message.Role == "system" {
			systems = append(systems, message.ContentText())
			continue
		}
		if len(message.ToolCalls) > 0 {
			continue
		}
		if message.Role != "user" && message.Role != "assistant" {
			continue
		}
		if message.Role == "assistant" {
			message.Role = assistantRole
		}
		filtered = append(filtered, message)
	}
	return strings.Join(systems, "\n\n"), filtered
}

func (c *HTTPClient) parse(body []byte) (Response, error) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return Response{}, err
	}
	switch c.kind {
	case Anthropic:
		content, _ := raw["content"].([]any)
		var text strings.Builder
		for _, item := range content {
			if object, ok := item.(map[string]any); ok && object["type"] == "text" {
				fmt.Fprint(&text, object["text"])
			}
		}
		return Response{Text: text.String(), FinishReason: stringField(raw, "stop_reason")}, nil
	case Gemini:
		return parseGemini(raw)
	default:
		if isOllama(c.config.API) {
			message, _ := raw["message"].(map[string]any)
			return Response{Text: contentString(message["content"]), FinishReason: stringField(raw, "done_reason"), ToolCalls: parseToolCalls(message["tool_calls"])}, nil
		}
		choices, _ := raw["choices"].([]any)
		if len(choices) == 0 {
			return Response{}, errors.New("API returned no choices")
		}
		choice, _ := choices[0].(map[string]any)
		message, _ := choice["message"].(map[string]any)
		return Response{Text: contentString(message["content"]), FinishReason: stringField(choice, "finish_reason"), ToolCalls: parseToolCalls(message["tool_calls"])}, nil
	}
}

func parseGemini(raw map[string]any) (Response, error) {
	candidates, _ := raw["candidates"].([]any)
	if len(candidates) == 0 {
		return Response{}, errors.New("API returned no candidates")
	}
	candidate, _ := candidates[0].(map[string]any)
	content, _ := candidate["content"].(map[string]any)
	parts, _ := content["parts"].([]any)
	var text strings.Builder
	for _, part := range parts {
		if object, ok := part.(map[string]any); ok {
			fmt.Fprint(&text, object["text"])
		}
	}
	return Response{Text: text.String(), FinishReason: stringField(candidate, "finishReason")}, nil
}

func parseToolCalls(value any) []model.ToolCall {
	items, _ := value.([]any)
	calls := make([]model.ToolCall, 0, len(items))
	for index, item := range items {
		object, _ := item.(map[string]any)
		function, _ := object["function"].(map[string]any)
		if function == nil {
			continue
		}
		arguments := contentString(function["arguments"])
		id := contentString(object["id"])
		if id == "" {
			id = fmt.Sprintf("ollama_tool_call_%d", index)
		}
		calls = append(calls, model.ToolCall{ID: id, Type: "function", Function: model.FunctionCall{Name: contentString(function["name"]), Arguments: arguments}})
	}
	return calls
}

func contentString(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	b, _ := json.Marshal(value)
	return string(b)
}

func stringField(value map[string]any, key string) string {
	text, _ := value[key].(string)
	return text
}

func extractError(body []byte) string {
	var value map[string]any
	if json.Unmarshal(body, &value) != nil {
		return ""
	}
	if object, ok := value["error"].(map[string]any); ok {
		return contentString(object["message"])
	}
	return contentString(value["error"])
}

func geminiEndpoint(endpoint, selectedModel string) string {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return endpoint
	}
	path := parsed.Path
	if marker := strings.Index(path, "/models/"); marker >= 0 {
		action := strings.Index(path[marker+8:], ":")
		if action >= 0 {
			action += marker + 8
			path = path[:marker+8] + selectedModel + path[action:]
			parsed.Path = path
			return parsed.String()
		}
	}
	parsed.Path = strings.TrimSuffix(path, "/") + "/models/" + selectedModel + ":generateContent"
	return parsed.String()
}

func isCompletions(endpoint string) bool {
	return strings.Contains(endpoint, "/completions")
}

func isOllama(endpoint string) bool {
	return strings.Contains(endpoint, "/api/chat")
}

func isReasoningModel(name string) bool {
	name = strings.ToLower(name)
	return strings.HasPrefix(name, "o") || strings.HasPrefix(name, "gpt-5") || strings.HasPrefix(name, "codex")
}

func responseSchema() map[string]any {
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"cmd":       map[string]any{"type": "string", "description": "Shell command to run. Use an empty string if no command should be suggested."},
			"info":      map[string]any{"type": "string", "description": "Short explanation of the answer or command."},
			"risk":      map[string]any{"type": "string", "enum": []string{model.RiskNone, model.RiskReversible, model.RiskDanger}},
			"variables": map[string]any{"type": "array", "items": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"name": map[string]any{"type": "string"}, "prompt": map[string]any{"type": "string"}}, "required": []string{"name", "prompt"}}},
		},
		"required": []string{"cmd", "info", "risk", "variables"},
	}
}
