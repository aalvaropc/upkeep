package queue

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/aalvaropc/upkeep/internal/engine"
	"github.com/aalvaropc/upkeep/internal/events"
	"github.com/aalvaropc/upkeep/internal/forge"
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
	PR      pull.Request  `json:"pr"`
	Dir     string        `json:"-"`
	Result  engine.Result `json:"-"`
	Outcome pull.Outcome  `json:"outcome"`
	Actions []Action      `json:"actions"`
}

type Summary struct {
	Items  []Item
	Counts map[pull.Outcome]int
	Totals receipt.Totals
}

type Runner struct {
	Forge     forge.Forge
	Processor Processor
	Emit      Emitter
	Work      string
	Base      string
	Parallel  int
	Done      func(Item)
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

	parallel := r.Parallel
	if parallel <= 0 {
		parallel = DefaultParallel
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range parallel {
		wg.Go(func() {
			for i := range jobs {
				if prepared[i] {
					items[i].Result = r.Processor.Process(ctx, engine.Input{PR: items[i].PR, Dir: items[i].Dir, Base: r.Base})
					r.act(ctx, &items[i])
				}
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

	summary := summarize(items)
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
		"changed": res.Changed, "flaky": res.Flaky, "summary": res.Summary, "actions": it.Actions,
	})
	if r.Done != nil {
		r.Done(*it)
	}
}

func (r Runner) record(it *Item, a Action) bool {
	it.Actions = append(it.Actions, a)
	r.emit(events.Action, it.PR.Number, a)
	return a.Status == "done"
}

func (r Runner) act(ctx context.Context, it *Item) {
	res := it.Result
	it.Outcome = res.Outcome
	switch res.Outcome {
	case pull.Mergeable:
		pushed := true
		if res.WantsPush {
			pushed = r.record(it, r.push(ctx, it))
		}
		if res.WantsPatch {
			r.record(it, r.patch(ctx, it))
		}
		if !pushed {
			it.Outcome = pull.Failed
			return
		}
		if res.WantsMerge {
			if r.record(it, r.merge(ctx, it)) {
				it.Outcome = pull.Merged
			} else {
				it.Outcome = pull.NeedsHuman
			}
			return
		}
		if len(it.Actions) == 0 {
			r.record(it, Action{Kind: "none", Status: "done", Detail: "nothing to change; a person decides when to merge"})
		}
	case pull.NeedsHuman:
		a := Action{Kind: "comment", Status: "done", Detail: "posted the diagnosis"}
		if err := r.Forge.Comment(ctx, it.PR, forge.Comment{Kind: "diagnosis", Body: diagnosisComment(res)}); err != nil {
			a = Action{Kind: "comment", Status: "failed", Detail: err.Error()}
		}
		r.record(it, a)
	case pull.Failed:
		r.record(it, Action{Kind: "none", Status: "failed", Detail: res.Reason})
	}
}

func (r Runner) push(ctx context.Context, it *Item) Action {
	if err := r.Forge.Push(ctx, it.Dir, it.PR); err != nil {
		return Action{Kind: "push", Status: "failed", Detail: err.Error()}
	}
	return Action{Kind: "push", Status: "done", Detail: "pushed " + short(it.Result.HeadAfter) + " to " + it.PR.Branch}
}

func (r Runner) patch(ctx context.Context, it *Item) Action {
	if err := r.Forge.Comment(ctx, it.PR, forge.Comment{Kind: "patch", Body: patchComment(it.Result)}); err != nil {
		return Action{Kind: "patch", Status: "failed", Detail: err.Error()}
	}
	return Action{Kind: "patch", Status: "done", Detail: "attached the fix as a patch"}
}

func (r Runner) merge(ctx context.Context, it *Item) Action {
	sha, err := r.Forge.Merge(ctx, it.PR)
	if err != nil {
		return Action{Kind: "merge", Status: "failed", Detail: err.Error()}
	}
	return Action{Kind: "merge", Status: "done", Detail: "merged as " + short(sha)}
}

func short(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}
