// The wikipedia command is a complete, copyable external CLAI tool server.
// It is a separate executable: CLAI discovers it through wikipedia.json and
// communicates with it using MCP over stdin/stdout.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	maxQueryLength  = 300
	maxResponseSize = 2 << 20
	userAgent       = "clai-wikipedia/1.0 (https://github.com/merefield/clai)"
)

var languagePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type wikipediaTool struct {
	client   *http.Client
	endpoint func(string) string
}

type arguments struct {
	Query    string `json:"query" jsonschema:"topic or article to find"`
	Language string `json:"language,omitempty" jsonschema:"Wikipedia language subdomain, such as en, de, or pt-br; defaults to en"`
}

type result struct {
	Found    bool   `json:"found"`
	Query    string `json:"query"`
	Language string `json:"language"`
	Title    string `json:"title,omitempty"`
	Summary  string `json:"summary,omitempty"`
	URL      string `json:"url,omitempty"`
}

func newWikipediaTool(client *http.Client) *wikipediaTool {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &wikipediaTool{
		client: client,
		endpoint: func(language string) string {
			return "https://" + language + ".wikipedia.org/w/api.php"
		},
	}
}

func (w *wikipediaTool) lookup(ctx context.Context, _ *mcp.CallToolRequest, input arguments) (*mcp.CallToolResult, result, error) {
	input.Query = strings.TrimSpace(input.Query)
	if input.Query == "" {
		return nil, result{}, fmt.Errorf("query is required")
	}
	if len(input.Query) > maxQueryLength {
		return nil, result{}, fmt.Errorf("query exceeds %d bytes", maxQueryLength)
	}
	input.Language = strings.ToLower(strings.TrimSpace(input.Language))
	if input.Language == "" {
		input.Language = "en"
	}
	if len(input.Language) > 20 || !languagePattern.MatchString(input.Language) {
		return nil, result{}, fmt.Errorf("invalid Wikipedia language %q", input.Language)
	}

	match, err := w.search(ctx, input.Language, input.Query)
	if err != nil {
		return nil, result{}, err
	}
	output := result{Found: match.PageID != 0, Query: input.Query, Language: input.Language}
	if !output.Found {
		return nil, output, nil
	}
	page, err := w.extract(ctx, input.Language, match.PageID)
	if err != nil {
		return nil, result{}, err
	}
	output.Title = page.Title
	output.Summary = strings.TrimSpace(page.Extract)
	output.URL = articleURL(input.Language, page.Title)
	return nil, output, nil
}

type page struct {
	PageID  int    `json:"pageid"`
	Title   string `json:"title"`
	Extract string `json:"extract"`
}

func (w *wikipediaTool) search(ctx context.Context, language, query string) (page, error) {
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
	if err := w.get(ctx, language, parameters, &response); err != nil {
		return page{}, fmt.Errorf("search Wikipedia: %w", err)
	}
	if len(response.Query.Search) == 0 {
		return page{}, nil
	}
	return response.Query.Search[0], nil
}

func (w *wikipediaTool) extract(ctx context.Context, language string, pageID int) (page, error) {
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
	if err := w.get(ctx, language, parameters, &response); err != nil {
		return page{}, fmt.Errorf("read Wikipedia article: %w", err)
	}
	if len(response.Query.Pages) == 0 {
		return page{}, fmt.Errorf("Wikipedia returned no page for id %d", pageID)
	}
	return response.Query.Pages[0], nil
}

func (w *wikipediaTool) get(ctx context.Context, language string, parameters url.Values, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, w.endpoint(language)+"?"+parameters.Encode(), nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", userAgent)
	response, err := w.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
		return fmt.Errorf("HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseSize+1))
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

func main() {
	wikipedia := newWikipediaTool(nil)
	server := mcp.NewServer(&mcp.Implementation{Name: "clai-wikipedia", Version: "1.0.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "lookup",
		Title:       "Wikipedia",
		Description: "Search Wikipedia and return the best matching article's title, introductory summary, and source URL. Use this for factual background that benefits from an encyclopedic source.",
	}, wikipedia.lookup)
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil && !normalShutdown(err) {
		log.Printf("Wikipedia MCP server failed: %v", err)
	}
}

func normalShutdown(err error) bool {
	return errors.Is(err, io.EOF) || strings.Contains(err.Error(), "server is closing: EOF")
}
