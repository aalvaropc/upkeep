package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	releasesPerPage = 100
	maxReleasePages = 3
)

var moduleVersionSuffix = regexp.MustCompile(`^v[0-9]+$`)

func ModuleRepo(module string) (owner, repo string, ok bool) {
	parts := strings.Split(module, "/")
	if len(parts) < 3 || parts[0] != "github.com" || parts[1] == "" || parts[2] == "" {
		return "", "", false
	}
	return parts[1], parts[2], true
}

func MajorSuffix(module string) string {
	last := module[strings.LastIndex(module, "/")+1:]
	if moduleVersionSuffix.MatchString(last) {
		return last
	}
	return ""
}

func (c *Client) base() string {
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return DefaultBaseURL
}

func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base()+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.Token.IsSet() {
		req.Header.Set("Authorization", "Bearer "+c.Token.Reveal())
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, redactToken(err, c)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, redactToken(err, c)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var shaped struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &shaped)
		return nil, &Error{Status: resp.StatusCode, Message: shaped.Message, RateLimited: resp.Header.Get("X-RateLimit-Remaining") == "0"}
	}
	return raw, nil
}

type Error struct {
	Status      int
	Message     string
	RateLimited bool
}

func (e *Error) Error() string {
	switch {
	case e.RateLimited:
		return fmt.Sprintf("github rate limit reached (HTTP %d)", e.Status)
	case e.Message != "":
		return fmt.Sprintf("github returned HTTP %d: %s", e.Status, e.Message)
	default:
		return fmt.Sprintf("github returned HTTP %d", e.Status)
	}
}

type releaseJSON struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
	Draft       bool      `json:"draft"`
}

func (c *Client) ListReleases(ctx context.Context, owner, repo string) ([]Release, error) {
	var out []Release
	for page := 1; page <= maxReleasePages; page++ {
		raw, err := c.get(ctx, fmt.Sprintf("/repos/%s/%s/releases?per_page=%d&page=%d", owner, repo, releasesPerPage, page))
		if err != nil {
			return nil, err
		}
		var batch []releaseJSON
		if err := json.Unmarshal(raw, &batch); err != nil {
			return nil, fmt.Errorf("github sent a release list that is not valid JSON: %w", err)
		}
		for _, r := range batch {
			if r.Draft {
				continue
			}
			out = append(out, Release{TagName: r.TagName, Name: r.Name, Body: r.Body, HTMLURL: r.HTMLURL, PublishedAt: r.PublishedAt})
		}
		if len(batch) < releasesPerPage {
			break
		}
	}
	return out, nil
}

func redactToken(err error, c *Client) error {
	if c.Token.IsSet() && strings.Contains(err.Error(), c.Token.Reveal()) {
		return fmt.Errorf("%s", strings.ReplaceAll(err.Error(), c.Token.Reveal(), "[redacted]"))
	}
	return err
}
