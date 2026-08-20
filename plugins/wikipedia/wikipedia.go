// Package wikipedia is a complete, copyable example of a native CLAI tool.
// Copy this directory to start another tool, then change Definition, Arguments,
// Result, and Execute. Register the new constructor in plugins/registry.go.
package wikipedia

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/merefield/clai/pkg/tool"
)

const (
	maxQueryLength  = 300
	maxResponseSize = 2 << 20
	userAgent       = "clai/1.1 (https://github.com/merefield/clai)"
)

var languagePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// Plugin looks up the best matching Wikipedia article and returns its lead
// extract. Wikipedia does not require an API key; see the README tutorial for
// injecting environment-backed credentials into tools that do.
type Plugin struct {
	client   *http.Client
	endpoint func(string) string
}

type Arguments struct {
	Query    string `json:"query"`
	Language string `json:"language,omitempty"`
}

type Result struct {
	Found    bool   `json:"found"`
	Query    string `json:"query"`
	Language string `json:"language"`
	Title    string `json:"title,omitempty"`
	Summary  string `json:"summary,omitempty"`
	URL      string `json:"url,omitempty"`
}

func New(client *http.Client) *Plugin {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &Plugin{
		client: client,
		endpoint: func(language string) string {
			return "https://" + language + ".wikipedia.org/w/api.php"
		},
	}
}

func (p *Plugin) Definition() tool.Definition {
	return tool.Definition{
		Name:         "wikipedia_lookup",
		Description:  "Search Wikipedia and return the best matching article's title, introductory summary, and source URL. Use this for factual background that benefits from an encyclopedic source.",
		Capabilities: []tool.Capability{tool.CapabilityNetworkRead},
		Parameters: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Topic or article to find.",
				},
				"language": map[string]any{
					"type":        "string",
					"description": "Wikipedia language subdomain, such as en, de, or pt-br. Defaults to en.",
					"pattern":     `^[a-z0-9]+(?:-[a-z0-9]+)*$`,
				},
			},
			"required": []string{"query"},
		},
	}
}

func (p *Plugin) Execute(ctx context.Context, raw json.RawMessage) (any, error) {
	var arguments Arguments
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&arguments); err != nil {
		return nil, fmt.Errorf("decode arguments: %w", err)
	}
	arguments.Query = strings.TrimSpace(arguments.Query)
	if arguments.Query == "" {
		return nil, fmt.Errorf("query is required")
	}
	if len(arguments.Query) > maxQueryLength {
		return nil, fmt.Errorf("query exceeds %d bytes", maxQueryLength)
	}
	arguments.Language = strings.ToLower(strings.TrimSpace(arguments.Language))
	if arguments.Language == "" {
		arguments.Language = "en"
	}
	if len(arguments.Language) > 20 || !languagePattern.MatchString(arguments.Language) {
		return nil, fmt.Errorf("invalid Wikipedia language %q", arguments.Language)
	}

	match, err := p.search(ctx, arguments.Language, arguments.Query)
	if err != nil {
		return nil, err
	}
	result := Result{Found: match.PageID != 0, Query: arguments.Query, Language: arguments.Language}
	if !result.Found {
		return result, nil
	}
	page, err := p.extract(ctx, arguments.Language, match.PageID)
	if err != nil {
		return nil, err
	}
	result.Title = page.Title
	result.Summary = strings.TrimSpace(page.Extract)
	result.URL = articleURL(arguments.Language, page.Title)
	return result, nil
}

type page struct {
	PageID  int    `json:"pageid"`
	Title   string `json:"title"`
	Extract string `json:"extract"`
}

func (p *Plugin) search(ctx context.Context, language, query string) (page, error) {
	parameters := url.Values{
		"action":        {"query"},
		"format":        {"json"},
		"formatversion": {"2"},
		"list":          {"search"},
		"srnamespace":   {"0"},
		"srlimit":       {"1"},
		"srsearch":      {query},
		"utf8":          {"1"},
	}
	var response struct {
		Query struct {
			Search []page `json:"search"`
		} `json:"query"`
	}
	if err := p.get(ctx, language, parameters, &response); err != nil {
		return page{}, fmt.Errorf("search Wikipedia: %w", err)
	}
	if len(response.Query.Search) == 0 {
		return page{}, nil
	}
	return response.Query.Search[0], nil
}

func (p *Plugin) extract(ctx context.Context, language string, pageID int) (page, error) {
	parameters := url.Values{
		"action":        {"query"},
		"exchars":       {"1200"},
		"exintro":       {"1"},
		"explaintext":   {"1"},
		"format":        {"json"},
		"formatversion": {"2"},
		"pageids":       {fmt.Sprintf("%d", pageID)},
		"prop":          {"extracts"},
	}
	var response struct {
		Query struct {
			Pages []page `json:"pages"`
		} `json:"query"`
	}
	if err := p.get(ctx, language, parameters, &response); err != nil {
		return page{}, fmt.Errorf("read Wikipedia article: %w", err)
	}
	if len(response.Query.Pages) == 0 {
		return page{}, fmt.Errorf("Wikipedia returned no page for id %d", pageID)
	}
	return response.Query.Pages[0], nil
}

func (p *Plugin) get(ctx context.Context, language string, parameters url.Values, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint(language)+"?"+parameters.Encode(), nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", userAgent)
	response, err := p.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
		return fmt.Errorf("HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	limited := io.LimitReader(response.Body, maxResponseSize+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return err
	}
	if len(body) > maxResponseSize {
		return fmt.Errorf("response exceeds %d bytes", maxResponseSize)
	}
	var envelope struct {
		Error *struct {
			Code string `json:"code"`
			Info string `json:"info"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &envelope) == nil && envelope.Error != nil {
		return fmt.Errorf("MediaWiki API error %s: %s", envelope.Error.Code, envelope.Error.Info)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func articleURL(language, title string) string {
	title = strings.ReplaceAll(title, " ", "_")
	return "https://" + language + ".wikipedia.org/wiki/" + url.PathEscape(title)
}
