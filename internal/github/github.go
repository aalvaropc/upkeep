package github

import (
	"context"
	"net/http"
	"time"

	"github.com/aalvaropc/upkeep/internal/config"
	"github.com/aalvaropc/upkeep/internal/stub"
)

const DefaultBaseURL = "https://api.github.com"

type User struct {
	Login string `json:"login"`
	Type  string `json:"type"`
}

type Label struct {
	Name string `json:"name"`
}

type Ref struct {
	Ref string `json:"ref"`
	SHA string `json:"sha"`
}

type PullRequest struct {
	Number  int     `json:"number"`
	Title   string  `json:"title"`
	User    User    `json:"user"`
	Labels  []Label `json:"labels"`
	Head    Ref     `json:"head"`
	Base    Ref     `json:"base"`
	Draft   bool    `json:"draft"`
	HTMLURL string  `json:"html_url"`
}

type Release struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
}

type Client struct {
	BaseURL string
	Token   config.Secret
	HTTP    *http.Client
}

func (c *Client) ListOpenPulls(_ context.Context, _, _ string) ([]PullRequest, error) {
	return nil, stub.Err("list open pull requests")
}

func (c *Client) Comment(_ context.Context, _, _ string, _ int, _ string) error {
	return stub.Err("comment on a pull request")
}

func (c *Client) Merge(_ context.Context, _, _ string, _ int, _ string) error {
	return stub.Err("merge a pull request")
}

func (c *Client) ListReleases(_ context.Context, _, _ string) ([]Release, error) {
	return nil, stub.Err("list releases")
}
