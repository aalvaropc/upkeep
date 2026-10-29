package detect

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/aalvaropc/upkeep/internal/gitx"
	"github.com/aalvaropc/upkeep/internal/policy"
	"github.com/aalvaropc/upkeep/internal/pull"
	"github.com/aalvaropc/upkeep/internal/sandbox"
)

type Workspace struct {
	Dir  string
	Base string
}

type Diagnosis struct {
	Blocker     pull.Blocker
	Behind      bool
	Merge       gitx.MergeResult
	Report      sandbox.Report
	Download    sandbox.Result
	Unformatted []string
	FlakyTests  []string
	FlakyRerun  sandbox.Result
}

func (d Diagnosis) Failure() (sandbox.Result, bool) {
	if !d.Download.OK() && len(d.Download.Args) > 0 {
		return d.Download, true
	}
	return d.Report.FirstFailure()
}

type Detector struct {
	Git    gitx.Git
	Runner sandbox.Runner
	Policy policy.Policy
}

var failedTest = regexp.MustCompile(`(?m)^--- FAIL: ([A-Za-z0-9_]+)`)

func FailedTests(output string) []string {
	var names []string
	for _, m := range failedTest.FindAllStringSubmatch(output, -1) {
		if !slices.Contains(names, m[1]) {
			names = append(names, m[1])
		}
	}
	return names
}

func TestPattern(names []string) string {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = regexp.QuoteMeta(n)
	}
	return "^(" + strings.Join(quoted, "|") + ")$"
}

func (d Detector) Detect(ctx context.Context, ws Workspace) (Diagnosis, error) {
	if d.Runner == nil {
		return Diagnosis{}, errors.New("detect: no runner")
	}
	var diag Diagnosis
	if _, err := os.Stat(filepath.Join(ws.Dir, "go.mod")); errors.Is(err, fs.ErrNotExist) {
		diag.Blocker = pull.Unsupported
		return diag, nil
	}
	ancestor, err := d.Git.IsAncestor(ctx, ws.Dir, ws.Base, "HEAD")
	if err != nil {
		return diag, fmt.Errorf("checking whether the branch is behind %s: %w", ws.Base, err)
	}
	diag.Behind = !ancestor

	if diag.Behind {
		merge, err := d.Git.Merge(ctx, ws.Dir, ws.Base)
		if err != nil {
			return diag, fmt.Errorf("merging %s: %w", ws.Base, err)
		}
		diag.Merge = merge
		if !merge.Clean {
			diag.Blocker = pull.ConflictLarge
			if d.small(merge) {
				diag.Blocker = pull.ConflictSmall
			}
			return diag, nil
		}
	}

	if diag.Download, err = d.Runner.Download(ctx, ws.Dir); err != nil {
		return diag, err
	}
	if !diag.Download.OK() {
		diag.Blocker = pull.Build
		return diag, nil
	}
	if diag.Report, err = d.Runner.Check(ctx, ws.Dir); err != nil {
		return diag, err
	}
	switch {
	case !diag.Report.Build.OK() || !diag.Report.Vet.OK():
		diag.Blocker = pull.Build
		return diag, nil
	case !diag.Report.Test.OK():
		if diag, err = d.classifyTestFailure(ctx, ws, diag); err != nil || diag.Blocker == pull.Test {
			return diag, err
		}
	}

	if diag.Unformatted, err = d.Runner.Unformatted(ctx, ws.Dir); err != nil {
		return diag, err
	}
	switch {
	case len(diag.Unformatted) > 0:
		diag.Blocker = pull.Format
	case diag.Behind:
		diag.Blocker = pull.Behind
	case len(diag.FlakyTests) > 0:
		diag.Blocker = pull.Flaky
	default:
		diag.Blocker = pull.Clean
	}
	return diag, nil
}

func (d Detector) small(m gitx.MergeResult) bool {
	return m.Hunks() <= d.Policy.MaxConflictHunks && m.LargestHunk() <= d.Policy.MaxHunkLines
}

func (d Detector) classifyTestFailure(ctx context.Context, ws Workspace, diag Diagnosis) (Diagnosis, error) {
	diag.Blocker = pull.Test
	if diag.Report.Test.TimedOut {
		return diag, nil
	}
	names := FailedTests(diag.Report.Test.Output)
	if len(names) == 0 {
		return diag, nil
	}
	rerun, err := d.Runner.Tests(ctx, ws.Dir, TestPattern(names))
	if err != nil {
		return diag, err
	}
	diag.FlakyRerun = rerun
	if rerun.OK() {
		diag.Blocker = pull.Flaky
		diag.FlakyTests = names
	}
	return diag, nil
}
