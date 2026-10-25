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
