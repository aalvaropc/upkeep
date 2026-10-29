package gitx

import (
	"slices"
	"strings"
	"testing"
)

func branchFrom(t *testing.T, g Git, dir, from, name string, files map[string]string) string {
	t.Helper()
	ctx := t.Context()
	if err := g.Checkout(ctx, dir, from, false); err != nil {
		t.Fatal(err)
	}
	if err := g.Checkout(ctx, dir, name, true); err != nil {
		t.Fatal(err)
	}
	for path, content := range files {
		write(t, dir, path, content)
	}
	head, err := g.Commit(ctx, dir, "change "+name)
	if err != nil {
		t.Fatal(err)
	}
	return head
}

func TestTouchedListsBothSidesOfARename(t *testing.T) {
	g, dir := newRepo(t)
	ctx := t.Context()
	branchFrom(t, g, dir, "main", "tools", map[string]string{"tools/helper.go": "package tools\n\nfunc Helper() int { return 1 }\n"})
	if err := g.Checkout(ctx, dir, "main", false); err != nil {
		t.Fatal(err)
	}
	gitOut(t, g, dir, "merge", "--quiet", "--ff-only", "tools")
	if err := g.Checkout(ctx, dir, "moved", true); err != nil {
		t.Fatal(err)
	}
	gitOut(t, g, dir, "mv", "tools/helper.go", "helper.go")
	if _, err := g.Commit(ctx, dir, "move the helper"); err != nil {
		t.Fatal(err)
	}
	files, err := g.Touched(ctx, dir, "main", "moved")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"helper.go", "tools/helper.go"}; !slices.Equal(files, want) {
		t.Fatalf("Touched = %v, want both paths of the rename %v", files, want)
	}
}

func TestTouchedListsOnlyWhatTheBranchChanged(t *testing.T) {
	g, dir := newRepo(t)
	ctx := t.Context()
	branchFrom(t, g, dir, "main", "feature", map[string]string{
		"notes/service.go": "package notes\n\nfunc List() int { return 1 }\n",
		"notes/search.go":  "package notes\n",
	})
	branchFrom(t, g, dir, "main", "later", map[string]string{"README.md": "# notes\n"})
	if err := g.Checkout(ctx, dir, "main", false); err != nil {
		t.Fatal(err)
	}
	gitOut(t, g, dir, "merge", "--quiet", "--ff-only", "later")
	files, err := g.Touched(ctx, dir, "main", "feature")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"notes/search.go", "notes/service.go"}; !slices.Equal(files, want) {
		t.Fatalf("Touched = %v, want %v", files, want)
	}
	if none, err := g.Touched(ctx, dir, "main", "main"); err != nil || len(none) != 0 {
		t.Fatalf("Touched(main, main) = %v %v", none, err)
	}
	if _, err := g.Touched(ctx, dir, "main", "--output=x"); err == nil {
		t.Fatal("an option passed as a revision was accepted")
	}
}

func TestMergeTreeOfBranchesThatChangeDifferentFilesIsClean(t *testing.T) {
	g, dir := newRepo(t)
	a := branchFrom(t, g, dir, "main", "a", map[string]string{"notes/service.go": "package notes\n\nfunc List() int { return 1 }\n"})
	b := branchFrom(t, g, dir, "main", "b", map[string]string{"notes/limits.go": "package notes\n\nconst (\n\tMaxTagLength = 64\n)\n"})
	clean, files, err := g.MergeTree(t.Context(), dir, a, b)
	if err != nil || !clean || len(files) != 0 {
		t.Fatalf("MergeTree = %v %v %v", clean, files, err)
	}
}

func TestMergeTreeNamesTheConflictingFilesAndLeavesTheCheckoutAlone(t *testing.T) {
	g, dir := conflictingBranches(t)
	ctx := t.Context()
	before := gitOut(t, g, dir, "rev-parse", "HEAD")
	clean, files, err := g.MergeTree(ctx, dir, "main", "feature")
	if err != nil {
		t.Fatal(err)
	}
	if clean || !slices.Equal(files, []string{"notes/limits.go"}) {
		t.Fatalf("MergeTree = %v %v", clean, files)
	}
	if after := gitOut(t, g, dir, "rev-parse", "HEAD"); after != before {
		t.Fatal("merge-tree moved HEAD")
	}
	if status := gitOut(t, g, dir, "status", "--porcelain"); strings.TrimSpace(status) != "" || g.MergeInProgress(dir) {
		t.Fatalf("merge-tree touched the checkout: %q", status)
	}
}

func TestMergeTreeTreatsChangesToAdjacentLinesAsAConflict(t *testing.T) {
	g, dir := newRepo(t)
	ctx := t.Context()
	module := func(jwt, clock string) map[string]string {
		return map[string]string{
			"go.mod": "module example.com/notes\n\ngo 1.26\n\nrequire (\n\tgithub.com/golang-jwt/jwt " + jwt + "\n\tgithub.com/jonboulle/clockwork " + clock + "\n\tgithub.com/rs/cors v1.11.0\n)\n",
		}
	}
	branchFrom(t, g, dir, "main", "base", module("v4.5.2", "v0.4.0"))
	jwt := branchFrom(t, g, dir, "base", "jwt", module("v5.3.1", "v0.4.0"))
	clock := branchFrom(t, g, dir, "base", "clock", module("v4.5.2", "v0.5.0"))
	clean, files, err := g.MergeTree(ctx, dir, jwt, clock)
	if err != nil {
		t.Fatal(err)
	}
	if clean || !slices.Equal(files, []string{"go.mod"}) {
		t.Fatalf("two bumps on adjacent lines: MergeTree = %v %v", clean, files)
	}
}

func TestMergeTreeOfAnUnknownRevisionIsAnError(t *testing.T) {
	g, dir := newRepo(t)
	if _, _, err := g.MergeTree(t.Context(), dir, "main", "does-not-exist"); err == nil {
		t.Fatal("an unknown revision returned no error")
	}
	if _, _, err := g.MergeTree(t.Context(), dir, "-main", "main"); err == nil {
		t.Fatal("an option passed as a revision was accepted")
	}
}
