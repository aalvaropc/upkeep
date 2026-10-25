package gitx

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
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

func read(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func gitOut(t *testing.T, g Git, dir string, args ...string) string {
	t.Helper()
	out, err := g.run(t.Context(), dir, "", args...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func newRepo(t *testing.T) (Git, string) {
	t.Helper()
	requireGit(t)
	g := Git{}
	dir := t.TempDir()
	gitOut(t, g, dir, "init", "--quiet", "-b", "main")
	write(t, dir, "notes/limits.go", "package notes\n\nconst (\n\tMaxTagLength = 32\n)\n")
	write(t, dir, "notes/service.go", "package notes\n\nfunc List() {}\n")
	if _, err := g.Commit(t.Context(), dir, "chore: initial commit"); err != nil {
		t.Fatal(err)
	}
	return g, dir
}

func TestCommitHeadAndAncestry(t *testing.T) {
	g, dir := newRepo(t)
	ctx := t.Context()
	first, err := g.Head(ctx, dir)
	if err != nil || len(first) != 40 {
		t.Fatalf("head %q error %v", first, err)
	}
	write(t, dir, "notes/service.go", "package notes\n\nfunc List() int { return 1 }\n")
	second, err := g.Commit(ctx, dir, "fix: return a value\n\nWith a body.")
	if err != nil || second == first {
		t.Fatalf("commit %q error %v", second, err)
	}
	if ok, err := g.IsAncestor(ctx, dir, first, second); err != nil || !ok {
		t.Fatalf("first should be an ancestor of second: %v %v", ok, err)
	}
	if ok, err := g.IsAncestor(ctx, dir, second, first); err != nil || ok {
		t.Fatalf("second is not an ancestor of first: %v %v", ok, err)
	}
	if _, err := g.IsAncestor(ctx, dir, "no-such-ref", first); err == nil {
		t.Fatal("an unknown ref returned no error")
	}
	if sha, err := g.RevParse(ctx, dir, "main"); err != nil || sha != second {
		t.Fatalf("RevParse(main) = %q %v", sha, err)
	}
	if _, err := g.RevParse(ctx, dir, "nope"); err == nil {
		t.Fatal("RevParse of an unknown ref returned no error")
	}
	author := gitOut(t, g, dir, "log", "-1", "--format=%an <%ae>%n%B")
	if !strings.HasPrefix(author, "Upkeep <upkeep@users.noreply.github.com>") || !strings.Contains(author, "With a body.") {
		t.Fatalf("commit = %q", author)
	}
	if _, err := g.Commit(ctx, dir, "  "); err == nil {
		t.Fatal("an empty message was accepted")
	}
}

func TestCleanMergeKeepsHistory(t *testing.T) {
	g, dir := newRepo(t)
	ctx := t.Context()
	if err := g.Checkout(ctx, dir, "feature", true); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "notes/service.go", "package notes\n\nfunc List() {}\n\nfunc Get() {}\n")
	if _, err := g.Commit(ctx, dir, "feat: add get"); err != nil {
		t.Fatal(err)
	}
	if err := g.Checkout(ctx, dir, "main", false); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "README.md", "# notes\n")
	if _, err := g.Commit(ctx, dir, "docs: add a readme"); err != nil {
		t.Fatal(err)
	}
	if err := g.Checkout(ctx, dir, "feature", false); err != nil {
		t.Fatal(err)
	}
	behind, _ := g.IsAncestor(ctx, dir, "main", "HEAD")
	if behind {
		t.Fatal("feature should be behind main")
	}
	result, err := g.Merge(ctx, dir, "main")
	if err != nil || !result.Clean || result.Hunks() != 0 {
		t.Fatalf("merge = %+v %v", result, err)
	}
	if _, err := g.Commit(ctx, dir, "merge main into feature"); err != nil {
		t.Fatal(err)
	}
	parents := strings.Fields(gitOut(t, g, dir, "rev-list", "--parents", "-n", "1", "HEAD"))
	if len(parents) != 3 {
		t.Fatalf("the merge commit has %d parents, want 2: %v", len(parents)-1, parents)
	}
	if ok, _ := g.IsAncestor(ctx, dir, "main", "HEAD"); !ok {
		t.Fatal("main should be an ancestor after the merge")
	}
	if read(t, dir, "README.md") != "# notes\n" {
		t.Fatal("the merge did not bring main's file")
	}
}

func conflictingBranches(t *testing.T) (Git, string) {
	t.Helper()
	g, dir := newRepo(t)
	ctx := t.Context()
	if err := g.Checkout(ctx, dir, "feature", true); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "notes/limits.go", "package notes\n\nconst (\n\tMaxTagLength = 32\n\n\tMaxNotesPerOwner = 1000\n)\n")
	if _, err := g.Commit(ctx, dir, "feat: cap the notes per owner"); err != nil {
		t.Fatal(err)
	}
	if err := g.Checkout(ctx, dir, "main", false); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "notes/limits.go", "package notes\n\nconst (\n\tMaxTagLength = 32\n\n\tMaxAttachments = 5\n)\n")
	if _, err := g.Commit(ctx, dir, "feat: cap the attachments"); err != nil {
		t.Fatal(err)
	}
	if err := g.Checkout(ctx, dir, "feature", false); err != nil {
		t.Fatal(err)
	}
	return g, dir
}

func TestMergeConflictAndResolution(t *testing.T) {
	g, dir := conflictingBranches(t)
	ctx := t.Context()
	result, err := g.Merge(ctx, dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if result.Clean || result.Hunks() != 1 || result.LargestHunk() != 2 {
		t.Fatalf("merge = %+v", result)
	}
	h := result.Conflicts[0]
	if h.File != "notes/limits.go" || h.Ours != "\tMaxNotesPerOwner = 1000" || h.Theirs != "\tMaxAttachments = 5" {
		t.Fatalf("hunk = %+v", h)
	}
	resolved, err := Resolve(read(t, dir, "notes/limits.go"), []string{"\tMaxNotesPerOwner = 1000\n\tMaxAttachments = 5"})
	if err != nil {
		t.Fatal(err)
	}
	write(t, dir, "notes/limits.go", resolved)
	if _, err := g.Commit(ctx, dir, "merge main into feature"); err != nil {
		t.Fatal(err)
	}
	final := read(t, dir, "notes/limits.go")
	if !strings.Contains(final, "MaxNotesPerOwner") || !strings.Contains(final, "MaxAttachments") || HasMarkers(final) {
		t.Fatalf("final file:\n%s", final)
	}
}

func TestMergeAbortRestoresTheBranch(t *testing.T) {
	g, dir := conflictingBranches(t)
	ctx := t.Context()
	before := gitOut(t, g, dir, "rev-parse", "HEAD")
	if _, err := g.Merge(ctx, dir, "main"); err != nil {
		t.Fatal(err)
	}
	if err := g.MergeAbort(ctx, dir); err != nil {
		t.Fatal(err)
	}
	if after := gitOut(t, g, dir, "rev-parse", "HEAD"); after != before || HasMarkers(read(t, dir, "notes/limits.go")) {
		t.Fatal("the abort did not restore the branch")
	}
}

func TestMergeOfAnUnknownRefIsAnError(t *testing.T) {
	g, dir := newRepo(t)
	if _, err := g.Merge(t.Context(), dir, "does-not-exist"); err == nil {
		t.Fatal("merging an unknown ref returned no error")
	}
}

func TestDiffCountsWhatChanged(t *testing.T) {
	g, dir := newRepo(t)
	ctx := t.Context()
	base, _ := g.Head(ctx, dir)
	write(t, dir, "notes/service.go", "package notes\n\nfunc List() int {\n\treturn 1\n}\n")
	write(t, dir, "notes/service_test.go", "package notes\n")
	write(t, dir, "blob.bin", "\x00\x01\x02\x03")
	if err := os.Remove(filepath.Join(dir, "notes/limits.go")); err != nil {
		t.Fatal(err)
	}
	changes, err := g.Diff(ctx, dir, base)
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]FileChange{}
	for _, c := range changes {
		byPath[c.Path] = c
	}
	if len(changes) != 4 {
		t.Fatalf("changes = %+v", changes)
	}
	if c := byPath["notes/service.go"]; c.Added != 3 || c.Deleted != 1 || c.IsTest() {
		t.Errorf("service.go = %+v", c)
	}
	if c := byPath["notes/service_test.go"]; c.Added != 1 || !c.IsTest() {
		t.Errorf("service_test.go = %+v", c)
	}
	if c := byPath["notes/limits.go"]; c.Added != 0 || c.Deleted != 5 {
		t.Errorf("limits.go = %+v", c)
	}
	if c := byPath["blob.bin"]; c.Added+c.Deleted == 0 {
		t.Errorf("a binary file must still count as changed: %+v", c)
	}
	patch, err := g.PatchAgainst(ctx, dir, base)
	if err != nil || !strings.Contains(patch, "+\treturn 1") || !strings.Contains(patch, "-func List() {}") {
		t.Fatalf("patch %q error %v", patch, err)
	}
	if none, err := g.Diff(ctx, dir, "HEAD"); err != nil || len(none) != 4 {
		t.Fatalf("diff against HEAD: %v %v", none, err)
	}
}

func TestParseNumstatHandlesRenames(t *testing.T) {
	out := "2\t1\tnotes/a.go\x003\t0\t\x00old/name.go\x00new/name.go\x00-\t-\timage.png\x00"
	got, err := parseNumstat(out)
	if err != nil {
		t.Fatal(err)
	}
	want := []FileChange{{Path: "notes/a.go", Added: 2, Deleted: 1}, {Path: "new/name.go", Added: 3}, {Path: "image.png", Added: 1, Deleted: 1}}
	if !slices.Equal(got, want) {
		t.Fatalf("changes = %+v, want %+v", got, want)
	}
	if _, err := parseNumstat("garbage\x00"); err == nil {
		t.Fatal("a malformed row was accepted")
	}
	if _, err := parseNumstat("x\ty\tfile\x00"); err == nil {
		t.Fatal("non-numeric counts were accepted")
	}
}

func TestResetToRemovesEditsAndNewFiles(t *testing.T) {
	g, dir := newRepo(t)
	ctx := t.Context()
	base, _ := g.Head(ctx, dir)
	write(t, dir, "notes/service.go", "broken")
	write(t, dir, "notes/extra.go", "package notes\n")
	if err := g.ResetTo(ctx, dir, base); err != nil {
		t.Fatal(err)
	}
	if read(t, dir, "notes/service.go") != "package notes\n\nfunc List() {}\n" {
		t.Fatal("the edit survived the reset")
	}
	if _, err := os.Stat(filepath.Join(dir, "notes/extra.go")); !os.IsNotExist(err) {
		t.Fatal("the new file survived the reset")
	}
}

func TestPushNeverForces(t *testing.T) {
	requireGit(t)
	g := Git{}
	ctx := t.Context()
	remote := filepath.Join(t.TempDir(), "remote.git")
	gitOut(t, g, "", "init", "--quiet", "--bare", "-b", "main", remote)
	a := filepath.Join(t.TempDir(), "a")
	b := filepath.Join(t.TempDir(), "b")
	for _, dir := range []string{a, b} {
		if err := g.Clone(ctx, remote, dir); err != nil {
			t.Fatal(err)
		}
	}
	write(t, a, "f.txt", "from a\n")
	if _, err := g.Commit(ctx, a, "feat: from a"); err != nil {
		t.Fatal(err)
	}
	if err := g.Push(ctx, a, "origin", "fix/shared"); err != nil {
		t.Fatal(err)
	}
	if got := gitOut(t, g, remote, "rev-parse", "refs/heads/fix/shared"); strings.TrimSpace(got) == "" {
		t.Fatal("the branch was not pushed")
	}

	write(t, b, "f.txt", "from b\n")
	if _, err := g.Commit(ctx, b, "feat: from b"); err != nil {
		t.Fatal(err)
	}
	err := g.Push(ctx, b, "origin", "fix/shared")
	if _, ok := errors.AsType[*Error](err); !ok {
		t.Fatalf("a push that would rewrite history should be rejected by the remote, got %v", err)
	}
	headA, _ := g.Head(ctx, a)
	if got := strings.TrimSpace(gitOut(t, g, remote, "rev-parse", "refs/heads/fix/shared")); got != headA {
		t.Fatal("the rejected push changed the remote branch")
	}

	for _, branch := range []string{"", "+fix/shared", "-f", "main:other", ":main", "a b"} {
		if err := g.Push(ctx, b, "origin", branch); !errors.Is(err, ErrForcePush) {
			t.Errorf("Push(%q) = %v, want ErrForcePush", branch, err)
		}
	}
}

func TestCloneFetchAndShow(t *testing.T) {
	g, src := newRepo(t)
	ctx := t.Context()
	dst := filepath.Join(t.TempDir(), "nested", "copy")
	if err := g.Clone(ctx, src, dst); err != nil {
		t.Fatal(err)
	}
	if read(t, dst, "notes/service.go") != read(t, src, "notes/service.go") {
		t.Fatal("the clone differs from the source")
	}
	write(t, src, "notes/service.go", "package notes\n\nfunc List() int { return 2 }\n")
	if _, err := g.Commit(ctx, src, "fix: return two"); err != nil {
		t.Fatal(err)
	}
	if err := g.Fetch(ctx, dst, "origin", "main"); err != nil {
		t.Fatal(err)
	}
	shown, err := g.Show(ctx, dst, "origin/main", "notes/service.go")
	if err != nil || !strings.Contains(shown, "return 2") {
		t.Fatalf("show %q error %v", shown, err)
	}
	if err := g.Clone(ctx, filepath.Join(t.TempDir(), "missing"), filepath.Join(t.TempDir(), "x")); err == nil {
		t.Fatal("cloning a missing repository returned no error")
	}
}

func TestErrorsCarryTheOutput(t *testing.T) {
	requireGit(t)
	_, err := Git{}.Head(t.Context(), t.TempDir())
	gitErr, ok := errors.AsType[*Error](err)
	if !ok || gitErr.Code == 0 || !strings.Contains(gitErr.Error(), "rev-parse") {
		t.Fatalf("error = %v", err)
	}
}

func TestCancelledContextStopsGit(t *testing.T) {
	g, dir := newRepo(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := g.Head(ctx, dir); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want the cancellation", err)
	}
	g.Timeout = time.Nanosecond
	if _, err := g.Head(t.Context(), dir); err == nil {
		t.Fatal("a timeout of one nanosecond returned no error")
	}
}

func TestRepositoryHooksNeverRun(t *testing.T) {
	g, dir := newRepo(t)
	hook := filepath.Join(dir, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho hook ran > hook-ran.txt\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "notes/service.go", "package notes\n\nfunc List() int { return 3 }\n")
	if _, err := g.Commit(t.Context(), dir, "fix: return three"); err != nil {
		t.Fatalf("commit failed because a hook ran: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "hook-ran.txt")); !os.IsNotExist(err) {
		t.Fatal("the repository's hook ran")
	}
}

func TestSubjects(t *testing.T) {
	g, dir := newRepo(t)
	ctx := t.Context()
	for _, msg := range []string{"feat: one", "feat: two", "fix: three"} {
		write(t, dir, "notes/"+strings.ReplaceAll(msg, " ", "_")+".go", "package notes\n")
		if _, err := g.Commit(ctx, dir, msg+"\n\nWith a body that must not appear."); err != nil {
			t.Fatal(err)
		}
	}
	all, err := g.Subjects(ctx, dir, "HEAD", 10)
	if err != nil || !slices.Equal(all, []string{"fix: three", "feat: two", "feat: one", "chore: initial commit"}) {
		t.Fatalf("subjects = %v %v", all, err)
	}
	if two, _ := g.Subjects(ctx, dir, "HEAD", 2); !slices.Equal(two, []string{"fix: three", "feat: two"}) {
		t.Fatalf("limited subjects = %v", two)
	}
	if one, _ := g.Subjects(ctx, dir, "HEAD", 0); len(one) != 1 {
		t.Fatalf("a limit of zero should still return one subject: %v", one)
	}
	if ranged, _ := g.Subjects(ctx, dir, "HEAD~2..HEAD", 10); !slices.Equal(ranged, []string{"fix: three", "feat: two"}) {
		t.Fatalf("range = %v", ranged)
	}
	if _, err := g.Subjects(ctx, dir, "--all", 5); err == nil {
		t.Fatal("an option passed as a revision was accepted")
	}
	if _, err := g.Subjects(ctx, dir, "no-such-ref", 5); err == nil {
		t.Fatal("an unknown revision returned no error")
	}
}

func TestMergeInProgress(t *testing.T) {
	g, dir := conflictingBranches(t)
	if g.MergeInProgress(dir) {
		t.Fatal("no merge has started")
	}
	if _, err := g.Merge(t.Context(), dir, "main"); err != nil {
		t.Fatal(err)
	}
	if !g.MergeInProgress(dir) {
		t.Fatal("a merge with conflicts is in progress")
	}
	if err := g.MergeAbort(t.Context(), dir); err != nil {
		t.Fatal(err)
	}
	if g.MergeInProgress(dir) {
		t.Fatal("the merge was aborted")
	}
}
