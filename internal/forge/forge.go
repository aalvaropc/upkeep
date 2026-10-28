package forge

import (
	"context"
	"errors"
	"time"

	"github.com/aalvaropc/upkeep/internal/pull"
)

var (
	ErrUnknown      = errors.New("unknown pull request")
	ErrClosed       = errors.New("the pull request is not open")
	ErrNotMergeable = errors.New("the pull request does not merge cleanly into the base branch")
	ErrDryRun       = errors.New("dry run: nothing was changed")
)

type State string

const (
	Open   State = "open"
	Merged State = "merged"
)

type Comment struct {
	Kind string    `json:"kind"`
	Body string    `json:"body"`
	At   time.Time `json:"at"`
}

type View struct {
	pull.Request
	State    State     `json:"state"`
	MergeSHA string    `json:"merge_sha,omitempty"`
	Comments []Comment `json:"comments,omitempty"`
}

type Forge interface {
	Pulls(ctx context.Context) ([]pull.Request, error)
	Prepare(ctx context.Context, pr pull.Request, dir string) error
	Push(ctx context.Context, dir string, pr pull.Request) error
	Merge(ctx context.Context, pr pull.Request) (string, error)
	Comment(ctx context.Context, pr pull.Request, c Comment) error
}
