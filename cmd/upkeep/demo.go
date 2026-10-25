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

	"github.com/aalvaropc/upkeep/internal/buildinfo"
	"github.com/aalvaropc/upkeep/internal/config"
	"github.com/aalvaropc/upkeep/internal/demo"
	"github.com/aalvaropc/upkeep/internal/engine"
	"github.com/aalvaropc/upkeep/internal/events"
	"github.com/aalvaropc/upkeep/internal/gitx"
	"github.com/aalvaropc/upkeep/internal/policy"
	"github.com/aalvaropc/upkeep/internal/runtime"
	"github.com/aalvaropc/upkeep/internal/scenario"
	"github.com/aalvaropc/upkeep/internal/store"
	"github.com/aalvaropc/upkeep/internal/web"
	"github.com/aalvaropc/upkeep/scenarios"
)

type demoFlags struct {
	policy      string
	fixtures    string
	source      string
	rebuild     bool
	pace        float64
	maxSessions int
	trustProxy  bool
}

func parseDemoFlags(args []string, stderr io.Writer) (demoFlags, error) {
	var f demoFlags
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&f.policy, "policy", "", "policy file; the built-in policy when empty")
	fs.StringVar(&f.fixtures, "fixtures", "", "directory of recorded runs; defaults to the data directory")
	fs.StringVar(&f.source, "source", "", "repository or path to build the scenario from; defaults to the scenario's repository")
	fs.BoolVar(&f.rebuild, "rebuild", false, "build the scenario repository again")
	fs.Float64Var(&f.pace, "pace", 1, "replay speed; 1 keeps the recorded latencies and 0 skips them")
	fs.IntVar(&f.maxSessions, "max-sessions", 50, "most sessions kept at once")
	fs.BoolVar(&f.trustProxy, "trust-proxy", false, "believe X-Forwarded-For; only behind a proxy you control")
	if err := fs.Parse(args); err != nil {
		return demoFlags{}, err
	}
	if fs.NArg() > 0 {
		return demoFlags{}, fmt.Errorf("demo takes no arguments, got %q", fs.Arg(0))
	}
	if f.pace < 0 || f.maxSessions < 1 {
		return demoFlags{}, errors.New("demo: -pace cannot be negative and -max-sessions must be at least 1")
	}
	return f, nil
}

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

func demoCommand(ctx context.Context, args []string, stdout, stderr io.Writer, environ []string) error {
	flags, err := parseDemoFlags(args, stderr)
	if err != nil {
		return err
	}
	cfg, err := config.Load(environ)
	if err != nil {
		return err
	}
	pol := policy.Default()
	if flags.policy != "" {
		if pol, err = policy.Load(flags.policy); err != nil {
			return err
		}
	}
	s, err := scenarios.Notes()
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(stderr, nil))
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return err
	}
	template := filepath.Join(cfg.DataDir, "scenario", "forge.git")
	if err := ensureTemplate(ctx, s, template, flags.source, flags.rebuild); err != nil {
		return fmt.Errorf("building the scenario repository: %w", err)
	}
	st, err := store.Open(ctx, filepath.Join(cfg.DataDir, "upkeep.db"))
	if err != nil {
		return err
	}
	defer st.Close()

	fixtures := flags.fixtures
	if fixtures == "" {
		fixtures = filepath.Join(cfg.DataDir, "fixtures")
	}
	rt, err := runtime.New(ctx, runtime.Options{
		Config: cfg, Policy: pol, Git: gitx.Git{}, FixturesDir: fixtures,
		CacheDir: filepath.Join(cfg.DataDir, "cache"), Pace: flags.pace, Logger: logger,
	})
	if err != nil {
		return err
	}

	sessions := filepath.Join(cfg.DataDir, "sessions")
	if err := os.RemoveAll(sessions); err != nil {
		return err
	}
	hub := events.NewHub(ringCapacity)
	manager, err := demo.NewManager(ctx, demo.Options{
		Scenario: s, Template: template, Root: sessions, Git: gitx.Git{}, Hub: hub,
		Processor: rt.Processor, Limits: engine.DefaultLimits, MaxSessions: flags.maxSessions, Logger: logger,
	})
	if err != nil {
		return err
	}
	defer manager.Shutdown()

	done := make(chan struct{})
	handler := web.New(web.Options{
		Hub: hub, Version: buildinfo.Version(), Logger: logger, Done: done,
		Demo: manager, TrustProxy: flags.trustProxy,
		Info: func() map[string]any {
			return map[string]any{
				"mode": string(rt.Mode()), "version": buildinfo.Version(), "public": cfg.Public,
				"prs_per_run": engine.DefaultLimits.PRsPerRun, "runs_per_server": engine.DefaultLimits.RunsPerServer,
				"daily_tokens": cfg.DailyTokens, "tokens_today": rt.Budget.Spent(),
			}
		},
	})
	return serveHTTP(ctx, cfg.Addr, handler, done, logger, stdout, cfg)
}
