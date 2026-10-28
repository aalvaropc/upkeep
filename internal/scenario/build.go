package scenario

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/aalvaropc/upkeep/internal/gitx"
)

type Builder struct {
	Git     gitx.Git
	Recipes fs.FS
}

type Built struct {
	Dir   string
	Fork  string
	Main  string
	Heads map[string]string
}

func (b Builder) Build(ctx context.Context, s Scenario, source, out string) (Built, error) {
	if err := s.Validate(); err != nil {
		return Built{}, err
	}
	if b.Recipes == nil {
		return Built{}, errors.New("scenario: no recipes to build from")
	}
	out, err := filepath.Abs(out)
	if err != nil {
		return Built{}, err
	}
	if entries, err := os.ReadDir(out); err == nil && len(entries) > 0 {
		return Built{}, fmt.Errorf("scenario: %s is not empty", out)
	}
	work, err := os.MkdirTemp("", "upkeep-scenario-")
	if err != nil {
		return Built{}, err
	}
	defer os.RemoveAll(work)
	dir := work + "/repo"
	if err := b.Git.Clone(ctx, source, dir); err != nil {
		return Built{}, err
	}
	if err := b.Git.Checkout(ctx, dir, s.Fork, false); err != nil {
		return Built{}, fmt.Errorf("scenario: fork commit %s: %w", s.Fork, err)
	}
	built := Built{Dir: out, Fork: s.Fork, Heads: map[string]string{}}
	if err := b.Git.Checkout(ctx, dir, s.Base, true); err != nil {
		return Built{}, err
	}
	for _, recipe := range s.Main {
		if err := b.apply(ctx, dir, recipe); err != nil {
			return Built{}, err
		}
	}
	if built.Main, err = b.Git.Head(ctx, dir); err != nil {
		return Built{}, err
	}
	built.Heads[s.Base] = built.Main
	for _, pr := range s.PRs {
		start := s.Fork
		if pr.Start() == OntoMain {
			start = built.Main
		}
		if err := b.Git.Checkout(ctx, dir, start, false); err != nil {
			return Built{}, err
		}
		if err := b.Git.Checkout(ctx, dir, pr.Branch, true); err != nil {
			return Built{}, err
		}
		if err := b.apply(ctx, dir, pr.Recipe); err != nil {
			return Built{}, fmt.Errorf("pull request %d: %w", pr.Number, err)
		}
		if built.Heads[pr.Branch], err = b.Git.Head(ctx, dir); err != nil {
			return Built{}, err
		}
	}
	if err := b.Git.InitBare(ctx, out); err != nil {
		return Built{}, err
	}
	branches := []string{s.Base}
	for _, pr := range s.PRs {
		branches = append(branches, pr.Branch)
	}
	for _, branch := range branches {
		if err := b.Git.Checkout(ctx, dir, built.Heads[branch], false); err != nil {
			return Built{}, err
		}
		if err := b.Git.Push(ctx, dir, out, branch); err != nil {
			return Built{}, err
		}
	}
	return built, nil
}

func (b Builder) apply(ctx context.Context, dir, recipe string) error {
	if recipe == "" || strings.ContainsAny(recipe, `/\`) || strings.HasPrefix(recipe, ".") {
		return fmt.Errorf("scenario: %q is not a recipe name", recipe)
	}
	root := path.Join("recipes", recipe)
	entries, err := fs.ReadDir(b.Recipes, root)
	if err != nil {
		return fmt.Errorf("scenario: recipe %q: %w", recipe, err)
	}
	var patches []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".patch") {
			patches = append(patches, e.Name())
		}
	}
	if len(patches) == 0 {
		return fmt.Errorf("scenario: recipe %q has no patches", recipe)
	}
	slices.Sort(patches)
	for _, name := range patches {
		data, err := fs.ReadFile(b.Recipes, path.Join(root, name))
		if err != nil {
			return err
		}
		if err := b.Git.Am(ctx, dir, string(data)); err != nil {
			return fmt.Errorf("scenario: recipe %q, %s: %w", recipe, name, err)
		}
	}
	return nil
}
