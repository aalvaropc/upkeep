package sandbox

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aalvaropc/upkeep/internal/fixture"
)

func requireGo(t *testing.T) {
	t.Helper()
	for _, bin := range []string{"go", "gofmt"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s is not installed", bin)
		}
	}
}

func module(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	all := map[string]string{"go.mod": "module example.test/m\n\ngo 1.25\n"}
	for k, v := range files {
		all[k] = v
	}
	for name, content := range all {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const goodSource = "package m\n\nfunc Add(a, b int) int { return a + b }\n"
const goodTest = "package m\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal(\"wrong\")\n\t}\n}\n"

type counting struct {
	inner Executor
	calls atomic.Int32
	seen  []string
}

func (c *counting) Run(ctx context.Context, spec Spec) (Result, error) {
	c.calls.Add(1)
	c.seen = append(c.seen, strings.Join(spec.Args, " "))
	return c.inner.Run(ctx, spec)
}

func TestCheckPasses(t *testing.T) {
	requireGo(t)
	dir := module(t, map[string]string{"m.go": goodSource, "m_test.go": goodTest})
	report, err := GoRunner{Executor: Exec{}}.Check(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Passed() || report.Cached {
		t.Fatalf("report = %+v", report)
	}
	if !strings.Contains(report.Test.Output, "ok") {
		t.Fatalf("test output = %q", report.Test.Output)
	}
}

func TestCheckStopsAtTheFirstFailure(t *testing.T) {
	requireGo(t)
	cases := map[string]struct {
		files   map[string]string
		failing func(Report) Result
		skipped func(Report) []Result
		output  string
	}{
		"build": {
			files:   map[string]string{"m.go": "package m\n\nfunc Add(a, b int) int { return missing }\n"},
			failing: func(r Report) Result { return r.Build },
			skipped: func(r Report) []Result { return []Result{r.Vet, r.Test} },
			output:  "undefined: missing",
		},
		"vet": {
			files:   map[string]string{"m.go": "package m\n\nimport \"fmt\"\n\nfunc Show() { fmt.Printf(\"%d\", \"text\") }\n"},
			failing: func(r Report) Result { return r.Vet },
			skipped: func(r Report) []Result { return []Result{r.Test} },
			output:  "Printf",
		},
		"test": {
			files:   map[string]string{"m.go": "package m\n\nfunc Add(a, b int) int { return a - b }\n", "m_test.go": goodTest},
			failing: func(r Report) Result { return r.Test },
			output:  "FAIL",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			report, err := GoRunner{Executor: Exec{}}.Check(t.Context(), module(t, tc.files))
			if err != nil {
				t.Fatal(err)
			}
			failing := tc.failing(report)
			if report.Passed() || failing.OK() || !strings.Contains(failing.Output, tc.output) {
				t.Fatalf("report = %+v", report)
			}
			if tc.skipped != nil {
				for _, s := range tc.skipped(report) {
					if !s.Skipped || s.OK() {
						t.Errorf("a step after the failure ran: %+v", s)
					}
				}
			}
			first, found := report.FirstFailure()
			if !found || first.Output != failing.Output {
				t.Errorf("FirstFailure = %+v %v", first, found)
			}
		})
	}
}

func TestCheckCachesByTreeContent(t *testing.T) {
	requireGo(t)
	dir := module(t, map[string]string{"m.go": goodSource, "m_test.go": goodTest})
	exec := &counting{inner: Exec{}}
	runner := GoRunner{Executor: exec, Cache: fixture.Dir{Path: t.TempDir()}, Variant: "test"}

	first, err := runner.Check(t.Context(), dir)
	if err != nil || !first.Passed() || first.Cached {
		t.Fatalf("first check: %+v %v", first, err)
	}
	if exec.calls.Load() != 3 {
		t.Fatalf("first check ran %d commands: %v", exec.calls.Load(), exec.seen)
	}
	second, err := runner.Check(t.Context(), dir)
	if err != nil || !second.Passed() || !second.Cached || exec.calls.Load() != 3 {
		t.Fatalf("second check: cached %v, %d commands, %v", second.Cached, exec.calls.Load(), err)
	}

	if err := os.WriteFile(filepath.Join(dir, "m.go"), []byte("package m\n\nfunc Add(a, b int) int { return a - b }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	third, err := runner.Check(t.Context(), dir)
	if err != nil || third.Passed() || third.Cached || exec.calls.Load() < 6 {
		t.Fatalf("after an edit: passed %v cached %v commands %d", third.Passed(), third.Cached, exec.calls.Load())
	}
	again, err := runner.Check(t.Context(), dir)
	if err != nil || again.Passed() || !again.Cached {
		t.Fatalf("a failing report is cached too: %+v %v", again, err)
	}
}

type timeoutExecutor struct{ calls atomic.Int32 }

func (e *timeoutExecutor) Run(_ context.Context, spec Spec) (Result, error) {
	e.calls.Add(1)
	return Result{Args: spec.Args, ExitCode: -1, TimedOut: true}, nil
}

func TestTimedOutReportsAreNotCached(t *testing.T) {
	dir := module(t, map[string]string{"m.go": goodSource})
	exec := &timeoutExecutor{}
	runner := GoRunner{Executor: exec, Cache: fixture.Dir{Path: t.TempDir()}}
	for i := 0; i < 2; i++ {
		report, err := runner.Check(t.Context(), dir)
		if err != nil || report.Cached || report.Build.TimedOut != true {
			t.Fatalf("run %d: %+v %v", i, report, err)
		}
	}
	if exec.calls.Load() != 2 {
		t.Fatalf("a timeout was cached: %d runs", exec.calls.Load())
	}
}

func TestTreeKey(t *testing.T) {
	a := module(t, map[string]string{"m.go": goodSource, "sub/x.txt": "x"})
	b := module(t, map[string]string{"m.go": goodSource, "sub/x.txt": "x"})
	keyA, err := TreeKey(a, "v1")
	if err != nil {
		t.Fatal(err)
	}
	if keyB, _ := TreeKey(b, "v1"); keyA != keyB || len(keyA) != 64 {
		t.Fatalf("the same content in two directories gave different keys: %s %s", keyA, keyB)
	}
	if k, _ := TreeKey(a, "v2"); k == keyA {
		t.Error("the variant did not change the key")
	}
	if err := os.WriteFile(filepath.Join(a, "sub", "x.txt"), []byte("y"), 0o600); err != nil {
		t.Fatal(err)
	}
	if k, _ := TreeKey(a, "v1"); k == keyA {
		t.Error("an edit did not change the key")
	}
	if err := os.WriteFile(filepath.Join(a, "sub", "x.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if k, _ := TreeKey(a, "v1"); k != keyA {
		t.Error("restoring the content did not restore the key")
	}
	if err := os.WriteFile(filepath.Join(a, "new.go"), []byte("package m\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	withNew, _ := TreeKey(a, "v1")
	if withNew == keyA {
		t.Error("a new file did not change the key")
	}
	if err := os.MkdirAll(filepath.Join(a, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a, ".git", "HEAD"), []byte("ref"), 0o600); err != nil {
		t.Fatal(err)
	}
	if k, _ := TreeKey(a, "v1"); k != withNew {
		t.Error("the .git directory changed the key")
	}
	if err := os.Chmod(filepath.Join(a, "new.go"), 0o755); err != nil {
		t.Fatal(err)
	}
	if k, _ := TreeKey(a, "v1"); k == withNew {
		t.Error("a permission change did not change the key")
	}
	if err := os.Symlink("/etc/hostname", filepath.Join(a, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := TreeKey(a, "v1"); err != nil {
		t.Fatalf("a symlink broke the key: %v", err)
	}
	if _, err := TreeKey(filepath.Join(a, "does-not-exist"), "v1"); err == nil {
		t.Error("a missing directory returned no error")
	}
}

func TestFormat(t *testing.T) {
	requireGo(t)
	dir := module(t, map[string]string{"m.go": "package m\nfunc   Add(a,b int)int{return a+b}\n", "ok.go": "package m\n\nfunc One() int { return 1 }\n"})
	runner := GoRunner{Executor: Exec{}}
	res, err := runner.Format(t.Context(), dir)
	if err != nil || !res.OK() || strings.TrimSpace(res.Output) != "m.go" {
		t.Fatalf("format result = %+v %v", res, err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "m.go"))
	if string(data) != "package m\n\nfunc Add(a, b int) int { return a + b }\n" {
		t.Fatalf("m.go = %q", data)
	}
	again, err := runner.Format(t.Context(), dir)
	if err != nil || !again.OK() || strings.TrimSpace(again.Output) != "" {
		t.Fatalf("formatting a formatted tree = %+v %v", again, err)
	}
}

func TestFormatReportsSyntaxErrors(t *testing.T) {
	requireGo(t)
	dir := module(t, map[string]string{"m.go": "package m\n\nfunc {\n"})
	res, err := GoRunner{Executor: Exec{}}.Format(t.Context(), dir)
	if err != nil || res.OK() {
		t.Fatalf("a file that does not parse: %+v %v", res, err)
	}
}

func TestDownloadWithoutDependencies(t *testing.T) {
	requireGo(t)
	res, err := GoRunner{Executor: Exec{}}.Download(t.Context(), module(t, map[string]string{"m.go": goodSource}))
	if err != nil || !res.OK() {
		t.Fatalf("download = %+v %v", res, err)
	}
}

func TestTestsRunsOnlyTheMatchingTests(t *testing.T) {
	requireGo(t)
	two := "package m\n\nimport \"testing\"\n\nfunc TestPasses(t *testing.T) {}\n\nfunc TestFails(t *testing.T) { t.Fatal(\"nope\") }\n"
	dir := module(t, map[string]string{"m.go": goodSource, "m_test.go": two})
	runner := GoRunner{Executor: Exec{}}
	if res, err := runner.Tests(t.Context(), dir, "^TestPasses$"); err != nil || !res.OK() {
		t.Fatalf("TestPasses: %+v %v", res, err)
	}
	if res, err := runner.Tests(t.Context(), dir, "^TestFails$"); err != nil || res.OK() || !strings.Contains(res.Output, "nope") {
		t.Fatalf("TestFails: %+v %v", res, err)
	}
}

func TestRunnerNeedsAnExecutor(t *testing.T) {
	if _, err := (GoRunner{}).Check(t.Context(), t.TempDir()); err == nil {
		t.Error("Check without an executor returned no error")
	}
	if _, err := (GoRunner{}).Download(t.Context(), t.TempDir()); err == nil {
		t.Error("Download without an executor returned no error")
	}
}

type failingExecutor struct{}

func (failingExecutor) Run(context.Context, Spec) (Result, error) {
	return Result{}, errors.New("docker is not running")
}

func TestInfrastructureFailuresAreErrors(t *testing.T) {
	dir := module(t, map[string]string{"m.go": goodSource})
	if _, err := (GoRunner{Executor: failingExecutor{}}).Check(t.Context(), dir); err == nil || !strings.Contains(err.Error(), "docker is not running") {
		t.Fatalf("error = %v", err)
	}
}
