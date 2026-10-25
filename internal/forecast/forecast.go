package forecast

import (
	"context"
	"slices"
	"time"
)

type Release struct {
	Version     string    `json:"version"`
	Name        string    `json:"name"`
	Notes       string    `json:"notes"`
	URL         string    `json:"url"`
	PublishedAt time.Time `json:"published_at"`
}

type Result struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

type Releases interface {
	Between(ctx context.Context, module, from, to string) ([]Release, error)
}

type Search interface {
	Search(ctx context.Context, query string, limit int) ([]Result, error)
}

type Change struct {
	Symbol      string `json:"symbol"`
	Description string `json:"description"`
}

type Forecast struct {
	Module    string    `json:"module"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Breaking  []Change  `json:"breaking"`
	Files     []string  `json:"files"`
	Summary   string    `json:"summary"`
	Sources   []string  `json:"sources"`
	FetchedAt time.Time `json:"fetched_at"`
}

func (f Forecast) Expects(symbol string) bool {
	return slices.ContainsFunc(f.Breaking, func(c Change) bool { return c.Symbol == symbol })
}
