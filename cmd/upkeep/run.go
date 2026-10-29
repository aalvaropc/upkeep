package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/aalvaropc/upkeep/internal/config"
	"github.com/aalvaropc/upkeep/internal/events"
	"github.com/aalvaropc/upkeep/internal/forge"
	"github.com/aalvaropc/upkeep/internal/github"
	"github.com/aalvaropc/upkeep/internal/gitx"
	"github.com/aalvaropc/upkeep/internal/learn"
	"github.com/aalvaropc/upkeep/internal/plan"
	"github.com/aalvaropc/upkeep/internal/policy"
	"github.com/aalvaropc/upkeep/internal/pull"
	"github.com/aalvaropc/upkeep/internal/queue"
	"github.com/aalvaropc/upkeep/internal/runtime"
)

type ownerFlags struct {
	repo     string
	dryRun   bool
	policy   string
	parallel int
	only     []int
	learn    bool
}

func parseNumbers(list string) ([]int, error) {
	var out []int
	for _, part := range strings.Split(list, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("run: %q is not a pull request number", part)
		}
		out = append(out, n)
	}
	return out, nil
}

func parseRunArgs(args []string, stderr io.Writer) (ownerFlags, string, string, error) {
	var f ownerFlags
	var only string
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&f.repo, "repo", "", "repository to work, as owner/name")
	fs.BoolVar(&f.dryRun, "dry-run", false, "detect and fix, but push, comment and merge nothing")
	fs.StringVar(&f.policy, "policy", "", "policy file; the built-in policy when empty")
	fs.IntVar(&f.parallel, "parallel", queue.DefaultParallel, "pull requests worked at once")
	fs.StringVar(&only, "pr", "", "work only these pull requests, as numbers separated by commas")
	fs.BoolVar(&f.learn, "learn", true, "remember verified fixes and reuse them when the same failure returns")
	if err := fs.Parse(args); err != nil {
		return ownerFlags{}, "", "", err
	}
	if fs.NArg() > 0 {
		return ownerFlags{}, "", "", fmt.Errorf("run takes no arguments, got %q", fs.Arg(0))
	}
	owner, name, ok := strings.Cut(f.repo, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return ownerFlags{}, "", "", errors.New("run: -repo owner/name is required")
	}
	if f.parallel < 1 {
		return ownerFlags{}, "", "", errors.New("run: -parallel must be at least 1")
	}
	numbers, err := parseNumbers(only)
	if err != nil {
		return ownerFlags{}, "", "", err
	}
	f.only = numbers
	return f, owner, name, nil
}

func fields(data any) map[string]any {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}

func progress(w io.Writer) queue.Emitter {
	return func(kind events.Kind, number int, data any) {
		m := fields(data)
		switch kind {
		case events.PRBlocker:
			fmt.Fprintf(w, "#%d blocked by %v\n", number, m["blocker"])
		case events.PRStep:
			if m["status"] != "running" {
				fmt.Fprintf(w, "#%d %v %v %v\n", number, m["step"], m["status"], m["detail"])
			}
		case events.Notice:
			if msg, ok := m["message"].(string); ok && msg != "" {
				fmt.Fprintln(w, msg)
			}
		}
	}
}

func runCommand(ctx context.Context, args []string, stdout, stderr io.Writer, environ []string) error {
	flags, owner, name, err := parseRunArgs(args, stderr)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(environ, func(kv string) bool { return strings.HasPrefix(kv, "UPKEEP_MODE=") }) {
		environ = append(slices.Clone(environ), "UPKEEP_MODE="+string(config.Live))
	}
	cfg, err := config.Load(environ)
	if err != nil {
		return err
	}
	if !cfg.GitHubToken.IsSet() && !flags.dryRun {
		return errors.New("run: GITHUB_TOKEN is required to push to and merge pull requests; a -dry-run of a public repository needs none")
	}
	pol := policy.Default()
	if flags.policy != "" {
		if pol, err = policy.Load(flags.policy); err != nil {
			return err
		}
	}
	logger := slog.New(slog.NewTextHandler(stderr, nil))
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return err
	}
	rt, err := runtime.New(ctx, runtime.Options{Config: cfg, Policy: pol, Repo: owner + "/" + name, Git: gitx.Git{}, CacheDir: filepath.Join(cfg.DataDir, "cache"), Logger: logger})
	if err != nil {
		return err
	}
	client := &github.Client{Token: cfg.GitHubToken}
	git := gitx.Git{}
	if cfg.GitHubToken.IsSet() {
		git = git.WithToken(cfg.GitHubToken.Reveal())
	}
	hub := forge.NewGitHub(client, git, owner, name, pol.Bots)
	hub.AllowForks = cfg.Sandbox == config.Docker
	base, err := hub.Base(ctx)
	if err != nil {
		return err
	}
	var selected forge.Forge = hub
	if len(flags.only) > 0 {
		selected = forge.Only{Forge: hub, Numbers: flags.only}
	}
	f := selected
	dry := &forge.DryRun{Forge: selected}
	if flags.dryRun {
		f = dry
	}
	var learned learn.Book
	if flags.learn {
		learned = &learn.Dir{Path: filepath.Join(cfg.DataDir, "learned")}
	}
	proc, err := rt.Processor(progress(stdout), learned)
	if err != nil {
		return err
	}
	work := filepath.Join(cfg.DataDir, "run", fmt.Sprintf("%s-%s-%s", owner, name, time.Now().UTC().Format("20060102T150405Z")))
	runner := queue.Runner{Forge: f, Processor: proc, Emit: progress(stdout), Work: work, Base: "origin/" + base, Parallel: flags.parallel}
	runner.Planner = plan.Planner{Git: git}
	fmt.Fprintf(stdout, "working the open pull requests of %s/%s into %s%s\n", owner, name, base, map[bool]string{true: " (dry run: nothing is pushed, commented or merged)", false: ""}[flags.dryRun])
	summary, err := runner.Run(ctx)
	if err != nil {
		return err
	}
	printSummary(stdout, summary)
	if flags.dryRun {
		for _, n := range dry.Notes {
			fmt.Fprintln(stdout, n)
		}
	}
	return nil
}

func printSummary(w io.Writer, s queue.Summary) {
	fmt.Fprintln(w)
	for _, it := range s.Items {
		actions := make([]string, 0, len(it.Actions))
		for _, a := range it.Actions {
			actions = append(actions, a.Kind+":"+a.Status)
		}
		reason := it.Result.Reason
		if it.Outcome == pull.Mergeable || it.Outcome == pull.Merged {
			reason = it.Result.Summary
		}
		fmt.Fprintf(w, "#%-4d %-12s %-14s %-24s %s\n", it.PR.Number, it.Outcome, it.Result.Blocker, strings.Join(actions, ","), reason)
	}
	printPlan(w, s.Plan)
	fmt.Fprintf(w, "\n%d model calls, $%.4f (the same tokens at the largest model's price: $%.4f)\n", s.Totals.Calls, float64(s.Totals.Charge.Micros)/1e6, float64(s.Totals.Charge.BaselineMicros)/1e6)
}
