package engine

import (
	"errors"
	"testing"

	"github.com/aalvaropc/upkeep/internal/pull"
)

func TestHappyPathTransitions(t *testing.T) {
	path := []State{Queued, Detecting, Forecasting, Detecting, Fixing, Verifying, Detecting, Acting, Done}
	s := path[0]
	for _, next := range path[1:] {
		var err error
		if s, err = Transition(s, next); err != nil {
			t.Fatal(err)
		}
	}
	if s != Done {
		t.Fatalf("final state = %s", s)
	}
}

func TestForbiddenTransitions(t *testing.T) {
	for _, tt := range [][2]State{
		{Queued, Fixing},
		{Detecting, Verifying},
		{Fixing, Acting},
		{Acting, Fixing},
		{Done, Detecting},
		{Forecasting, Acting},
	} {
		if CanTransition(tt[0], tt[1]) {
			t.Errorf("CanTransition(%s, %s) = true", tt[0], tt[1])
		}
		if s, err := Transition(tt[0], tt[1]); err == nil || s != tt[0] {
			t.Errorf("Transition(%s, %s) = %s, %v", tt[0], tt[1], s, err)
		}
	}
}

func TestEveryStateHasTransitions(t *testing.T) {
	for _, s := range []State{Queued, Detecting, Forecasting, Fixing, Verifying, Acting, Done} {
		if _, ok := transitions[s]; !ok {
			t.Errorf("state %s has no transition entry", s)
		}
	}
}

func TestColumnOf(t *testing.T) {
	tests := []struct {
		state   State
		outcome pull.Outcome
		want    Column
	}{
		{Queued, pull.Pending, Blocked},
		{Detecting, pull.Pending, Working},
		{Forecasting, pull.Pending, Working},
		{Fixing, pull.Pending, Working},
		{Verifying, pull.Pending, Working},
		{Acting, pull.Pending, Verified},
		{Done, pull.Merged, DoneColumn},
		{Done, pull.NeedsHuman, DoneColumn},
		{Acting, pull.Mergeable, DoneColumn},
	}
	for _, tt := range tests {
		if got := ColumnOf(tt.state, tt.outcome); got != tt.want {
			t.Errorf("ColumnOf(%s, %q) = %s, want %s", tt.state, tt.outcome, got, tt.want)
		}
	}
}

func TestNewAppliesDefaultLimits(t *testing.T) {
	if got := New(Deps{}, Limits{}).Limits(); got != DefaultLimits {
		t.Fatalf("limits = %+v, want %+v", got, DefaultLimits)
	}
	if got := New(Deps{}, Limits{PRsPerRun: 2, RunsPerServer: 1}).Limits(); got.PRsPerRun != 2 || got.RunsPerServer != 1 {
		t.Fatalf("limits = %+v", got)
	}
	if err := New(Deps{}, Limits{}).Work(t.Context(), "r1"); !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("Work() = %v, want ErrUnsupported until the runner exists", err)
	}
}
