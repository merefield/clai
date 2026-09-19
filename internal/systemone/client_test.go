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
				return jsonResponse(`{"answers":{"risk":{"type":"choice","choice":"none","confidence":1,"probabilities":{"none":1,"reversible_change":0,"danger_zone":0}}}}`), nil
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
					return jsonResponse(`{"answers":{"risk":{"type":"choice","choice":"none","confidence":1,"probabilities":{"none":1,"reversible_change":0,"danger_zone":0}}}}`), nil
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
				choice, probabilities := "none", `{"none":1,"reversible_change":0,"danger_zone":0}`
				if kind == "intent" {
					choice, probabilities = "execute", `{"execute":1,"question":0,"clear_history":0}`
				}
				httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					return jsonResponse(fmt.Sprintf(`{"answers":{"%s":{"type":"choice","choice":%q,"confidence":%s,"probabilities":%s}}}`, kind, choice, confidence, probabilities)), nil
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

func TestChoiceResponseValidation(t *testing.T) {
	for _, kind := range []string{"risk", "intent"} {
		for _, tc := range []struct {
			name          string
			choice        string
			confidence    string
			probabilities string
			valid         bool
		}{
			{"missing distribution", "selected", "1", "", false},
			{"null distribution", "selected", "1", "null", false},
			{"empty distribution", "selected", "1", `{}`, false},
			{"missing option", "selected", "1", `{"selected":1,"other":0}`, false},
			{"unknown option", "selected", "1", `{"selected":1,"other":0,"unknown":0}`, false},
			{"extra option", "selected", "1", `{"selected":1,"other":0,"last":0,"unknown":0}`, false},
			{"null probability", "selected", "1", `{"selected":1,"other":null,"last":0}`, false},
			{"negative probability", "selected", "1", `{"selected":1,"other":-0.1,"last":0.1}`, false},
			{"large probability", "selected", "1", `{"selected":2,"other":0,"last":0}`, false},
			{"invalid sum", "selected", "1", `{"selected":0.5,"other":0,"last":0}`, false},
			{"unknown choice", "unknown", "1", `{"selected":1,"other":0,"last":0}`, false},
			{"nonwinning choice", "other", "1", `{"selected":1,"other":0,"last":0}`, false},
			{"null confidence", "selected", "null", `{"selected":1,"other":0,"last":0}`, false},
			{"missing confidence", "selected", "", `{"selected":1,"other":0,"last":0}`, false},
			{"valid", "selected", "1", `{"selected":1,"other":0,"last":0}`, true},
			{"tie", "selected", "0", `{"selected":0.5,"other":0.5,"last":0}`, true},
			{"rounding", "selected", "0", `{"selected":0.3333333,"other":0.3333333,"last":0.3333333}`, true},
		} {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				replacer := strings.NewReplacer("selected", "none", "other", "reversible_change", "last", "danger_zone")
				if kind == "intent" {
					replacer = strings.NewReplacer("selected", "execute", "other", "question", "last", "clear_history")
				}
				body := fmt.Sprintf(`{"answers":{"%s":{"type":"choice","choice":%q`, kind, replacer.Replace(tc.choice))
				if tc.confidence != "" {
					body += `,"confidence":` + tc.confidence
				}
				if tc.probabilities != "" {
					body += `,"probabilities":` + replacer.Replace(tc.probabilities)
				}
				body += `}}}`
				client, err := New("key", "https://example.test", "model", &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
					return jsonResponse(body), nil
				})})
				if err != nil {
					t.Fatal(err)
				}
				if kind == "risk" {
					_, err = client.AuditRisk(context.Background(), RiskRequest{})
				} else {
					_, err = client.RouteIntent(context.Background(), IntentRequest{})
				}
				if (err == nil) != tc.valid {
					t.Fatalf("valid=%v error=%v", tc.valid, err)
				}
			})
		}
	}
}

func TestHTTPErrorEscapesTerminalControls(t *testing.T) {
	for _, body := range []string{
		"service unavailable",
		"failure\x1b[2J\x1b[Hforged success",
		"failure\x1b]52;c;c2VjcmV0\a",
		"failure\rforged\nmessage\b\t\x00",
		"failure\u009b2J\u009d52;c;data\u009c",
	} {
		t.Run(fmt.Sprintf("%q", body), func(t *testing.T) {
			client, err := New("key", "https://example.test", "model", &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				response := jsonResponse(body)
				response.StatusCode = http.StatusBadGateway
				return response, nil
			})})
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.AuditRisk(context.Background(), RiskRequest{})
			want := fmt.Sprintf("system one request failed (HTTP 502): %q", strings.TrimSpace(body))
			if err == nil || err.Error() != want {
				t.Fatalf("error = %v; want %s", err, want)
			}
			for _, r := range err.Error() {
				if r < 32 || (r >= 127 && r <= 159) {
					t.Fatalf("raw terminal control %U in error", r)
				}
			}
		})
	}
}
