package scenarios

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/aalvaropc/upkeep/internal/forecast"
)

func TestNotesScenarioIsValidAndEveryRecipeExists(t *testing.T) {
	s, err := Notes()
	if err != nil {
		t.Fatal(err)
	}
	if len(s.PRs) != 9 {
		t.Fatalf("the scenario has %d pull requests, want nine", len(s.PRs))
	}
	recipes := append([]string(nil), s.Main...)
	for _, pr := range s.PRs {
		recipes = append(recipes, pr.Recipe)
	}
	for _, name := range recipes {
		matches, err := fs.Glob(NotesFS(), path.Join("recipes", name, "*.patch"))
		if err != nil || len(matches) == 0 {
			t.Errorf("recipe %q has no patches (%v)", name, err)
		}
	}
}

func TestNoRecipeIsUnused(t *testing.T) {
	s, err := Notes()
	if err != nil {
		t.Fatal(err)
	}
	used := map[string]bool{}
	for _, name := range s.Main {
		used[name] = true
	}
	for _, pr := range s.PRs {
		used[pr.Recipe] = true
	}
	entries, err := fs.ReadDir(NotesFS(), "recipes")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !used[e.Name()] {
			t.Errorf("recipe %q is not used by the scenario", e.Name())
		}
	}
}

func TestTheRecordedRunIsEmbeddedAndComplete(t *testing.T) {
	store, ok := Fixtures()
	if !ok {
		t.Fatal("the binary carries no recorded run")
	}
	var prices map[string]struct {
		Input float64 `json:"input_per_million"`
	}
	if err := store.Load("meta", "prices", &prices); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"nvidia/Nemotron-3-Ultra-550b-a55b", "nvidia/nemotron-3-super-120b-a12b", "nvidia/NVIDIA-Nemotron-3-Nano-30B-A3B"} {
		if prices[name].Input <= 0 {
			t.Errorf("no recorded price for %s", name)
		}
	}
	sub, err := fs.Sub(files, "notes/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	for kind, atLeast := range map[string]int{"model": 20, "forecast": 2} {
		entries, err := fs.ReadDir(sub, kind)
		if err != nil || len(entries) < atLeast {
			t.Errorf("%s: %d recordings (%v), want at least %d", kind, len(entries), err, atLeast)
		}
	}
}

func embeddedForecasts(t *testing.T) map[string]forecast.Forecast {
	t.Helper()
	store, ok := Fixtures()
	if !ok {
		t.Fatal("the binary carries no recorded run")
	}
	entries, err := fs.ReadDir(NotesFS(), path.Join("fixtures", forecast.CacheKind))
	if err != nil || len(entries) == 0 {
		t.Fatalf("no recorded forecasts: %v", err)
	}
	out := map[string]forecast.Forecast{}
	for _, e := range entries {
		var rec struct {
			Forecast forecast.Forecast `json:"forecast"`
		}
		if err := store.Load(forecast.CacheKind, strings.TrimSuffix(e.Name(), ".json"), &rec); err != nil {
			t.Fatal(err)
		}
		out[rec.Forecast.Module] = rec.Forecast
	}
	return out
}

func TestTheRecordedForecastNotesDoNotChange(t *testing.T) {
	want := map[string]string{
		"github.com/jonboulle/clockwork": "6fe1f52ad9e655e9bca593352876f6bd474aee9adb58d4ad6e6029de3ca0bd8c",
		"github.com/golang-jwt/jwt/v5":   "832cf47ae3e1c5668fad5ae185a86fd32fc52ef6f49680348dce766eaae8f38b",
	}
	got := embeddedForecasts(t)
	if len(got) != len(want) {
		t.Fatalf("recorded forecasts for %d modules, want %d", len(got), len(want))
	}
	for module, sum := range want {
		digest := sha256.Sum256([]byte(got[module].Notes()))
		if hex.EncodeToString(digest[:]) != sum {
			t.Errorf("the notes for %s changed, so the recorded fixer requests no longer match:\n%s", module, got[module].Notes())
		}
	}
}

func TestEveryRecordedForecastSaysWhatItSearched(t *testing.T) {
	for module, f := range embeddedForecasts(t) {
		s := f.Search
		if s == nil {
			t.Errorf("%s: no search", module)
			continue
		}
		if s.Provider != "tavily" || s.Query != forecast.Query(f.Module, f.From, f.To) || !s.Recorded || !s.At.Equal(f.FetchedAt) || s.LatencyMS != 0 || len(s.Kept) == 0 {
			t.Errorf("%s: search = %+v", module, s)
		}
		if len(s.Kept)+len(f.Releases) != len(f.Sources) {
			t.Errorf("%s: %d pages and %d release notes for %d sources", module, len(s.Kept), len(f.Releases), len(f.Sources))
		}
		for _, u := range f.Releases {
			if !strings.Contains(u, "/releases/tag/") || !slices.Contains(f.Sources, u) {
				t.Errorf("%s: %s is not one of its release pages", module, u)
			}
		}
		for _, u := range s.Kept {
			if strings.Contains(u, "/releases/tag/") || !slices.Contains(f.Sources, u) {
				t.Errorf("%s: %s is not one of the pages the search found", module, u)
			}
		}
	}
}
