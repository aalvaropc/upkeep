package runtime

import (
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
	if _, err := rt.Processor(nil); err != nil {
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
	if _, err := rt.Processor(nil); err != nil {
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
	if _, err := rt.Processor(c.emit); err != nil || len(c.events) != 0 {
		t.Fatalf("with budget left: error %v, events %v", err, c.events)
	}
	rt.Budget.Add(150)
	if !rt.Budget.Exhausted() {
		t.Fatal("the budget should be exhausted")
	}
	if _, err := rt.Processor(c.emit); err != nil {
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
	if _, err := rt.Processor(c.emit); err != nil {
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
	if _, err := rt.Processor(nil); err == nil || !strings.Contains(err.Error(), "Token Factory") {
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
	if _, err := rt.Processor(nil); err == nil || !strings.Contains(err.Error(), "budget") {
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
