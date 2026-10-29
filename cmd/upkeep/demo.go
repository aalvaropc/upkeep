package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/aalvaropc/upkeep/internal/buildinfo"
	"github.com/aalvaropc/upkeep/internal/engine"
	"github.com/aalvaropc/upkeep/internal/web"
)

type demoFlags struct {
	stackFlags
	trustProxy bool
}

func parseDemoFlags(args []string, stderr io.Writer) (demoFlags, error) {
	var f demoFlags
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)
	fs.SetOutput(stderr)
	f.register(fs, 1)
	fs.IntVar(&f.runsPerServer, "runs", engine.DefaultLimits.RunsPerServer, "most runs the server works on at once; later ones wait in line")
	fs.BoolVar(&f.trustProxy, "trust-proxy", false, "believe X-Forwarded-For; only behind a proxy you control")
	if err := fs.Parse(args); err != nil {
		return demoFlags{}, err
	}
	if fs.NArg() > 0 {
		return demoFlags{}, fmt.Errorf("demo takes no arguments, got %q", fs.Arg(0))
	}
	if err := f.validate(); err != nil {
		return demoFlags{}, fmt.Errorf("demo: %w", err)
	}
	if f.runsPerServer < 1 {
		return demoFlags{}, errors.New("demo: -runs must be at least 1")
	}
	return f, nil
}

func demoCommand(ctx context.Context, args []string, stdout, stderr io.Writer, environ []string) error {
	flags, err := parseDemoFlags(args, stderr)
	if err != nil {
		return err
	}
	st, err := openStack(ctx, stderr, environ, flags.stackFlags, "")
	if err != nil {
		return err
	}
	defer st.close()

	done := make(chan struct{})
	handler := web.New(web.Options{
		Hub: st.hub, Version: buildinfo.Version(), Logger: st.logger, Done: done,
		Demo: st.manager, TrustProxy: flags.trustProxy,
		Info: func() map[string]any { return demoInfo(st) },
	})
	return serveHTTP(ctx, st.cfg.Addr, handler, done, st.logger, stdout, st.cfg)
}

func demoInfo(st *stack) map[string]any {
	limits := st.manager.Limits()
	return map[string]any{
		"mode": string(st.runtime.Mode()), "version": buildinfo.Version(), "public": st.cfg.Public,
		"parallel_prs": limits.PRsPerRun, "runs_per_server": limits.RunsPerServer,
		"daily_tokens": st.cfg.DailyTokens, "tokens_today": st.runtime.Budget.Spent(), "rules": st.policy.Rules(),
		"search_daily_limit": st.cfg.SearchDailyLimit, "searches_today": st.runtime.Searches.Spent(),
	}
}
