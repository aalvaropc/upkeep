package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"

	"github.com/aalvaropc/upkeep/internal/config"
	"github.com/aalvaropc/upkeep/internal/demo"
	"github.com/aalvaropc/upkeep/internal/engine"
	"github.com/aalvaropc/upkeep/internal/events"
	"github.com/aalvaropc/upkeep/internal/fixture"
	"github.com/aalvaropc/upkeep/internal/gitx"
	"github.com/aalvaropc/upkeep/internal/policy"
	"github.com/aalvaropc/upkeep/internal/runtime"
	"github.com/aalvaropc/upkeep/internal/scenario"
	"github.com/aalvaropc/upkeep/scenarios"
)

type stackFlags struct {
	policy        string
	fixtures      string
	source        string
	rebuild       bool
	pace          float64
	maxSessions   int
	runsPerServer int
}

func (f *stackFlags) register(fs *flag.FlagSet, pace float64) {
	fs.StringVar(&f.policy, "policy", "", "policy file; the built-in policy when empty")
	fs.StringVar(&f.fixtures, "fixtures", "", "directory of recorded runs; the ones built into the binary when empty")
	fs.StringVar(&f.source, "source", "", "repository or path to build the scenario from; defaults to the scenario's repository")
	fs.BoolVar(&f.rebuild, "rebuild", false, "build the scenario repository again")
	fs.Float64Var(&f.pace, "pace", pace, "replay speed; 1 keeps the recorded latencies and 0 skips them")
	fs.IntVar(&f.maxSessions, "max-sessions", 50, "most sessions kept at once")
}

func (f stackFlags) validate() error {
	if f.pace < 0 || f.maxSessions < 1 {
		return errors.New("-pace cannot be negative and -max-sessions must be at least 1")
	}
	return nil
}

func (f stackFlags) limits() engine.Limits {
	limits := engine.DefaultLimits
	if f.runsPerServer > 0 {
		limits.RunsPerServer = f.runsPerServer
	}
	return limits
}

func newManager(ctx context.Context, f stackFlags, s scenario.Scenario, template, root string, hub *events.Hub, processor demo.Factory, logger *slog.Logger) (*demo.Manager, error) {
	return demo.NewManager(ctx, demo.Options{
		Scenario: s, Template: template, Root: root, Git: gitx.Git{}, Hub: hub,
		Processor: processor, Limits: f.limits(), MaxSessions: f.maxSessions, Logger: logger,
	})
}

type stack struct {
	policy   policy.Policy
	cfg      config.Config
	scenario scenario.Scenario
	runtime  *runtime.Runtime
	manager  *demo.Manager
	hub      *events.Hub
	logger   *slog.Logger
}

func (s *stack) close() { s.manager.Shutdown() }

func ensureTemplate(ctx context.Context, s scenario.Scenario, dir, source string, rebuild bool) error {
	if rebuild {
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
	}
	if _, err := os.Stat(dir); err == nil {
		return nil
	}
	from := s.Repository
	if source != "" {
		from = source
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o750); err != nil {
		return err
	}
	_, err := scenario.Builder{Git: gitx.Git{}, Recipes: scenarios.NotesFS()}.Build(ctx, s, from, dir)
	return err
}

func openStack(ctx context.Context, stderr io.Writer, environ []string, f stackFlags, forceMode config.Mode) (*stack, error) {
	if forceMode != "" {
		environ = append(slices.Clone(environ), "UPKEEP_MODE="+string(forceMode))
	}
	cfg, err := config.Load(environ)
	if err != nil {
		return nil, err
	}
	pol := policy.Default()
	if f.policy != "" {
		if pol, err = policy.Load(f.policy); err != nil {
			return nil, err
		}
	}
	s, err := scenarios.Notes()
	if err != nil {
		return nil, err
	}
	logger := slog.New(slog.NewTextHandler(stderr, nil))
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return nil, err
	}
	template := filepath.Join(cfg.DataDir, "scenario", "forge.git")
	if err := ensureTemplate(ctx, s, template, f.source, f.rebuild); err != nil {
		return nil, fmt.Errorf("building the scenario repository: %w", err)
	}

	var read, record fixture.Store
	switch {
	case f.fixtures != "":
		dir := fixture.Dir{Path: f.fixtures}
		read, record = dir, dir
	case cfg.Mode == config.Record:
		return nil, errors.New("record mode needs -fixtures DIR to save the exchanges in")
	default:
		if embedded, ok := scenarios.Fixtures(); ok {
			read = embedded
		}
	}
	rt, err := runtime.New(ctx, runtime.Options{
		Config: cfg, Policy: pol, Repo: s.Repository, Git: gitx.Git{}, Fixtures: read, Record: record,
		CacheDir: filepath.Join(cfg.DataDir, "cache"), Pace: f.pace, Logger: logger,
	})
	if err != nil {
		return nil, err
	}

	sessions := filepath.Join(cfg.DataDir, "sessions")
	if err := os.RemoveAll(sessions); err != nil {
		return nil, err
	}
	hub := events.NewHub(ringCapacity)
	manager, err := newManager(ctx, f, s, template, sessions, hub, rt.Processor, logger)
	if err != nil {
		return nil, err
	}
	return &stack{policy: pol, cfg: cfg, scenario: s, runtime: rt, manager: manager, hub: hub, logger: logger}, nil
}
