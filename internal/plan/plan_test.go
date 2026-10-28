package plan_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/aalvaropc/upkeep/internal/plan"
	"github.com/aalvaropc/upkeep/internal/pull"
)

func person(number int) pull.Request {
	return pull.Request{Number: number, Title: "change", Author: pull.Author{Login: "alice", Kind: pull.Person}, Kind: pull.Change}
}

func bot(number int, level pull.Level) pull.Request {
	return pull.Request{
		Number: number, Title: "bump", Author: pull.Author{Login: "dependabot[bot]", Kind: pull.Bot}, Kind: pull.Dependency,
		Bump: &pull.Bump{Module: "example.com/m", From: "v1.0.0", To: "v1.1.0", Level: level},
	}
}

func TestBuild(t *testing.T) {
	tests := []struct {
		name      string
		prs       []pull.Request
		touched   map[int][]string
		conflicts map[plan.Pair][]string
		order     []int
		overlaps  []plan.Overlap
		reasons   map[int]string
	}{
		{
			name:    "nothing to plan",
			order:   []int{},
			reasons: map[int]string{},
		},
		{
			name: "small bot bumps go first, then requests that share nothing, then the conflicting ones",
			prs:  []pull.Request{bot(7, pull.Minor), person(2), bot(5, pull.Major), person(1), bot(6, pull.Patch)},
			touched: map[int][]string{
				1: {"notes/search.go"},
				2: {"notes/limits.go"},
				5: {"auth/token.go", "go.mod", "go.sum"},
				6: {"go.mod", "go.sum"},
				7: {"go.sum", "go.mod"},
			},
			conflicts: map[plan.Pair][]string{plan.PairOf(7, 5): {"go.mod", "go.sum"}},
			order:     []int{6, 7, 1, 2, 5},
			overlaps: []plan.Overlap{
				{A: 5, B: 6, Files: []string{"go.mod", "go.sum"}},
				{A: 5, B: 7, Files: []string{"go.mod", "go.sum"}, Conflict: true},
				{A: 6, B: 7, Files: []string{"go.mod", "go.sum"}},
			},
			reasons: map[int]string{
				1: "Shares no files with another open pull request, so it can merge in any order.",
				2: "Shares no files with another open pull request, so it can merge in any order.",
				5: "Conflicts with #7 in go.mod, go.sum: merge it after #7 and update its branch.",
				6: "A bot's patch version bump: small and low risk, so it goes first.",
				7: "A bot's minor version bump: small and low risk, so it goes first. Conflicts with #5 in go.mod, go.sum, so #5 updates its branch after this one merges.",
			},
		},
		{
			name:      "the smaller of two conflicting changes goes first",
			prs:       []pull.Request{person(3), person(4)},
			touched:   map[int][]string{3: {"x.go", "y.go", "z.go"}, 4: {"x.go"}},
			conflicts: map[plan.Pair][]string{plan.PairOf(3, 4): {"x.go"}},
			order:     []int{4, 3},
			overlaps:  []plan.Overlap{{A: 3, B: 4, Files: []string{"x.go"}, Conflict: true}},
			reasons: map[int]string{
				3: "Conflicts with #4 in x.go: merge it after #4 and update its branch.",
				4: "Conflicts with #3 in x.go: it is the smaller change, so it goes first and #3 updates its branch after.",
			},
		},
		{
			name:      "of two conflicting changes of the same size the older one goes first",
			prs:       []pull.Request{person(9), person(8)},
			touched:   map[int][]string{8: {"x.go"}, 9: {"x.go"}},
			conflicts: map[plan.Pair][]string{plan.PairOf(8, 9): nil},
			order:     []int{8, 9},
			overlaps:  []plan.Overlap{{A: 8, B: 9, Files: []string{"x.go"}, Conflict: true}},
			reasons: map[int]string{
				8: "Conflicts with #9 in x.go: it was opened first, so it goes first and #9 updates its branch after.",
				9: "Conflicts with #8 in x.go: merge it after #8 and update its branch.",
			},
		},
		{
			name:     "requests that share files but merge cleanly come after the ones that share nothing",
			prs:      []pull.Request{person(1), person(2), person(3), bot(4, pull.Major)},
			touched:  map[int][]string{1: {"a.go", "shared.go"}, 2: {"shared.go"}, 3: {"c.go"}, 4: {"go.mod"}},
			order:    []int{3, 4, 1, 2},
			overlaps: []plan.Overlap{{A: 1, B: 2, Files: []string{"shared.go"}}},
			reasons: map[int]string{
				1: "Shares shared.go with #2, but the two merge cleanly in either order.",
				2: "Shares shared.go with #1, but the two merge cleanly in either order.",
				3: "Shares no files with another open pull request, so it can merge in any order.",
				4: "Shares no files with another open pull request, so it can merge in any order.",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := plan.Build(tt.prs, tt.touched, tt.conflicts)
			if !slices.Equal(got.Order, tt.order) {
				t.Errorf("order = %v, want %v", got.Order, tt.order)
			}
			if !slices.EqualFunc(got.Overlaps, tt.overlaps, func(a, b plan.Overlap) bool {
				return a.A == b.A && a.B == b.B && a.Conflict == b.Conflict && slices.Equal(a.Files, b.Files)
			}) {
				t.Errorf("overlaps = %+v, want %+v", got.Overlaps, tt.overlaps)
			}
			if len(got.Reasons) != len(tt.reasons) {
				t.Errorf("reasons = %v, want %v", got.Reasons, tt.reasons)
			}
			for n, want := range tt.reasons {
				if got.Reasons[n] != want {
					t.Errorf("reason for #%d = %q, want %q", n, got.Reasons[n], want)
				}
			}
			if got.Stale == nil || len(got.Stale) != 0 || got.Checked {
				t.Errorf("a new plan has stale = %v, checked = %v", got.Stale, got.Checked)
			}
		})
	}
}

func TestAPlanEncodesForTheDashboard(t *testing.T) {
	p := plan.Build(
		[]pull.Request{person(5), person(7)},
		map[int][]string{5: {"go.mod"}, 7: {"go.mod"}},
		map[plan.Pair][]string{plan.PairOf(5, 7): {"go.mod"}},
	)
	p.Stale = []plan.Stale{{PR: 7, Because: "conflicts with main after #5 merged: go.mod"}}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"order":[5,7]`, `"overlaps":[{"a":5,"b":7,"files":["go.mod"],"conflict":true}]`, `"reasons":{"5":`, `"stale":[{"pr":7,"because":"conflicts with main after #5 merged: go.mod"}]`, `"checked":false`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("%s does not contain %s", data, want)
		}
	}
	empty, err := json.Marshal(plan.Build(nil, nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(empty), `"order":[]`) || !strings.Contains(string(empty), `"overlaps":[]`) || !strings.Contains(string(empty), `"stale":[]`) {
		t.Fatalf("an empty plan encodes as %s", empty)
	}
}
