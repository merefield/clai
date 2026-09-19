package systemone

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	IntentExecute      = "execute"
	IntentQuestion     = "question"
	IntentClearHistory = "clear_history"
)

type Client interface {
	RouteIntent(context.Context, IntentRequest) (IntentDecision, error)
	AuditRisk(context.Context, RiskRequest) (RiskDecision, error)
}

type HTTPClient struct {
	Key    string
	API    string
	Model  string
	Client *http.Client
}

type IntentRequest struct {
	UserRequest string `json:"user_request"`
}

type IntentDecision struct {
	Intent     string
	Confidence float64
}

type RiskRequest struct {
	UserRequest string `json:"user_request"`
	Command     string `json:"command"`
	Info        string `json:"info"`
	LLMRisk     string `json:"llm_risk"`
}

type RiskDecision struct {
	Risk       string
	Confidence float64
}

type question map[string]any

type request struct {
	State     any                 `json:"state"`
	Model     string              `json:"model"`
	Questions map[string]question `json:"questions"`
}

type response struct {
	Answers map[string]answer `json:"answers"`
}

type answer struct {
	Type       string              `json:"type"`
	Choice     string              `json:"choice,omitempty"`
	Confidence *float64            `json:"confidence,omitempty"`
	Prob       map[string]*float64 `json:"probabilities,omitempty"`
	Noul       float64             `json:"noul,omitempty"`
}

func Configured(key, api, model string) bool {
	return strings.TrimSpace(key) != "" && validEndpoint(api) && strings.TrimSpace(model) != ""
}

func validEndpoint(api string) bool {
	endpoint, err := url.Parse(api)
	return err == nil && endpoint.Scheme == "https" && endpoint.Hostname() != "" && endpoint.User == nil && endpoint.Fragment == ""
}

func ValidConfidence(value float64) bool {
	return !math.IsNaN(value) && value >= 0 && value <= 1
}

func New(key, api, model string, client *http.Client) (*HTTPClient, error) {
	if !validEndpoint(api) {
		return nil, fmt.Errorf("system_one_api must be a valid HTTPS endpoint without user info or fragment")
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	secureClient := *client
	origin, _ := url.Parse(api)
	secureClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if !validEndpoint(req.URL.String()) {
			return fmt.Errorf("system one redirect requires HTTPS")
		}
		if !strings.EqualFold(req.URL.Hostname(), origin.Hostname()) || httpsPort(req.URL) != httpsPort(origin) {
			return fmt.Errorf("system one redirect must remain on the configured origin")
		}
		if client.CheckRedirect != nil {
			return client.CheckRedirect(req, via)
		}
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		return nil
	}
	return &HTTPClient{Key: key, API: api, Model: model, Client: &secureClient}, nil
}

func httpsPort(endpoint *url.URL) string {
	if port := endpoint.Port(); port != "" {
		return port
	}
	return "443"
}

func (c *HTTPClient) RouteIntent(ctx context.Context, input IntentRequest) (IntentDecision, error) {
	payload := request{
		State: input,
		Model: c.Model,
		Questions: map[string]question{
			"intent": {
				"type":         "choice",
				"instructions": "Which CLAI workflow should handle this user request?",
				"criteria": map[string]string{
					IntentExecute:      "The user wants CLAI to propose a shell command or perform a terminal task.",
					IntentQuestion:     "The user asks for an explanation or answer and no shell command should be proposed.",
					IntentClearHistory: "The user wants to clear, reset, forget, or flush CLAI conversation history.",
				},
			},
		},
	}
	answers, err := c.evaluate(ctx, payload)
	if err != nil {
		return IntentDecision{}, err
	}
	value, ok := answers["intent"]
	if !ok || value.Type != "choice" || value.Choice == "" {
		return IntentDecision{}, fmt.Errorf("system one intent response missing choice")
	}
	return IntentDecision{Intent: value.Choice, Confidence: *value.Confidence}, nil
}

func (c *HTTPClient) AuditRisk(ctx context.Context, input RiskRequest) (RiskDecision, error) {
	payload := request{
		State: input,
		Model: c.Model,
		Questions: map[string]question{
			"risk": {
				"type":         "choice",
				"instructions": "Classify the risk of running the proposed shell command for this user request.",
				"criteria": map[string]string{
					"none":              "Read-only inspection or harmless output; no meaningful filesystem, process, network, or configuration change.",
					"reversible_change": "A change that is normally undoable, such as creating files, installing dependencies, editing working-tree files, or switching branches.",
					"danger_zone":       "Deletion, overwrite, reset, force, permission, credential, destructive, privileged, or hard-to-reverse changes.",
				},
			},
		},
	}
	answers, err := c.evaluate(ctx, payload)
	if err != nil {
		return RiskDecision{}, err
	}
	value, ok := answers["risk"]
	if !ok || value.Type != "choice" || value.Choice == "" {
		return RiskDecision{}, fmt.Errorf("system one risk response missing choice")
	}
	return RiskDecision{Risk: value.Choice, Confidence: *value.Confidence}, nil
}

func (c *HTTPClient) evaluate(ctx context.Context, payload request) (map[string]answer, error) {
	if !validEndpoint(c.API) {
		return nil, fmt.Errorf("system_one_api must be a valid HTTPS endpoint")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode system one request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.API, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("system one request failed: %w", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("read system one response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("system one request failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	var decoded response
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return nil, fmt.Errorf("parse system one response: %w", err)
	}
	if len(decoded.Answers) == 0 {
		return nil, fmt.Errorf("system one response returned no answers")
	}
	for name, question := range payload.Questions {
		value, present := decoded.Answers[name]
		criteria, ok := question["criteria"].(map[string]string)
		if !present || !ok || value.Type != "choice" {
			return nil, fmt.Errorf("system one %s response missing valid choice", name)
		}
		if err := validateChoice(value, criteria); err != nil {
			return nil, fmt.Errorf("system one %s response: %w", name, err)
		}
	}
	return decoded.Answers, nil
}

func validateChoice(value answer, criteria map[string]string) error {
	if _, ok := criteria[value.Choice]; !ok {
		return fmt.Errorf("choice is not a submitted criterion")
	}
	if value.Confidence == nil || !ValidConfidence(*value.Confidence) {
		return fmt.Errorf("confidence must be present and within [0, 1]")
	}
	if len(value.Prob) != len(criteria) {
		return fmt.Errorf("probabilities must include exactly the submitted criteria")
	}
	total := 0.0
	for option := range criteria {
		probability := value.Prob[option]
		if probability == nil || !ValidConfidence(*probability) {
			return fmt.Errorf("probability for %q must be present and within [0, 1]", option)
		}
		total += *probability
	}
	// Allow floating-point rounding without accepting an unnormalized distribution.
	if math.Abs(total-1) > 1e-6 {
		return fmt.Errorf("probabilities must sum to 1")
	}
	for _, probability := range value.Prob {
		if *probability > *value.Prob[value.Choice] {
			return fmt.Errorf("choice must have the highest probability")
		}
	}
	return nil
}
