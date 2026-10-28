package forge

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/aalvaropc/upkeep/internal/github"
	"github.com/aalvaropc/upkeep/internal/gitx"
	"github.com/aalvaropc/upkeep/internal/pull"
)

type GitHub struct {
	Client   *github.Client
	Git      gitx.Git
	Owner    string
	Repo     string
	CloneURL string
	Bots     []string

	base  string
	heads map[int]string
}

func NewGitHub(c *github.Client, g gitx.Git, owner, repo string, bots []string) *GitHub {
	return &GitHub{Client: c, Git: g, Owner: owner, Repo: repo, Bots: bots, CloneURL: fmt.Sprintf("https://github.com/%s/%s.git", owner, repo), heads: map[int]string{}}
}

func (g *GitHub) fullName() string { return g.Owner + "/" + g.Repo }

func (g *GitHub) defaultBranch(ctx context.Context) (string, error) {
	if g.base == "" {
		branch, err := g.Client.DefaultBranch(ctx, g.Owner, g.Repo)
		if err != nil {
			return "", err
		}
		g.base = branch
	}
	return g.base, nil
}

func (g *GitHub) Base(ctx context.Context) (string, error) { return g.defaultBranch(ctx) }

func (g *GitHub) kindOf(u github.User) pull.AuthorKind {
	if strings.EqualFold(u.Type, "Bot") || slices.Contains(g.Bots, u.Login) {
		return pull.Bot
	}
	return pull.Person
}

func (g *GitHub) Pulls(ctx context.Context) ([]pull.Request, error) {
	base, err := g.defaultBranch(ctx)
	if err != nil {
		return nil, err
	}
	list, err := g.Client.ListOpenPulls(ctx, g.Owner, g.Repo)
	if err != nil {
		return nil, err
	}
	var out []pull.Request
	for _, p := range list {
		if p.Draft || p.Base.Ref != base || p.Head.Repo == nil || !strings.EqualFold(p.Head.Repo.FullName, g.fullName()) {
			continue
		}
		req := pull.Request{
			Number:  p.Number,
			Title:   p.Title,
			Author:  pull.Author{Login: p.User.Login, Kind: g.kindOf(p.User)},
			Base:    p.Base.Ref,
			Branch:  p.Head.Ref,
			HeadSHA: p.Head.SHA,
			Kind:    pull.Change,
			Bump:    BumpFromPull(p.Title, p.Head.Ref, p.Body),
		}
		for _, l := range p.Labels {
			req.Labels = append(req.Labels, l.Name)
		}
		if req.Bump != nil {
			req.Kind = pull.Dependency
		}
		g.heads[p.Number] = p.NodeID
		out = append(out, req)
	}
	slices.SortFunc(out, func(a, b pull.Request) int { return a.Number - b.Number })
	return out, nil
}

func (g *GitHub) Prepare(ctx context.Context, pr pull.Request, dir string) error {
	if err := g.Git.Clone(ctx, g.CloneURL, dir); err != nil {
		return err
	}
	if err := g.Git.Checkout(ctx, dir, "origin/"+pr.Branch, false); err != nil {
		return err
	}
	return g.Git.Checkout(ctx, dir, pr.Branch, true)
}

func (g *GitHub) Push(ctx context.Context, dir string, pr pull.Request) error {
	return g.Git.Push(ctx, dir, "origin", pr.Branch)
}

func commentMarker(kind string) string { return "<!-- upkeep:" + kind + " -->" }

func (g *GitHub) Comment(ctx context.Context, pr pull.Request, c Comment) error {
	if c.Kind == "" {
		return g.Client.Comment(ctx, g.Owner, g.Repo, pr.Number, c.Body)
	}
	marker := commentMarker(c.Kind)
	body := marker + "\n" + c.Body
	existing, err := g.Client.ListComments(ctx, g.Owner, g.Repo, pr.Number)
	if err != nil {
		return err
	}
	for _, old := range slices.Backward(existing) {
		if !strings.HasPrefix(strings.TrimSpace(old.Body), marker) {
			continue
		}
		if old.Body == body {
			return nil
		}
		err := g.Client.EditComment(ctx, g.Owner, g.Repo, old.ID, body)
		var apiErr *github.Error
		if errors.As(err, &apiErr) && (apiErr.Status == http.StatusForbidden || apiErr.Status == http.StatusNotFound) {
			continue
		}
		return err
	}
	return g.Client.Comment(ctx, g.Owner, g.Repo, pr.Number, body)
}

func (g *GitHub) Merge(ctx context.Context, pr pull.Request) (string, error) {
	current, err := g.Client.GetPull(ctx, g.Owner, g.Repo, pr.Number)
	if err != nil {
		return "", err
	}
	node := current.NodeID
	if node == "" {
		node = g.heads[pr.Number]
	}
	err = g.Client.EnableAutoMerge(ctx, node)
	switch {
	case err == nil:
		return "", nil
	case errors.Is(err, github.ErrAutoMergeClean):
	default:
		return "", fmt.Errorf("%w: %w", ErrNotMergeable, err)
	}
	sha, err := g.Client.Merge(ctx, g.Owner, g.Repo, pr.Number, current.Head.SHA, current.Title)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrNotMergeable, err)
	}
	return sha, nil
}

type DryRun struct {
	Forge
	Notes []string

	mu sync.Mutex
}

func (d *DryRun) note(format string, args ...any) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Notes = append(d.Notes, fmt.Sprintf(format, args...))
	return ErrDryRun
}

func (d *DryRun) Push(_ context.Context, _ string, pr pull.Request) error {
	return d.note("would push the fix to %s (#%d)", pr.Branch, pr.Number)
}

func (d *DryRun) Merge(_ context.Context, pr pull.Request) (string, error) {
	return "", d.note("would merge #%d", pr.Number)
}

func (d *DryRun) Comment(_ context.Context, pr pull.Request, c Comment) error {
	return d.note("would post a %s comment on #%d", c.Kind, pr.Number)
}

type Only struct {
	Forge
	Numbers []int
}

func (o Only) Pulls(ctx context.Context) ([]pull.Request, error) {
	all, err := o.Forge.Pulls(ctx)
	if err != nil {
		return nil, err
	}
	var out []pull.Request
	for _, p := range all {
		if slices.Contains(o.Numbers, p.Number) {
			out = append(out, p)
		}
	}
	return out, nil
}
