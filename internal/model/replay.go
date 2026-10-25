package model

import (
	"context"

	"github.com/aalvaropc/upkeep/internal/fixture"
	"github.com/aalvaropc/upkeep/internal/stub"
)

const FixtureKind = "model"

type Recorder struct {
	Inner    Model
	Fixtures fixture.Store
}

func (r *Recorder) Complete(_ context.Context, _ Request) (Response, error) {
	return Response{}, stub.Err("record model exchange")
}

type Replayer struct {
	Fixtures fixture.Store
}

func (r *Replayer) Complete(_ context.Context, _ Request) (Response, error) {
	return Response{}, stub.Err("replay model exchange")
}
