package github

import (
	"net/http"
	"time"

	"github.com/aalvaropc/upkeep/internal/config"
)

const DefaultBaseURL = "https://api.github.com"

type User struct {
	Login string `json:"login"`
	Type  string `json:"type"`
}

type Label struct {
	Name string `json:"name"`
}

type Repo struct {
	FullName string `json:"full_name"`
}

type Ref struct {
	Ref  string `json:"ref"`
	SHA  string `json:"sha"`
	Repo *Repo  `json:"repo"`
}

type PullRequest struct {
	NodeID  string  `json:"node_id"`
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
