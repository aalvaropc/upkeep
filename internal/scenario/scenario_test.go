package scenario

import (
	"strings"
	"testing"

	"github.com/aalvaropc/upkeep/internal/pull"
)

const valid = `{
  "name": "sample",
  "repository": "upkeep-sample",
  "base": "main",
  "prs": [
    {
      "number": 1,
      "title": "Add note search",
      "branch": "feat/search",
      "author": {"login": "alice", "kind": "person"},
      "recipe": "behind-main",
      "expect": {"blocker": "behind", "outcome": "mergeable"}
    },
    {
      "number": 6,
      "title": "Bump a module",
      "branch": "dependabot/go_modules/example",
      "author": {"login": "dependabot[bot]", "kind": "bot"},
      "bump": {"module": "example.com/m", "from": "v1.2.3", "to": "v1.2.4", "level": "patch"},
      "recipe": "patch-bump",
      "expect": {"blocker": "clean", "outcome": "merged", "pushed": false}
    }
  ]
}`

func TestLoadValid(t *testing.T) {
	s, err := Load(strings.NewReader(valid))
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "sample" || len(s.PRs) != 2 {
		t.Fatalf("scenario = %+v", s)
	}
	bump := s.PRs[1]
	if bump.Author.Kind != pull.Bot || bump.Bump == nil || bump.Bump.Level != pull.Patch || bump.Expect.Outcome != pull.Merged {
		t.Fatalf("second pull request = %+v", bump)
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	src := strings.Replace(valid, `"name": "sample",`, `"name": "sample", "colour": "red",`, 1)
	if _, err := Load(strings.NewReader(src)); err == nil || !strings.Contains(err.Error(), "colour") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidate(t *testing.T) {
	base, err := Load(strings.NewReader(valid))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*Scenario)
		want   string
	}{
		{"no name", func(s *Scenario) { s.Name = " " }, "name is required"},
		{"no base", func(s *Scenario) { s.Base = "" }, "base branch is required"},
		{"no pull requests", func(s *Scenario) { s.PRs = nil }, "at least one"},
		{"duplicate number", func(s *Scenario) { s.PRs[1].Number = 1 }, "number is used twice"},
		{"zero number", func(s *Scenario) { s.PRs[0].Number = 0 }, "must be positive"},
		{"duplicate branch", func(s *Scenario) { s.PRs[1].Branch = "feat/search" }, "is used twice"},
		{"base as branch", func(s *Scenario) { s.PRs[0].Branch = "main" }, "needs its own branch"},
		{"no title", func(s *Scenario) { s.PRs[0].Title = "" }, "title is required"},
		{"bad author", func(s *Scenario) { s.PRs[0].Author.Kind = "robot" }, "kind of person or bot"},
		{"no recipe", func(s *Scenario) { s.PRs[0].Recipe = "" }, "recipe is required"},
		{"bad blocker", func(s *Scenario) { s.PRs[0].Expect.Blocker = "stuck" }, "unknown expected blocker"},
		{"bad outcome", func(s *Scenario) { s.PRs[0].Expect.Outcome = "shipped" }, "unknown expected outcome"},
		{"pushed and patch", func(s *Scenario) { s.PRs[0].Expect.Pushed, s.PRs[0].Expect.Patch = true, true }, "not both"},
		{"person merged", func(s *Scenario) { s.PRs[0].Expect.Outcome = pull.Merged }, "only a bot's pull request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := base
			s.PRs = append([]PR(nil), base.PRs...)
			tt.mutate(&s)
			if err := s.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}
