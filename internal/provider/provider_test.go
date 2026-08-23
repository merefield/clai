package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/merefield/clai/internal/config"
	"github.com/merefield/clai/internal/model"
)

func TestOpenAIPayloadUsesStructuredOutputAndTools(t *testing.T) {
	cfg := &config.Config{API: "https://api.openai.com/v1/chat/completions", Model: "gpt-4.1", Key: "secret", JSONMode: true, Tokens: 500, Temperature: 0.1}
	client := &HTTPClient{config: cfg, kind: OpenAI}
	payload := client.openAIPayload(Request{Messages: []model.Message{model.TextMessage("user", "hello")}, Tools: []model.ToolDefinition{{"type": "function"}}})
	format, ok := payload["response_format"].(map[string]any)
	if !ok || format["type"] != "json_schema" {
		t.Fatalf("response_format = %#v", payload["response_format"])
	}
	if payload["tool_choice"] != "auto" {
		t.Fatalf("tool choice = %#v", payload["tool_choice"])
	}
}

func TestResponsesPayloadUsesNativeInputStructuredOutputReasoningAndTools(t *testing.T) {
	cfg := &config.Config{API: "https://api.openai.com/v1/responses", Model: "gpt-5.2", Key: "secret", JSONMode: true, Reasoning: "medium", Tokens: 500, Temperature: 0.1}
	client := &HTTPClient{config: cfg, kind: OpenAI}
	payload := client.openAIPayload(Request{
		Messages: []model.Message{
			model.TextMessage("system", "follow policy"),
			model.TextMessage("user", "hello"),
			model.TextMessage("assistant", "hi"),
			{Role: "assistant", ToolCalls: []model.ToolCall{{ID: "call_1", Type: "function", Function: model.FunctionCall{Name: "wiki__lookup", Arguments: `{"query":"Ada"}`}}}},
			func() model.Message {
				message := model.TextMessage("tool", `{"answer":"Ada Lovelace"}`)
				message.ToolCallID = "call_1"
				return message
			}(),
		},
		Tools: []model.ToolDefinition{{"type": "function", "function": map[string]any{"name": "wiki__lookup", "description": "looks up a topic", "parameters": map[string]any{"type": "object"}}}},
	})
	if payload["instructions"] != "follow policy" {
		t.Fatalf("instructions = %#v", payload["instructions"])
	}
	if _, exists := payload["messages"]; exists {
		t.Fatalf("responses payload should not contain chat messages: %#v", payload)
	}
	input := payload["input"].([]map[string]any)
	if input[0]["role"] != "user" || input[1]["role"] != "assistant" || input[2]["type"] != "function_call" || input[3]["type"] != "function_call_output" {
		t.Fatalf("input = %#v", input)
	}
	text := payload["text"].(map[string]any)
	if text["format"].(map[string]any)["type"] != "json_schema" {
		t.Fatalf("text format = %#v", text)
	}
	if payload["reasoning"].(map[string]any)["effort"] != "medium" {
		t.Fatalf("reasoning = %#v", payload["reasoning"])
	}
	tools := payload["tools"].([]model.ToolDefinition)
	if tools[0]["name"] != "wiki__lookup" {
		t.Fatalf("tools = %#v", tools)
	}
	if _, exists := tools[0]["function"]; exists {
		t.Fatalf("responses tool should be flat: %#v", tools[0])
	}
}

func TestCompleteOpenAIResponsesParsesMessagesAndToolCalls(t *testing.T) {
	var request map[string]any
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://api.openai.com/v1/responses" {
			t.Errorf("endpoint = %q", r.URL.String())
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		body := `{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"partial "},{"type":"output_text","text":"answer"}]},{"type":"function_call","call_id":"call_abc","name":"wiki__lookup","arguments":"{\"query\":\"Ada\"}"}]}`
		return jsonResponse(body), nil
	})}
	cfg := &config.Config{API: "https://api.openai.com/v1/responses", Model: "gpt-5.2", Key: "test-key", Tokens: 42, Temperature: 0.2}
	client := New(cfg, httpClient)
	response, err := client.Complete(context.Background(), Request{Messages: []model.Message{model.TextMessage("user", "hello")}})
	if err != nil {
		t.Fatal(err)
	}
	if response.Text != "partial answer" || response.FinishReason != "tool_calls" || len(response.ToolCalls) != 1 {
		t.Fatalf("response = %#v", response)
	}
	call := response.ToolCalls[0]
	if call.ID != "call_abc" || call.Function.Name != "wiki__lookup" || call.Function.Arguments != `{"query":"Ada"}` {
		t.Fatalf("tool call = %#v", call)
	}
	if request["max_output_tokens"].(float64) != 42 {
		t.Fatalf("payload = %#v", request)
	}
}

func TestCompleteGenericOpenAICompatible(t *testing.T) {
	var request map[string]any
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		body := `{"choices":[{"message":{"content":"{\"cmd\":\"\",\"info\":\"ok\",\"risk\":\"none\",\"variables\":[]}"},"finish_reason":"stop"}]}`
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	cfg := &config.Config{API: "http://generic.invalid/v1/chat/completions", Model: "test", Key: "test-key", Tokens: 42, Temperature: 0.2}
	client := New(cfg, httpClient)
	response, err := client.Complete(context.Background(), Request{Messages: []model.Message{model.TextMessage("user", "hello")}})
	if err != nil {
		t.Fatal(err)
	}
	if response.FinishReason != "stop" || response.Text == "" {
		t.Fatalf("response = %#v", response)
	}
	if request["max_completion_tokens"].(float64) != 42 {
		t.Fatalf("payload = %#v", request)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestGeminiEndpointUsesConfiguredModel(t *testing.T) {
	got := geminiEndpoint("https://generativelanguage.googleapis.com/v1beta/models/old:generateContent", "gemini-new")
	want := "https://generativelanguage.googleapis.com/v1beta/models/gemini-new:generateContent"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestDetectProviders(t *testing.T) {
	if Detect("https://api.openai.com/v1/chat/completions") != OpenAI {
		t.Fatal("openai detection")
	}
	if Detect("https://api.openai.com/v1/responses") != OpenAI {
		t.Fatal("openai responses detection")
	}
	if Detect("https://api.anthropic.com/v1/messages") != Anthropic {
		t.Fatal("anthropic detection")
	}
	if Detect("http://localhost:11434/api/chat") != Generic {
		t.Fatal("generic detection")
	}
}

func TestCompleteAnthropicBuildsAndParsesNativeMessages(t *testing.T) {
	var request map[string]any
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("x-api-key") != "anthropic-key" || r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("headers = %#v", r.Header)
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		body := `{"content":[{"type":"text","text":"first "},{"type":"text","text":"second"}],"stop_reason":"end_turn"}`
		return jsonResponse(body), nil
	})}
	cfg := &config.Config{API: "https://api.anthropic.com/v1/messages", Model: "claude-test", Key: "anthropic-key", JSONMode: true, Tokens: 321, Temperature: 0.3}
	client := New(cfg, httpClient)
	response, err := client.Complete(context.Background(), Request{Messages: []model.Message{
		model.TextMessage("system", "system one"),
		model.TextMessage("system", "system two"),
		model.TextMessage("user", "hello"),
		model.TextMessage("assistant", "hi"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if response.Text != "first second" || response.FinishReason != "end_turn" {
		t.Fatalf("response = %#v", response)
	}
	if request["system"] != "system one\n\nsystem two" || request["model"] != "claude-test" || request["max_tokens"].(float64) != 321 {
		t.Fatalf("payload = %#v", request)
	}
	messages := request["messages"].([]any)
	if messages[0].(map[string]any)["role"] != "user" || messages[1].(map[string]any)["role"] != "assistant" {
		t.Fatalf("message roles = %#v", messages)
	}
	outputConfig, ok := request["output_config"].(map[string]any)
	if !ok || outputConfig["format"].(map[string]any)["type"] != "json_schema" {
		t.Fatalf("structured output = %#v", request["output_config"])
	}
}

func TestCompleteGeminiBuildsAndParsesNativeContent(t *testing.T) {
	var request map[string]any
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://generativelanguage.googleapis.com/v1beta/models/gemini-new:generateContent" {
			t.Errorf("endpoint = %q", r.URL.String())
		}
		if r.Header.Get("x-goog-api-key") != "gemini-key" {
			t.Errorf("API key = %q", r.Header.Get("x-goog-api-key"))
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		body := `{"candidates":[{"content":{"parts":[{"text":"gemini "},{"text":"answer"}]},"finishReason":"STOP"}]}`
		return jsonResponse(body), nil
	})}
	cfg := &config.Config{API: "https://generativelanguage.googleapis.com/v1beta", Model: "gemini-new", Key: "gemini-key", JSONMode: true, Tokens: 222, Temperature: 0.4}
	client := New(cfg, httpClient)
	response, err := client.Complete(context.Background(), Request{Messages: []model.Message{
		model.TextMessage("system", "follow policy"),
		model.TextMessage("user", "hello"),
		model.TextMessage("assistant", "hi"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if response.Text != "gemini answer" || response.FinishReason != "STOP" {
		t.Fatalf("response = %#v", response)
	}
	contents := request["contents"].([]any)
	if contents[0].(map[string]any)["role"] != "user" || contents[1].(map[string]any)["role"] != "model" {
		t.Fatalf("content roles = %#v", contents)
	}
	if request["systemInstruction"].(map[string]any)["parts"].([]any)[0].(map[string]any)["text"] != "follow policy" {
		t.Fatalf("system instruction = %#v", request["systemInstruction"])
	}
	generation := request["generationConfig"].(map[string]any)
	if generation["maxOutputTokens"].(float64) != 222 || generation["responseMimeType"] != "application/json" {
		t.Fatalf("generation config = %#v", generation)
	}
}

func TestCompleteOllamaUsesNativeOptionsAndParsesToolCalls(t *testing.T) {
	var request map[string]any
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		body := `{"message":{"content":{"cmd":"","info":"ok","risk":"none","variables":[]},"tool_calls":[{"function":{"name":"wiki__lookup","arguments":{"query":"Ada Lovelace"}}}]},"done_reason":"stop"}`
		return jsonResponse(body), nil
	})}
	cfg := &config.Config{API: "http://localhost:11434/api/chat", Model: "qwen-test", Key: "unused", JSONMode: true, Reasoning: "high", Tokens: 111, Temperature: 0.5}
	client := New(cfg, httpClient)
	response, err := client.Complete(context.Background(), Request{
		Messages: []model.Message{model.TextMessage("user", "look up Ada")},
		Tools:    []model.ToolDefinition{{"type": "function"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if request["stream"] != false || request["format"] != "json" || request["think"] != true || request["tool_choice"] != "auto" {
		t.Fatalf("payload = %#v", request)
	}
	options := request["options"].(map[string]any)
	if options["num_predict"].(float64) != 111 || options["temperature"].(float64) != 0.5 {
		t.Fatalf("options = %#v", options)
	}
	if response.FinishReason != "stop" || !strings.Contains(response.Text, `"info":"ok"`) || len(response.ToolCalls) != 1 {
		t.Fatalf("response = %#v", response)
	}
	call := response.ToolCalls[0]
	if call.ID != "ollama_tool_call_0" || call.Function.Name != "wiki__lookup" || call.Function.Arguments != `{"query":"Ada Lovelace"}` {
		t.Fatalf("tool call = %#v", call)
	}
}

func jsonResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}
