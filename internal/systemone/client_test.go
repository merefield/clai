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

func TestRejectsInsecureEndpoints(t *testing.T) {
	for _, endpoint := range []string{"http://example.test/v1/systemone", "http://localhost:8080", "", "example.test", "/v1/systemone", "https:///missing-host", "https://user:pass@example.test", "https://example.test/#fragment", "https://example.test:bad"} {
		t.Run(endpoint, func(t *testing.T) {
			if Configured("key", endpoint, "model") {
				t.Fatal("insecure endpoint enabled")
			}
			if _, err := New("key", endpoint, "model", nil); err == nil {
				t.Fatal("constructor accepted insecure endpoint")
			}
			client := &HTTPClient{Key: "key", API: endpoint}
			if _, err := client.AuditRisk(context.Background(), RiskRequest{}); err == nil {
				t.Fatal("request accepted insecure endpoint")
			}
		})
	}
}

func TestRedirectRequiresHTTPS(t *testing.T) {
	for _, scheme := range []string{"http", "https"} {
		t.Run(scheme, func(t *testing.T) {
			calls := 0
			httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{scheme + "://example.test/next"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
				}
				return jsonResponse(`{"answers":{"risk":{"type":"choice","choice":"none","confidence":1}}}`), nil
			})}
			client, err := New("secret", "https://example.test/start", "model", httpClient)
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.AuditRisk(context.Background(), RiskRequest{})
			if scheme == "http" && (err == nil || calls != 1) {
				t.Fatalf("plaintext redirect: calls=%d error=%v", calls, err)
			}
			if scheme == "https" && (err != nil || calls != 2) {
				t.Fatalf("HTTPS redirect: calls=%d error=%v", calls, err)
			}
			if httpClient.CheckRedirect != nil {
				t.Fatal("modified caller's HTTP client")
			}
		})
	}
}

func TestRedirectRequiresConfiguredOrigin(t *testing.T) {
	for _, tc := range []struct {
		location string
		allowed  bool
	}{
		{"/next", true},
		{"https://example.test/next", true},
		{"https://EXAMPLE.test:443/next", true},
		{"https://other.test/next", false},
		{"https://sub.example.test/next", false},
		{"https://example.test:8443/next", false},
	} {
		t.Run(tc.location, func(t *testing.T) {
			calls := 0
			policyCalls := 0
			httpClient := &http.Client{
				CheckRedirect: func(*http.Request, []*http.Request) error { policyCalls++; return nil },
				Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					calls++
					if calls == 1 {
						return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{tc.location}}, Body: io.NopCloser(strings.NewReader(""))}, nil
					}
					return jsonResponse(`{"answers":{"risk":{"type":"choice","choice":"none","confidence":1}}}`), nil
				}),
			}
			client, err := New("secret", "https://example.test/start", "model", httpClient)
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.AuditRisk(context.Background(), RiskRequest{})
			if tc.allowed {
				if err != nil || calls != 2 || policyCalls != 1 {
					t.Fatalf("same-origin redirect: calls=%d policy=%d err=%v", calls, policyCalls, err)
				}
			} else if err == nil || calls != 1 || policyCalls != 0 {
				t.Fatalf("cross-origin redirect was not blocked before sending: calls=%d policy=%d err=%v", calls, policyCalls, err)
			}
		})
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
	client, err := New("system-key", "https://system-one.example/v1/systemone", "jev-test", httpClient)
	if err != nil {
		t.Fatal(err)
	}
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
	client, err := New("system-key", "https://system-one.example/v1/systemone", "jev-test", httpClient)
	if err != nil {
		t.Fatal(err)
	}
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
				client, err := New("key", "https://example.test", "model", httpClient)
				if err != nil {
					t.Fatal(err)
				}
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
