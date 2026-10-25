package verify

import (
	"testing"

	"github.com/aalvaropc/upkeep/internal/gitx"
	"github.com/aalvaropc/upkeep/internal/policy"
	"github.com/aalvaropc/upkeep/internal/sandbox"
)

func byID(c Checklist) map[string]Check {
	m := make(map[string]Check, len(c))
	for _, check := range c {
		m[check.ID] = check
	}
	return m
}

func TestVerifyPasses(t *testing.T) {
	c := Verify(Input{
		Policy:     policy.Default(),
		Changes:    []gitx.FileChange{{Path: "auth/token.go", Added: 4, Deleted: 2}, {Path: "go.mod", Added: 1, Deleted: 1}},
		Dependency: true,
	})
	if !c.Passed() || len(c.Failed()) != 0 {
		t.Fatalf("checklist = %+v", c)
	}
	if got := byID(c)["lines"].Detail; got != "8 lines changed" {
		t.Fatalf("lines detail = %q", got)
	}
	if byID(c)["go-only"].Label != "changes only Go files and the module files" {
		t.Fatalf("label = %q", byID(c)["go-only"].Label)
	}
}

func TestOnlyGoFiles(t *testing.T) {
	c := Verify(Input{
		Policy:  policy.Default(),
		Changes: []gitx.FileChange{{Path: "auth/token.go", Added: 1}, {Path: "go.mod", Added: 1}, {Path: ".github/workflows/ci.yml", Added: 1}, {Path: "README.md", Added: 1}},
	})
	goOnly := byID(c)["go-only"]
	if goOnly.Pass || goOnly.Detail != "go.mod, .github/workflows/ci.yml, README.md" {
		t.Fatalf("go-only = %+v", goOnly)
	}
	dep := Verify(Input{Policy: policy.Default(), Dependency: true, Changes: []gitx.FileChange{{Path: "go.mod", Added: 1}, {Path: "go.sum", Added: 1}, {Path: "ci.yml", Added: 1}}})
	if got := byID(dep)["go-only"]; got.Pass || got.Detail != "ci.yml" {
		t.Fatalf("a dependency change may touch only the module files: %+v", got)
	}
}

func TestVerifyFailures(t *testing.T) {
	p := policy.Default()
	p.MaxFilesChanged = 1
	p.MaxLinesChanged = 5
	c := Verify(Input{
		Policy: p,
		Changes: []gitx.FileChange{
			{Path: "auth/token.go", Added: 3},
			{Path: "auth/token_test.go", Deleted: 4},
		},
		Report:      sandbox.Report{Test: sandbox.Result{ExitCode: 1}, Vet: sandbox.Result{TimedOut: true}},
		Comments:    2,
		Unformatted: []string{"auth/token.go"},
	})
	if c.Passed() {
		t.Fatal("a failing checklist passed")
	}
	checks := byID(c)
	for id, detail := range map[string]string{
		"tests":           "exit code 1",
		"vet":             "timed out",
		"files":           "2 files changed",
		"lines":           "7 lines changed",
		"tests-untouched": "auth/token_test.go",
		"no-comments":     "2 added",
		"formatted":       "auth/token.go",
	} {
		if checks[id].Pass || checks[id].Detail != detail {
			t.Errorf("%s = %+v, want a failure with detail %q", id, checks[id], detail)
		}
	}
	if !checks["build"].Pass || !checks["go-only"].Pass {
		t.Error("build and go-only should pass here")
	}
	if len(c.Failed()) != 7 {
		t.Errorf("Failed() = %d checks, want 7", len(c.Failed()))
	}
}

func TestSkippedStepsAreNotPassing(t *testing.T) {
	c := Verify(Input{Policy: policy.Default(), Report: sandbox.Report{Build: sandbox.Result{ExitCode: 1}, Vet: sandbox.Result{Skipped: true}, Test: sandbox.Result{Skipped: true}}})
	checks := byID(c)
	if checks["vet"].Pass || checks["vet"].Detail != "not run" || checks["tests"].Pass {
		t.Fatalf("skipped steps = %+v %+v", checks["vet"], checks["tests"])
	}
}

func TestEmptyChecklistDoesNotPass(t *testing.T) {
	if (Checklist{}).Passed() {
		t.Fatal("an empty checklist passed")
	}
}
