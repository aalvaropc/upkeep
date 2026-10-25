package detect

import (
	"context"

	"github.com/aalvaropc/upkeep/internal/gitx"
	"github.com/aalvaropc/upkeep/internal/pull"
	"github.com/aalvaropc/upkeep/internal/sandbox"
	"github.com/aalvaropc/upkeep/internal/stub"
)

type Workspace struct {
	Dir  string
	Base string
	PR   pull.Request
}

type Detector struct {
	Git    gitx.Git
	Runner sandbox.Runner
}

var Order = []pull.Blocker{
	pull.Behind,
	pull.ConflictSmall,
	pull.ConflictLarge,
	pull.Build,
	pull.Test,
	pull.Flaky,
	pull.Format,
	pull.Clean,
}

func (d Detector) Detect(_ context.Context, _ Workspace) (pull.Blocker, error) {
	return "", stub.Err("detect blockers")
}
