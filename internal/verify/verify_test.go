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
		Policy:  policy.Default(),
		Changes: []gitx.FileChange{{Path: "auth/token.go", Added: 4, Deleted: 2}, {Path: "go.mod", Added: 1, Deleted: 1}},
	})
	if !c.Passed() || len(c.Failed()) != 0 {
		t.Fatalf("checklist = %+v", c)
	}
	if got := byID(c)["lines"].Detail; got != "8 lines changed" {
		t.Fatalf("lines detail = %q", got)
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
		Report: sandbox.Report{Test: sandbox.Result{ExitCode: 1}, Vet: sandbox.Result{TimedOut: true}},
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
	} {
		if checks[id].Pass || checks[id].Detail != detail {
			t.Errorf("%s = %+v, want a failure with detail %q", id, checks[id], detail)
		}
	}
	if !checks["build"].Pass {
		t.Error("build failed although it exited 0")
	}
	if len(c.Failed()) != 5 {
		t.Errorf("Failed() = %d checks, want 5", len(c.Failed()))
	}
}

func TestEmptyChecklistDoesNotPass(t *testing.T) {
	if (Checklist{}).Passed() {
		t.Fatal("an empty checklist passed")
	}
}
