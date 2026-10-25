package forecast

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fmt"

	"github.com/aalvaropc/upkeep/internal/fixture"
	"github.com/aalvaropc/upkeep/internal/github"
	"github.com/aalvaropc/upkeep/internal/model"
)

type fakeReleases struct {
	calls   atomic.Int32
	list    []Release
	err     error
	modules []string
}

func (f *fakeReleases) Between(_ context.Context, module, from, to string) ([]Release, error) {
	f.calls.Add(1)
	f.modules = append(f.modules, module+" "+from+" "+to)
	return f.list, f.err
}

type fakeSearch struct {
	calls   atomic.Int32
	results []Result
	err     error
	queries []string
}

func (f *fakeSearch) Search(_ context.Context, query string, limit int) ([]Result, error) {
	f.calls.Add(1)
	f.queries = append(f.queries, fmt.Sprintf("%s|%d", query, limit))
	return f.results, f.err
}

type fakeModel struct {
	calls    atomic.Int32
	answer   string
	err      error
	requests []model.Request
}

func (f *fakeModel) Complete(_ context.Context, req model.Request) (model.Response, error) {
	f.calls.Add(1)
	f.requests = append(f.requests, req)
	if f.err != nil {
		return model.Response{}, f.err
	}
	return model.Response{
		Message: model.Message{Role: "assistant", Content: f.answer},
		Usage:   model.Usage{PromptTokens: 5000, CompletionTokens: 300},
		Latency: 1200 * time.Millisecond,
	}, nil
}

const jwtAnswer = `Here is the forecast:
{"summary":"v5 replaces the claims types. Expiry handling moved to validators.",
 "breaking":[{"symbol":"jwt.StandardClaims","change":"replaced by jwt.RegisteredClaims"},{"symbol":"VerifyExpiresAt","change":"removed; use GetExpirationTime"}]}`

func newForecaster(t *testing.T) (*Forecaster, *fakeReleases, *fakeSearch, *fakeModel) {
	t.Helper()
	rel := &fakeReleases{list: []Release{{Version: "v5.0.0", Notes: "Breaking: StandardClaims is gone and VerifyExpiresAt was removed.", URL: "https://github.com/golang-jwt/jwt/releases/tag/v5.0.0"}}}
	srch := &fakeSearch{results: []Result{{Title: "Migration guide", URL: "https://github.com/golang-jwt/jwt/blob/main/MIGRATION_GUIDE.md", Content: "short", RawContent: "# Migration\nStandardClaims is replaced and VerifyExpiresAt was removed.\nlong guide text", Score: 0.9}}}
	m := &fakeModel{answer: jwtAnswer}
	book := model.PriceBook{"nano": {InputPerMillion: 0.06, OutputPerMillion: 0.24}, "ultra": {InputPerMillion: 1, OutputPerMillion: 3}}
	return &Forecaster{
		Releases:  rel,
		Search:    srch,
		Model:     m,
		ModelName: "nano",
		Meter:     model.Meter{Prices: book, Baseline: "ultra"},
		Cache:     fixture.Dir{Path: t.TempDir()},
		Now:       func() time.Time { return time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC) },
	}, rel, srch, m
}

func jwtInput(dir string) Input {
	return Input{Module: "github.com/golang-jwt/jwt/v5", From: "v4.5.2", To: "v5.3.1", Dir: dir}
}

func TestForecast(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "token.go"), []byte("package auth\n\nimport \"github.com/golang-jwt/jwt/v5\"\n\nvar _ = jwt.New\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, rel, srch, m := newForecaster(t)
	got, cost, err := f.Forecast(t.Context(), jwtInput(dir))
	if err != nil {
		t.Fatal(err)
	}
	if got.Module != "github.com/golang-jwt/jwt/v5" || got.From != "v4.5.2" || got.To != "v5.3.1" || !strings.HasPrefix(got.Summary, "v5 replaces") {
		t.Fatalf("forecast = %+v", got)
	}
	if len(got.Breaking) != 2 || got.Breaking[0].Symbol != "jwt.StandardClaims" || got.Breaking[0].Description != "replaced by jwt.RegisteredClaims" {
		t.Fatalf("breaking = %+v", got.Breaking)
	}
	if got.Excerpt != "# Migration\nStandardClaims is replaced and VerifyExpiresAt was removed.\nlong guide text" || got.ExcerptSource != "https://github.com/golang-jwt/jwt/blob/main/MIGRATION_GUIDE.md" || got.Dropped != 0 {
		t.Fatalf("excerpt %q from %q, dropped %d", got.Excerpt, got.ExcerptSource, got.Dropped)
	}
	if !slices.Equal(got.Files, []string{"token.go"}) || !got.FetchedAt.Equal(time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("files %v fetched %v", got.Files, got.FetchedAt)
	}
	if !slices.Equal(got.Sources, []string{"https://github.com/golang-jwt/jwt/blob/main/MIGRATION_GUIDE.md", "https://github.com/golang-jwt/jwt/releases/tag/v5.0.0"}) {
		t.Fatalf("sources = %v", got.Sources)
	}
	if cost.Cached || cost.Model != "nano" || cost.Usage.PromptTokens != 5000 || !cost.Charge.Known || cost.Charge.Micros != 372 || cost.Latency != 1200*time.Millisecond {
		t.Fatalf("cost = %+v", cost)
	}
	if rel.modules[0] != "github.com/golang-jwt/jwt/v5 v4.5.2 v5.3.1" || srch.queries[0] != "github.com/golang-jwt/jwt/v5 migration guide|4" {
		t.Fatalf("releases %v, queries %v", rel.modules, srch.queries)
	}
	req := m.requests[0]
	prompt := req.Messages[1].Content
	for _, want := range []string{"Upgrading from v4.5.2 to v5.3.1", "token.go", "uses these identifiers from it, written as the project writes them: jwt.New.", "StandardClaims is gone", "long guide text", "MIGRATION_GUIDE.md"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the prompt does not contain %q:\n%s", want, prompt)
		}
	}
	if req.Reasoning == nil || *req.Reasoning || req.Model != "nano" || req.Messages[0].Content != systemInstructions {
		t.Fatalf("request = %+v", req)
	}
}

func TestForecastIsCached(t *testing.T) {
	f, rel, srch, m := newForecaster(t)
	dir := t.TempDir()
	first, _, err := f.Forecast(t.Context(), jwtInput(dir))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "later.go"), []byte("package x\n\nimport _ \"github.com/golang-jwt/jwt/v5\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, cost, err := f.Forecast(t.Context(), jwtInput(dir))
	if err != nil {
		t.Fatal(err)
	}
	if !cost.Cached || cost.Usage.Total() != 0 || cost.Charge.Known {
		t.Fatalf("a cached forecast costs nothing: %+v", cost)
	}
	if rel.calls.Load() != 1 || srch.calls.Load() != 1 || m.calls.Load() != 1 {
		t.Fatalf("calls: releases %d, search %d, model %d; want one each", rel.calls.Load(), srch.calls.Load(), m.calls.Load())
	}
	if second.Summary != first.Summary || len(second.Breaking) != 2 || !second.FetchedAt.Equal(first.FetchedAt) {
		t.Fatalf("cached = %+v", second)
	}
	if !slices.Equal(second.Files, []string{"later.go"}) {
		t.Fatalf("the files are looked up again for the project at hand: %v", second.Files)
	}
	other := jwtInput(dir)
	other.To = "v5.3.0"
	if _, cost, err := f.Forecast(t.Context(), other); err != nil || cost.Cached {
		t.Fatalf("another version must not hit the cache: %+v %v", cost, err)
	}
}

func TestForecastWithoutAModelUsesTheRecordedOne(t *testing.T) {
	f, _, _, _ := newForecaster(t)
	if _, _, err := f.Forecast(t.Context(), jwtInput("")); err != nil {
		t.Fatal(err)
	}
	replay := Forecaster{Cache: f.Cache}
	got, cost, err := replay.Forecast(t.Context(), jwtInput(""))
	if err != nil || !cost.Cached || len(got.Breaking) != 2 {
		t.Fatalf("forecast %+v cost %+v error %v", got, cost, err)
	}
	if _, _, err := (Forecaster{Cache: fixture.Dir{Path: t.TempDir()}}).Forecast(t.Context(), jwtInput("")); err == nil {
		t.Fatal("no recording and no model returned no error")
	}
}

func TestAReplayedForecastShowsTheCostItHadWhenRecorded(t *testing.T) {
	f, _, _, _ := newForecaster(t)
	_, recordedCost, err := f.Forecast(t.Context(), jwtInput(""))
	if err != nil {
		t.Fatal(err)
	}
	replay := Forecaster{Cache: f.Cache, Replay: true, Pace: 0.001}
	began := time.Now()
	got, cost, err := replay.Forecast(t.Context(), jwtInput(""))
	if err != nil || len(got.Breaking) != 2 {
		t.Fatalf("forecast %+v error %v", got, err)
	}
	if cost.Cached || cost.Model != "nano" || cost.Role != model.Super || cost.Charge.Micros != recordedCost.Charge.Micros || cost.Usage.PromptTokens != 5000 {
		t.Fatalf("replayed cost = %+v, recorded %+v", cost, recordedCost)
	}
	if took := time.Since(began); took > 3*time.Second {
		t.Fatalf("pacing at 0.001 of 1.2 seconds took %s", took)
	}
	live := Forecaster{Cache: f.Cache}
	if _, cost, err := live.Forecast(t.Context(), jwtInput("")); err != nil || !cost.Cached || cost.Charge.Known {
		t.Fatalf("a live cache hit is free: %+v %v", cost, err)
	}
}

func TestAReplayedForecastStopsWhenCancelled(t *testing.T) {
	f, _, _, _ := newForecaster(t)
	if _, _, err := f.Forecast(t.Context(), jwtInput("")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := (Forecaster{Cache: f.Cache, Replay: true, Pace: 1}).Forecast(ctx, jwtInput("")); err == nil {
		t.Fatal("a cancelled replay returned no error")
	}
}

func TestAboutMatchesOnlyTheModulesOwnPages(t *testing.T) {
	const module = "github.com/jonboulle/clockwork"
	for url, want := range map[string]bool{
		"https://github.com/jonboulle/clockwork/releases":              true,
		"https://github.com/JonBoulle/Clockwork/blob/master/README.md": true,
		"https://pkg.go.dev/github.com/jonboulle/clockwork":            true,
		"https://pkg.go.dev/github.com/jonboulle/clockwork@v0.5.0":     true,
		"https://github.com/knative/eventing/releases":                 false,
		"https://github.com/jonboulle/clockwork-extras":                false,
		"https://github.com/jonboulle/other":                           false,
		"https://example.com/jonboulle/clockwork":                      false,
	} {
		if got := About(url, module); got != want {
			t.Errorf("About(%q) = %v, want %v", url, got, want)
		}
	}
	if !About("https://example.com/anything", "golang.org/x/text") {
		t.Error("a module that does not live on GitHub cannot be matched by address, so every page is kept")
	}
}

func TestPagesAboutOtherProjectsNeverReachTheModel(t *testing.T) {
	f, _, srch, m := newForecaster(t)
	srch.results = append([]Result{{Title: "Releases - knative/eventing", URL: "https://github.com/knative/eventing/releases", Content: "Update widget-sdk-v2 monorepo", RawContent: "# knative eventing\nUpdate widget-sdk-v2 monorepo"}}, srch.results...)
	if _, _, err := f.Forecast(t.Context(), jwtInput("")); err != nil {
		t.Fatal(err)
	}
	prompt := m.requests[0].Messages[1].Content
	if strings.Contains(prompt, "knative") || strings.Contains(prompt, "widget-sdk") {
		t.Fatalf("a page about another project reached the prompt:\n%s", prompt)
	}
	if !strings.Contains(prompt, "MIGRATION_GUIDE.md") {
		t.Fatalf("the module's own page is missing:\n%s", prompt)
	}
}

func TestForecastToleratesOneSourceFailing(t *testing.T) {
	f, rel, srch, _ := newForecaster(t)
	rel.err = errors.New("github is rate limited")
	if got, _, err := f.Forecast(t.Context(), jwtInput("")); err != nil || len(got.Breaking) != 2 || len(got.Sources) != 1 {
		t.Fatalf("with releases down: %+v %v", got, err)
	}
	f2, rel2, srch2, _ := newForecaster(t)
	srch2.err = errors.New("tavily is down")
	if got, _, err := f2.Forecast(t.Context(), jwtInput("")); err != nil || len(got.Breaking) != 2 {
		t.Fatalf("with search down: %+v %v", got, err)
	}
	_ = rel2
	_ = srch

	f3, rel3, srch3, m3 := newForecaster(t)
	rel3.err, srch3.err = errors.New("down"), errors.New("down")
	if _, _, err := f3.Forecast(t.Context(), jwtInput("")); err == nil || m3.calls.Load() != 0 {
		t.Fatalf("with no sources the model must not be asked: %v, %d calls", err, m3.calls.Load())
	}
	f4, rel4, srch4, _ := newForecaster(t)
	rel4.list, srch4.results = nil, []Result{{Title: "x", URL: "u", Content: "  "}}
	if _, _, err := f4.Forecast(t.Context(), jwtInput("")); err == nil {
		t.Fatal("empty sources returned no error")
	}
}

func TestForecastRejectsBadAnswers(t *testing.T) {
	for name, answer := range map[string]string{
		"prose":   "I could not find anything.",
		"invalid": `{"summary": "x", "breaking": [}`,
		"empty":   `{"summary":"","breaking":[],"migration":""}`,
	} {
		f, _, _, m := newForecaster(t)
		m.answer = answer
		_, cost, err := f.Forecast(t.Context(), jwtInput(""))
		if err == nil {
			t.Errorf("%s: no error", name)
		}
		if cost.Usage.Total() == 0 {
			t.Errorf("%s: the tokens that were spent must still be reported", name)
		}
		var cached Forecast
		key, _ := cacheKey(jwtInput(""))
		if f.Cache.Load(CacheKind, key, &cached) == nil {
			t.Errorf("%s: a bad answer was cached", name)
		}
	}
	f, _, _, m := newForecaster(t)
	m.err = errors.New("token factory is down")
	if _, _, err := f.Forecast(t.Context(), jwtInput("")); err == nil || !strings.Contains(err.Error(), "token factory is down") {
		t.Errorf("a model failure = %v", err)
	}
	if _, _, err := f.Forecast(t.Context(), Input{Module: "m"}); err == nil {
		t.Error("an incomplete input returned no error")
	}
}

func TestChangesThatAreNotInTheSourcesAreDropped(t *testing.T) {
	f, _, _, m := newForecaster(t)
	m.answer = `{"summary":"s","breaking":[
		{"symbol":"jwt.StandardClaims","change":"replaced by jwt.RegisteredClaims"},
		{"symbol":"Claims.Getsig","change":"invented"},
		{"symbol":"VerifyExpiresAt","change":"removed"},
		{"symbol":"x.Go","change":"too short to check"}]}`
	got, _, err := f.Forecast(t.Context(), jwtInput(""))
	if err != nil {
		t.Fatal(err)
	}
	var kept []string
	for _, c := range got.Breaking {
		kept = append(kept, c.Symbol)
	}
	if !slices.Equal(kept, []string{"jwt.StandardClaims", "VerifyExpiresAt"}) || got.Dropped != 2 {
		t.Fatalf("kept %v, dropped %d", kept, got.Dropped)
	}
	cached, cost, err := f.Forecast(t.Context(), jwtInput(""))
	if err != nil || !cost.Cached || cached.Dropped != 2 || len(cached.Breaking) != 2 {
		t.Fatalf("the filtered forecast is what gets cached: %+v %v", cached, err)
	}
}

func TestParseKeepsOnlyWhatIsUsable(t *testing.T) {
	if got, _ := parse(`{"summary":"s","breaking":[{"symbol":"  ","change":"x"},{"symbol":"A","change":"y"}]}`); len(got.Breaking) != 1 {
		t.Errorf("a change without a symbol must be dropped: %+v", got.Breaking)
	}
	if got, err := parse(`{"summary":"only a summary","breaking":[]}`); err != nil || got.Summary != "only a summary" {
		t.Errorf("a summary alone is a forecast: %+v %v", got, err)
	}
	if got, err := parse(`{"breaking":[{"symbol":"Add","change":"gone"}]}`); err != nil || len(got.Breaking) != 1 {
		t.Errorf("changes alone are a forecast: %+v %v", got, err)
	}
}

func TestConfirm(t *testing.T) {
	f := Forecast{Breaking: []Change{
		{Symbol: "jwt.StandardClaims"}, {Symbol: "VerifyExpiresAt"}, {Symbol: "jwt.Parser.ParseWithClaims()"}, {Symbol: "x.Go"},
	}}
	out := "auth/token.go:24:6: undefined: jwt.StandardClaims\nauth/token.go:80:13: claims.VerifyExpiresAt undefined\n"
	confirmed, missed := f.Confirm(out)
	if !slices.Equal(confirmed, []string{"jwt.StandardClaims", "VerifyExpiresAt"}) || !slices.Equal(missed, []string{"jwt.Parser.ParseWithClaims()"}) {
		t.Fatalf("confirmed %v missed %v", confirmed, missed)
	}
	if c, m := f.Confirm("StandardClaimsExtra only"); len(c) != 0 || len(m) != 3 {
		t.Fatalf("a longer identifier must not confirm a shorter one: %v %v", c, m)
	}
	if !f.Expects("VerifyExpiresAt") || f.Expects("Nothing") {
		t.Fatal("Expects is wrong")
	}
}

func TestNotes(t *testing.T) {
	f := Forecast{
		Module: "github.com/golang-jwt/jwt/v5", From: "v4.5.2", To: "v5.3.1",
		Summary:       "not sent to the fixer: it is a prediction",
		Breaking:      []Change{{Symbol: "jwt.StandardClaims", Description: "replaced by jwt.RegisteredClaims"}},
		Excerpt:       "# Migration Guide (v5.0.0)\nVersion v5 reworks the claims.",
		ExcerptSource: "https://github.com/golang-jwt/jwt/blob/main/MIGRATION_GUIDE.md",
	}
	want := "Upgrade of github.com/golang-jwt/jwt/v5 from v4.5.2 to v5.3.1.\n\nBreaking changes found in the release notes and the migration guide:\n- jwt.StandardClaims: replaced by jwt.RegisteredClaims\n\nExcerpt from https://github.com/golang-jwt/jwt/blob/main/MIGRATION_GUIDE.md:\n\n# Migration Guide (v5.0.0)\nVersion v5 reworks the claims."
	if got := f.Notes(); got != want {
		t.Fatalf("Notes =\n%s\nwant\n%s", got, want)
	}
	if got := (Forecast{Module: "m", From: "v1.0.0", To: "v1.0.1"}).Notes(); got != "Upgrade of m from v1.0.0 to v1.0.1." {
		t.Fatalf("an empty forecast: %q", got)
	}
	if got := (Forecast{Module: "m", From: "v1", To: "v2", Excerpt: "text"}).Notes(); !strings.Contains(got, "Excerpt from the migration guide:") {
		t.Fatalf("an excerpt without a source: %q", got)
	}
}

func TestQueryAndDomains(t *testing.T) {
	if got := Query("github.com/golang-jwt/jwt/v5", "v4.5.2", "v5.3.1"); got != "github.com/golang-jwt/jwt/v5 migration guide" {
		t.Errorf("major bump query = %q", got)
	}
	if got := Query("github.com/rs/cors", "v1.10.0", "v1.11.0"); got != "github.com/rs/cors v1.11.0 release notes changelog" {
		t.Errorf("minor bump query = %q", got)
	}
	if got := Query("m", "garbage", "v1.0.0"); got != "m v1.0.0 release notes changelog" {
		t.Errorf("unparseable version query = %q", got)
	}
	if got := Domains("github.com/rs/cors"); !slices.Equal(got, []string{"github.com", "pkg.go.dev"}) {
		t.Errorf("github domains = %v", got)
	}
	if Domains("golang.org/x/text") != nil {
		t.Error("a module that is not on GitHub is searched everywhere")
	}
}

func TestImportersAndTruncation(t *testing.T) {
	dir := t.TempDir()
	for name, src := range map[string]string{
		"a.go":         "package a\n\nimport \"example.com/lib\"\n",
		"sub/b.go":     "package b\n\nimport (\n\t\"fmt\"\n\t\"example.com/lib/extra\"\n)\n",
		"c.go":         "package a\n\nimport \"example.com/library\"\n",
		"a_test.go":    "package a\n\nimport \"example.com/lib\"\n",
		"vendor/v.go":  "package v\n\nimport \"example.com/lib\"\n",
		".hidden/h.go": "package h\n\nimport \"example.com/lib\"\n",
		"broken.go":    "package {\n",
		"notes.txt":    "example.com/lib",
	} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if got := importers(dir, "example.com/lib"); !slices.Equal(got, []string{"a.go", "sub/b.go"}) {
		t.Fatalf("importers = %v", got)
	}
	if importers("", "x") != nil || importers(filepath.Join(dir, "missing"), "x") != nil {
		t.Fatal("no directory means no files")
	}
	if got := truncate(strings.Repeat("a", 50), 10); got != strings.Repeat("a", 10)+"\n... (cut)" {
		t.Fatalf("truncate = %q", got)
	}
	if truncate("short", 10) != "short" {
		t.Fatal("short text must not be cut")
	}
}

func TestGitHubReleasesBetween(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[
			{"tag_name":"v5.3.1","name":"5.3.1","body":"latest","html_url":"u531","published_at":"2026-03-01T00:00:00Z"},
			{"tag_name":"v5.3.2","name":"newer than the target","body":"x","html_url":"u532"},
			{"tag_name":"v5.0.0-rc.1","name":"candidate","body":"x","html_url":"urc"},
			{"tag_name":"v5.0.0","name":"5.0.0","body":"the big one","html_url":"u500"},
			{"tag_name":"5.1.0","name":"no v prefix","body":"minor","html_url":"u510"},
			{"tag_name":"v4.5.2","name":"installed","body":"old","html_url":"u452"},
			{"tag_name":"v4.5.1","name":"older","body":"old","html_url":"u451"},
			{"tag_name":"nightly","name":"not a version","body":"x","html_url":"un"}]`)
	}))
	defer srv.Close()
	g := GitHubReleases{Client: &github.Client{BaseURL: srv.URL, HTTP: srv.Client()}}
	got, err := g.Between(t.Context(), "github.com/golang-jwt/jwt/v5", "v4.5.2", "v5.3.1")
	if err != nil {
		t.Fatal(err)
	}
	var versions []string
	for _, r := range got {
		versions = append(versions, r.Version)
	}
	if !slices.Equal(versions, []string{"v5.3.1", "v5.1.0", "v5.0.0"}) {
		t.Fatalf("versions = %v, want the releases after the installed one up to the target, newest first, without prereleases", versions)
	}
	if got[0].Notes != "latest" || got[0].URL != "u531" || got[0].PublishedAt.IsZero() {
		t.Fatalf("release = %+v", got[0])
	}
	if _, err := g.Between(t.Context(), "golang.org/x/text", "v0.1.0", "v0.2.0"); err == nil {
		t.Fatal("a module that is not on GitHub returned no error")
	}
	if _, err := g.Between(t.Context(), "github.com/a/b", "garbage", "v1.0.0"); err == nil {
		t.Fatal("a bad version returned no error")
	}
}

func TestGitHubReleasesKeepTheNewestEight(t *testing.T) {
	var items []string
	for i := 1; i <= 20; i++ {
		items = append(items, fmt.Sprintf(`{"tag_name":"v1.%d.0","name":"%d","body":"b","html_url":"u"}`, i, i))
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "["+strings.Join(items, ",")+"]")
	}))
	defer srv.Close()
	g := GitHubReleases{Client: &github.Client{BaseURL: srv.URL, HTTP: srv.Client()}}
	got, err := g.Between(t.Context(), "github.com/o/r", "v1.0.0", "v1.20.0")
	if err != nil || len(got) != maxReleases || got[0].Version != "v1.20.0" || got[maxReleases-1].Version != "v1.13.0" {
		t.Fatalf("got %d releases, first %v: %v", len(got), got[0].Version, err)
	}
}

func TestCleanPage(t *testing.T) {
	page := strings.Join([]string{
		"[Skip to content](#start)",
		"## Navigation Menu",
		"[Sign in](/login)",
		"/",
		"195 lines (157 loc) · 8.48 KB",
		"* [Notifications](/login?return_to=x)  You must be signed in to change notification settings",
		"* [Fork 448](/login)",
		"# [MIGRATION\\_GUIDE.md](/golang-jwt/jwt/blob/main/MIGRATION_GUIDE.md)",
		"",
		"",
		"# [MIGRATION\\_GUIDE.md](/golang-jwt/jwt/blob/main/MIGRATION_GUIDE.md)",
		"Copy raw file",
		"[History](/commits)",
		"# Migration Guide (v5.0.0)",
		"Version `v5` reworks the claims. See [the guide](https://example.test) for more.",
		"[one](/1) [two](/2) [three](/3)",
		"```",
		"\"github.com/golang-jwt/jwt/v5\"",
		"```",
	}, "\n")
	got := CleanPage(page)
	for _, gone := range []string{"Skip to content", "Navigation Menu", "Sign in", "Notifications", "Fork 448", "Copy raw file", "History", "[one]"} {
		if strings.Contains(got, gone) {
			t.Errorf("the cleaned page still has %q:\n%s", gone, got)
		}
	}
	for _, kept := range []string{"# MIGRATION\\_GUIDE.md", "# Migration Guide (v5.0.0)", "Version `v5` reworks the claims. See [the guide](https://example.test) for more.", "\"github.com/golang-jwt/jwt/v5\""} {
		if !strings.Contains(got, kept) {
			t.Errorf("the cleaned page lost %q:\n%s", kept, got)
		}
	}
	if strings.Contains(got, "\n\n\n") {
		t.Error("blank lines should not pile up")
	}
	if strings.Count(got, "# MIGRATION\\_GUIDE.md") != 1 || strings.Contains(got, "195 lines") || strings.HasPrefix(got, "/") {
		t.Errorf("repeated titles and file details must go:\n%s", got)
	}
	if CleanPage("") != "" || CleanPage("[Sign in](/x)") != "" {
		t.Error("a page with only chrome cleans to nothing")
	}
}

func TestTheMigrationGuideComesBeforeTheReleaseNotes(t *testing.T) {
	f, _, _, m := newForecaster(t)
	if _, _, err := f.Forecast(t.Context(), jwtInput("")); err != nil {
		t.Fatal(err)
	}
	prompt := m.requests[0].Messages[1].Content
	guide, notes := strings.Index(prompt, "long guide text"), strings.Index(prompt, "StandardClaims is gone")
	if guide < 0 || notes < 0 || guide > notes {
		t.Fatalf("the guide must come first: guide at %d, notes at %d", guide, notes)
	}
	if !strings.Contains(m.requests[0].Messages[0].Content, "base the answer on it") {
		t.Fatal("the instructions should prefer the migration guide")
	}
}
