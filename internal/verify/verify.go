package verify

import (
	"fmt"
	"strings"

	"github.com/aalvaropc/upkeep/internal/gitx"
	"github.com/aalvaropc/upkeep/internal/policy"
	"github.com/aalvaropc/upkeep/internal/sandbox"
)

type Check struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Pass   bool   `json:"pass"`
	Detail string `json:"detail,omitempty"`
}

type Checklist []Check

func (c Checklist) Passed() bool {
	if len(c) == 0 {
		return false
	}
	for _, check := range c {
		if !check.Pass {
			return false
		}
	}
	return true
}

func (c Checklist) Failed() []Check {
	var failed []Check
	for _, check := range c {
		if !check.Pass {
			failed = append(failed, check)
		}
	}
	return failed
}

type Input struct {
	Policy      policy.Policy
	Changes     []gitx.FileChange
	Report      sandbox.Report
	Comments    int
	Unformatted []string
	Dependency  bool
}

func dependencyFile(path string) bool {
	return path == "go.mod" || path == "go.sum"
}

func Verify(in Input) Checklist {
	files := len(in.Changes)
	lines := 0
	var tests, foreign []string
	for _, c := range in.Changes {
		lines += c.Added + c.Deleted
		if c.IsTest() {
			tests = append(tests, c.Path)
		}
		if !strings.HasSuffix(c.Path, ".go") && (!in.Dependency || !dependencyFile(c.Path)) {
			foreign = append(foreign, c.Path)
		}
	}
	return Checklist{
		result("build", "go build passes", in.Report.Build),
		result("vet", "go vet passes", in.Report.Vet),
		result("tests", "tests pass", in.Report.Test),
		{
			ID:     "files",
			Label:  fmt.Sprintf("changes at most %d files", in.Policy.MaxFilesChanged),
			Pass:   files <= in.Policy.MaxFilesChanged,
			Detail: fmt.Sprintf("%d files changed", files),
		},
		{
			ID:     "lines",
			Label:  fmt.Sprintf("changes at most %d lines", in.Policy.MaxLinesChanged),
			Pass:   lines <= in.Policy.MaxLinesChanged,
			Detail: fmt.Sprintf("%d lines changed", lines),
		},
		{
			ID:     "tests-untouched",
			Label:  "does not edit, delete or skip tests",
			Pass:   len(tests) == 0,
			Detail: strings.Join(tests, ", "),
		},
		{
			ID:     "go-only",
			Label:  goOnlyLabel(in.Dependency),
			Pass:   len(foreign) == 0,
			Detail: strings.Join(foreign, ", "),
		},
		{
			ID:     "no-comments",
			Label:  "adds no comments",
			Pass:   in.Comments == 0,
			Detail: commentDetail(in.Comments),
		},
		{
			ID:     "formatted",
			Label:  "leaves the code formatted",
			Pass:   len(in.Unformatted) == 0,
			Detail: strings.Join(in.Unformatted, ", "),
		},
	}
}

func goOnlyLabel(dependency bool) string {
	if dependency {
		return "changes only Go files and the module files"
	}
	return "changes only Go files"
}

func commentDetail(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("%d added", n)
}

func result(id, label string, r sandbox.Result) Check {
	c := Check{ID: id, Label: label, Pass: r.OK()}
	switch {
	case r.Skipped:
		c.Detail = "not run"
	case r.TimedOut:
		c.Detail = "timed out"
	case !c.Pass:
		c.Detail = fmt.Sprintf("exit code %d", r.ExitCode)
	}
	return c
}
