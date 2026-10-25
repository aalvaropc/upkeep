package forge

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/aalvaropc/upkeep/internal/gitx"
	"github.com/aalvaropc/upkeep/internal/pull"
	"github.com/aalvaropc/upkeep/internal/scenario"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-c", "user.name=Author", "-c", "user.email=author@example.com", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)
	cmd := exec.CommandContext(t.Context(), "git", full...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func testScenario() scenario.Scenario {
	person := pull.Author{Login: "alice", Kind: pull.Person}
	return scenario.Scenario{
		Name: "tiny",
		Base: "main",
		PRs: []scenario.PR{
			{Number: 2, Title: "Change b", Branch: "feat/b", Author: person, Labels: []string{"upkeep"}},
			{Number: 1, Title: "Change a", Branch: "feat/a", Author: person},
			{Number: 3, Title: "Also change a", Branch: "feat/a2", Author: person},
			{Number: 4, Title: "Bump", Branch: "dependabot/x", Author: pull.Author{Login: "dependabot[bot]", Kind: pull.Bot}, Bump: &pull.Bump{Module: "example.com/x", From: "v1.0.0", To: "v1.0.1", Level: pull.Patch}},
		},
	}
}

func newLocal(t *testing.T) *Local {
	t.Helper()
	root := t.TempDir()
	src := filepath.Join(root, "src")
	if err := os.MkdirAll(src, 0o750); err != nil {
		t.Fatal(err)
	}
	git(t, src, "init", "--quiet", "--initial-branch=main")
	write(t, src, "a.txt", "one\n")
	write(t, src, "b.txt", "bee\n")
	git(t, src, "add", "-A")
	git(t, src, "commit", "--quiet", "-m", "chore: start")
	branch := func(name, file, content string) {
		git(t, src, "switch", "--quiet", "-c", name, "main")
		write(t, src, file, content)
		git(t, src, "add", "-A")
		git(t, src, "commit", "--quiet", "-m", "change "+file)
	}
	branch("feat/a", "a.txt", "two\n")
	branch("feat/a2", "a.txt", "three\n")
	branch("feat/b", "b.txt", "see\n")
	branch("dependabot/x", "c.txt", "new\n")
	git(t, src, "switch", "--quiet", "main")
	bare := filepath.Join(root, "forge.git")
	g := gitx.Git{}
	if err := g.CloneBare(t.Context(), src, bare); err != nil {
		t.Fatal(err)
	}
	return NewLocal(g, bare, testScenario())
}

func TestPullsListsOpenRequestsInOrder(t *testing.T) {
	l := newLocal(t)
	pulls, err := l.Pulls(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var numbers []int
	for _, p := range pulls {
		numbers = append(numbers, p.Number)
		if p.HeadSHA == "" || p.Base != "main" {
			t.Errorf("#%d: head %q base %q", p.Number, p.HeadSHA, p.Base)
		}
	}
	if len(numbers) != 4 || numbers[0] != 1 || numbers[3] != 4 {
		t.Fatalf("numbers = %v", numbers)
	}
	if pulls[1].Labels[0] != "upkeep" || pulls[3].Kind != pull.Dependency || pulls[0].Kind != pull.Change {
		t.Fatalf("labels or kinds were lost: %+v", pulls)
	}
}

func TestPrepareChecksOutTheBranchWithTheBaseAvailable(t *testing.T) {
	l := newLocal(t)
	pulls, _ := l.Pulls(t.Context())
	dir := filepath.Join(t.TempDir(), "work")
	if err := l.Prepare(t.Context(), pulls[0], dir); err != nil {
		t.Fatal(err)
	}
	if got := git(t, dir, "rev-parse", "--abbrev-ref", "HEAD"); got != "feat/a" {
		t.Errorf("checked out %q", got)
	}
	if got := git(t, dir, "show", "origin/main:a.txt"); got != "one" {
		t.Errorf("origin/main:a.txt = %q", got)
	}
	if got := git(t, dir, "show", "HEAD:a.txt"); got != "two" {
		t.Errorf("HEAD:a.txt = %q", got)
	}
}

func TestPushAcceptsAFastForwardAndRejectsAnythingElse(t *testing.T) {
	l := newLocal(t)
	pulls, _ := l.Pulls(t.Context())
	dir := filepath.Join(t.TempDir(), "work")
	if err := l.Prepare(t.Context(), pulls[0], dir); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "a.txt", "fixed\n")
	git(t, dir, "commit", "--quiet", "-am", "fix")
	if err := l.Push(t.Context(), dir, pulls[0]); err != nil {
		t.Fatal(err)
	}
	if got := git(t, l.Repo, "show", "feat/a:a.txt"); got != "fixed" {
		t.Fatalf("the pushed branch has a.txt = %q", got)
	}

	git(t, dir, "reset", "--quiet", "--hard", "origin/main")
	write(t, dir, "a.txt", "rewritten\n")
	git(t, dir, "commit", "--quiet", "-am", "rewrite")
	if err := l.Push(t.Context(), dir, pulls[0]); err == nil {
		t.Fatal("a push that rewrites the branch was accepted")
	}
	if got := git(t, l.Repo, "show", "feat/a:a.txt"); got != "fixed" {
		t.Fatalf("a rejected push changed the branch: a.txt = %q", got)
	}
}

func TestMergeCreatesAMergeCommitAndClosesTheRequest(t *testing.T) {
	l := newLocal(t)
	pulls, _ := l.Pulls(t.Context())
	sha, err := l.Merge(t.Context(), pulls[2])
	if err != nil {
		t.Fatal(err)
	}
	if got := git(t, l.Repo, "rev-parse", "main"); got != sha {
		t.Fatalf("main is %s, merge returned %s", got, sha)
	}
	if parents := strings.Fields(git(t, l.Repo, "log", "-1", "--format=%P", "main")); len(parents) != 2 {
		t.Fatalf("the merge has %d parents, want 2", len(parents))
	}
	if subject := git(t, l.Repo, "log", "-1", "--format=%s", "main"); subject != "Merge pull request #3 from feat/a2" {
		t.Fatalf("subject = %q", subject)
	}
	after, _ := l.Pulls(t.Context())
	if len(after) != 3 {
		t.Fatalf("%d requests are still open, want 3", len(after))
	}
	if _, err := l.Merge(t.Context(), pulls[2]); !errors.Is(err, ErrClosed) {
		t.Fatalf("merging twice: %v", err)
	}
	views := l.Views(t.Context())
	if views[2].State != Merged || views[2].MergeSHA != sha {
		t.Fatalf("view = %+v", views[2])
	}
}

func TestMergeRefusesAConflictAndLeavesTheBaseAlone(t *testing.T) {
	l := newLocal(t)
	pulls, _ := l.Pulls(t.Context())
	if _, err := l.Merge(t.Context(), pulls[0]); err != nil {
		t.Fatal(err)
	}
	before := git(t, l.Repo, "rev-parse", "main")
	_, err := l.Merge(t.Context(), pulls[2])
	if !errors.Is(err, ErrNotMergeable) {
		t.Fatalf("error = %v, want ErrNotMergeable", err)
	}
	if after := git(t, l.Repo, "rev-parse", "main"); after != before {
		t.Fatalf("a refused merge moved main from %s to %s", before, after)
	}
	if views := l.Views(t.Context()); views[2].State != Open {
		t.Fatalf("state = %s, want open", views[2].State)
	}
}

func TestConcurrentMergesOfIndependentRequestsAllLand(t *testing.T) {
	l := newLocal(t)
	pulls, _ := l.Pulls(t.Context())
	independent := []pull.Request{pulls[0], pulls[1], pulls[3]}
	var wg sync.WaitGroup
	errs := make(chan error, len(independent))
	for _, p := range independent {
		wg.Go(func() {
			_, err := l.Merge(t.Context(), p)
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if open, _ := l.Pulls(t.Context()); len(open) != 1 || open[0].Number != 3 {
		t.Fatalf("open after merging three: %+v", open)
	}
	for _, file := range []string{"a.txt", "b.txt", "c.txt"} {
		git(t, l.Repo, "cat-file", "-e", "main:"+file)
	}
}

func TestCommentsAreKeptWithTheirKind(t *testing.T) {
	l := newLocal(t)
	pulls, _ := l.Pulls(t.Context())
	if err := l.Comment(t.Context(), pulls[0], Comment{Kind: "patch", Body: "diff --git"}); err != nil {
		t.Fatal(err)
	}
	if err := l.Comment(t.Context(), pull.Request{Number: 99}, Comment{Kind: "note"}); !errors.Is(err, ErrUnknown) {
		t.Fatalf("commenting on an unknown request: %v", err)
	}
	views := l.Views(t.Context())
	if len(views[0].Comments) != 1 || views[0].Comments[0].Kind != "patch" || views[0].Comments[0].At.IsZero() {
		t.Fatalf("comments = %+v", views[0].Comments)
	}
}

func TestCopyLocalGivesEachSessionItsOwnRepository(t *testing.T) {
	l := newLocal(t)
	copyA, err := CopyLocal(t.Context(), l.Git, l.Repo, filepath.Join(t.TempDir(), "a.git"), testScenario())
	if err != nil {
		t.Fatal(err)
	}
	pulls, _ := copyA.Pulls(t.Context())
	if _, err := copyA.Merge(t.Context(), pulls[0]); err != nil {
		t.Fatal(err)
	}
	if got := git(t, l.Repo, "show", "main:a.txt"); got != "one" {
		t.Fatalf("merging in a copy changed the template: a.txt = %q", got)
	}
	if open, _ := l.Pulls(t.Context()); len(open) != 4 {
		t.Fatalf("the template lost requests: %d open", len(open))
	}
}
