package queue_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/aalvaropc/upkeep/internal/confidence"
	"github.com/aalvaropc/upkeep/internal/engine"
	"github.com/aalvaropc/upkeep/internal/events"
	"github.com/aalvaropc/upkeep/internal/plan"
	"github.com/aalvaropc/upkeep/internal/pull"
)

type fixedPlan struct{ plan plan.Plan }

func (f fixedPlan) Plan(context.Context, string, []plan.Clone) (plan.Plan, error) {
	return f.plan, nil
}

func (f fixedPlan) Stale(context.Context, string, []plan.Clone, []int, plan.Plan) ([]plan.Stale, error) {
	return nil, nil
}

func TestAConflictWithAnEarlierMergeLowersTheConfidence(t *testing.T) {
	fixed := confidence.Assess(confidence.Input{Outcome: pull.Mergeable, Blocker: pull.Build, Attempts: 1, Files: 1, Lines: 4, MaxLines: 80})
	p := &scripted{t: t, results: map[int]engine.Result{
		1: {Outcome: pull.Mergeable, Confidence: fixed},
		2: {Outcome: pull.Mergeable, Confidence: fixed},
		3: {Outcome: pull.NeedsHuman, Reason: "a large conflict"},
	}}
	r, f, l := newRunner(t, p)
	r.Planner = fixedPlan{plan: plan.Plan{
		Order:    []int{1, 2, 3, 4},
		Overlaps: []plan.Overlap{{A: 2, B: 1, Files: []string{"go.mod", "go.sum"}, Conflict: true}},
	}}
	if _, err := r.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	levels := map[int]*confidence.Assessment{}
	for _, e := range l.kinds(events.PRDone) {
		var done struct {
			Confidence *confidence.Assessment `json:"confidence"`
		}
		if err := json.Unmarshal(e.Data, &done); err != nil {
			t.Fatal(err)
		}
		levels[e.PR] = done.Confidence
	}
	if levels[1] == nil || levels[1].Level != confidence.High {
		t.Fatalf("#1 merges first and keeps its level: %+v", levels[1])
	}
	if levels[2] == nil || levels[2].Level != confidence.Medium || !strings.Contains(strings.Join(levels[2].Reasons, "; "), "conflicts with #1 in go.mod, go.sum, which merges first") {
		t.Fatalf("#2 follows #1: %+v", levels[2])
	}
	if levels[3] != nil {
		t.Fatalf("a pull request handed to a person is not rated: %+v", levels[3])
	}
	c := comments(f, 2, "receipt")
	if len(c) != 1 || !strings.Contains(c[0].Body, "Merge confidence: medium (fixed on the first attempt; 4 lines in 1 file; conflicts with #1 in go.mod, go.sum, which merges first).") {
		t.Fatalf("receipt comment = %+v", c)
	}
}
