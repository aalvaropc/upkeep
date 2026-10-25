package engine

import (
	"context"

	"github.com/aalvaropc/upkeep/internal/detect"
	"github.com/aalvaropc/upkeep/internal/events"
	"github.com/aalvaropc/upkeep/internal/gitx"
	"github.com/aalvaropc/upkeep/internal/model"
	"github.com/aalvaropc/upkeep/internal/policy"
	"github.com/aalvaropc/upkeep/internal/pull"
	"github.com/aalvaropc/upkeep/internal/sandbox"
	"github.com/aalvaropc/upkeep/internal/store"
	"github.com/aalvaropc/upkeep/internal/strategy"
	"github.com/aalvaropc/upkeep/internal/stub"
)

type Forge interface {
	Pulls(ctx context.Context) ([]pull.Request, error)
	Push(ctx context.Context, pr pull.Request, dir string) error
	Comment(ctx context.Context, pr pull.Request, body string) error
	Merge(ctx context.Context, pr pull.Request) error
}

type Deps struct {
	Forge      Forge
	Git        gitx.Git
	Runner     sandbox.Runner
	Detector   detect.Detector
	Strategies *strategy.Registry
	Router     *model.Router
	Policy     policy.Policy
	Events     *events.Ring
	Store      *store.Store
}

type Limits struct {
	PRsPerRun     int
	RunsPerServer int
}

var DefaultLimits = Limits{PRsPerRun: 4, RunsPerServer: 2}

type Engine struct {
	deps   Deps
	limits Limits
}

func New(deps Deps, limits Limits) *Engine {
	if limits.PRsPerRun <= 0 {
		limits.PRsPerRun = DefaultLimits.PRsPerRun
	}
	if limits.RunsPerServer <= 0 {
		limits.RunsPerServer = DefaultLimits.RunsPerServer
	}
	return &Engine{deps: deps, limits: limits}
}

func (e *Engine) Limits() Limits { return e.limits }

func (e *Engine) Work(_ context.Context, _ string) error {
	return stub.Err("work the queue")
}
