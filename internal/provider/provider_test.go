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
	if Detect("https://api.anthropic.com/v1/messages") != Anthropic {
		t.Fatal("anthropic detection")
	}
	if Detect("http://localhost:11434/api/chat") != Generic {
		t.Fatal("generic detection")
	}
}
