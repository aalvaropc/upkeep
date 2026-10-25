package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/aalvaropc/upkeep/internal/buildinfo"
	"github.com/aalvaropc/upkeep/internal/config"
	"github.com/aalvaropc/upkeep/internal/eval"
)

type runFlags struct {
	stackFlags
	runs int
	out  string
}

func parseRunFlags(name string, args []string, stderr io.Writer, pace float64, withOut bool) (runFlags, error) {
	var f runFlags
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	f.register(fs, pace)
	fs.IntVar(&f.runs, "runs", 1, "how many times to run the scenario")
	if withOut {
		fs.StringVar(&f.out, "out", "docs/evals", "directory the results and the summary are written to")
	}
	if err := fs.Parse(args); err != nil {
		return runFlags{}, err
	}
	if fs.NArg() > 0 {
		return runFlags{}, fmt.Errorf("%s takes no arguments, got %q", name, fs.Arg(0))
	}
	if err := f.validate(); err != nil {
		return runFlags{}, fmt.Errorf("%s: %w", name, err)
	}
	if f.runs < 1 {
		return runFlags{}, fmt.Errorf("%s: -runs must be at least 1", name)
	}
	return f, nil
}

func runScenario(ctx context.Context, st *stack, runs int) (eval.Report, error) {
	return eval.Run(ctx, st.manager, st.scenario, runs, string(st.runtime.Mode()), buildinfo.Version())
}

var errDifferences = errors.New("some pull requests did not end as the scenario expects")

func recordCommand(ctx context.Context, args []string, stdout, stderr io.Writer, environ []string) error {
	flags, err := parseRunFlags("record", args, stderr, 0, false)
	if err != nil {
		return err
	}
	if flags.fixtures == "" {
		return errors.New("record: -fixtures DIR is required")
	}
	st, err := openStack(ctx, stderr, environ, flags.stackFlags, config.Record)
	if err != nil {
		return err
	}
	defer st.close()
	report, err := runScenario(ctx, st, flags.runs)
	if err != nil {
		return err
	}
	fmt.Fprint(stdout, report.Markdown())
	if !report.AllPassed() {
		return fmt.Errorf("%w (the exchanges were still recorded)", errDifferences)
	}
	return nil
}

func evalCommand(ctx context.Context, args []string, stdout, stderr io.Writer, environ []string) error {
	flags, err := parseRunFlags("eval", args, stderr, 0, true)
	if err != nil {
		return err
	}
	st, err := openStack(ctx, stderr, environ, flags.stackFlags, "")
	if err != nil {
		return err
	}
	defer st.close()
	report, err := runScenario(ctx, st, flags.runs)
	if err != nil {
		return err
	}
	if err := writeReport(flags.out, report); err != nil {
		return err
	}
	fmt.Fprint(stdout, report.Markdown())
	if !report.AllPassed() {
		return errDifferences
	}
	return nil
}

func writeReport(dir string, report eval.Report) error {
	results := filepath.Join(dir, "results")
	if err := os.MkdirAll(results, 0o750); err != nil {
		return err
	}
	data, err := report.JSON()
	if err != nil {
		return err
	}
	name := fmt.Sprintf("%s-%s.json", report.At.Format("20060102T150405Z"), report.Mode)
	if err := os.WriteFile(filepath.Join(results, name), append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "latest.md"), []byte(report.Markdown()), 0o600)
}
