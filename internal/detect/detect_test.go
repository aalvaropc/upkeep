package detect

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/aalvaropc/upkeep/internal/gitx"
	"github.com/aalvaropc/upkeep/internal/policy"
	"github.com/aalvaropc/upkeep/internal/pull"
	"github.com/aalvaropc/upkeep/internal/sandbox"
)

type fakeRunner struct {
	download    sandbox.Result
	report      sandbox.Report
	rerun       sandbox.Result
	unformatted []string
	err         error
	reran       string
}

func (f *fakeRunner) Download(context.Context, string) (sandbox.Result, error) {
	return f.download, f.err
}

func (f *fakeRunner) Format(context.Context, string) (sandbox.Result, error) {
	return sandbox.Result{}, nil
}

func (f *fakeRunner) Unformatted(context.Context, string) ([]string, error) {
	return f.unformatted, nil
}

func (f *fakeRunner) Check(context.Context, string) (sandbox.Report, error) {
	return f.report, f.err
}

func (f *fakeRunner) Tests(_ context.Context, _, pattern string) (sandbox.Result, error) {
	f.reran = pattern
	return f.rerun, nil
}

func passing() sandbox.Report {
	return sandbox.Report{Build: sandbox.Result{}, Vet: sandbox.Result{}, Test: sandbox.Result{}}
}

func TestFailedTests(t *testing.T) {
	out := "=== RUN   TestA\n--- FAIL: TestA (0.00s)\n    --- FAIL: TestA/sub (0.00s)\n--- FAIL: TestB_two (0.01s)\n--- FAIL: TestA (0.00s)\nFAIL\n"
	if got := FailedTests(out); !slices.Equal(got, []string{"TestA", "TestB_two"}) {
		t.Fatalf("FailedTests = %v, want the top-level tests once each", got)
	}
	if got := FailedTests("FAIL example.test/m [build failed]\n"); len(got) != 0 {
		t.Fatalf("a build failure names no tests: %v", got)
	}
}

func TestTestPattern(t *testing.T) {
	if got := TestPattern([]string{"TestA", "TestB_two"}); got != "^(TestA|TestB_two)$" {
		t.Fatalf("pattern = %q", got)
	}
	if got := TestPattern([]string{"Test.weird"}); got != `^(Test\.weird)$` {
		t.Fatalf("pattern = %q", got)
	}
}

func TestDetectWithoutARunner(t *testing.T) {
	if _, err := (Detector{}).Detect(t.Context(), Workspace{Dir: t.TempDir(), Base: "main"}); err == nil {
		t.Fatal("a detector without a runner returned no error")
	}
}

func TestSmallConflictLimits(t *testing.T) {
	d := Detector{Policy: policy.Default()}
	three := gitx.MergeResult{Conflicts: []gitx.Hunk{{Lines: 4}, {Lines: 20}, {Lines: 1}}}
	four := gitx.MergeResult{Conflicts: []gitx.Hunk{{Lines: 1}, {Lines: 1}, {Lines: 1}, {Lines: 1}}}
	wide := gitx.MergeResult{Conflicts: []gitx.Hunk{{Lines: 21}}}
	if !d.small(three) {
		t.Error("three hunks of at most 20 lines is small")
	}
	if d.small(four) || d.small(wide) {
		t.Error("too many hunks, or a hunk over 20 lines, is large")
	}
}

func requireTools(t *testing.T) {
	t.Helper()
	for _, bin := range []string{"git", "go", "gofmt"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s is not installed", bin)
		}
	}
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

const (
	limits  = "package m\n\nconst (\n\tMaxTagLength = 32\n)\n"
	service = "package m\n\nfunc Add(a, b int) int { return a + b }\n"
	addTest = "package m\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal(\"wrong\")\n\t}\n}\n"
)

type project struct {
	t   *testing.T
	g   gitx.Git
	dir string
}

func newProject(t *testing.T) *project {
	t.Helper()
	requireTools(t)
	p := &project{t: t, g: gitx.Git{}, dir: t.TempDir()}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "git", append([]string{"-c", "commit.gpgsign=false"}, args...)...)
		cmd.Dir = p.dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "--quiet", "-b", "main")
	write(t, p.dir, "go.mod", "module example.test/m\n\ngo 1.25\n")
	write(t, p.dir, "limits.go", limits)
	write(t, p.dir, "service.go", service)
	write(t, p.dir, "service_test.go", addTest)
	p.commit("chore: initial commit")
	return p
}

func (p *project) commit(msg string) {
	p.t.Helper()
	if _, err := p.g.Commit(p.t.Context(), p.dir, msg); err != nil {
		p.t.Fatal(err)
	}
}

func (p *project) branch(name string) {
	p.t.Helper()
	if err := p.g.Checkout(p.t.Context(), p.dir, name, true); err != nil {
		p.t.Fatal(err)
	}
}

func (p *project) checkout(name string) {
	p.t.Helper()
	if err := p.g.Checkout(p.t.Context(), p.dir, name, false); err != nil {
		p.t.Fatal(err)
	}
}

func (p *project) detect() Diagnosis {
	p.t.Helper()
	d := Detector{Git: p.g, Runner: sandbox.GoRunner{Executor: sandbox.Exec{}}, Policy: policy.Default()}
	diag, err := d.Detect(p.t.Context(), Workspace{Dir: p.dir, Base: "main"})
	if err != nil {
		p.t.Fatal(err)
	}
	return diag
}

func TestDetectClean(t *testing.T) {
	p := newProject(t)
	p.branch("feature")
	write(t, p.dir, "extra.go", "package m\n\nfunc Extra() int { return 1 }\n")
	p.commit("feat: add extra")
	diag := p.detect()
	if diag.Blocker != pull.Clean || diag.Behind || !diag.Report.Passed() {
		t.Fatalf("diagnosis = %+v", diag)
	}
}

func TestDetectBehindWithACleanMerge(t *testing.T) {
	p := newProject(t)
	p.branch("feature")
	write(t, p.dir, "extra.go", "package m\n\nfunc Extra() int { return 1 }\n")
	p.commit("feat: add extra")
	p.checkout("main")
	write(t, p.dir, "README.md", "# m\n")
	p.commit("docs: add a readme")
	p.checkout("feature")
	diag := p.detect()
	if diag.Blocker != pull.Behind || !diag.Behind || !diag.Merge.Clean {
		t.Fatalf("diagnosis = %+v", diag)
	}
	if _, err := os.Stat(filepath.Join(p.dir, "README.md")); err != nil {
		t.Fatal("the merge result should be in the working tree for the next steps")
	}
}

func TestDetectConflicts(t *testing.T) {
	p := newProject(t)
	p.branch("feature")
	write(t, p.dir, "limits.go", "package m\n\nconst (\n\tMaxTagLength = 32\n\n\tMaxNotesPerOwner = 1000\n)\n")
	p.commit("feat: cap the notes")
	p.checkout("main")
	write(t, p.dir, "limits.go", "package m\n\nconst (\n\tMaxTagLength = 32\n\n\tMaxAttachments = 5\n)\n")
	p.commit("feat: cap the attachments")
	p.checkout("feature")
	diag := p.detect()
	if diag.Blocker != pull.ConflictSmall || diag.Merge.Hunks() != 1 {
		t.Fatalf("diagnosis = %+v", diag)
	}
}

func TestDetectLargeConflict(t *testing.T) {
	p := newProject(t)
	var ours, theirs strings.Builder
	for i := range 30 {
		ours.WriteString("\tOurs" + string(rune('A'+i%26)) + string(rune('a'+i/26)) + " = 1\n")
		theirs.WriteString("\tTheirs" + string(rune('A'+i%26)) + string(rune('a'+i/26)) + " = 2\n")
	}
	p.branch("feature")
	write(t, p.dir, "limits.go", "package m\n\nconst (\n\tMaxTagLength = 32\n\n"+ours.String()+")\n")
	p.commit("feat: many on one side")
	p.checkout("main")
	write(t, p.dir, "limits.go", "package m\n\nconst (\n\tMaxTagLength = 32\n\n"+theirs.String()+")\n")
	p.commit("feat: many on the other")
	p.checkout("feature")
	diag := p.detect()
	if diag.Blocker != pull.ConflictLarge || diag.Merge.LargestHunk() < 30 {
		t.Fatalf("diagnosis = %+v", diag.Blocker)
	}
}

func TestDetectBuildBrokenByMain(t *testing.T) {
	p := newProject(t)
	p.branch("feature")
	write(t, p.dir, "caller.go", "package m\n\nfunc Total() int { return Add(1, 2) }\n")
	p.commit("feat: add a caller")
	p.checkout("main")
	write(t, p.dir, "service.go", "package m\n\nfunc Sum(a, b int) int { return a + b }\n")
	write(t, p.dir, "service_test.go", strings.ReplaceAll(addTest, "Add(", "Sum("))
	p.commit("refactor: rename Add to Sum")
	p.checkout("feature")
	diag := p.detect()
	if diag.Blocker != pull.Build || !diag.Behind {
		t.Fatalf("blocker = %s", diag.Blocker)
	}
	failure, found := diag.Failure()
	if !found || !strings.Contains(failure.Output, "undefined: Add") {
		t.Fatalf("failure = %+v", failure)
	}
}

func TestDetectFailingTest(t *testing.T) {
	p := newProject(t)
	p.branch("feature")
	write(t, p.dir, "service.go", "package m\n\nfunc Add(a, b int) int { return a - b }\n")
	p.commit("fix: change the operator")
	diag := p.detect()
	if diag.Blocker != pull.Test || len(diag.FlakyTests) != 0 || diag.FlakyRerun.OK() {
		t.Fatalf("diagnosis = %+v", diag.Blocker)
	}
}

func TestDetectFlakyTest(t *testing.T) {
	p := newProject(t)
	marker := filepath.Join(t.TempDir(), "seen")
	flaky := "package m\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestFlaky(t *testing.T) {\n\tif _, err := os.Stat(" + "\"" + marker + "\"" + "); err != nil {\n\t\tos.WriteFile(" + "\"" + marker + "\"" + ", []byte(\"x\"), 0o600)\n\t\tt.Fatal(\"first run fails\")\n\t}\n}\n"
	p.branch("feature")
	write(t, p.dir, "flaky_test.go", flaky)
	p.commit("test: add a flaky test")
	diag := p.detect()
	if diag.Blocker != pull.Flaky || !slices.Equal(diag.FlakyTests, []string{"TestFlaky"}) || !diag.FlakyRerun.OK() {
		t.Fatalf("blocker %s flaky %v", diag.Blocker, diag.FlakyTests)
	}
}

func TestDetectFormat(t *testing.T) {
	p := newProject(t)
	p.branch("feature")
	write(t, p.dir, "extra.go", "package m\nfunc   Extra()int{return 1}\n")
	p.commit("feat: add extra")
	diag := p.detect()
	if diag.Blocker != pull.Format || !slices.Equal(diag.Unformatted, []string{"extra.go"}) {
		t.Fatalf("blocker %s unformatted %v", diag.Blocker, diag.Unformatted)
	}
}

func TestDetectFormatBeatsBehind(t *testing.T) {
	p := newProject(t)
	p.branch("feature")
	write(t, p.dir, "extra.go", "package m\nfunc   Extra()int{return 1}\n")
	p.commit("feat: add extra")
	p.checkout("main")
	write(t, p.dir, "README.md", "# m\n")
	p.commit("docs: add a readme")
	p.checkout("feature")
	diag := p.detect()
	if diag.Blocker != pull.Format || !diag.Behind {
		t.Fatalf("blocker %s behind %v", diag.Blocker, diag.Behind)
	}
}

func TestDetectDownloadFailureIsABuildBlocker(t *testing.T) {
	p := newProject(t)
	p.branch("feature")
	write(t, p.dir, "go.mod", "module example.test/m\n\ngo 1.25\n\nrequire example.invalid/nothing v1.0.0\n")
	p.commit("chore: bump a module that does not exist")
	diag := p.detect()
	if diag.Blocker != pull.Build {
		t.Fatalf("blocker = %s", diag.Blocker)
	}
	if failure, found := diag.Failure(); !found || failure.OK() {
		t.Fatalf("failure = %+v", failure)
	}
}

func TestDetectWithAFakeRunner(t *testing.T) {
	p := newProject(t)
	cases := map[string]struct {
		runner *fakeRunner
		want   pull.Blocker
		reran  string
	}{
		"clean":                 {&fakeRunner{report: passing()}, pull.Clean, ""},
		"build":                 {&fakeRunner{report: sandbox.Report{Build: sandbox.Result{ExitCode: 1}, Vet: sandbox.Result{Skipped: true}, Test: sandbox.Result{Skipped: true}}}, pull.Build, ""},
		"vet":                   {&fakeRunner{report: sandbox.Report{Vet: sandbox.Result{ExitCode: 1}, Test: sandbox.Result{Skipped: true}}}, pull.Build, ""},
		"test timeout":          {&fakeRunner{report: sandbox.Report{Test: sandbox.Result{ExitCode: -1, TimedOut: true, Output: "--- FAIL: TestSlow (9.00s)\n"}}}, pull.Test, ""},
		"test without names":    {&fakeRunner{report: sandbox.Report{Test: sandbox.Result{ExitCode: 1, Output: "FAIL example.test/m [build failed]\n"}}}, pull.Test, ""},
		"test fails twice":      {&fakeRunner{report: sandbox.Report{Test: sandbox.Result{ExitCode: 1, Output: "--- FAIL: TestA (0.00s)\n"}}, rerun: sandbox.Result{ExitCode: 1}}, pull.Test, "^(TestA)$"},
		"flaky":                 {&fakeRunner{report: sandbox.Report{Test: sandbox.Result{ExitCode: 1, Output: "--- FAIL: TestA (0.00s)\n--- FAIL: TestB (0.00s)\n"}}}, pull.Flaky, "^(TestA|TestB)$"},
		"flaky and unformatted": {&fakeRunner{report: sandbox.Report{Test: sandbox.Result{ExitCode: 1, Output: "--- FAIL: TestA (0.00s)\n"}}, unformatted: []string{"a.go"}}, pull.Format, "^(TestA)$"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			d := Detector{Git: p.g, Runner: tc.runner, Policy: policy.Default()}
			diag, err := d.Detect(t.Context(), Workspace{Dir: p.dir, Base: "main"})
			if err != nil {
				t.Fatal(err)
			}
			if diag.Blocker != tc.want || tc.runner.reran != tc.reran {
				t.Fatalf("blocker %s (want %s), rerun pattern %q (want %q)", diag.Blocker, tc.want, tc.runner.reran, tc.reran)
			}
		})
	}
}

func TestDetectPropagatesRunnerErrors(t *testing.T) {
	p := newProject(t)
	boom := errors.New("docker is not running")
	d := Detector{Git: p.g, Runner: &fakeRunner{err: boom}, Policy: policy.Default()}
	if _, err := d.Detect(t.Context(), Workspace{Dir: p.dir, Base: "main"}); !errors.Is(err, boom) {
		t.Fatalf("error = %v", err)
	}
	if _, err := d.Detect(t.Context(), Workspace{Dir: p.dir, Base: "no-such-branch"}); err == nil {
		t.Fatal("an unknown base returned no error")
	}
}
