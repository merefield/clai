package wikipedia

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestWikipediaLookupSearchesThenReturnsPlainTextExtract(t *testing.T) {
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		if request.Header.Get("User-Agent") == "" {
			t.Error("missing User-Agent")
		}
		query := request.URL.Query()
		if query.Get("format") != "json" || query.Get("formatversion") != "2" {
			t.Errorf("common query parameters = %v", query)
		}
		switch query.Get("list") {
		case "search":
			if query.Get("srsearch") != "K-Pg extinction" || query.Get("srlimit") != "1" {
				t.Errorf("search query = %v", query)
			}
			return jsonResponse(`{"query":{"search":[{"pageid":123,"title":"Cretaceous–Paleogene extinction event"}]}}`), nil
		default:
			if query.Get("prop") != "extracts" || query.Get("pageids") != "123" || query.Get("explaintext") != "1" {
				t.Errorf("extract query = %v", query)
			}
			return jsonResponse(`{"query":{"pages":[{"pageid":123,"title":"Cretaceous–Paleogene extinction event","extract":"A mass extinction event."}]}}`), nil
		}
	})}

	plugin := New(client)
	value, err := plugin.Execute(context.Background(), json.RawMessage(`{"query":"K-Pg extinction"}`))
	if err != nil {
		t.Fatal(err)
	}
	result := value.(Result)
	if requests != 2 || !result.Found || result.Summary != "A mass extinction event." {
		t.Fatalf("requests=%d result=%#v", requests, result)
	}
	if result.URL != "https://en.wikipedia.org/wiki/Cretaceous%E2%80%93Paleogene_extinction_event" {
		t.Fatalf("url = %q", result.URL)
	}
}

func TestWikipediaLookupReturnsStructuredNotFoundResult(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(`{"query":{"search":[]}}`), nil
	})}
	plugin := New(client)
	value, err := plugin.Execute(context.Background(), json.RawMessage(`{"query":"no such topic","language":"de"}`))
	if err != nil {
		t.Fatal(err)
	}
	result := value.(Result)
	if result.Found || result.Language != "de" || result.Query != "no such topic" {
		t.Fatalf("result = %#v", result)
	}
}

func TestWikipediaLookupValidatesArgumentsAndResponseLimits(t *testing.T) {
	plugin := New(nil)
	for _, arguments := range []string{
		`{"query":""}`,
		`{"query":"topic","language":"../../internal"}`,
		`{"query":"topic","unexpected":true}`,
	} {
		if _, err := plugin.Execute(context.Background(), json.RawMessage(arguments)); err == nil {
			t.Fatalf("expected validation error for %s", arguments)
		}
	}

	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(strings.Repeat("x", maxResponseSize+1)), nil
	})}
	plugin = New(client)
	if _, err := plugin.Execute(context.Background(), json.RawMessage(`{"query":"topic"}`)); err == nil || !strings.Contains(err.Error(), "response exceeds") {
		t.Fatalf("unexpected oversized-response error: %v", err)
	}
}

func TestWikipediaLookupReportsMediaWikiAPIErrors(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(`{"error":{"code":"badrequest","info":"invalid search"}}`), nil
	})}
	plugin := New(client)
	_, err := plugin.Execute(context.Background(), json.RawMessage(`{"query":"topic"}`))
	if err == nil || !strings.Contains(err.Error(), "badrequest") {
		t.Fatalf("unexpected API error: %v", err)
	}
}
