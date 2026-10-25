package strategy

import (
	"context"
	"fmt"
	"time"

	"github.com/aalvaropc/upkeep/internal/events"
	"github.com/aalvaropc/upkeep/internal/forecast"
	"github.com/aalvaropc/upkeep/internal/gitx"
	"github.com/aalvaropc/upkeep/internal/model"
	"github.com/aalvaropc/upkeep/internal/policy"
	"github.com/aalvaropc/upkeep/internal/pull"
	"github.com/aalvaropc/upkeep/internal/receipt"
	"github.com/aalvaropc/upkeep/internal/sandbox"
)

type Forecaster interface {
	Forecast(ctx context.Context, in forecast.Input) (forecast.Forecast, forecast.Cost, error)
}

type Env struct {
	Forecaster Forecaster
	Git        gitx.Git
	Runner     sandbox.Runner
	Executor   sandbox.Executor
	Models     *model.Router
	Meter      model.Meter
	Policy     policy.Policy
	Emit       func(kind events.Kind, pr int, data any)
}

type Job struct {
	PR      pull.Request
	Dir     string
	Base    string
	Notes   string
	Receipt *receipt.Receipt
}

func (e Env) emit(job *Job, kind events.Kind, data any) {
	if e.Emit != nil {
		e.Emit(kind, job.PR.Number, data)
	}
}

func (e Env) step(job *Job, name, status, detail string) {
	e.emit(job, events.PRStep, map[string]any{"step": name, "status": status, "detail": detail})
}

func (e Env) complete(ctx context.Context, job *Job, role model.Role, system, user string, reasoning bool, maxTokens int) (string, error) {
	name, m, err := e.Models.For(role)
	if err != nil {
		return "", err
	}
	req := model.Request{
		Model:     name,
		MaxTokens: maxTokens,
		Messages:  []model.Message{{Role: "system", Content: system}, {Role: "user", Content: user}},
	}
	if !reasoning {
		off := false
		req.Reasoning = &off
	}
	resp, err := m.Complete(ctx, req)
	if err != nil {
		return "", fmt.Errorf("%s call failed: %w", role, err)
	}
	charge := e.Meter.Charge(name, resp.Usage)
	job.Receipt.AddCall(role, name, resp.Usage, charge, resp.Latency)
	e.emit(job, events.ModelCall, map[string]any{
		"model": name, "role": string(role), "latency_ms": resp.Latency.Milliseconds(), "prompt": resp.Usage.PromptTokens,
		"completion": resp.Usage.CompletionTokens, "reasoning": resp.Usage.ReasoningTokens, "micros": charge.Micros,
	})
	return resp.Message.Content, nil
}

func since(start time.Time) time.Duration { return time.Since(start) }
