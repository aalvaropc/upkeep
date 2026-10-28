package forge

import (
	"context"
	"fmt"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/aalvaropc/upkeep/internal/gitx"
	"github.com/aalvaropc/upkeep/internal/pull"
	"github.com/aalvaropc/upkeep/internal/scenario"
)

type record struct {
	req      pull.Request
	state    State
	mergeSHA string
	comments []Comment
}

type Local struct {
	Git  gitx.Git
	Repo string
	Base string
	Now  func() time.Time

	mu      sync.Mutex
	records []*record
}

func NewLocal(g gitx.Git, repo string, s scenario.Scenario) *Local {
	l := &Local{Git: g, Repo: repo, Base: s.Base, Now: time.Now}
	for _, pr := range s.PRs {
		l.records = append(l.records, &record{req: pr.Request(s.Base), state: Open})
	}
	slices.SortFunc(l.records, func(a, b *record) int { return a.req.Number - b.req.Number })
	return l
}

func CopyLocal(ctx context.Context, g gitx.Git, template, dst string, s scenario.Scenario) (*Local, error) {
	if err := g.CloneBare(ctx, template, dst); err != nil {
		return nil, err
	}
	return NewLocal(g, dst, s), nil
}

func (l *Local) find(number int) (*record, error) {
	for _, r := range l.records {
		if r.req.Number == number {
			return r, nil
		}
	}
	return nil, fmt.Errorf("%w: #%d", ErrUnknown, number)
}

func (l *Local) head(ctx context.Context, branch string) string {
	sha, err := l.Git.RevParse(ctx, l.Repo, "refs/heads/"+branch)
	if err != nil {
		return ""
	}
	return sha
}

func (l *Local) Pulls(ctx context.Context) ([]pull.Request, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []pull.Request
	for _, r := range l.records {
		if r.state != Open {
			continue
		}
		req := r.req
		req.HeadSHA = l.head(ctx, req.Branch)
		out = append(out, req)
	}
	return out, nil
}

func (l *Local) Views(ctx context.Context) []View {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]View, 0, len(l.records))
	for _, r := range l.records {
		req := r.req
		req.HeadSHA = l.head(ctx, req.Branch)
		out = append(out, View{Request: req, State: r.state, MergeSHA: r.mergeSHA, Comments: slices.Clone(r.comments)})
	}
	return out
}

func (l *Local) Prepare(ctx context.Context, pr pull.Request, dir string) error {
	l.mu.Lock()
	r, err := l.find(pr.Number)
	l.mu.Unlock()
	if err != nil {
		return err
	}
	if err := l.Git.Clone(ctx, l.Repo, dir); err != nil {
		return err
	}
	if err := l.Git.Checkout(ctx, dir, "origin/"+r.req.Branch, false); err != nil {
		return err
	}
	return l.Git.Checkout(ctx, dir, r.req.Branch, true)
}

func (l *Local) Push(ctx context.Context, dir string, pr pull.Request) error {
	l.mu.Lock()
	r, err := l.find(pr.Number)
	l.mu.Unlock()
	if err != nil {
		return err
	}
	return l.Git.Push(ctx, dir, "origin", r.req.Branch)
}

func (l *Local) Merge(ctx context.Context, pr pull.Request) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	r, err := l.find(pr.Number)
	if err != nil {
		return "", err
	}
	if r.state != Open {
		return "", fmt.Errorf("%w: #%d", ErrClosed, pr.Number)
	}
	work, err := os.MkdirTemp("", "upkeep-merge-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(work)
	dir := work + "/repo"
	if err := l.Git.Clone(ctx, l.Repo, dir); err != nil {
		return "", err
	}
	if err := l.Git.Checkout(ctx, dir, l.Base, true); err != nil {
		return "", err
	}
	res, err := l.Git.Merge(ctx, dir, "origin/"+r.req.Branch)
	if err != nil {
		return "", err
	}
	if !res.Clean {
		_ = l.Git.MergeAbort(ctx, dir)
		return "", fmt.Errorf("%w: #%d", ErrNotMergeable, pr.Number)
	}
	message := fmt.Sprintf("Merge pull request #%d from %s\n\n%s", r.req.Number, r.req.Branch, r.req.Title)
	sha, err := l.Git.Commit(ctx, dir, message)
	if err != nil {
		return "", err
	}
	if err := l.Git.Push(ctx, dir, "origin", l.Base); err != nil {
		return "", err
	}
	r.state, r.mergeSHA = Merged, sha
	return sha, nil
}

func (l *Local) Comment(_ context.Context, pr pull.Request, c Comment) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	r, err := l.find(pr.Number)
	if err != nil {
		return err
	}
	if c.At.IsZero() {
		c.At = l.Now().UTC()
	}
	if i := slices.IndexFunc(r.comments, func(old Comment) bool { return c.Kind != "" && old.Kind == c.Kind }); i >= 0 {
		r.comments[i] = c
		return nil
	}
	r.comments = append(r.comments, c)
	return nil
}
