package runtime

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aalvaropc/upkeep/internal/config"
	"github.com/aalvaropc/upkeep/internal/events"
	"github.com/aalvaropc/upkeep/internal/fixture"
	"github.com/aalvaropc/upkeep/internal/forecast"
	"github.com/aalvaropc/upkeep/internal/gitx"
	"github.com/aalvaropc/upkeep/internal/model"
	"github.com/aalvaropc/upkeep/internal/policy"
)

const catalog = `{"data":[
  {"id":"nvidia/Nemotron-3-Ultra-550b-a55b","pricing":{"prompt":"0.000001","completion":"0.000003"}},
  {"id":"nvidia/nemotron-3-super-120b-a12b","pricing":{"prompt":"0.0000003","completion":"0.0000009"}},
  {"id":"nvidia/NVIDIA-Nemotron-3-Nano-30B-A3B","pricing":{"prompt":"0.00000006","completion":"0.00000024"}}
]}`

const completion = `{"choices":[{"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":1}}`

func tokenFactoryServer(t *testing.T, chatStatus int) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			_, _ = w.Write([]byte(catalog))
		case "/chat/completions":
			if chatStatus != http.StatusOK {
				http.Error(w, `{"detail":"insufficient balance"}`, chatStatus)
				return
			}
			_, _ = w.Write([]byte(completion))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func options(t *testing.T, mode config.Mode) Options {
	t.Helper()
	return optionsAt(t, mode, tokenFactoryServer(t, http.StatusOK))
}

func optionsAt(t *testing.T, mode config.Mode, baseURL string) Options {
	t.Helper()
	cfg, err := config.Load([]string{"NEBIUS_API_KEY=test-key", "UPKEEP_MODE=" + string(mode), "UPKEEP_SANDBOX=exec", "NEBIUS_BASE_URL=" + baseURL})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	dir := fixture.Dir{Path: filepath.Join(root, "fixtures")}
	return Options{Config: cfg, Policy: policy.Default(), Git: gitx.Git{}, Fixtures: dir, Record: dir, CacheDir: filepath.Join(root, "cache")}
}

type collector struct {
	mu     sync.Mutex
	events []events.Kind
	data   []any
}

func (c *collector) emit(kind events.Kind, _ int, data any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, kind)
	c.data = append(c.data, data)
}

func TestRecordingSavesThePricesThatReplayLaterReads(t *testing.T) {
	rec := options(t, config.Record)
	if _, err := New(t.Context(), rec); err != nil {
		t.Fatal(err)
	}
	var saved model.PriceBook
	if err := rec.Fixtures.Load("meta", "prices", &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved) != 3 {
		t.Fatalf("saved prices = %v", saved)
	}

	rep := options(t, config.Replay)
	rep.Fixtures = rec.Fixtures
	rt, err := New(t.Context(), rep)
	if err != nil {
		t.Fatal(err)
	}
	if rt.primary.mode != config.Replay {
		t.Fatalf("mode = %s", rt.primary.mode)
	}
	got, ok := rt.primary.meter.Prices.Cost("nvidia/nemotron-3-super-120b-a12b", model.Usage{PromptTokens: 1_000_000})
	if !ok || got != 300_000 {
		t.Fatalf("a replayed run priced 1M prompt tokens at %d micro-USD, want 300000", got)
	}
}

func TestRecordModeNeedsAPlaceToSave(t *testing.T) {
	o := options(t, config.Record)
	o.Record = nil
	if _, err := New(t.Context(), o); err == nil || !strings.Contains(err.Error(), "somewhere to save") {
		t.Fatalf("New() = %v", err)
	}
}

func TestReplayWithoutRecordedPricesStillRuns(t *testing.T) {
	o := options(t, config.Replay)
	rt, err := New(t.Context(), o)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Processor(nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestReplayWithoutAnyFixturesStillStarts(t *testing.T) {
	o := options(t, config.Replay)
	o.Fixtures = nil
	rt, err := New(t.Context(), o)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Processor(nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestLiveModeNeedsAKey(t *testing.T) {
	o := options(t, config.Live)
	cfg, err := config.Load([]string{"UPKEEP_MODE=live", "UPKEEP_SANDBOX=exec"})
	if err != nil {
		t.Skipf("the configuration rejects a live run without a key: %v", err)
	}
	o.Config = cfg
	if _, err := New(t.Context(), o); err == nil || !strings.Contains(err.Error(), "NEBIUS_API_KEY") {
		t.Fatalf("New() = %v, want a complaint about the key", err)
	}
}

func TestAnExhaustedLiveBudgetFallsBackToRecordedRuns(t *testing.T) {
	rec := options(t, config.Record)
	if _, err := New(t.Context(), rec); err != nil {
		t.Fatal(err)
	}
	live := options(t, config.Live)
	live.Fixtures = rec.Fixtures
	live.Config.DailyTokens = 100
	rt, err := New(t.Context(), live)
	if err != nil {
		t.Fatal(err)
	}
	c := &collector{}
	if _, err := rt.Processor(c.emit, nil); err != nil || len(c.events) != 0 {
		t.Fatalf("with budget left: error %v, events %v", err, c.events)
	}
	rt.Budget.Add(150)
	if !rt.Budget.Exhausted() {
		t.Fatal("the budget should be exhausted")
	}
	if _, err := rt.Processor(c.emit, nil); err != nil {
		t.Fatal(err)
	}
	if len(c.events) != 1 || c.events[0] != events.Notice {
		t.Fatalf("events = %v, want a notice", c.events)
	}
}

func TestARefusingTokenFactoryFallsBackToRecordedRuns(t *testing.T) {
	rec := options(t, config.Record)
	if _, err := New(t.Context(), rec); err != nil {
		t.Fatal(err)
	}
	live := optionsAt(t, config.Live, tokenFactoryServer(t, http.StatusPaymentRequired))
	live.Fixtures = rec.Fixtures
	rt, err := New(t.Context(), live)
	if err != nil {
		t.Fatal(err)
	}
	c := &collector{}
	if _, err := rt.Processor(c.emit, nil); err != nil {
		t.Fatal(err)
	}
	if len(c.events) != 1 || c.events[0] != events.Notice {
		t.Fatalf("events = %v, want a notice", c.events)
	}
	notice, _ := c.data[0].(map[string]string)
	if !strings.Contains(notice["message"], "Token Factory") || !strings.Contains(notice["message"], "replays a recorded one") {
		t.Fatalf("notice = %q", notice["message"])
	}
}

func TestARefusingTokenFactoryWithNothingToReplayStopsTheRun(t *testing.T) {
	live := optionsAt(t, config.Live, tokenFactoryServer(t, http.StatusUnauthorized))
	live.Fixtures = nil
	rt, err := New(t.Context(), live)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Processor(nil, nil); err == nil || !strings.Contains(err.Error(), "Token Factory") {
		t.Fatalf("Processor() = %v", err)
	}
}

func TestAnExhaustedLiveBudgetWithNothingToReplayRefuses(t *testing.T) {
	live := options(t, config.Live)
	live.Fixtures = nil
	live.Config.DailyTokens = 10
	rt, err := New(t.Context(), live)
	if err != nil {
		t.Fatal(err)
	}
	rt.Budget.Add(50)
	if _, err := rt.Processor(nil, nil); err == nil || !strings.Contains(err.Error(), "budget") {
		t.Fatalf("Processor() = %v", err)
	}
}

func TestBudgetCountsPersistsAndRollsOver(t *testing.T) {
	now := time.Date(2026, 10, 25, 23, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "usage.json")
	b := &Budget{Path: path, Limit: 1000, Now: func() time.Time { return now }}
	b.Add(400)
	b.Add(-5)
	b.Add(0)
	if b.Spent() != 400 || b.Exhausted() {
		t.Fatalf("spent %d, exhausted %v", b.Spent(), b.Exhausted())
	}
	reopened := &Budget{Path: path, Limit: 1000, Now: func() time.Time { return now }}
	if reopened.Spent() != 400 {
		t.Fatalf("a reopened budget remembers %d tokens, want 400", reopened.Spent())
	}
	reopened.Add(700)
	if !reopened.Exhausted() {
		t.Fatal("1100 of 1000 tokens should be exhausted")
	}
	now = now.Add(2 * time.Hour)
	if reopened.Spent() != 0 || reopened.Exhausted() {
		t.Fatalf("the next day starts at %d", reopened.Spent())
	}
}

type countingSearch struct {
	calls int
}

func (c *countingSearch) Search(context.Context, string, int) ([]forecast.Result, error) {
	c.calls++
	return []forecast.Result{{Title: "t", URL: "https://github.com/a/b"}}, nil
}

func liveForecaster(t *testing.T, o Options) forecast.Forecaster {
	t.Helper()
	rt, err := New(t.Context(), o)
	if err != nil {
		t.Fatal(err)
	}
	f, ok := rt.primary.forecaster.(forecast.Forecaster)
	if !ok {
		t.Fatalf("forecaster = %T", rt.primary.forecaster)
	}
	return f
}

func TestLiveSearchesCountAgainstTheDailyLimit(t *testing.T) {
	o := options(t, config.Live)
	o.Config.TavilyAPIKey = config.Secret("tvly-test")
	o.Config.SearchDailyLimit = 2
	o.Config.SearchPrice = 0.008
	rt, err := New(t.Context(), o)
	if err != nil {
		t.Fatal(err)
	}
	f := rt.primary.forecaster.(forecast.Forecaster)
	if !f.Fresh || f.SearchMicros != 8000 || forecast.ProviderOf(f.Search) != "tavily" {
		t.Fatalf("a live forecaster searches on every run: fresh %v, micros %d, provider %q", f.Fresh, f.SearchMicros, forecast.ProviderOf(f.Search))
	}
	if _, ok := f.Releases.(forecast.CachedReleases); !ok {
		t.Fatalf("live release lookups are cached: %T", f.Releases)
	}
	limited, ok := f.Search.(limitedSearch)
	if !ok {
		t.Fatalf("search = %T", f.Search)
	}
	inner := &countingSearch{}
	limited.inner = inner
	for i := range 2 {
		if _, err := limited.Search(t.Context(), "q", 4); err != nil {
			t.Fatalf("search %d: %v", i+1, err)
		}
	}
	_, err = limited.Search(t.Context(), "q", 4)
	var skip *forecast.SkipError
	if !errors.As(err, &skip) || skip.Reason != "the daily search limit is reached" || inner.calls != 2 {
		t.Fatalf("the third search = %v, %d calls", err, inner.calls)
	}
	if rt.Searches.Spent() != 2 {
		t.Fatalf("searches today = %d", rt.Searches.Spent())
	}
	reopened := &Budget{Path: filepath.Join(o.CacheDir, "searches.json")}
	if reopened.Spent() != 2 {
		t.Fatalf("the count is saved: %d", reopened.Spent())
	}
}

func TestASearchLimitOfZeroTurnsTheSearchOff(t *testing.T) {
	o := options(t, config.Live)
	o.Config.TavilyAPIKey = config.Secret("tvly-test")
	o.Config.SearchDailyLimit = 0
	if f := liveForecaster(t, o); f.Search != nil {
		t.Fatalf("search = %T", f.Search)
	}
	rec := options(t, config.Record)
	rec.Config.TavilyAPIKey = config.Secret("tvly-test")
	if f := liveForecaster(t, rec); f.Fresh || f.Search == nil {
		t.Fatalf("a recording keeps its cached forecasts: fresh %v, search %T", f.Fresh, f.Search)
	}
}

func TestTakeStopsAtTheLimit(t *testing.T) {
	b := &Budget{Limit: 3}
	if !b.Take(2) || b.Take(2) || !b.Take(1) || b.Take(1) || b.Spent() != 3 {
		t.Fatalf("spent %d", b.Spent())
	}
	var none *Budget
	if !none.Take(5) || !(&Budget{}).Take(1<<20) {
		t.Fatal("a nil or unlimited budget always has room")
	}
}

func TestUnlimitedAndNilBudgetsNeverRunOut(t *testing.T) {
	var none *Budget
	none.Add(100)
	if none.Exhausted() || none.Spent() != 0 {
		t.Fatal("a nil budget should be inert")
	}
	free := &Budget{}
	free.Add(1 << 40)
	if free.Exhausted() {
		t.Fatal("a budget without a limit should never be exhausted")
	}
}

func nanoCall(t *testing.T, rt *Runtime) error {
	t.Helper()
	name, m, err := rt.primary.router.For(model.Nano)
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.Complete(t.Context(), model.Request{Model: name, Messages: []model.Message{{Role: "user", Content: "hello"}}, MaxTokens: 1})
	return err
}

func TestABudgetThatRunsOutMidRunStopsTheNextCall(t *testing.T) {
	for _, mode := range []config.Mode{config.Live, config.Record} {
		t.Run(string(mode), func(t *testing.T) {
			o := options(t, mode)
			o.Config.DailyTokens = 20
			rt, err := New(t.Context(), o)
			if err != nil {
				t.Fatal(err)
			}
			for i := range 2 {
				if err := nanoCall(t, rt); err != nil {
					t.Fatalf("call %d with budget left: %v", i+1, err)
				}
			}
			if rt.Budget.Spent() != 26 || !rt.Budget.Exhausted() {
				t.Fatalf("spent %d, exhausted %v", rt.Budget.Spent(), rt.Budget.Exhausted())
			}
			err = nanoCall(t, rt)
			if err == nil || err.Error() != "the daily token budget of 20 tokens ran out during the run" {
				t.Fatalf("the call after the budget ran out = %v", err)
			}
			if rt.Budget.Spent() != 26 {
				t.Fatalf("a refused call spent tokens: %d", rt.Budget.Spent())
			}
		})
	}
}

func TestTheCheckCallCountsAgainstTheBudget(t *testing.T) {
	rt, err := New(t.Context(), options(t, config.Live))
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.probe(t.Context()); err != nil {
		t.Fatal(err)
	}
	if rt.Budget.Spent() != 13 {
		t.Fatalf("the check call spent %d tokens, want 13", rt.Budget.Spent())
	}
}

func TestReplayNeverRunsOutOfBudget(t *testing.T) {
	rec := options(t, config.Record)
	recorder, err := New(t.Context(), rec)
	if err != nil {
		t.Fatal(err)
	}
	if err := nanoCall(t, recorder); err != nil {
		t.Fatal(err)
	}
	rep := options(t, config.Replay)
	rep.Fixtures = rec.Fixtures
	rep.Config.DailyTokens = 1
	rt, err := New(t.Context(), rep)
	if err != nil {
		t.Fatal(err)
	}
	rt.Budget.Add(1000)
	for i := range 3 {
		if err := nanoCall(t, rt); err != nil {
			t.Fatalf("replayed call %d: %v", i+1, err)
		}
	}
	if rt.Budget.Spent() != 1000 {
		t.Fatalf("a replay spent live tokens: %d", rt.Budget.Spent())
	}
}
