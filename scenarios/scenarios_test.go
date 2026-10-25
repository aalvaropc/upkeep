package scenarios

import (
	"io/fs"
	"path"
	"testing"
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
