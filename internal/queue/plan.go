package queue

import (
	"context"

	"github.com/aalvaropc/upkeep/internal/events"
	"github.com/aalvaropc/upkeep/internal/plan"
	"github.com/aalvaropc/upkeep/internal/pull"
)

type Planner interface {
	Plan(ctx context.Context, base string, clones []plan.Clone) (plan.Plan, error)
	Stale(ctx context.Context, base string, clones []plan.Clone, merged []int, current plan.Plan) ([]plan.Stale, error)
}

func (r Runner) note(message string) {
	r.emit(events.Notice, 0, map[string]string{"message": message})
}

func (r Runner) advise(ctx context.Context, items []Item, prepared []bool) *plan.Plan {
	if r.Planner == nil || ctx.Err() != nil {
		return nil
	}
	var clones []plan.Clone
	for i, it := range items {
		if prepared[i] {
			clones = append(clones, plan.Clone{PR: it.PR, Dir: it.Dir})
		}
	}
	p, err := r.Planner.Plan(ctx, r.Base, clones)
	if err != nil {
		r.note("Upkeep could not plan the merge order, so the run goes on without it: " + err.Error())
		return nil
	}
	r.emit(events.RunPlan, 0, p)
	return &p
}

func (r Runner) recheck(ctx context.Context, items []Item, current *plan.Plan) {
	if current == nil || ctx.Err() != nil {
		return
	}
	var clones []plan.Clone
	var merged []int
	for _, it := range items {
		switch it.Outcome {
		case pull.Mergeable:
			clones = append(clones, plan.Clone{PR: it.PR, Dir: it.Dir})
		case pull.Merged:
			merged = append(merged, it.PR.Number)
		}
	}
	stale, err := r.Planner.Stale(ctx, r.Base, clones, merged, *current)
	if err != nil {
		r.note("Upkeep could not check the ready pull requests against the base branch again: " + err.Error())
		return
	}
	if stale == nil {
		stale = []plan.Stale{}
	}
	current.Stale = stale
	current.Checked = true
	r.emit(events.RunPlan, 0, *current)
}
