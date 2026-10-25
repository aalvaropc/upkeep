package strategy

import (
	"context"
	"testing"

	"github.com/aalvaropc/upkeep/internal/pull"
)

type format struct{}

func (format) Blocker() pull.Blocker { return pull.Format }

func (format) Apply(context.Context, *Job) error { return nil }

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(format{}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(format{}); err == nil {
		t.Fatal("registering a second strategy for the same blocker returned no error")
	}
	if s, ok := r.For(pull.Format); !ok || s.Blocker() != pull.Format {
		t.Fatalf("For(format) = %v, %v", s, ok)
	}
	if _, ok := r.For(pull.Build); ok {
		t.Fatal("For found a strategy that was never registered")
	}
}
