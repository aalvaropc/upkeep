package scenarios

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/aalvaropc/upkeep/internal/detect"
	"github.com/aalvaropc/upkeep/internal/gitx"
	"github.com/aalvaropc/upkeep/internal/policy"
	"github.com/aalvaropc/upkeep/internal/sandbox"
	"github.com/aalvaropc/upkeep/internal/scenario"
)

func TestEveryPullRequestShowsItsExpectedBlocker(t *testing.T) {
	if os.Getenv("UPKEEP_LIVE_TESTS") != "1" {
		t.Skip("set UPKEEP_LIVE_TESTS=1 to build the scenario from the sample repository")
	}
	s, err := Notes()
	if err != nil {
		t.Fatal(err)
	}
	source := s.Repository
	if override := os.Getenv("UPKEEP_SAMPLE_SOURCE"); override != "" {
		source = override
	}
	g := gitx.Git{}
	forge := filepath.Join(t.TempDir(), "forge.git")
	built, err := scenario.Builder{Git: g, Recipes: NotesFS()}.Build(t.Context(), s, source, forge)
	if err != nil {
		t.Skipf("the scenario could not be built from %s: %v", source, err)
	}
	t.Logf("main is %s", built.Main[:8])

	detector := detect.Detector{Git: g, Runner: sandbox.GoRunner{Executor: sandbox.Exec{}}, Policy: policy.Default()}
	for _, pr := range s.PRs {
		t.Run(fmt.Sprintf("pr%d-%s", pr.Number, pr.Recipe), func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "work")
			if err := g.Clone(t.Context(), forge, dir); err != nil {
				t.Fatal(err)
			}
			if err := g.Checkout(t.Context(), dir, "origin/"+pr.Branch, false); err != nil {
				t.Fatal(err)
			}
			if err := g.Checkout(t.Context(), dir, pr.Branch, true); err != nil {
				t.Fatal(err)
			}
			diag, err := detector.Detect(t.Context(), detect.Workspace{Dir: dir, Base: "origin/" + s.Base})
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("blocker %s, behind %v, conflicts %d (largest %d lines), unformatted %v, flaky %v", diag.Blocker, diag.Behind, diag.Merge.Hunks(), diag.Merge.LargestHunk(), diag.Unformatted, diag.FlakyTests)
			if diag.Blocker != pr.Expect.Blocker {
				t.Errorf("blocker = %s, want %s", diag.Blocker, pr.Expect.Blocker)
			}
		})
	}
}
