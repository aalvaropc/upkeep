package strategy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aalvaropc/upkeep/internal/events"
	"github.com/aalvaropc/upkeep/internal/fixer"
	"github.com/aalvaropc/upkeep/internal/model"
	"github.com/aalvaropc/upkeep/internal/sandbox"
)

type Repair struct {
	Fixed    bool
	Attempts int
	Edits    []fixer.Edit
	Failure  string
	Stop     fixer.Stop
	Err      error
}

func (e Env) Repair(ctx context.Context, job *Job, failure string, start string) Repair {
	var out Repair
	name, m, err := e.Models.For(model.Super)
	if err != nil {
		out.Err = err
		return out
	}
	deadline := time.Duration(e.Policy.MaxSeconds) * time.Second
	ctx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()

	spent := 0
	prior := ""
	for attempt := 1; attempt <= e.Policy.MaxAttempts; attempt++ {
		if attempt > 1 {
			if err := e.Git.ResetTo(ctx, job.Dir, start); err != nil {
				out.Err = err
				return out
			}
		}
		remaining := e.Policy.MaxTokens - spent
		if remaining <= 0 {
			out.Stop = fixer.StopTokens
			return out
		}
		e.stepWith(job, "repair", "running", fmt.Sprintf("attempt %d of %d", attempt, e.Policy.MaxAttempts), map[string]any{"attempt": attempt, "of": e.Policy.MaxAttempts})
		f := fixer.Fixer{
			Model:     m,
			ModelName: name,
			Meter:     e.Meter,
			Runner:    e.Runner,
			Executor:  e.Executor,
			Attempt:   attempt,
			MaxTokens: remaining,
			Emit: func(kind events.Kind, data any) {
				e.emit(job, kind, data)
			},
		}
		began := time.Now()
		a := f.Run(ctx, fixer.Task{Dir: job.Dir, Failure: failure, Notes: job.Notes, Prior: prior})
		out.Attempts = attempt
		job.Receipt.AddAttempt()
		for _, c := range a.Calls {
			job.Receipt.AddCall(model.Super, c.Model, c.Usage, c.Charge, c.Latency)
		}
		spent += a.Usage.Total()
		out.Stop, out.Edits, out.Failure = a.Stop, a.Edits, a.Failure
		e.stepWith(job, "repair", status(a.Success), summary(attempt, a, since(began)), outcome(attempt, e.Policy.MaxAttempts, a))
		if a.Err != nil && ctx.Err() == nil {
			out.Err = a.Err
			return out
		}
		if a.Success {
			out.Fixed = true
			return out
		}
		if ctx.Err() != nil {
			out.Stop = fixer.StopDeadline
			return out
		}
		prior = a.Failure
		if prior == "" {
			prior = string(a.Stop)
		}
	}
	return out
}

func status(ok bool) string {
	if ok {
		return "done"
	}
	return "failed"
}

func editedFiles(edits []fixer.Edit) int {
	files := map[string]bool{}
	for _, ed := range edits {
		files[ed.Path] = true
	}
	return len(files)
}

func summary(attempt int, a fixer.Attempt, took time.Duration) string {
	return fmt.Sprintf("attempt %d: %s after %d turns, %d edits in %d files, %s", attempt, a.Stop, a.Turns, len(a.Edits), editedFiles(a.Edits), took.Round(100*time.Millisecond))
}

func outcome(attempt, of int, a fixer.Attempt) map[string]any {
	fields := map[string]any{"attempt": attempt, "of": of, "stop": string(a.Stop), "turns": a.Turns, "edits": len(a.Edits), "files": editedFiles(a.Edits)}
	if !a.Success && a.Failure != "" {
		fields["failure"] = fixer.FirstError(a.Failure)
	}
	return fields
}

func FailureText(r sandbox.Result) string {
	return fmt.Sprintf("%s failed:\n%s", strings.Join(r.Args, " "), strings.TrimSpace(r.Output))
}
