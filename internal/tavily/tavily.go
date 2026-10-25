package tavily

import (
	"context"
	"net/http"

	"github.com/aalvaropc/upkeep/internal/config"
	"github.com/aalvaropc/upkeep/internal/forecast"
	"github.com/aalvaropc/upkeep/internal/stub"
)

const DefaultBaseURL = "https://api.tavily.com"

type Client struct {
	BaseURL string
	Key     config.Secret
	HTTP    *http.Client
}

func (c *Client) Search(_ context.Context, _ string, _ int) ([]forecast.Result, error) {
	return nil, stub.Err("tavily search")
}
