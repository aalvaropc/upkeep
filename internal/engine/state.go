package engine

import (
	"fmt"
	"slices"

	"github.com/aalvaropc/upkeep/internal/pull"
)

type State string

const (
	Queued      State = "queued"
	Detecting   State = "detecting"
	Forecasting State = "forecasting"
	Fixing      State = "fixing"
	Verifying   State = "verifying"
	Acting      State = "acting"
	Done        State = "done"
)

var transitions = map[State][]State{
	Queued:      {Detecting, Done},
	Detecting:   {Forecasting, Fixing, Acting, Done},
	Forecasting: {Detecting, Fixing},
	Fixing:      {Verifying, Done},
	Verifying:   {Detecting, Fixing, Acting, Done},
	Acting:      {Done},
	Done:        {},
}

func CanTransition(from, to State) bool {
	return slices.Contains(transitions[from], to)
}

func Transition(from, to State) (State, error) {
	if !CanTransition(from, to) {
		return from, fmt.Errorf("a pull request cannot go from %s to %s", from, to)
	}
	return to, nil
}

type Column string

const (
	Blocked    Column = "blocked"
	Working    Column = "working"
	Verified   Column = "verified"
	DoneColumn Column = "done"
)

func ColumnOf(s State, o pull.Outcome) Column {
	switch {
	case s == Done || o != pull.Pending:
		return DoneColumn
	case s == Acting:
		return Verified
	case s == Queued:
		return Blocked
	default:
		return Working
	}
}
