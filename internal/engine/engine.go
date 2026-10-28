package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/aalvaropc/upkeep/internal/confidence"
	"github.com/aalvaropc/upkeep/internal/detect"
	"github.com/aalvaropc/upkeep/internal/events"
	"github.com/aalvaropc/upkeep/internal/fixer"
	"github.com/aalvaropc/upkeep/internal/forecast"
	"github.com/aalvaropc/upkeep/internal/gitx"
	"github.com/aalvaropc/upkeep/internal/gocomments"
	"github.com/aalvaropc/upkeep/internal/learn"
	"github.com/aalvaropc/upkeep/internal/model"
	"github.com/aalvaropc/upkeep/internal/pull"
	"github.com/aalvaropc/upkeep/internal/receipt"
	"github.com/aalvaropc/upkeep/internal/strategy"
	"github.com/aalvaropc/upkeep/internal/verify"
)

const (
	maxRounds       = 6
	maxFailureChars = 4000
)

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n... (cut)"
}

type Limits struct {
	PRsPerRun     int
	RunsPerServer int
}

var DefaultLimits = Limits{PRsPerRun: 4, RunsPerServer: 2}

type Input struct {
	PR    pull.Request
	Dir   string
	Base  string
	Notes string
}

type Result struct {
	PR         pull.Request
	Blocker    pull.Blocker
	Outcome    pull.Outcome
	Reason     string
	Diagnosis  string
	HeadBefore string
	HeadAfter  string
	FixBase    string
	Changed    bool
	Patch      string
	Changes    []gitx.FileChange
	Checklist  verify.Checklist
	Flaky      []string
	Forecast   *forecast.Forecast
	Confirmed  []string
	Missed     []string
	Receipt    *receipt.Receipt
	Summary    string
	Failure    string
	Attempts   int
	WantsPush  bool
	WantsPatch bool
	WantsMerge bool
	Confidence confidence.Assessment
	Learned    *Learned
	Err        error
	fixes      []learn.Fix
}

type Processor struct {
	Env     strategy.Env
	Learned learn.Book
	Repo    string
}

func (p Processor) emit(res *Result, kind events.Kind, data any) {
	if p.Env.Emit != nil {
		p.Env.Emit(kind, res.PR.Number, data)
	}
}

func (p Processor) step(res *Result, name, status, detail string) {
	p.emit(res, events.PRStep, map[string]any{"step": name, "status": status, "detail": detail})
}

func (p Processor) Process(ctx context.Context, in Input) Result {
	env := p.Env
	job := &strategy.Job{PR: in.PR, Dir: in.Dir, Base: in.Base, Notes: in.Notes, Receipt: receipt.New()}
	res := Result{PR: in.PR, Receipt: job.Receipt}

	head, err := env.Git.Head(ctx, in.Dir)
	if err != nil {
		return p.failed(&res, fmt.Errorf("reading the branch head: %w", err))
	}
	res.HeadBefore, res.FixBase = head, head
	deferred := p.defersForecast(job)
	if !deferred {
		p.forecast(ctx, &res, job)
	}
	detector := detect.Detector{Git: env.Git, Runner: env.Runner, Policy: env.Policy}
	ws := detect.Workspace{Dir: in.Dir, Base: in.Base}

	diagnose := func() (detect.Diagnosis, error) {
		began := time.Now()
		p.step(&res, "detect", "running", "")
		diag, err := detector.Detect(ctx, ws)
		if err != nil {
			p.step(&res, "detect", "failed", err.Error())
			return diag, err
		}
		detail := string(diag.Blocker)
		p.step(&res, "detect", "done", detail)
		job.Receipt.AddStep(receipt.Step{Name: "detect", Detail: detail, Duration: time.Since(began), OK: true})
		return diag, nil
	}

	diag, err := diagnose()
	if err != nil {
		return p.failed(&res, err)
	}
	res.Blocker = diag.Blocker
	p.emit(&res, events.PRBlocker, map[string]any{"blocker": string(diag.Blocker), "behind": diag.Behind})
	p.step(&res, "plan", "done", approach(diag.Blocker, env.Policy, strings.TrimPrefix(in.Base, "origin/")))

	seen := map[pull.Blocker]int{}
	tried := map[string]bool{}
	for round := 0; round < maxRounds; round++ {
		seen[diag.Blocker]++
		if seen[diag.Blocker] > 2 {
			return p.escalate(ctx, &res, job, diag, "the same blocker came back after it was fixed")
		}
		switch diag.Blocker {
		case pull.Clean, pull.Flaky:
			return p.finish(ctx, &res, job, diag)

		case pull.Behind:
			if err := p.commitPending(ctx, &res, job); err != nil {
				return p.failed(&res, err)
			}
			return p.finish(ctx, &res, job, diag)

		case pull.Format:
			if err := p.commitPending(ctx, &res, job); err != nil {
				return p.failed(&res, err)
			}
			p.step(&res, "format", "running", "")
			files, err := env.Format(ctx, job)
			if err != nil {
				return p.failed(&res, err)
			}
			p.step(&res, "format", "done", strings.Join(files, ", "))
			if diag, err = diagnose(); err != nil {
				return p.failed(&res, err)
			}

		case pull.ConflictSmall:
			p.step(&res, "conflict", "running", fmt.Sprintf("%d hunks", diag.Merge.Hunks()))
			resolved, err := env.ResolveConflict(ctx, job, diag.Merge.Conflicts)
			if err != nil {
				p.step(&res, "conflict", "failed", err.Error())
				out := p.escalate(ctx, &res, job, diag, "the conflict could not be resolved safely: "+err.Error())
				_ = env.Git.MergeAbort(ctx, in.Dir)
				return out
			}
			p.step(&res, "conflict", "done", fmt.Sprintf("%d hunks in %d files", resolved.Hunks, resolved.Files))
			if err := env.CommitMerge(ctx, job); err != nil {
				return p.failed(&res, err)
			}
			if res.FixBase, err = env.Git.Head(ctx, in.Dir); err != nil {
				return p.failed(&res, err)
			}
			if diag, err = diagnose(); err != nil {
				return p.failed(&res, err)
			}

		case pull.ConflictLarge:
			out := p.escalate(ctx, &res, job, diag, fmt.Sprintf("%d conflicting hunks are too large to resolve automatically", diag.Merge.Hunks()))
			_ = env.Git.MergeAbort(ctx, in.Dir)
			return out

		case pull.Build, pull.Test:
			if err := p.commitPending(ctx, &res, job); err != nil {
				return p.failed(&res, err)
			}
			failure, _ := diag.Failure()
			if res.Failure == "" {
				res.Failure = clip(strategy.FailureText(failure), maxFailureChars)
			}
			p.confirmForecast(&res, strategy.FailureText(failure))
			key := p.key(job, strategy.FailureText(failure))
			reused, err := p.reuse(ctx, &res, job, key, tried)
			if err != nil {
				return p.failed(&res, err)
			}
			if reused {
				if diag, err = diagnose(); err != nil {
					return p.failed(&res, err)
				}
				continue
			}
			if deferred {
				deferred = false
				p.forecast(ctx, &res, job)
				p.confirmForecast(&res, strategy.FailureText(failure))
			}
			rep := env.Repair(ctx, job, strategy.FailureText(failure), res.FixBase)
			res.Attempts = rep.Attempts
			if rep.Err != nil {
				return p.failed(&res, rep.Err)
			}
			if !rep.Fixed {
				_ = env.Git.ResetTo(ctx, in.Dir, res.FixBase)
				return p.escalate(ctx, &res, job, diag, fmt.Sprintf("%d attempts did not fix it (%s)", rep.Attempts, rep.Stop))
			}
			if _, err := env.Git.Commit(ctx, in.Dir, fixMessage(in.PR, rep.Edits)); err != nil {
				return p.failed(&res, fmt.Errorf("committing the fix: %w", err))
			}
			p.remember(&res, job, key, strategy.FailureText(failure), rep)
			if diag, err = diagnose(); err != nil {
				return p.failed(&res, err)
			}

		default:
			return p.failed(&res, fmt.Errorf("no way to handle the blocker %q", diag.Blocker))
		}
	}
	return p.escalate(ctx, &res, job, diag, "it needed more rounds of fixing than the limit")
}

func (p Processor) forecast(ctx context.Context, res *Result, job *strategy.Job) {
	env := p.Env
	bump := job.PR.Bump
	if bump == nil || bump.Level == pull.Patch || env.Forecaster == nil || strings.TrimSpace(job.Notes) != "" {
		return
	}
	began := time.Now()
	p.step(res, "forecast", "running", fmt.Sprintf("%s %s to %s", bump.Module, bump.From, bump.To))
	fc, cost, err := env.Forecaster.Forecast(ctx, forecast.Input{Module: bump.Module, From: bump.From, To: bump.To, Dir: job.Dir})
	if s := fc.Search; s != nil {
		if s.Skipped == "" {
			job.Receipt.AddSearch(receipt.Search{
				Provider: s.Provider, Query: s.Query, Results: len(s.Results), Kept: len(s.Kept),
				Latency: time.Duration(s.LatencyMS) * time.Millisecond, Micros: s.Micros, Error: s.Error, Recorded: s.Recorded, Reused: s.Reused,
			})
		}
		p.emit(res, events.SearchCall, searchData(s))
	}
	if cost.Model != "" && !cost.Cached {
		role := cost.Role
		if role == "" {
			role = model.Super
		}
		job.Receipt.AddCall(role, cost.Model, cost.Usage, cost.Charge, cost.Latency)
		p.emit(res, events.ModelCall, map[string]any{
			"model": cost.Model, "role": string(role), "latency_ms": cost.Latency.Milliseconds(), "prompt": cost.Usage.PromptTokens,
			"completion": cost.Usage.CompletionTokens, "reasoning": cost.Usage.ReasoningTokens, "micros": cost.Charge.Micros, "baseline_micros": cost.Charge.BaselineMicros,
			"step": "forecast", "purpose": "forecast",
		})
	}
	if err != nil {
		p.step(res, "forecast", "failed", err.Error())
		job.Receipt.AddStep(receipt.Step{Name: "forecast", Detail: err.Error(), Duration: time.Since(began)})
		return
	}
	job.Notes = fc.Notes()
	res.Forecast = &fc
	detail := fmt.Sprintf("%d breaking changes expected", len(fc.Breaking))
	if cost.Cached {
		detail += ", recorded " + fc.FetchedAt.Format("2006-01-02")
	}
	if fc.Search != nil {
		detail += "; " + searchDetail(fc.Search)
	}
	p.step(res, "forecast", "done", detail)
	job.Receipt.AddStep(receipt.Step{Name: "forecast", Detail: detail, Duration: time.Since(began), OK: true})
}

func searchData(s *forecast.SearchTrace) map[string]any {
	results := make([]map[string]any, 0, len(s.Results))
	for _, r := range s.Results {
		results = append(results, map[string]any{"title": r.Title, "url": r.URL, "score": r.Score, "kept": r.Kept})
	}
	kept := s.Kept
	if kept == nil {
		kept = []string{}
	}
	data := map[string]any{
		"provider": s.Provider, "query": s.Query, "results": results, "kept": kept,
		"latency_ms": s.LatencyMS, "recorded": s.Recorded, "reused": s.Reused,
	}
	if s.Error != "" {
		data["error"] = s.Error
	}
	if s.Skipped != "" {
		data["skipped"] = s.Skipped
	}
	if s.Micros > 0 {
		data["micros"] = s.Micros
	}
	return data
}

func pagesText(n int) string {
	if n == 1 {
		return "1 page"
	}
	return fmt.Sprintf("%d pages", n)
}

func searchDetail(s *forecast.SearchTrace) string {
	name := forecast.ProviderLabel(s.Provider)
	switch {
	case s.Skipped != "":
		return name + " was not asked: " + s.Skipped
	case s.Error != "":
		return name + " failed: " + s.Error
	}
	var out string
	if len(s.Results) == 0 && len(s.Kept) > 0 {
		out = fmt.Sprintf("%s kept %s", name, pagesText(len(s.Kept)))
	} else {
		out = fmt.Sprintf("%s kept %d of %s", name, len(s.Kept), pagesText(len(s.Results)))
	}
	switch {
	case s.Recorded:
		out += " in the recorded run"
	case s.Reused:
		out += ", none new"
	}
	return out
}

func (p Processor) confirmForecast(res *Result, failure string) {
	if res.Forecast == nil || res.Confirmed != nil || res.Missed != nil {
		return
	}
	res.Confirmed, res.Missed = res.Forecast.Confirm(failure)
	total := len(res.Confirmed) + len(res.Missed)
	if total == 0 {
		return
	}
	p.step(res, "forecast", "done", fmt.Sprintf("the build confirmed %d of %d predicted changes", len(res.Confirmed), total))
}

func fixMessage(pr pull.Request, edits []fixer.Edit) string {
	var paths []string
	for _, e := range edits {
		if !slices.Contains(paths, e.Path) {
			paths = append(paths, e.Path)
		}
	}
	subject := "fix: make the checks pass again"
	if pr.Bump != nil {
		subject = fmt.Sprintf("fix: adapt the code to %s %s", pr.Bump.Module, pr.Bump.To)
	}
	return subject + "\n\nEdited: " + strings.Join(paths, ", ")
}

func (p Processor) commitPending(ctx context.Context, res *Result, job *strategy.Job) error {
	if !p.Env.Git.MergeInProgress(job.Dir) {
		return nil
	}
	if err := p.Env.CommitMerge(ctx, job); err != nil {
		return err
	}
	head, err := p.Env.Git.Head(ctx, job.Dir)
	if err != nil {
		return err
	}
	res.FixBase = head
	p.step(res, "merge", "done", "merged "+strings.TrimPrefix(job.Base, "origin/"))
	return nil
}

func (p Processor) failed(res *Result, err error) Result {
	res.Err = err
	res.Outcome = pull.Failed
	res.Reason = err.Error()
	p.step(res, "run", "failed", err.Error())
	return *res
}

func (p Processor) escalate(ctx context.Context, res *Result, job *strategy.Job, diag detect.Diagnosis, reason string) Result {
	res.Outcome = pull.NeedsHuman
	res.Reason = reason
	p.step(res, "diagnose", "running", "")
	var text string
	var err error
	switch diag.Blocker {
	case pull.ConflictLarge, pull.ConflictSmall:
		text, err = p.Env.DiagnoseConflict(ctx, job, diag.Merge.Conflicts)
	default:
		failure, _ := diag.Failure()
		text, err = p.Env.DiagnoseFailure(ctx, job, strategy.FailureText(failure), res.Attempts)
	}
	if err != nil {
		p.step(res, "diagnose", "failed", err.Error())
		text = ""
	} else {
		p.step(res, "diagnose", "done", "")
	}
	res.Diagnosis = text
	if head, err := p.Env.Git.Head(ctx, job.Dir); err == nil {
		res.HeadAfter = head
	}
	p.emit(res, events.Receipt, receiptData(res.Receipt))
	return *res
}

func (p Processor) finish(ctx context.Context, res *Result, job *strategy.Job, diag detect.Diagnosis) Result {
	env := p.Env
	head, err := env.Git.Head(ctx, job.Dir)
	if err != nil {
		return p.failed(res, err)
	}
	res.HeadAfter = head
	res.Flaky = diag.FlakyTests
	p.step(res, "verify", "running", "")

	changes, err := env.Git.Diff(ctx, job.Dir, res.FixBase)
	if err != nil {
		return p.failed(res, fmt.Errorf("measuring the change: %w", err))
	}
	res.Changes = changes
	comments, err := p.addedComments(ctx, job, changes, res.FixBase)
	if err != nil {
		return p.failed(res, err)
	}
	report := diag.Report
	if len(diag.FlakyTests) > 0 && diag.FlakyRerun.OK() {
		report.Test = diag.FlakyRerun
	}
	res.Checklist = verify.Verify(verify.Input{
		Policy:      env.Policy,
		Changes:     changes,
		Report:      report,
		Comments:    comments,
		Unformatted: diag.Unformatted,
		Dependency:  job.PR.Kind == pull.Dependency,
	})
	p.emit(res, events.VerifyResult, map[string]any{"passed": res.Checklist.Passed(), "checks": res.Checklist})

	if !res.Checklist.Passed() {
		failed := res.Checklist.Failed()
		labels := make([]string, len(failed))
		for i, c := range failed {
			labels[i] = c.Label
		}
		_ = env.Git.ResetTo(ctx, job.Dir, res.HeadBefore)
		res.HeadAfter = res.HeadBefore
		p.step(res, "verify", "failed", strings.Join(labels, "; "))
		res.Outcome = pull.NeedsHuman
		res.Reason = "the fix did not pass verification: " + strings.Join(labels, "; ")
		p.emit(res, events.Receipt, receiptData(res.Receipt))
		return *res
	}

	res.Changed = res.HeadAfter != res.HeadBefore
	if res.Changed {
		if res.Patch, err = env.Git.PatchAgainst(ctx, job.Dir, res.FixBase); err != nil {
			return p.failed(res, err)
		}
	}
	if res.Changed {
		res.Summary = p.summarize(ctx, res, job)
		p.learn(res)
	}
	res.Outcome = pull.Mergeable
	res.WantsPush = res.Changed && env.Policy.MayPush(job.PR)
	res.WantsPatch = res.Changed && !res.WantsPush
	res.WantsMerge = env.Policy.MayAutoMerge(job.PR.Author, job.PR.Bump)
	p.step(res, "verify", "done", "every check passed")
	res.Confidence = assess(res, job, env.Policy.MaxLinesChanged)
	p.emit(res, events.Receipt, receiptData(res.Receipt))
	return *res
}

func assess(res *Result, job *strategy.Job, maxLines int) confidence.Assessment {
	lines := 0
	for _, c := range res.Changes {
		lines += c.Added + c.Deleted
	}
	from := 0
	if res.Learned != nil {
		from = res.Learned.From
	}
	return confidence.Assess(confidence.Input{
		Outcome:   res.Outcome,
		Blocker:   res.Blocker,
		Bump:      job.PR.Bump,
		Attempts:  res.Attempts,
		Learned:   from,
		Files:     len(res.Changes),
		Lines:     lines,
		MaxLines:  maxLines,
		Predicted: len(res.Confirmed) + len(res.Missed),
		Confirmed: len(res.Confirmed),
		Flaky:     res.Flaky,
	})
}

func (p Processor) addedComments(ctx context.Context, job *strategy.Job, changes []gitx.FileChange, base string) (int, error) {
	total := 0
	for _, c := range changes {
		if !strings.HasSuffix(c.Path, ".go") {
			continue
		}
		after, err := os.ReadFile(filepath.Join(job.Dir, c.Path))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return 0, err
		}
		before, err := p.Env.Git.Show(ctx, job.Dir, base, c.Path)
		if err != nil {
			before = ""
		}
		total += gocomments.Added([]byte(before), after)
	}
	return total, nil
}

func receiptData(r *receipt.Receipt) map[string]any {
	t := r.Totals()
	return map[string]any{
		"calls": t.Calls, "prompt": t.Usage.PromptTokens, "completion": t.Usage.CompletionTokens,
		"micros": t.Charge.Micros, "baseline_micros": t.Charge.BaselineMicros, "attempts": r.Attempts(),
	}
}

func (p Processor) summarize(ctx context.Context, res *Result, job *strategy.Job) string {
	env := p.Env
	paths := make([]string, len(res.Changes))
	for i, c := range res.Changes {
		paths[i] = c.Path
	}
	edited := "Edited " + strings.Join(paths, ", ") + "."
	if len(paths) == 0 {
		edited = "Merged " + strings.TrimPrefix(job.Base, "origin/") + " into the branch."
	}
	switch res.Blocker {
	case pull.Behind:
		return "Merged " + strings.TrimPrefix(job.Base, "origin/") + " into the branch; nothing conflicted."
	case pull.Format:
		return "Formatted " + strings.Join(paths, ", ") + " with gofmt."
	}
	if res.Blocker == pull.ConflictSmall && len(paths) == 0 {
		return "Resolved the conflict with " + strings.TrimPrefix(job.Base, "origin/") + " and merged it into the branch."
	}
	if note, ok := res.Learned.reusedNote(res.Patch); ok {
		return note
	}
	if !env.CanExplain() {
		return edited
	}
	p.step(res, "explain", "running", "")
	summary, err := env.Explain(ctx, job, res.Failure, res.Patch)
	switch {
	case errors.Is(err, strategy.ErrUngrounded):
		p.step(res, "explain", "failed", "the note named things that are not in the change, so it was dropped")
		return edited
	case err != nil:
		p.step(res, "explain", "failed", err.Error())
		return edited
	case summary == "":
		return edited
	}
	p.step(res, "explain", "done", summary)
	return summary
}
