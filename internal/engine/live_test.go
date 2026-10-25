package engine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aalvaropc/upkeep/internal/config"
	"github.com/aalvaropc/upkeep/internal/events"
	"github.com/aalvaropc/upkeep/internal/forecast"
	"github.com/aalvaropc/upkeep/internal/github"
	"github.com/aalvaropc/upkeep/internal/gitx"
	"github.com/aalvaropc/upkeep/internal/model"
	"github.com/aalvaropc/upkeep/internal/policy"
	"github.com/aalvaropc/upkeep/internal/pull"
	"github.com/aalvaropc/upkeep/internal/sandbox"
	"github.com/aalvaropc/upkeep/internal/strategy"
	"github.com/aalvaropc/upkeep/internal/tavily"
)

const sampleRepository = "https://github.com/aalvaropc/upkeep-sample.git"

func liveRun(t *testing.T, withForecast bool) Result {
	t.Helper()
	if os.Getenv("UPKEEP_LIVE_TESTS") != "1" {
		t.Skip("set UPKEEP_LIVE_TESTS=1 and NEBIUS_API_KEY to call Token Factory")
	}
	cfg, err := config.Load(os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.NebiusAPIKey.IsSet() {
		t.Skip("NEBIUS_API_KEY is not set")
	}

	g := gitx.Git{}
	dir := filepath.Join(t.TempDir(), "sample")
	if err := g.Clone(t.Context(), sampleRepository, dir); err != nil {
		t.Skipf("the sample repository is not reachable: %v", err)
	}
	if err := g.Checkout(t.Context(), dir, "dependabot/go_modules/github.com/golang-jwt/jwt/v5-5.3.1", true); err != nil {
		t.Fatal(err)
	}
	sh := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	sh("sed", "-i", "s#github.com/golang-jwt/jwt/v4#github.com/golang-jwt/jwt/v5#", "auth/token.go")
	sh("go", "get", "github.com/golang-jwt/jwt/v5@v5.3.1")
	sh("go", "mod", "tidy")
	if _, err := g.Commit(t.Context(), dir, "build(deps): bump github.com/golang-jwt/jwt/v4 to v5.3.1"); err != nil {
		t.Fatal(err)
	}

	tf := model.NewTokenFactory(cfg.NebiusBaseURL, cfg.NebiusAPIKey)
	book, err := tf.FetchPrices(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	router := model.NewRouter()
	router.Set(model.Ultra, cfg.Models.Ultra, tf)
	router.Set(model.Super, cfg.Models.Super, tf)
	router.Set(model.Nano, cfg.Models.Nano, tf)
	env := strategy.Env{
		Git:      g,
		Runner:   sandbox.GoRunner{Executor: sandbox.Exec{}},
		Executor: sandbox.Exec{},
		Models:   router,
		Meter:    model.Meter{Prices: book, Baseline: cfg.Models.Ultra},
		Policy:   policy.Default(),
		Emit: func(kind events.Kind, _ int, data any) {
			if kind == events.PRStep {
				if m, ok := data.(map[string]any); ok {
					t.Logf("%-8s %-8s %v", m["step"], m["status"], m["detail"])
				}
			}
		},
	}
	if withForecast && cfg.TavilyAPIKey.IsSet() {
		env.Forecaster = forecast.Forecaster{
			Releases:  forecast.GitHubReleases{Client: &github.Client{Token: cfg.GitHubToken}},
			Search:    tavily.New(cfg.TavilyAPIKey, forecast.Domains("github.com/golang-jwt/jwt/v5")...),
			Model:     tf,
			ModelName: cfg.Models.Nano,
			Meter:     env.Meter,
		}
	}
	pr := pull.Request{
		Number: 5,
		Title:  "Bump github.com/golang-jwt/jwt/v4 to v5.3.1",
		Author: pull.Author{Login: "dependabot[bot]", Kind: pull.Bot},
		Branch: "dependabot/go_modules/github.com/golang-jwt/jwt/v5-5.3.1",
		Kind:   pull.Dependency,
		Bump:   &pull.Bump{Module: "github.com/golang-jwt/jwt/v5", From: "v4.5.2", To: "v5.3.1", Level: pull.Major},
	}
	began := time.Now()
	res := Processor{Env: env}.Process(t.Context(), Input{PR: pr, Dir: dir, Base: "origin/main"})
	totals := res.Receipt.Totals()
	t.Logf("outcome %s after %s: blocker %s, %d attempts, %d model calls, %d prompt and %d completion tokens, %d micro-USD (%d at the baseline)",
		res.Outcome, time.Since(began).Round(time.Second), res.Blocker, res.Attempts, totals.Calls, totals.Usage.PromptTokens, totals.Usage.CompletionTokens, totals.Charge.Micros, totals.Charge.BaselineMicros)
	if res.Forecast != nil {
		t.Logf("forecast: %d breaking changes (%d dropped as not grounded), confirmed by the build: %v, not seen: %v", len(res.Forecast.Breaking), res.Forecast.Dropped, res.Confirmed, res.Missed)
	}
	return res
}

func TestLiveMajorDependencyBump(t *testing.T) {
	res := liveRun(t, true)
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	t.Logf("changes: %+v", res.Changes)
	t.Logf("patch:\n%s", res.Patch)
	if res.Outcome != pull.Mergeable {
		t.Fatalf("outcome %s: %s\n%s", res.Outcome, res.Reason, res.Diagnosis)
	}
	if res.WantsMerge {
		t.Fatal("a major bump must never be merged on its own")
	}
	if !strings.Contains(res.Patch, "RegisteredClaims") {
		t.Errorf("the fix should use the v5 claims type")
	}
}
