package queue_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/aalvaropc/upkeep/internal/engine"
	"github.com/aalvaropc/upkeep/internal/events"
	"github.com/aalvaropc/upkeep/internal/gitx"
	"github.com/aalvaropc/upkeep/internal/plan"
	"github.com/aalvaropc/upkeep/internal/pull"
)

type stepping struct {
	*scripted
	log *log
}

func (s stepping) Process(ctx context.Context, in engine.Input) engine.Result {
	s.log.emit(events.PRStep, in.PR.Number, map[string]string{"step": "detect", "status": "running"})
	return s.scripted.Process(ctx, in)
}

type broken struct {
	plan  error
	stale error
}

func (b broken) Plan(ctx context.Context, base string, clones []plan.Clone) (plan.Plan, error) {
	if b.plan != nil {
		return plan.Plan{}, b.plan
	}
	return plan.Planner{Git: gitx.Git{}}.Plan(ctx, base, clones)
}

func (b broken) Stale(context.Context, string, []plan.Clone, []int, plan.Plan) ([]plan.Stale, error) {
	return nil, b.stale
}

func (l *log) index(kind events.Kind) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.IndexFunc(l.events, func(e events.Event) bool { return e.Kind == kind })
}

func decodePlan(t *testing.T, e events.Event) plan.Plan {
	t.Helper()
	var p plan.Plan
	if err := json.Unmarshal(e.Data, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestThePlanIsAnnouncedBeforeTheFirstStep(t *testing.T) {
	p := &scripted{t: t, results: map[int]engine.Result{}}
	r, _, l := newRunner(t, p)
	r.Processor = stepping{scripted: p, log: l}
	r.Planner = plan.Planner{Git: gitx.Git{}}
	s, err := r.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	started, planned, stepped := l.index(events.RunStarted), l.index(events.RunPlan), l.index(events.PRStep)
	if started < 0 || planned < 0 || stepped < 0 || started >= planned || planned >= stepped {
		t.Fatalf("run.started at %d, run.plan at %d, the first pr.step at %d", started, planned, stepped)
	}
	first := decodePlan(t, l.kinds(events.RunPlan)[0])
	if !slices.Equal(first.Order, []int{4, 2, 1, 3}) || len(first.Overlaps) != 1 || !first.Overlaps[0].Conflict || first.Checked {
		t.Fatalf("plan = %+v", first)
	}
	if s.Plan == nil || !slices.Equal(s.Plan.Order, first.Order) {
		t.Fatalf("summary plan = %+v", s.Plan)
	}
}

func TestTheWorkStillStartsInNumberOrderWithAPlan(t *testing.T) {
	p := &scripted{t: t, results: map[int]engine.Result{}}
	r, _, _ := newRunner(t, p)
	r.Parallel = 1
	r.Planner = plan.Planner{Git: gitx.Git{}}
	if _, err := r.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := p.started; !slices.Equal(got, []int{1, 2, 3, 4}) {
		t.Fatalf("started in order %v", got)
	}
}

func TestAPlannerErrorStillLetsTheRunFinish(t *testing.T) {
	p := &scripted{t: t, results: map[int]engine.Result{1: {Outcome: pull.Mergeable}, 2: {Outcome: pull.Mergeable}}}
	r, _, l := newRunner(t, p)
	r.Planner = broken{plan: errors.New("merge-tree is missing")}
	s, err := r.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Items) != 4 || len(l.kinds(events.PRDone)) != 4 || len(l.kinds(events.RunFinished)) != 1 {
		t.Fatalf("%d items, %d pr.done events", len(s.Items), len(l.kinds(events.PRDone)))
	}
	if s.Plan != nil || len(l.kinds(events.RunPlan)) != 0 {
		t.Fatalf("a failed plan was announced: %+v", s.Plan)
	}
	notices := l.kinds(events.Notice)
	if len(notices) != 1 || !strings.Contains(string(notices[0].Data), "merge-tree is missing") {
		t.Fatalf("notices = %+v", notices)
	}
}

func TestAFailedRecheckKeepsThePlanAndFinishesTheRun(t *testing.T) {
	p := &scripted{t: t, results: map[int]engine.Result{1: {Outcome: pull.Mergeable}}}
	r, _, l := newRunner(t, p)
	r.Planner = broken{stale: errors.New("the fetch failed")}
	s, err := r.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if s.Plan == nil || s.Plan.Checked || len(l.kinds(events.RunPlan)) != 1 || len(l.kinds(events.RunFinished)) != 1 {
		t.Fatalf("plan = %+v, %d run.plan events", s.Plan, len(l.kinds(events.RunPlan)))
	}
	if notices := l.kinds(events.Notice); len(notices) != 1 || !strings.Contains(string(notices[0].Data), "the fetch failed") {
		t.Fatalf("notices = %+v", notices)
	}
}

func TestARequestMadeStaleByAMergeIsReportedAfterTheRun(t *testing.T) {
	p := &scripted{t: t, results: map[int]engine.Result{
		1: {Outcome: pull.Mergeable, WantsMerge: true},
		2: {Outcome: pull.Mergeable},
		3: {Outcome: pull.Mergeable},
	}}
	r, _, l := newRunner(t, p)
	r.Planner = plan.Planner{Git: gitx.Git{}}
	s, err := r.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got := item(t, s, 1); got.Outcome != pull.Merged {
		t.Fatalf("#1 = %+v", got)
	}
	plans := l.kinds(events.RunPlan)
	if len(plans) != 2 {
		t.Fatalf("%d run.plan events, want 2", len(plans))
	}
	last := decodePlan(t, plans[1])
	want := []plan.Stale{{PR: 3, Because: "conflicts with main after #1 merged: a.txt"}}
	if !last.Checked || !slices.Equal(last.Stale, want) || !slices.Equal(last.Order, []int{4, 2, 1, 3}) {
		t.Fatalf("the plan after the run = %+v", last)
	}
	if s.Plan == nil || !slices.Equal(s.Plan.Stale, want) {
		t.Fatalf("summary plan = %+v", s.Plan)
	}
	if got := item(t, s, 3); got.Outcome != pull.Mergeable {
		t.Fatalf("the check changed the outcome of #3: %+v", got)
	}
	if l.index(events.RunFinished) < l.indexLast(events.RunPlan) || l.indexLast(events.RunPlan) < l.indexLast(events.PRDone) {
		t.Fatal("the checked plan was not announced between the last pr.done and run.finished")
	}
}

func (l *log) indexLast(kind events.Kind) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := len(l.events) - 1; i >= 0; i-- {
		if l.events[i].Kind == kind {
			return i
		}
	}
	return -1
}
