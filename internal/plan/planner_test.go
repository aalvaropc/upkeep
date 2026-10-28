package plan_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/aalvaropc/upkeep/internal/forge"
	"github.com/aalvaropc/upkeep/internal/forge/forgetest"
	"github.com/aalvaropc/upkeep/internal/gitx"
	"github.com/aalvaropc/upkeep/internal/plan"
	"github.com/aalvaropc/upkeep/internal/pull"
	"github.com/aalvaropc/upkeep/internal/scenario"
	"github.com/aalvaropc/upkeep/scenarios"
)

func prepare(t *testing.T, f forge.Forge) map[int]plan.Clone {
	t.Helper()
	pulls, err := f.Pulls(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	clones := map[int]plan.Clone{}
	for _, pr := range pulls {
		dir := filepath.Join(work, pr.Branch)
		if err := f.Prepare(t.Context(), pr, dir); err != nil {
			t.Fatal(err)
		}
		clones[pr.Number] = plan.Clone{PR: pr, Dir: dir}
	}
	return clones
}

func pick(clones map[int]plan.Clone, numbers ...int) []plan.Clone {
	out := make([]plan.Clone, 0, len(numbers))
	for _, n := range numbers {
		out = append(out, clones[n])
	}
	return out
}

func TestThePlannerFindsTheRequestsThatConflict(t *testing.T) {
	f := forgetest.New(t)
	clones := prepare(t, f)
	p, err := plan.Planner{Git: gitx.Git{}}.Plan(t.Context(), "origin/main", pick(clones, 1, 2, 3, 4))
	if err != nil {
		t.Fatal(err)
	}
	if want := []int{4, 2, 1, 3}; !slices.Equal(p.Order, want) {
		t.Fatalf("order = %v, want %v", p.Order, want)
	}
	if len(p.Overlaps) != 1 || p.Overlaps[0].A != 1 || p.Overlaps[0].B != 3 || !p.Overlaps[0].Conflict || !slices.Equal(p.Overlaps[0].Files, []string{"a.txt"}) {
		t.Fatalf("overlaps = %+v", p.Overlaps)
	}
	if got := p.Reasons[3]; got != "Conflicts with #1 in a.txt: merge it after #1 and update its branch." {
		t.Fatalf("reason for #3 = %q", got)
	}
}

func TestARequestGoesStaleWhenAnotherMergesIntoMain(t *testing.T) {
	f := forgetest.New(t)
	ctx := t.Context()
	clones := pick(prepare(t, f), 1, 2, 3)
	planner := plan.Planner{Git: gitx.Git{}}
	current, err := planner.Plan(ctx, "origin/main", clones)
	if err != nil {
		t.Fatal(err)
	}
	if stale, err := planner.Stale(ctx, "origin/main", clones[1:], nil, current); err != nil || len(stale) != 0 {
		t.Fatalf("before anything merged: stale = %v %v", stale, err)
	}
	if _, err := f.Merge(ctx, clones[0].PR); err != nil {
		t.Fatal(err)
	}
	stale, err := planner.Stale(ctx, "origin/main", clones[1:], []int{1}, current)
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != 1 || stale[0].PR != 3 || stale[0].Because != "conflicts with main after #1 merged: a.txt" {
		t.Fatalf("stale = %+v", stale)
	}
}

func TestAConflictWithAChangeMadeOutsideTheRunIsStillReported(t *testing.T) {
	f := forgetest.New(t)
	ctx := t.Context()
	clones := pick(prepare(t, f), 3)
	planner := plan.Planner{Git: gitx.Git{}}
	current, err := planner.Plan(ctx, "origin/main", clones)
	if err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(t.TempDir(), "other")
	g := gitx.Git{}
	if err := g.Clone(ctx, f.Repo, other); err != nil {
		t.Fatal(err)
	}
	forgetest.Write(t, other, "a.txt", "four\n")
	if _, err := g.Commit(ctx, other, "change a"); err != nil {
		t.Fatal(err)
	}
	if err := g.Push(ctx, other, "origin", "main"); err != nil {
		t.Fatal(err)
	}
	stale, err := planner.Stale(ctx, "origin/main", clones, nil, current)
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != 1 || stale[0].Because != "conflicts with main, which moved during the run: a.txt" {
		t.Fatalf("stale = %+v", stale)
	}
}

func TestPlanningFailsWhenAClonesBaseIsMissing(t *testing.T) {
	f := forgetest.New(t)
	clones := pick(prepare(t, f), 1, 3)
	if _, err := (plan.Planner{Git: gitx.Git{}}).Plan(t.Context(), "origin/nope", clones); err == nil {
		t.Fatal("a missing base returned no error")
	}
}

func TestTheScenarioBumpsOfJWTAndClockworkConflict(t *testing.T) {
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
	f := forge.NewLocal(g, template, s)
	clones := prepare(t, f)
	all := make([]plan.Clone, 0, len(clones))
	for _, pr := range s.PRs {
		all = append(all, clones[pr.Number])
	}
	p, err := plan.Planner{Git: g}.Plan(t.Context(), "origin/"+s.Base, all)
	if err != nil {
		t.Fatal(err)
	}
	var conflicts []plan.Overlap
	for _, o := range p.Overlaps {
		if o.Conflict {
			conflicts = append(conflicts, o)
		}
	}
	if len(conflicts) != 1 || conflicts[0].A != 5 || conflicts[0].B != 7 || !slices.Equal(conflicts[0].Files, []string{"go.mod", "go.sum"}) {
		t.Fatalf("conflicting pairs = %+v", conflicts)
	}
	if p.Order[len(p.Order)-1] != 5 {
		t.Fatalf("order = %v, want #5 last", p.Order)
	}
	if !slices.ContainsFunc(s.PRs, func(pr scenario.PR) bool { return pr.Number == 7 && pr.Author.Kind == pull.Bot }) {
		t.Fatal("the scenario no longer has the clockwork bump as #7")
	}
}
