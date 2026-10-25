package strategy

import (
	"context"
	"fmt"

	"github.com/aalvaropc/upkeep/internal/pull"
)

type Job struct {
	PR      pull.Request
	Dir     string
	Blocker pull.Blocker
	Attempt int
}

type Strategy interface {
	Blocker() pull.Blocker
	Apply(ctx context.Context, job *Job) error
}

type Registry struct {
	byBlocker map[pull.Blocker]Strategy
}

func NewRegistry() *Registry {
	return &Registry{byBlocker: make(map[pull.Blocker]Strategy)}
}

func (r *Registry) Register(s Strategy) error {
	b := s.Blocker()
	if _, ok := r.byBlocker[b]; ok {
		return fmt.Errorf("a strategy for %q is already registered", b)
	}
	r.byBlocker[b] = s
	return nil
}

func (r *Registry) For(b pull.Blocker) (Strategy, bool) {
	s, ok := r.byBlocker[b]
	return s, ok
}
