package forecast_test

import (
	"os"
	"strings"
	"testing"

	"github.com/aalvaropc/upkeep/internal/config"
	"github.com/aalvaropc/upkeep/internal/forecast"
	"github.com/aalvaropc/upkeep/internal/github"
	"github.com/aalvaropc/upkeep/internal/model"
	"github.com/aalvaropc/upkeep/internal/tavily"
)

func liveForecaster(t *testing.T) (forecast.Forecaster, config.Config) {
	t.Helper()
	if os.Getenv("UPKEEP_LIVE_TESTS") != "1" {
		t.Skip("set UPKEEP_LIVE_TESTS=1, NEBIUS_API_KEY and TAVILY_API_KEY to call the live services")
	}
	cfg, err := config.Load(os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.NebiusAPIKey.IsSet() || !cfg.TavilyAPIKey.IsSet() {
		t.Skip("NEBIUS_API_KEY and TAVILY_API_KEY are needed")
	}
	tf := model.NewTokenFactory(cfg.NebiusBaseURL, cfg.NebiusAPIKey)
	book, err := tf.FetchPrices(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return forecast.Forecaster{
		Releases:  forecast.GitHubReleases{Client: &github.Client{Token: cfg.GitHubToken}},
		Search:    tavily.New(cfg.TavilyAPIKey, forecast.Domains("github.com/golang-jwt/jwt/v5")...),
		Model:     tf,
		ModelName: cfg.Models.Nano,
		Meter:     model.Meter{Prices: book, Baseline: cfg.Models.Ultra},
	}, cfg
}

func TestLiveForecastOfAMajorUpgrade(t *testing.T) {
	f, _ := liveForecaster(t)
	got, cost, err := f.Forecast(t.Context(), forecast.Input{Module: "github.com/golang-jwt/jwt/v5", From: "v4.5.2", To: "v5.3.1"})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("nano: %v, %+v tokens, %d micro-USD", cost.Latency.Round(100*1e6), cost.Usage, cost.Charge.Micros)
	t.Logf("\n%s", got.Notes())
	all := strings.ToLower(got.Notes())
	if !strings.Contains(all, "registeredclaims") && !strings.Contains(all, "standardclaims") {
		t.Errorf("the forecast should mention the claims types")
	}
	if len(got.Breaking) == 0 || len(got.Sources) == 0 {
		t.Errorf("breaking %d sources %d", len(got.Breaking), len(got.Sources))
	}
}
