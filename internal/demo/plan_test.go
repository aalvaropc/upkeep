package demo_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/aalvaropc/upkeep/internal/demo"
	"github.com/aalvaropc/upkeep/internal/engine"
	"github.com/aalvaropc/upkeep/internal/events"
	"github.com/aalvaropc/upkeep/internal/plan"
	"github.com/aalvaropc/upkeep/internal/pull"
)

func TestARunPlansTheMergeOrderAndRechecksItAfterTheMerges(t *testing.T) {
	p := &processor{results: map[int]engine.Result{
		1: {Outcome: pull.Mergeable, WantsMerge: true},
		3: {Outcome: pull.Mergeable},
	}}
	h := newHarness(t, nil, p)
	s := h.session()
	if _, err := h.m.Start(s.ID); err != nil {
		t.Fatal(err)
	}
	h.waitStatus(s, demo.Finished)
	plans := h.kinds(s, events.RunPlan)
	if len(plans) != 2 {
		t.Fatalf("%d run.plan events, want 2", len(plans))
	}
	var first, last plan.Plan
	if err := json.Unmarshal(plans[0].Data, &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(plans[1].Data, &last); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(first.Order, []int{4, 2, 1, 3}) || len(first.Overlaps) != 1 || !first.Overlaps[0].Conflict || plans[0].Run == "" {
		t.Fatalf("first plan = %+v", first)
	}
	if !last.Checked || len(last.Stale) != 1 || last.Stale[0].PR != 3 || last.Stale[0].Because != "conflicts with main after #1 merged: a.txt" {
		t.Fatalf("plan after the run = %+v", last)
	}
}
