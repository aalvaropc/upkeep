package model

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aalvaropc/upkeep/internal/config"
)

const catalog = `{"data":[
  {"id":"nvidia/Nemotron-3-Ultra-550b-a55b","pricing":{"prompt":"0.000001","completion":"0.000003","input_cache_read":"0.00000012"}},
  {"id":"nvidia/nemotron-3-super-120b-a12b","pricing":{"prompt":"0.0000003","completion":"0.0000009","input_cache_read":"0.00000006"}},
  {"id":"nvidia/NVIDIA-Nemotron-3-Nano-30B-A3B","pricing":{"prompt":"0.00000006","completion":"0.00000024","input_cache_read":null}},
  {"id":"free/model","pricing":{"prompt":"0","completion":"0"}},
  {"id":"broken/model","pricing":{"prompt":"cheap","completion":"0.1"}},
  {"id":"","pricing":{"prompt":"0.1","completion":"0.1"}}
]}`

func TestParseCatalog(t *testing.T) {
	book, err := ParseCatalog([]byte(catalog))
	if err != nil {
		t.Fatal(err)
	}
	if len(book) != 3 {
		t.Fatalf("book has %d models, want only the three priced ones: %v", len(book), book)
	}
	ultra := book["nvidia/Nemotron-3-Ultra-550b-a55b"]
	if !near(ultra.InputPerMillion, 1) || !near(ultra.OutputPerMillion, 3) || !near(ultra.CachedInputPerMillion, 0.12) {
		t.Fatalf("ultra = %+v", ultra)
	}
	nano := book["nvidia/NVIDIA-Nemotron-3-Nano-30B-A3B"]
	if !near(nano.InputPerMillion, 0.06) || !near(nano.OutputPerMillion, 0.24) || nano.CachedInputPerMillion != 0 {
		t.Fatalf("nano = %+v", nano)
	}
}

func near(a, b float64) bool {
	d := a - b
	return d < 1e-9 && d > -1e-9
}

func TestParseCatalogErrors(t *testing.T) {
	for name, raw := range map[string]string{
		"not json":      "nope",
		"empty":         `{"data":[]}`,
		"only unpriced": `{"data":[{"id":"x","pricing":{"prompt":"0","completion":"0"}}]}`,
	} {
		if _, err := ParseCatalog([]byte(raw)); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestCost(t *testing.T) {
	ultra := Price{InputPerMillion: 1, OutputPerMillion: 3, CachedInputPerMillion: 0.12}
	if got := ultra.Cost(Usage{PromptTokens: 1_000_000, CompletionTokens: 1_000_000}); got != 4_000_000 {
		t.Errorf("1M in and 1M out = %d micro-USD, want 4000000", got)
	}
	if got := ultra.Cost(Usage{PromptTokens: 1000, CachedPromptTokens: 1000}); got != 120 {
		t.Errorf("fully cached prompt = %d, want 120", got)
	}
	if got := ultra.Cost(Usage{PromptTokens: 1000, CachedPromptTokens: 9999}); got != 120 {
		t.Errorf("cached tokens above the prompt must be clamped: %d", got)
	}
	noCache := Price{InputPerMillion: 0.06, OutputPerMillion: 0.24}
	if got := noCache.Cost(Usage{PromptTokens: 1000, CachedPromptTokens: 500}); got != 60 {
		t.Errorf("without a cached rate the input rate applies: %d", got)
	}
	if got := ultra.Cost(Usage{}); got != 0 {
		t.Errorf("no usage = %d", got)
	}
}

func TestMeterChargeAndBaseline(t *testing.T) {
	book, err := ParseCatalog([]byte(catalog))
	if err != nil {
		t.Fatal(err)
	}
	const ultra, super = "nvidia/Nemotron-3-Ultra-550b-a55b", "nvidia/nemotron-3-super-120b-a12b"
	m := Meter{Prices: book, Baseline: ultra}
	u := Usage{PromptTokens: 50_000, CompletionTokens: 2_000}
	c := m.Charge(super, u)
	if !c.Known || c.Micros != 16_800 || c.BaselineMicros != 56_000 || c.SavedMicros() != 39_200 {
		t.Fatalf("charge = %+v", c)
	}
	if unknown := m.Charge("other/model", u); unknown.Known || unknown.Micros != 0 || unknown.BaselineMicros != 0 {
		t.Fatalf("unknown model charge = %+v, want zero and not known", unknown)
	}
	total := c.Add(m.Charge(ultra, u))
	if total.Micros != 16_800+56_000 || total.BaselineMicros != 112_000 {
		t.Fatalf("total = %+v", total)
	}
	noBaseline := Meter{Prices: book}.Charge(super, u)
	if !noBaseline.Known || noBaseline.BaselineMicros != 0 {
		t.Fatalf("without a baseline model: %+v", noBaseline)
	}
}

func TestApplyOverrides(t *testing.T) {
	book, _ := ParseCatalog([]byte(catalog))
	models := config.Models{Ultra: "nvidia/Nemotron-3-Ultra-550b-a55b", Super: "nvidia/nemotron-3-super-120b-a12b", Nano: "nvidia/NVIDIA-Nemotron-3-Nano-30B-A3B"}
	out := ApplyOverrides(book, models, config.Prices{Super: &config.Price{Input: 0.5, Output: 1.5}})
	super := out[models.Super]
	if !near(super.InputPerMillion, 0.5) || !near(super.OutputPerMillion, 1.5) || !near(super.CachedInputPerMillion, 0.06) {
		t.Fatalf("super = %+v, want the override with the catalog's cached rate", super)
	}
	if !near(book[models.Super].InputPerMillion, 0.3) {
		t.Fatal("the original book was modified")
	}
	if !near(out[models.Ultra].InputPerMillion, 1) {
		t.Fatal("models without an override should keep the catalog price")
	}
	added := ApplyOverrides(PriceBook{}, models, config.Prices{Nano: &config.Price{Input: 0.1, Output: 0.2}})
	if !near(added[models.Nano].InputPerMillion, 0.1) || len(added) != 1 {
		t.Fatalf("an override should price a model the catalog does not list: %v", added)
	}
}

func TestFetchPrices(t *testing.T) {
	var query, auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query, auth = r.URL.RawQuery, r.Header.Get("Authorization")
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, catalog)
	}))
	defer srv.Close()
	book, err := client(srv).FetchPrices(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if query != "verbose=true" || auth != "Bearer "+testKey.Reveal() || len(book) != 3 {
		t.Fatalf("query %q, book %v", query, book)
	}

	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"bad key"}}`)
	}))
	defer failing.Close()
	if _, err := client(failing).FetchPrices(t.Context()); err == nil {
		t.Fatal("a 401 returned no error")
	}
}
