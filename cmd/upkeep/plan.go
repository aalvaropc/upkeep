package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/aalvaropc/upkeep/internal/plan"
)

func printPlan(w io.Writer, p *plan.Plan) {
	if p == nil {
		return
	}
	order := make([]string, len(p.Order))
	for i, n := range p.Order {
		order[i] = fmt.Sprintf("#%d", n)
	}
	fmt.Fprintf(w, "\nmerge plan: %s\n", strings.Join(order, ", "))
	for i, n := range p.Order {
		fmt.Fprintf(w, "  %d. #%-4d %s\n", i+1, n, p.Reasons[n])
	}
	if len(p.Overlaps) > 0 {
		fmt.Fprintln(w, "shared files:")
	}
	for _, o := range p.Overlaps {
		verdict := "they merge cleanly together"
		if o.Conflict {
			verdict = "they conflict"
		}
		fmt.Fprintf(w, "  #%d and #%d both change %s; %s\n", o.A, o.B, strings.Join(o.Files, ", "), verdict)
	}
	switch {
	case len(p.Stale) > 0:
		fmt.Fprintln(w, "no longer ready to merge after the run:")
		for _, s := range p.Stale {
			fmt.Fprintf(w, "  #%d %s\n", s.PR, s.Because)
		}
	case p.Checked:
		fmt.Fprintln(w, "every pull request that is ready to merge still merges into the base branch after the run")
	}
}
