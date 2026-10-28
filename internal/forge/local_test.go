package forge_test

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/aalvaropc/upkeep/internal/forge"
	"github.com/aalvaropc/upkeep/internal/forge/forgetest"
	"github.com/aalvaropc/upkeep/internal/pull"
)

var git = forgetest.Git
var write = forgetest.Write
var newLocal = forgetest.New

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
	if _, err := l.Merge(t.Context(), pulls[2]); !errors.Is(err, forge.ErrClosed) {
		t.Fatalf("merging twice: %v", err)
	}
	views := l.Views(t.Context())
	if views[2].State != forge.Merged || views[2].MergeSHA != sha {
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
	if !errors.Is(err, forge.ErrNotMergeable) {
		t.Fatalf("error = %v, want forge.ErrNotMergeable", err)
	}
	if after := git(t, l.Repo, "rev-parse", "main"); after != before {
		t.Fatalf("a refused merge moved main from %s to %s", before, after)
	}
	if views := l.Views(t.Context()); views[2].State != forge.Open {
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
	if err := l.Comment(t.Context(), pulls[0], forge.Comment{Kind: "patch", Body: "diff --git"}); err != nil {
		t.Fatal(err)
	}
	if err := l.Comment(t.Context(), pull.Request{Number: 99}, forge.Comment{Kind: "note"}); !errors.Is(err, forge.ErrUnknown) {
		t.Fatalf("commenting on an unknown request: %v", err)
	}
	views := l.Views(t.Context())
	if len(views[0].Comments) != 1 || views[0].Comments[0].Kind != "patch" || views[0].Comments[0].At.IsZero() {
		t.Fatalf("comments = %+v", views[0].Comments)
	}
}

func TestACommentReplacesTheEarlierOneOfItsKind(t *testing.T) {
	l := newLocal(t)
	pulls, _ := l.Pulls(t.Context())
	for _, c := range []forge.Comment{{Kind: "receipt", Body: "first"}, {Kind: "patch", Body: "diff"}, {Kind: "receipt", Body: "second"}} {
		if err := l.Comment(t.Context(), pulls[0], c); err != nil {
			t.Fatal(err)
		}
	}
	got := l.Views(t.Context())[0].Comments
	if len(got) != 2 || got[0].Kind != "receipt" || got[0].Body != "second" || got[1].Kind != "patch" {
		t.Fatalf("comments = %+v", got)
	}
}

func TestCopyLocalGivesEachSessionItsOwnRepository(t *testing.T) {
	l := newLocal(t)
	copyA, err := forge.CopyLocal(t.Context(), l.Git, l.Repo, filepath.Join(t.TempDir(), "a.git"), forgetest.Scenario())
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
