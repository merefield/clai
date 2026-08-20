package main

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

func TestLookupSearchesThenReturnsPlainTextExtract(t *testing.T) {
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		if request.Header.Get("User-Agent") == "" {
			t.Error("missing User-Agent")
		}
		query := request.URL.Query()
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

	wikipedia := newWikipediaTool(client)
	_, output, err := wikipedia.lookup(context.Background(), nil, arguments{Query: "K-Pg extinction"})
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || !output.Found || output.Summary != "A mass extinction event." {
		t.Fatalf("requests=%d result=%#v", requests, output)
	}
	if output.URL != "https://en.wikipedia.org/wiki/Cretaceous%E2%80%93Paleogene_extinction_event" {
		t.Fatalf("url = %q", output.URL)
	}
}

func TestLookupValidatesArgumentsAndResponseLimits(t *testing.T) {
	wikipedia := newWikipediaTool(nil)
	for _, input := range []arguments{
		{Query: ""},
		{Query: "topic", Language: "../../internal"},
	} {
		if _, _, err := wikipedia.lookup(context.Background(), nil, input); err == nil {
			t.Fatalf("expected validation error for %#v", input)
		}
	}

	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(strings.Repeat("x", maxResponseSize+1)), nil
	})}
	wikipedia = newWikipediaTool(client)
	if _, _, err := wikipedia.lookup(context.Background(), nil, arguments{Query: "topic"}); err == nil || !strings.Contains(err.Error(), "response exceeds") {
		t.Fatalf("unexpected oversized-response error: %v", err)
	}
}

func TestLookupReportsMediaWikiAPIErrors(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(`{"error":{"code":"badrequest","info":"invalid search"}}`), nil
	})}
	wikipedia := newWikipediaTool(client)
	_, _, err := wikipedia.lookup(context.Background(), nil, arguments{Query: "topic"})
	if err == nil || !strings.Contains(err.Error(), "badrequest") {
		t.Fatalf("unexpected API error: %v", err)
	}
}

func TestResultIsJSONMarshalable(t *testing.T) {
	if _, err := json.Marshal(result{Found: true, Query: "test", Language: "en"}); err != nil {
		t.Fatal(err)
	}
}
