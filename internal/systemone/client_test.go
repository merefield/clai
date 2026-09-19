package systemone

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestConfiguredRequiresKeyAPIAndModel(t *testing.T) {
	if Configured("", "https://api.typesafe.ai/v1/systemone", "jev-latest") {
		t.Fatal("empty key should not be configured")
	}
	if !Configured("key", "https://api.typesafe.ai/v1/systemone", "jev-latest") {
		t.Fatal("complete system one config should be configured")
	}
}

func TestRouteIntentUsesChoiceQuestion(t *testing.T) {
	var request map[string]any
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer system-key" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		body := `{"answers":{"intent":{"type":"choice","choice":"question","confidence":0.88,"probabilities":{"question":0.88,"execute":0.11,"clear_history":0.01}}}}`
		return jsonResponse(body), nil
	})}
	client := New("system-key", "https://system-one.example/v1/systemone", "jev-test", httpClient)
	decision, err := client.RouteIntent(context.Background(), IntentRequest{UserRequest: "how much is 3*pi"})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Intent != IntentQuestion || decision.Confidence != 0.88 {
		t.Fatalf("decision = %#v", decision)
	}
	if request["model"] != "jev-test" {
		t.Fatalf("request = %#v", request)
	}
	questions := request["questions"].(map[string]any)
	intent := questions["intent"].(map[string]any)
	if intent["type"] != "choice" {
		t.Fatalf("intent question = %#v", intent)
	}
}

func TestAuditRiskUsesChoiceQuestion(t *testing.T) {
	var request map[string]any
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		body := `{"answers":{"risk":{"type":"choice","choice":"danger_zone","confidence":0.93,"probabilities":{"danger_zone":0.93,"reversible_change":0.06,"none":0.01}}}}`
		return jsonResponse(body), nil
	})}
	client := New("system-key", "https://system-one.example/v1/systemone", "jev-test", httpClient)
	decision, err := client.AuditRisk(context.Background(), RiskRequest{UserRequest: "remove it", Command: "rm -rf tmp", LLMRisk: "none"})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Risk != "danger_zone" || decision.Confidence != 0.93 {
		t.Fatalf("decision = %#v", decision)
	}
	state := request["state"].(map[string]any)
	if state["command"] != "rm -rf tmp" || state["llm_risk"] != "none" {
		t.Fatalf("state = %#v", state)
	}
}

func jsonResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestConfidenceValidation(t *testing.T) {
	for _, value := range []float64{-1, 2, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if ValidConfidence(value) {
			t.Errorf("accepted invalid confidence %v", value)
		}
	}
	for _, value := range []float64{0, 0.65, 1} {
		if !ValidConfidence(value) {
			t.Errorf("rejected valid confidence %v", value)
		}
	}
}

func TestResponseConfidenceRange(t *testing.T) {
	for _, kind := range []string{"intent", "risk"} {
		for _, confidence := range []string{"-0.1", "2", "0", "1"} {
			t.Run(kind+"/"+confidence, func(t *testing.T) {
				httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					return jsonResponse(fmt.Sprintf(`{"answers":{"%s":{"type":"choice","choice":"none","confidence":%s}}}`, kind, confidence)), nil
				})}
				client := New("key", "https://example.test", "model", httpClient)
				var err error
				if kind == "intent" {
					_, err = client.RouteIntent(context.Background(), IntentRequest{})
				} else {
					_, err = client.AuditRisk(context.Background(), RiskRequest{})
				}
				invalid := confidence == "-0.1" || confidence == "2"
				if (err != nil) != invalid {
					t.Fatalf("confidence %s: error = %v", confidence, err)
				}
			})
		}
	}
}
