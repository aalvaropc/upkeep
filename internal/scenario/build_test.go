package scenario

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/aalvaropc/upkeep/internal/gitx"
	"github.com/aalvaropc/upkeep/internal/pull"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-c", "user.name=Author", "-c", "user.email=author@example.com", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)
	cmd := exec.CommandContext(t.Context(), "git", full...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_AUTHOR_DATE=2026-10-01T10:00:00Z", "GIT_COMMITTER_DATE=2026-10-01T10:00:00Z")
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

type fixture struct {
	source  string
	fork    string
	recipes fstest.MapFS
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	source := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(source, 0o750); err != nil {
		t.Fatal(err)
	}
	git(t, source, "init", "--quiet", "--initial-branch=main")
	write(t, source, "a.txt", "one\n")
	git(t, source, "add", "-A")
	git(t, source, "commit", "--quiet", "-m", "chore: start")
	fork := git(t, source, "rev-parse", "HEAD")

	recipes := fstest.MapFS{}
	record := func(name, file, content, message string) {
		write(t, source, file, content)
		git(t, source, "add", "-A")
		git(t, source, "commit", "--quiet", "-m", message)
		mbox := git(t, source, "format-patch", "-1", "--stdout")
		recipes["recipes/"+name+"/01-change.patch"] = &fstest.MapFile{Data: []byte(mbox + "\n")}
		git(t, source, "reset", "--hard", "--quiet", fork)
	}
	record("edit-a", "a.txt", "two\n", "feat: edit the first file")
	record("add-b", "b.txt", "bee\n", "feat: add a second file")
	record("add-c", "c.txt", "sea\n", "feat: add a third file")
	return fixture{source: source, fork: fork, recipes: recipes}
}

func (f fixture) scenario() Scenario {
	return Scenario{
		Name:       "tiny",
		Repository: "tiny",
		Fork:       f.fork,
		Base:       "main",
		Main:       []string{"edit-a"},
		PRs: []PR{
			{Number: 1, Title: "Add b", Branch: "feat/b", Author: pull.Author{Login: "alice", Kind: pull.Person}, Recipe: "add-b", Expect: Expect{Blocker: pull.Behind, Outcome: pull.Mergeable}},
			{Number: 2, Title: "Add c", Branch: "feat/c", Onto: OntoMain, Author: pull.Author{Login: "bob", Kind: pull.Person}, Recipe: "add-c", Expect: Expect{Blocker: pull.Clean, Outcome: pull.Mergeable}},
		},
	}
}

func TestBuildCreatesTheBranchesOnTheirStartingPoints(t *testing.T) {
	f := newFixture(t)
	out := filepath.Join(t.TempDir(), "forge.git")
	built, err := Builder{Git: gitx.Git{}, Recipes: f.recipes}.Build(t.Context(), f.scenario(), f.source, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(built.Heads) != 3 || built.Main != built.Heads["main"] {
		t.Fatalf("heads = %v, main = %s", built.Heads, built.Main)
	}
	if got := git(t, out, "show", "main:a.txt"); got != "two" {
		t.Errorf("main has a.txt = %q, want the main recipe applied", got)
	}
	if got := git(t, out, "show", "feat/b:a.txt"); got != "one" {
		t.Errorf("a pull request starting at the fork has a.txt = %q, want the original", got)
	}
	if got := git(t, out, "show", "feat/c:a.txt"); got != "two" {
		t.Errorf("a pull request starting at main has a.txt = %q, want main's version", got)
	}
	if got := git(t, out, "show", "feat/c:c.txt"); got != "sea" {
		t.Errorf("feat/c:c.txt = %q", got)
	}
	if got := git(t, out, "log", "-1", "--format=%s", "feat/b"); got != "feat: add a second file" {
		t.Errorf("the recipe's commit message was lost: %q", got)
	}
	if behind := git(t, out, "rev-list", "--count", "feat/b..main"); behind != "1" {
		t.Errorf("feat/b is %s commits behind main, want 1", behind)
	}
	if behind := git(t, out, "rev-list", "--count", "feat/c..main"); behind != "0" {
		t.Errorf("feat/c is %s commits behind main, want 0", behind)
	}
}

func TestBuildIsReproducible(t *testing.T) {
	f := newFixture(t)
	b := Builder{Git: gitx.Git{}, Recipes: f.recipes}
	first, err := b.Build(t.Context(), f.scenario(), f.source, filepath.Join(t.TempDir(), "one.git"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := b.Build(t.Context(), f.scenario(), f.source, filepath.Join(t.TempDir(), "two.git"))
	if err != nil {
		t.Fatal(err)
	}
	for branch, sha := range first.Heads {
		if second.Heads[branch] != sha {
			t.Errorf("%s: %s then %s", branch, sha, second.Heads[branch])
		}
	}
}

func TestBuildRefusesWhatItCannotTrust(t *testing.T) {
	f := newFixture(t)
	tests := []struct {
		name   string
		mutate func(*Scenario)
		want   string
	}{
		{"missing recipe", func(s *Scenario) { s.PRs[0].Recipe = "nowhere" }, "recipe"},
		{"path in recipe name", func(s *Scenario) { s.PRs[0].Recipe = "../add-b" }, "not a recipe name"},
		{"unknown fork", func(s *Scenario) { s.Fork = strings.Repeat("0", 40) }, "fork commit"},
		{"invalid scenario", func(s *Scenario) { s.Name = "" }, "name is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := f.scenario()
			s.PRs = append([]PR(nil), s.PRs...)
			tt.mutate(&s)
			_, err := Builder{Git: gitx.Git{}, Recipes: f.recipes}.Build(t.Context(), s, f.source, filepath.Join(t.TempDir(), "forge.git"))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Build() = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

func TestBuildDoesNotOverwrite(t *testing.T) {
	f := newFixture(t)
	out := t.TempDir()
	write(t, out, "keep.txt", "mine\n")
	_, err := Builder{Git: gitx.Git{}, Recipes: f.recipes}.Build(context.Background(), f.scenario(), f.source, out)
	if err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("Build() = %v, want a refusal to use a non-empty directory", err)
	}
}
