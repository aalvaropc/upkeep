package confidence

import (
	"slices"
	"strings"
	"testing"

	"github.com/aalvaropc/upkeep/internal/pull"
)

func TestAssess(t *testing.T) {
	major := &pull.Bump{Module: "github.com/golang-jwt/jwt/v5", From: "v4.5.2", To: "v5.3.1", Level: pull.Major}
	minor := &pull.Bump{Module: "github.com/jonboulle/clockwork", From: "v0.4.0", To: "v0.5.0", Level: pull.Minor}
	cases := []struct {
		name    string
		in      Input
		level   Level
		reasons []string
	}{
		{
			name:    "a model fix with the pull request's own tests",
			in:      Input{Outcome: pull.Mergeable, Blocker: pull.Build, Attempts: 1, Files: 1, Lines: 6, MaxLines: 80, Change: true, OwnTests: []string{"TestA", "TestB", "TestC", "TestD"}},
			level:   High,
			reasons: []string{"fixed on the first attempt", "6 lines in 1 file", "the tests it adds pass: TestA, TestB, TestC and 1 more"},
		},
		{
			name:    "a model fix of a change without tests of its own",
			in:      Input{Outcome: pull.Mergeable, Blocker: pull.Build, Attempts: 1, Files: 1, Lines: 6, MaxLines: 80, Change: true},
			level:   High,
			reasons: []string{"fixed on the first attempt", "6 lines in 1 file", "it adds no test of its own"},
		},
		{
			name:    "an order-dependent test",
			in:      Input{Outcome: pull.Mergeable, Blocker: pull.Flaky, Flaky: []string{"TestSummary"}, FlakyKind: "order-dependent"},
			level:   Medium,
			reasons: []string{"no code written by a model", "a test that depends on the order tests run in: TestSummary"},
		},
		{
			name:    "a forecast whose build passed",
			in:      Input{Outcome: pull.Mergeable, Blocker: pull.Clean, Bump: minor, Predicted: 2, BuildFine: true},
			level:   High,
			reasons: []string{"no code written by a model", "the build passed, so none of the 2 predicted changes broke it"},
		},
		{
			name:    "a forecast the fix confirmed further",
			in:      Input{Outcome: pull.Mergeable, Blocker: pull.Build, Bump: minor, Attempts: 1, Files: 1, Lines: 4, MaxLines: 80, Predicted: 3, Confirmed: 1, FixedToo: 1},
			level:   High,
			reasons: []string{"fixed on the first attempt", "4 lines in 1 file", "the build confirmed 1 of 3 predicted changes, and the fix changed 1 more"},
		},
		{
			name:    "a merge of main",
			in:      Input{Outcome: pull.Mergeable, Blocker: pull.Behind, Files: 4, Lines: 120, MaxLines: 80},
			level:   High,
			reasons: []string{"no code written by a model"},
		},
		{
			name:    "a small conflict",
			in:      Input{Outcome: pull.Mergeable, Blocker: pull.ConflictSmall, Files: 1, Lines: 2, MaxLines: 80},
			level:   High,
			reasons: []string{"a model resolved a small conflict"},
		},
		{
			name:    "a minor bump fixed at once",
			in:      Input{Outcome: pull.Merged, Blocker: pull.Build, Bump: minor, Attempts: 1, Files: 1, Lines: 4, MaxLines: 80, Predicted: 1, Confirmed: 1},
			level:   High,
			reasons: []string{"fixed on the first attempt", "4 lines in 1 file", "the build confirmed 1 of 1 predicted changes"},
		},
		{
			name:    "a major bump",
			in:      Input{Outcome: pull.Mergeable, Blocker: pull.Build, Bump: major, Attempts: 1, Files: 1, Lines: 12, MaxLines: 80, Predicted: 4, Confirmed: 1},
			level:   Medium,
			reasons: []string{"fixed on the first attempt", "12 lines in 1 file", "a major upgrade", "the build confirmed 1 of 4 predicted changes"},
		},
		{
			name:    "a third attempt close to the limit",
			in:      Input{Outcome: pull.Mergeable, Blocker: pull.Build, Attempts: 3, Files: 3, Lines: 60, MaxLines: 80},
			level:   Low,
			reasons: []string{"fixed on attempt 3", "60 lines in 3 files", "more than half of the 80 lines a fix may change"},
		},
		{
			name:    "a flaky test",
			in:      Input{Outcome: pull.Mergeable, Blocker: pull.Format, Flaky: []string{"TestSummaryOfAnOwner"}},
			level:   Medium,
			reasons: []string{"no code written by a model", "a flaky test: TestSummaryOfAnOwner"},
		},
		{
			name:    "a learned fix",
			in:      Input{Outcome: pull.Mergeable, Blocker: pull.Build, Learned: 5, Files: 1, Lines: 4, MaxLines: 80},
			level:   High,
			reasons: []string{"reuses the verified fix learned from #5", "4 lines in 1 file"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Assess(c.in)
			if got.Level != c.level || !slices.Equal(got.Reasons, c.reasons) {
				t.Fatalf("Assess() = %s %q, want %s %q", got.Level, got.Reasons, c.level, c.reasons)
			}
		})
	}
}

func TestOnlyReadyPullRequestsAreRated(t *testing.T) {
	for _, outcome := range []pull.Outcome{pull.NeedsHuman, pull.Failed, pull.Pending} {
		if got := Assess(Input{Outcome: outcome, Attempts: 1}); got.Known() {
			t.Errorf("%q was rated %+v", outcome, got)
		}
	}
}

func TestAConflictWithAnEarlierMergeLowersTheLevel(t *testing.T) {
	base := Assess(Input{Outcome: pull.Mergeable, Blocker: pull.Build, Attempts: 1, Files: 1, Lines: 4, MaxLines: 80})
	after := After(base, 7, []string{"go.mod", "go.sum"})
	if base.Level != High || after.Level != Medium {
		t.Fatalf("levels = %s then %s", base.Level, after.Level)
	}
	if len(base.Reasons) != 2 || !strings.Contains(after.Reasons[len(after.Reasons)-1], "conflicts with #7 in go.mod, go.sum") {
		t.Fatalf("reasons = %q then %q", base.Reasons, after.Reasons)
	}
	if got := After(Assessment{}, 7, nil); got.Known() {
		t.Fatalf("an unrated pull request was rated after a conflict: %+v", got)
	}
}
