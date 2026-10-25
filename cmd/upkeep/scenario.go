package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"

	"github.com/aalvaropc/upkeep/internal/gitx"
	"github.com/aalvaropc/upkeep/internal/scenario"
	"github.com/aalvaropc/upkeep/scenarios"
)

func scenarioCommand(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] != "build" {
		return errors.New("usage: upkeep scenario build -out DIR [-source REPOSITORY]")
	}
	fs := flag.NewFlagSet("scenario build", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("out", "", "directory to create the bare repository in; it must be empty or missing")
	source := fs.String("source", "", "repository or path to build from; defaults to the scenario's repository")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *out == "" {
		return errors.New("scenario build: -out is required")
	}
	s, err := scenarios.Notes()
	if err != nil {
		return err
	}
	from := s.Repository
	if *source != "" {
		from = *source
	}
	built, err := scenario.Builder{Git: gitx.Git{}, Recipes: scenarios.NotesFS()}.Build(ctx, s, from, *out)
	if err != nil {
		return err
	}
	branches := make([]string, 0, len(built.Heads))
	for branch := range built.Heads {
		branches = append(branches, branch)
	}
	sort.Strings(branches)
	fmt.Fprintf(stdout, "built %s in %s\n", s.Name, built.Dir)
	for _, branch := range branches {
		fmt.Fprintf(stdout, "  %-62s %s\n", branch, built.Heads[branch][:12])
	}
	return nil
}
