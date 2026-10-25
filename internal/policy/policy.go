package policy

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"strings"

	"github.com/aalvaropc/upkeep/internal/pull"
)

type AutoMerge string

const (
	AutoMergeBots AutoMerge = "bots"
	AutoMergeNone AutoMerge = "none"
)

const MergeMethod = "merge"

type Policy struct {
	Label            string
	Bots             []string
	AutoMerge        AutoMerge
	AutoMergeBumps   []pull.Level
	MergeMethod      string
	MaxFilesChanged  int
	MaxLinesChanged  int
	MaxConflictHunks int
	MaxHunkLines     int
	MaxAttempts      int
	MaxSeconds       int
	MaxTokens        int
}

func Default() Policy {
	return Policy{
		Label:            "upkeep",
		Bots:             []string{"dependabot[bot]", "renovate[bot]"},
		AutoMerge:        AutoMergeBots,
		AutoMergeBumps:   []pull.Level{pull.Patch, pull.Minor},
		MergeMethod:      MergeMethod,
		MaxFilesChanged:  5,
		MaxLinesChanged:  80,
		MaxConflictHunks: 3,
		MaxHunkLines:     20,
		MaxAttempts:      3,
		MaxSeconds:       180,
		MaxTokens:        80_000,
	}
}

var hardRules = map[string]string{
	"force_push":      "Upkeep never force-pushes",
	"merge_human_prs": "Upkeep never merges a pull request opened by a person",
	"edit_tests":      "a fix may never edit, delete or skip tests",
}

func Load(path string) (Policy, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Policy{}, err
	}
	defer f.Close()
	p, err := Parse(f)
	if err != nil {
		return Policy{}, fmt.Errorf("%s: %w", path, err)
	}
	return p, nil
}

func Parse(r io.Reader) (Policy, error) {
	values, err := parseTOML(r)
	if err != nil {
		return Policy{}, err
	}
	p := Default()
	ints := map[string]*int{
		"max_files_changed":  &p.MaxFilesChanged,
		"max_lines_changed":  &p.MaxLinesChanged,
		"max_conflict_hunks": &p.MaxConflictHunks,
		"max_hunk_lines":     &p.MaxHunkLines,
		"max_attempts":       &p.MaxAttempts,
		"max_seconds":        &p.MaxSeconds,
		"max_tokens":         &p.MaxTokens,
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b string) int { return values[a].line - values[b].line })
	for _, key := range keys {
		v := values[key]
		if why, ok := hardRules[key]; ok {
			return Policy{}, errorf(v.line, "%s is a hard rule and cannot be configured: %s", key, why)
		}
		want := stringValue
		switch key {
		case "label", "auto_merge", "merge_method":
		case "bots", "auto_merge_bumps":
			want = listValue
		default:
			if _, ok := ints[key]; !ok {
				return Policy{}, errorf(v.line, "unknown key %q", key)
			}
			want = intValue
		}
		if v.kind != want {
			return Policy{}, errorf(v.line, "%s must be %s", key, want)
		}
		switch key {
		case "label":
			p.Label = v.str
		case "auto_merge":
			p.AutoMerge = AutoMerge(v.str)
		case "merge_method":
			p.MergeMethod = v.str
		case "bots":
			p.Bots = v.list
		case "auto_merge_bumps":
			p.AutoMergeBumps = make([]pull.Level, len(v.list))
			for i, s := range v.list {
				p.AutoMergeBumps[i] = pull.Level(s)
			}
		default:
			if v.num < 0 || v.num > 1<<31-1 {
				return Policy{}, errorf(v.line, "%s is out of range", key)
			}
			*ints[key] = int(v.num)
		}
	}
	if err := p.Validate(); err != nil {
		return Policy{}, err
	}
	return p, nil
}

func (p Policy) Validate() error {
	var errs []error
	if strings.TrimSpace(p.Label) == "" {
		errs = append(errs, errors.New("label must not be empty"))
	}
	switch p.AutoMerge {
	case AutoMergeBots, AutoMergeNone:
	default:
		errs = append(errs, fmt.Errorf("auto_merge %q must be %q or %q", p.AutoMerge, AutoMergeBots, AutoMergeNone))
	}
	if p.MergeMethod != MergeMethod {
		errs = append(errs, fmt.Errorf("merge_method %q is not allowed: Upkeep only merges with a merge commit", p.MergeMethod))
	}
	for _, level := range p.AutoMergeBumps {
		switch level {
		case pull.Patch, pull.Minor:
		case pull.Major:
			errs = append(errs, errors.New("auto_merge_bumps cannot include major: majors are always left to a person"))
		default:
			errs = append(errs, fmt.Errorf("auto_merge_bumps: unknown level %q", level))
		}
	}
	for _, l := range []struct {
		name  string
		value int
	}{
		{"max_files_changed", p.MaxFilesChanged},
		{"max_lines_changed", p.MaxLinesChanged},
		{"max_conflict_hunks", p.MaxConflictHunks},
		{"max_hunk_lines", p.MaxHunkLines},
		{"max_attempts", p.MaxAttempts},
		{"max_seconds", p.MaxSeconds},
		{"max_tokens", p.MaxTokens},
	} {
		if l.value <= 0 {
			errs = append(errs, fmt.Errorf("%s must be greater than zero", l.name))
		}
	}
	return errors.Join(errs...)
}

func (p Policy) IsBot(login, accountType string) bool {
	if accountType == "Bot" {
		return true
	}
	for _, b := range p.Bots {
		if strings.EqualFold(b, login) {
			return true
		}
	}
	return false
}

func (p Policy) MayAutoMerge(author pull.Author, bump *pull.Bump) bool {
	if author.Kind != pull.Bot || p.AutoMerge != AutoMergeBots || bump == nil || bump.Level == pull.Major {
		return false
	}
	return slices.Contains(p.AutoMergeBumps, bump.Level)
}

func (p Policy) MayPush(pr pull.Request) bool {
	return pr.Author.Kind == pull.Bot || pr.HasLabel(p.Label)
}

func (p Policy) Rules() []string {
	merge := "Never merges anything on its own."
	if p.AutoMerge == AutoMergeBots && len(p.AutoMergeBumps) > 0 {
		levels := make([]string, len(p.AutoMergeBumps))
		for i, l := range p.AutoMergeBumps {
			levels[i] = string(l)
		}
		merge = fmt.Sprintf("Merges a bot's dependency bump on its own only when it is a %s bump and every check passes; majors wait for a person.", strings.Join(levels, " or "))
	}
	return []string{
		"Never force-pushes: main is merged into the branch and history is kept.",
		"Never merges a pull request opened by a person.",
		fmt.Sprintf("Pushes fixes to a person's branch only when the pull request has the %q label; otherwise it attaches the fix as a patch.", p.Label),
		"A fix may never edit, delete or skip tests.",
		"Build, vet and tests must pass after every change, or nothing is pushed.",
		merge,
		fmt.Sprintf("A fix changes at most %d files and %d lines.", p.MaxFilesChanged, p.MaxLinesChanged),
		fmt.Sprintf("Resolves a conflict on its own only when it has at most %d hunks of at most %d lines each.", p.MaxConflictHunks, p.MaxHunkLines),
		fmt.Sprintf("Stops after %d attempts, %d seconds or %d tokens per pull request.", p.MaxAttempts, p.MaxSeconds, p.MaxTokens),
	}
}
