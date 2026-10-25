package forge_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aalvaropc/upkeep/internal/forge"
	"github.com/aalvaropc/upkeep/internal/gitx"
	"github.com/aalvaropc/upkeep/internal/scenario"
	"github.com/aalvaropc/upkeep/scenarios"
)

func TestTheTwoDependencyRequestsMergeOneAfterTheOther(t *testing.T) {
	if os.Getenv("UPKEEP_LIVE_TESTS") != "1" {
		t.Skip("set UPKEEP_LIVE_TESTS=1 to build the scenario from the sample repository")
	}
	s, err := scenarios.Notes()
	if err != nil {
		t.Fatal(err)
	}
	source := s.Repository
	if override := os.Getenv("UPKEEP_SAMPLE_SOURCE"); override != "" {
		source = override
	}
	g := gitx.Git{}
	template := filepath.Join(t.TempDir(), "forge.git")
	if _, err := (scenario.Builder{Git: g, Recipes: scenarios.NotesFS()}).Build(t.Context(), s, source, template); err != nil {
		t.Skipf("the scenario could not be built from %s: %v", source, err)
	}
	l, err := forge.CopyLocal(t.Context(), g, template, filepath.Join(t.TempDir(), "session.git"), s)
	if err != nil {
		t.Fatal(err)
	}
	pulls, err := l.Pulls(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	byNumber := map[int]int{}
	for i, p := range pulls {
		byNumber[p.Number] = i
	}
	for _, number := range []int{6, 7} {
		if _, err := l.Merge(t.Context(), pulls[byNumber[number]]); err != nil {
			t.Fatalf("merging #%d after the other dependency request: %v", number, err)
		}
	}
}
