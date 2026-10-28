package queue

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/aalvaropc/upkeep/internal/engine"
	"github.com/aalvaropc/upkeep/internal/events"
	"github.com/aalvaropc/upkeep/internal/forge"
	"github.com/aalvaropc/upkeep/internal/plan"
	"github.com/aalvaropc/upkeep/internal/pull"
	"github.com/aalvaropc/upkeep/internal/receipt"
)

const DefaultParallel = 4

type Processor interface {
	Process(ctx context.Context, in engine.Input) engine.Result
}

type Emitter func(kind events.Kind, pr int, data any)

type Action struct {
	Kind   string `json:"kind"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type Item struct {
	PR       pull.Request  `json:"pr"`
	Dir      string        `json:"-"`
	Result   engine.Result `json:"-"`
	Outcome  pull.Outcome  `json:"outcome"`
	Actions  []Action      `json:"actions"`
	Duration time.Duration `json:"-"`
}

type Summary struct {
	Items  []Item
	Counts map[pull.Outcome]int
	Totals receipt.Totals
	Plan   *plan.Plan
}

type Runner struct {
	Forge     forge.Forge
	Processor Processor
	Emit      Emitter
	Work      string
	Base      string
	Parallel  int
	Done      func(Item)
	Planner   Planner
}

func (r Runner) emit(kind events.Kind, pr int, data any) {
	if r.Emit != nil {
		r.Emit(kind, pr, data)
	}
}

func (r Runner) Run(ctx context.Context) (Summary, error) {
	pulls, err := r.Forge.Pulls(ctx)
	if err != nil {
		return Summary{}, err
	}
	r.emit(events.RunStarted, 0, map[string]any{"prs": pulls})

	items := make([]Item, len(pulls))
	prepared := make([]bool, len(pulls))
	for i, pr := range pulls {
		items[i] = Item{PR: pr, Dir: filepath.Join(r.Work, fmt.Sprintf("pr-%d", pr.Number))}
		if err := r.Forge.Prepare(ctx, pr, items[i].Dir); err != nil {
			items[i].Result = engine.Result{PR: pr, Outcome: pull.Failed, Reason: err.Error(), Err: err}
			continue
		}
		prepared[i] = true
	}
	advice := r.advise(ctx, items, prepared)

	parallel := r.Parallel
	if parallel <= 0 {
		parallel = DefaultParallel
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range parallel {
		wg.Go(func() {
			for i := range jobs {
				began := time.Now()
				if prepared[i] {
					items[i].Result = r.Processor.Process(ctx, engine.Input{PR: items[i].PR, Dir: items[i].Dir, Base: r.Base})
					r.act(ctx, &items[i])
				}
				items[i].Duration = time.Since(began)
				r.finish(ctx, &items[i])
			}
		})
	}
feed:
	for i := range items {
		select {
		case jobs <- i:
		case <-ctx.Done():
			for j := i; j < len(items); j++ {
				items[j].Result = engine.Result{PR: items[j].PR, Outcome: pull.Failed, Reason: ctx.Err().Error(), Err: ctx.Err()}
				r.finish(ctx, &items[j])
			}
			break feed
		}
	}
	close(jobs)
	wg.Wait()
	r.recheck(ctx, items, advice)

	summary := summarize(items)
	summary.Plan = advice
	r.emit(events.RunFinished, 0, map[string]any{
		"counts": summary.Counts, "calls": summary.Totals.Calls, "micros": summary.Totals.Charge.Micros,
		"baseline_micros": summary.Totals.Charge.BaselineMicros, "prompt": summary.Totals.Usage.PromptTokens,
		"completion": summary.Totals.Usage.CompletionTokens,
	})
	return summary, ctx.Err()
}

func summarize(items []Item) Summary {
	s := Summary{Items: items, Counts: map[pull.Outcome]int{}}
	for _, it := range items {
		s.Counts[it.Outcome]++
		if it.Result.Receipt != nil {
			t := it.Result.Receipt.Totals()
			s.Totals.Calls += t.Calls
			s.Totals.Usage = s.Totals.Usage.Add(t.Usage)
			s.Totals.Charge = s.Totals.Charge.Add(t.Charge)
			s.Totals.Latency += t.Latency
		}
	}
	return s
}

func (r Runner) finish(ctx context.Context, it *Item) {
	if it.Outcome == pull.Pending {
		it.Outcome = it.Result.Outcome
	}
	if it.Result.Receipt != nil {
		_ = r.Forge.Comment(ctx, it.PR, forge.Comment{Kind: "receipt", Body: receiptComment(it.Result)})
	}
	res := it.Result
	r.emit(events.PRDone, it.PR.Number, map[string]any{
		"outcome": string(it.Outcome), "blocker": string(res.Blocker), "reason": res.Reason, "attempts": res.Attempts,
		"changed": res.Changed, "flaky": res.Flaky, "summary": res.Summary, "actions": it.Actions, "duration_ms": it.Duration.Milliseconds(),
		"learned": res.Learned,
	})
	if r.Done != nil {
		r.Done(*it)
	}
}

func (r Runner) record(it *Item, a Action) {
	it.Actions = append(it.Actions, a)
	r.emit(events.Action, it.PR.Number, a)
}

func (r Runner) act(ctx context.Context, it *Item) {
	res := it.Result
	it.Outcome = res.Outcome
	switch res.Outcome {
	case pull.Mergeable:
		pushed := true
		if res.WantsPush {
			a := r.push(ctx, it)
			r.record(it, a)
			pushed = a.Status != "failed"
		}
		if res.WantsPatch {
			r.record(it, r.patch(ctx, it))
		}
		if !pushed {
			it.Outcome = pull.Failed
			return
		}
		if res.WantsMerge {
			a, merged := r.merge(ctx, it)
			r.record(it, a)
			switch {
			case merged:
				it.Outcome = pull.Merged
			case a.Status == "failed":
				it.Outcome = pull.NeedsHuman
			}
			return
		}
		if len(it.Actions) == 0 {
			r.record(it, Action{Kind: "none", Status: "done", Detail: "nothing to change; a person decides when to merge"})
		}
	case pull.NeedsHuman:
		err := r.Forge.Comment(ctx, it.PR, forge.Comment{Kind: "diagnosis", Body: diagnosisComment(res)})
		r.record(it, settled("comment", err, "posted the diagnosis", "would post the diagnosis"))
	case pull.Failed:
		r.record(it, Action{Kind: "none", Status: "failed", Detail: res.Reason})
	}
}

func settled(kind string, err error, did, would string) Action {
	switch {
	case errors.Is(err, forge.ErrDryRun):
		return Action{Kind: kind, Status: "skipped", Detail: "dry run: " + would}
	case err != nil:
		return Action{Kind: kind, Status: "failed", Detail: err.Error()}
	}
	return Action{Kind: kind, Status: "done", Detail: did}
}

func (r Runner) push(ctx context.Context, it *Item) Action {
	target := short(it.Result.HeadAfter) + " to " + it.PR.Branch
	return settled("push", r.Forge.Push(ctx, it.Dir, it.PR), "pushed "+target, "would push "+target)
}

func (r Runner) patch(ctx context.Context, it *Item) Action {
	err := r.Forge.Comment(ctx, it.PR, forge.Comment{Kind: "patch", Body: patchComment(it.Result)})
	return settled("patch", err, "attached the fix as a patch", "would attach the fix as a patch")
}

func (r Runner) merge(ctx context.Context, it *Item) (Action, bool) {
	sha, err := r.Forge.Merge(ctx, it.PR)
	switch {
	case err != nil:
		return settled("merge", err, "", "would merge it once the checks pass"), false
	case sha == "":
		return Action{Kind: "merge", Status: "done", Detail: "auto-merge is armed; it merges once the checks pass"}, false
	}
	return Action{Kind: "merge", Status: "done", Detail: "merged as " + short(sha)}, true
}

func short(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}
