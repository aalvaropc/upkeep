package main

import (
	"context"
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
	return map[string]any{
		"mode": string(st.runtime.Mode()), "version": buildinfo.Version(), "public": st.cfg.Public,
		"prs_per_run": engine.DefaultLimits.PRsPerRun, "runs_per_server": engine.DefaultLimits.RunsPerServer,
		"daily_tokens": st.cfg.DailyTokens, "tokens_today": st.runtime.Budget.Spent(), "rules": st.policy.Rules(),
		"search_daily_limit": st.cfg.SearchDailyLimit, "searches_today": st.runtime.Searches.Spent(),
	}
}
