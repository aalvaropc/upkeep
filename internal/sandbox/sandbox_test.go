package sandbox

import (
	"strings"
	"testing"
	"time"
)

func TestExecSuccess(t *testing.T) {
	res, err := Exec{}.Run(t.Context(), Spec{Dir: t.TempDir(), Args: []string{"sh", "-c", "pwd; echo done"}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK() || !strings.Contains(res.Output, "done") {
		t.Fatalf("result = %+v", res)
	}
}

func TestExecFailureKeepsOutputAndCode(t *testing.T) {
	res, err := Exec{}.Run(t.Context(), Spec{Args: []string{"sh", "-c", "echo broken >&2; exit 3"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.OK() || res.ExitCode != 3 || !strings.Contains(res.Output, "broken") {
		t.Fatalf("result = %+v", res)
	}
}

func TestExecTimeout(t *testing.T) {
	res, err := Exec{}.Run(t.Context(), Spec{Args: []string{"sleep", "5"}, Timeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if !res.TimedOut || res.OK() {
		t.Fatalf("result = %+v, want a timeout", res)
	}
}

func TestExecErrors(t *testing.T) {
	if _, err := (Exec{}).Run(t.Context(), Spec{}); err == nil {
		t.Error("Run with no command returned no error")
	}
	if _, err := (Exec{}).Run(t.Context(), Spec{Args: []string{"definitely-not-a-command-xyz"}}); err == nil {
		t.Error("Run of a missing binary returned no error")
	}
}

func TestReportPassed(t *testing.T) {
	ok := Result{}
	failed := Result{ExitCode: 1}
	if !(Report{Build: ok, Vet: ok, Test: ok}).Passed() {
		t.Error("a clean report did not pass")
	}
	if (Report{Build: ok, Vet: ok, Test: failed}).Passed() {
		t.Error("a report with failing tests passed")
	}
	if (Report{Build: ok, Vet: Result{TimedOut: true}, Test: ok}).Passed() {
		t.Error("a report with a timeout passed")
	}
}
