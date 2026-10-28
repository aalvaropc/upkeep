package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/aalvaropc/upkeep/internal/plan"
	"github.com/aalvaropc/upkeep/internal/queue"
)

func TestTheSummaryPrintsTheMergePlan(t *testing.T) {
	p := &plan.Plan{
		Order:    []int{7, 5},
		Overlaps: []plan.Overlap{{A: 5, B: 7, Files: []string{"go.mod", "go.sum"}, Conflict: true}},
		Reasons:  map[int]string{7: "A bot's minor version bump.", 5: "Conflicts with #7 in go.mod, go.sum."},
		Stale:    []plan.Stale{{PR: 5, Because: "conflicts with main after #7 merged: go.mod, go.sum"}},
		Checked:  true,
	}
	var out bytes.Buffer
	printSummary(&out, queue.Summary{Plan: p})
	for _, want := range []string{
		"merge plan: #7, #5\n",
		"  1. #7    A bot's minor version bump.\n",
		"  2. #5    Conflicts with #7 in go.mod, go.sum.\n",
		"  #5 and #7 both change go.mod, go.sum; they conflict\n",
		"no longer ready to merge after the run:\n  #5 conflicts with main after #7 merged: go.mod, go.sum\n",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("summary:\n%s\nwant it to contain %q", out.String(), want)
		}
	}

	p.Stale = nil
	out.Reset()
	printSummary(&out, queue.Summary{Plan: p})
	if !strings.Contains(out.String(), "still merges into the base branch after the run") {
		t.Errorf("summary without stale requests:\n%s", out.String())
	}

	out.Reset()
	printSummary(&out, queue.Summary{})
	if strings.Contains(out.String(), "merge plan") {
		t.Errorf("a summary without a plan printed one:\n%s", out.String())
	}
}
