package tavily

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aalvaropc/upkeep/internal/config"
	"github.com/aalvaropc/upkeep/internal/forecast"
)

const (
	DefaultBaseURL  = "https://api.tavily.com"
	maxResponseSize = 16 << 20
	maxErrorSize    = 2048
)

type Client struct {
	BaseURL string
	Key     config.Secret
	HTTP    *http.Client
	Domains []string
}

func New(key config.Secret, domains ...string) *Client {
	return &Client{
		BaseURL: DefaultBaseURL,
		Key:     key,
		HTTP:    &http.Client{Timeout: 45 * time.Second},
		Domains: domains,
	}
}

type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("tavily returned HTTP %d", e.Status)
	}
	return fmt.Sprintf("tavily returned HTTP %d: %s", e.Status, e.Message)
}

type request struct {
	Query             string   `json:"query"`
	MaxResults        int      `json:"max_results"`
	SearchDepth       string   `json:"search_depth"`
	IncludeRawContent string   `json:"include_raw_content"`
	IncludeDomains    []string `json:"include_domains,omitempty"`
}

type response struct {
	Results []struct {
		Title      string  `json:"title"`
		URL        string  `json:"url"`
		Content    string  `json:"content"`
		RawContent *string `json:"raw_content"`
		Score      float64 `json:"score"`
	} `json:"results"`
}

func (c *Client) Search(ctx context.Context, query string, limit int) ([]forecast.Result, error) {
	if !c.Key.IsSet() {
		return nil, errors.New("tavily: no API key")
	}
	if strings.TrimSpace(query) == "" {
		return nil, errors.New("tavily: empty query")
	}
	body, err := json.Marshal(request{
		Query:             query,
		MaxResults:        max(limit, 1),
		SearchDepth:       "basic",
		IncludeRawContent: "markdown",
		IncludeDomains:    c.Domains,
	})
	if err != nil {
		return nil, err
	}
	base := c.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/search", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Key.Reveal())
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, redact(err, c.Key)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorSize))
		return nil, &APIError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, redact(err, c.Key)
	}
	var doc response
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("tavily sent a response that is not valid JSON: %w", err)
	}
	out := make([]forecast.Result, 0, len(doc.Results))
	for _, r := range doc.Results {
		res := forecast.Result{Title: r.Title, URL: r.URL, Content: r.Content, Score: r.Score}
		if r.RawContent != nil {
			res.RawContent = *r.RawContent
		}
		out = append(out, res)
	}
	return out, nil
}

func redact(err error, key config.Secret) error {
	if key.IsSet() && strings.Contains(err.Error(), key.Reveal()) {
		return errors.New(strings.ReplaceAll(err.Error(), key.Reveal(), "[redacted]"))
	}
	return err
}

func errorMessage(raw []byte) string {
	var shaped struct {
		Detail struct {
			Error string `json:"error"`
		} `json:"detail"`
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &shaped) == nil {
		switch {
		case shaped.Detail.Error != "":
			return shaped.Detail.Error
		case shaped.Error != "":
			return shaped.Error
		case shaped.Message != "":
			return shaped.Message
		}
	}
	return strings.TrimSpace(string(raw))
}
