package plan

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/aalvaropc/upkeep/internal/gitx"
	"github.com/aalvaropc/upkeep/internal/pull"
)

type Clone struct {
	PR  pull.Request
	Dir string
}

type Planner struct {
	Git gitx.Git
}

func (p Planner) Plan(ctx context.Context, base string, clones []Clone) (Plan, error) {
	prs := make([]pull.Request, 0, len(clones))
	touched := map[int][]string{}
	heads := map[int]string{}
	for _, c := range clones {
		n := c.PR.Number
		files, err := p.Git.Touched(ctx, c.Dir, base, "HEAD")
		if err != nil {
			return Plan{}, fmt.Errorf("listing the files #%d changes: %w", n, err)
		}
		head, err := p.Git.Head(ctx, c.Dir)
		if err != nil {
			return Plan{}, fmt.Errorf("reading the head of #%d: %w", n, err)
		}
		prs = append(prs, c.PR)
		touched[n] = files
		heads[n] = head
	}
	conflicts := map[Pair][]string{}
	for i, a := range clones {
		for _, b := range clones[i+1:] {
			if len(shared(touched[a.PR.Number], touched[b.PR.Number])) == 0 {
				continue
			}
			clean, files, err := p.Git.MergeTree(ctx, b.Dir, heads[a.PR.Number], heads[b.PR.Number])
			if err != nil {
				return Plan{}, fmt.Errorf("merging #%d with #%d: %w", a.PR.Number, b.PR.Number, err)
			}
			if !clean {
				conflicts[PairOf(a.PR.Number, b.PR.Number)] = files
			}
		}
	}
	return Build(prs, touched, conflicts), nil
}

func (p Planner) Stale(ctx context.Context, base string, clones []Clone, merged []int, current Plan) ([]Stale, error) {
	branch := strings.TrimPrefix(base, "origin/")
	out := []Stale{}
	for _, c := range clones {
		before, err := p.Git.RevParse(ctx, c.Dir, base)
		if err != nil {
			return out, fmt.Errorf("reading %s in #%d: %w", branch, c.PR.Number, err)
		}
		if err := p.Git.Fetch(ctx, c.Dir, "origin", branch); err != nil {
			return out, fmt.Errorf("fetching %s for #%d: %w", branch, c.PR.Number, err)
		}
		after, err := p.Git.RevParse(ctx, c.Dir, base)
		if err != nil {
			return out, fmt.Errorf("reading %s in #%d: %w", branch, c.PR.Number, err)
		}
		if after == before {
			continue
		}
		clean, files, err := p.Git.MergeTree(ctx, c.Dir, base, "HEAD")
		if err != nil {
			return out, fmt.Errorf("merging %s into #%d: %w", branch, c.PR.Number, err)
		}
		if !clean {
			out = append(out, Stale{PR: c.PR.Number, Because: because(branch, c.PR.Number, files, merged, current)})
		}
	}
	return out, nil
}

func because(branch string, number int, files []string, merged []int, current Plan) string {
	var conflicting, sharing []int
	for _, o := range current.Overlapping(number) {
		q := o.Other(number)
		if !slices.Contains(merged, q) {
			continue
		}
		switch {
		case o.Conflict:
			conflicting = append(conflicting, q)
		case len(shared(o.Files, files)) > 0:
			sharing = append(sharing, q)
		}
	}
	culprits := conflicting
	if len(culprits) == 0 {
		culprits = sharing
	}
	text := fmt.Sprintf("conflicts with %s, which moved during the run", branch)
	if len(culprits) > 0 {
		slices.Sort(culprits)
		names := make([]string, len(culprits))
		for i, q := range culprits {
			names[i] = fmt.Sprintf("#%d", q)
		}
		text = fmt.Sprintf("conflicts with %s after %s merged", branch, joinAnd(names))
	}
	if len(files) > 0 {
		text += ": " + strings.Join(files, ", ")
	}
	return text
}

func joinAnd(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}
