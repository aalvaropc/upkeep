package engine

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/aalvaropc/upkeep/internal/events"
	"github.com/aalvaropc/upkeep/internal/fixer"
	"github.com/aalvaropc/upkeep/internal/learn"
	"github.com/aalvaropc/upkeep/internal/receipt"
	"github.com/aalvaropc/upkeep/internal/strategy"
)

type Learned struct {
	From        int   `json:"from"`
	SavedCalls  int   `json:"saved_calls"`
	SavedMicros int64 `json:"saved_micros"`
	patch       string
	note        string
}

func (l *Learned) reusedNote(patch string) (string, bool) {
	if l == nil || l.note == "" || l.patch != patch {
		return "", false
	}
	return l.note, true
}

func usd(micros int64) string {
	return fmt.Sprintf("$%.4f", float64(micros)/1_000_000)
}

func relative(job *strategy.Job, failure string) string {
	return strings.ReplaceAll(failure, job.Dir+string(os.PathSeparator), "")
}

func (p Processor) key(job *strategy.Job, failure string) string {
	return learn.Key(p.Repo, job.PR.Bump, relative(job, failure))
}

func (p Processor) defersForecast(job *strategy.Job) bool {
	return p.Learned != nil && job.PR.Bump != nil && p.Learned.Covers(learn.Scope(p.Repo, job.PR.Bump))
}

func (p Processor) learnedStep(res *Result, status, detail string, fix learn.Fix) {
	p.emit(res, events.PRStep, map[string]any{"step": "learned", "status": status, "detail": detail, "from": fix.PR, "saved_micros": fix.Micros})
}

func describeEdits(edits []fixer.Edit) string {
	var paths []string
	for _, e := range edits {
		if !slices.Contains(paths, e.Path) {
			paths = append(paths, e.Path)
		}
	}
	noun := "edits"
	if len(edits) == 1 {
		noun = "edit"
	}
	return fmt.Sprintf("%d %s in %s", len(edits), noun, strings.Join(paths, ", "))
}

func (p Processor) reuse(ctx context.Context, res *Result, job *strategy.Job, key string, tried map[string]bool) (bool, error) {
	if p.Learned == nil || tried[key] {
		return false, nil
	}
	tried[key] = true
	fix, ok := p.Learned.Find(key)
	if !ok || len(fix.Edits) == 0 {
		return false, nil
	}
	env := p.Env
	began := time.Now()
	p.learnedStep(res, "running", fmt.Sprintf("trying the fix learned from #%d: %s", fix.PR, describeEdits(fix.Edits)), fix)
	stale := func(why string) (bool, error) {
		if err := env.Git.ResetTo(ctx, job.Dir, res.FixBase); err != nil {
			return false, fmt.Errorf("undoing the learned fix: %w", err)
		}
		p.learnedStep(res, "failed", why+"; repairing it from the start", fix)
		job.Receipt.AddStep(receipt.Step{Name: "learned", Detail: why, Duration: time.Since(began)})
		return false, nil
	}
	if err := fixer.ApplyEdits(job.Dir, fix.Edits); err != nil {
		return stale(fmt.Sprintf("the fix learned from #%d no longer applies (%v)", fix.PR, err))
	}
	report, err := env.Runner.Check(ctx, job.Dir)
	if err != nil {
		return stale(fmt.Sprintf("the checks could not run with the fix learned from #%d (%v)", fix.PR, err))
	}
	if !report.Passed() {
		return stale(fmt.Sprintf("the checks still fail with the fix learned from #%d", fix.PR))
	}
	if _, err := env.Git.Commit(ctx, job.Dir, fixMessage(job.PR, fix.Edits)); err != nil {
		return false, fmt.Errorf("committing the learned fix: %w", err)
	}
	res.Learned = &Learned{From: fix.PR, SavedCalls: fix.Calls, SavedMicros: fix.Micros, patch: fix.Patch, note: fix.Note}
	p.learnedStep(res, "done", fmt.Sprintf("reused the fix learned from #%d and the checks pass: 0 model calls; the first time cost %s", fix.PR, usd(fix.Micros)), fix)
	job.Receipt.AddStep(receipt.Step{Name: "learned", Detail: "learned fix: 0 model calls; the first time cost " + usd(fix.Micros), Duration: time.Since(began), OK: true})
	return true, nil
}

func (p Processor) remember(res *Result, job *strategy.Job, key, failure string, rep strategy.Repair) {
	if p.Learned == nil || len(rep.Edits) == 0 {
		return
	}
	fix := learn.Fix{
		Key:       key,
		Repo:      p.Repo,
		Signature: learn.Signature(relative(job, failure)),
		Edits:     slices.Clone(rep.Edits),
		Summary:   describeEdits(rep.Edits),
		PR:        job.PR.Number,
	}
	if b := job.PR.Bump; b != nil {
		fix.Module, fix.From, fix.To = b.Module, b.From, b.To
	}
	res.fixes = append(res.fixes, fix)
}

func (p Processor) learn(res *Result) {
	if p.Learned == nil || len(res.fixes) == 0 {
		return
	}
	total := res.Receipt.Totals()
	for _, fix := range res.fixes {
		fix.Patch, fix.Note = res.Patch, res.Summary
		fix.Calls, fix.Micros = total.Calls, total.Charge.Micros
		fix.At = time.Now().UTC()
		if err := p.Learned.Save(fix); err != nil {
			p.step(res, "learn", "failed", "the fix could not be remembered: "+err.Error())
			continue
		}
		p.step(res, "learn", "done", "remembered the fix for the next time this failure returns")
	}
	res.fixes = nil
}
