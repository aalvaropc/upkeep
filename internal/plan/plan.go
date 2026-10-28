package plan

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/aalvaropc/upkeep/internal/pull"
)

type Overlap struct {
	A        int      `json:"a"`
	B        int      `json:"b"`
	Files    []string `json:"files"`
	Conflict bool     `json:"conflict"`
}

type Stale struct {
	PR      int    `json:"pr"`
	Because string `json:"because"`
}

type Plan struct {
	Order    []int          `json:"order"`
	Overlaps []Overlap      `json:"overlaps"`
	Reasons  map[int]string `json:"reasons"`
	Stale    []Stale        `json:"stale"`
	Checked  bool           `json:"checked"`
}

type Pair struct {
	A int
	B int
}

func PairOf(a, b int) Pair {
	if a > b {
		a, b = b, a
	}
	return Pair{A: a, B: b}
}

type group int

const (
	safeBump group = iota
	alone
	sharesCleanly
	conflicting
)

func shared(a, b []string) []string {
	var out []string
	for _, f := range a {
		if slices.Contains(b, f) && !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	slices.Sort(out)
	return out
}

func isSafeBump(pr pull.Request) bool {
	return pr.Author.Kind == pull.Bot && pr.Bump != nil && (pr.Bump.Level == pull.Patch || pr.Bump.Level == pull.Minor)
}

func (p Plan) Overlapping(number int) []Overlap {
	var out []Overlap
	for _, o := range p.Overlaps {
		if o.A == number || o.B == number {
			out = append(out, o)
		}
	}
	return out
}

func (o Overlap) Other(number int) int {
	if o.A == number {
		return o.B
	}
	return o.A
}

func Build(prs []pull.Request, touched map[int][]string, conflicts map[Pair][]string) Plan {
	sorted := slices.Clone(prs)
	slices.SortFunc(sorted, func(a, b pull.Request) int { return cmp.Compare(a.Number, b.Number) })
	p := Plan{Order: []int{}, Overlaps: []Overlap{}, Reasons: map[int]string{}, Stale: []Stale{}}
	clashes := map[int][]int{}
	shares := map[int][]int{}
	for i, a := range sorted {
		for _, b := range sorted[i+1:] {
			files := shared(touched[a.Number], touched[b.Number])
			if len(files) == 0 {
				continue
			}
			_, conflict := conflicts[PairOf(a.Number, b.Number)]
			p.Overlaps = append(p.Overlaps, Overlap{A: a.Number, B: b.Number, Files: files, Conflict: conflict})
			if conflict {
				clashes[a.Number] = append(clashes[a.Number], b.Number)
				clashes[b.Number] = append(clashes[b.Number], a.Number)
			} else {
				shares[a.Number] = append(shares[a.Number], b.Number)
				shares[b.Number] = append(shares[b.Number], a.Number)
			}
		}
	}
	groupOf := func(pr pull.Request) group {
		switch {
		case isSafeBump(pr):
			return safeBump
		case len(clashes[pr.Number]) > 0:
			return conflicting
		case len(shares[pr.Number]) > 0:
			return sharesCleanly
		}
		return alone
	}
	size := func(pr pull.Request) int { return len(touched[pr.Number]) }
	slices.SortStableFunc(sorted, func(a, b pull.Request) int {
		ga, gb := groupOf(a), groupOf(b)
		if ga != gb {
			return cmp.Compare(ga, gb)
		}
		if ga == safeBump || ga == conflicting {
			if c := cmp.Compare(size(a), size(b)); c != 0 {
				return c
			}
		}
		return cmp.Compare(a.Number, b.Number)
	})
	position := map[int]int{}
	sizes := map[int]int{}
	for i, pr := range sorted {
		p.Order = append(p.Order, pr.Number)
		position[pr.Number] = i
		sizes[pr.Number] = size(pr)
	}
	for _, pr := range sorted {
		n := pr.Number
		var parts []string
		switch groupOf(pr) {
		case safeBump:
			parts = append(parts, fmt.Sprintf("A bot's %s version bump: small and low risk, so it goes first.", pr.Bump.Level))
		case alone:
			parts = append(parts, "Shares no files with another open pull request, so it can merge in any order.")
		case sharesCleanly:
			for _, q := range shares[n] {
				files := shared(touched[n], touched[q])
				parts = append(parts, fmt.Sprintf("Shares %s with #%d, but the two merge cleanly in either order.", strings.Join(files, ", "), q))
			}
		}
		for _, q := range clashes[n] {
			files := conflicts[PairOf(n, q)]
			if len(files) == 0 {
				files = shared(touched[n], touched[q])
			}
			parts = append(parts, clash(n, q, files, position, sizes, groupOf(pr) == safeBump))
		}
		p.Reasons[n] = strings.Join(parts, " ")
	}
	return p
}

func clash(n, q int, files []string, position, sizes map[int]int, bump bool) string {
	list := strings.Join(files, ", ")
	switch {
	case position[q] < position[n]:
		return fmt.Sprintf("Conflicts with #%d in %s: merge it after #%d and update its branch.", q, list, q)
	case bump:
		return fmt.Sprintf("Conflicts with #%d in %s, so #%d updates its branch after this one merges.", q, list, q)
	case sizes[n] < sizes[q]:
		return fmt.Sprintf("Conflicts with #%d in %s: it is the smaller change, so it goes first and #%d updates its branch after.", q, list, q)
	}
	return fmt.Sprintf("Conflicts with #%d in %s: it was opened first, so it goes first and #%d updates its branch after.", q, list, q)
}
