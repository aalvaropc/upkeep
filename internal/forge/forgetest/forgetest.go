package forgetest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aalvaropc/upkeep/internal/forge"
	"github.com/aalvaropc/upkeep/internal/gitx"
	"github.com/aalvaropc/upkeep/internal/pull"
	"github.com/aalvaropc/upkeep/internal/scenario"
)

func Git(t testing.TB, dir string, args ...string) string {
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

func Write(t testing.TB, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func Scenario() scenario.Scenario {
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

func New(t testing.TB) *forge.Local {
	t.Helper()
	root := t.TempDir()
	src := filepath.Join(root, "src")
	if err := os.MkdirAll(src, 0o750); err != nil {
		t.Fatal(err)
	}
	Git(t, src, "init", "--quiet", "--initial-branch=main")
	Write(t, src, "a.txt", "one\n")
	Write(t, src, "b.txt", "bee\n")
	Git(t, src, "add", "-A")
	Git(t, src, "commit", "--quiet", "-m", "chore: start")
	branch := func(name, file, content string) {
		Git(t, src, "switch", "--quiet", "-c", name, "main")
		Write(t, src, file, content)
		Git(t, src, "add", "-A")
		Git(t, src, "commit", "--quiet", "-m", "change "+file)
	}
	branch("feat/a", "a.txt", "two\n")
	branch("feat/a2", "a.txt", "three\n")
	branch("feat/b", "b.txt", "see\n")
	branch("dependabot/x", "c.txt", "new\n")
	Git(t, src, "switch", "--quiet", "main")
	bare := filepath.Join(root, "forge.git")
	g := gitx.Git{}
	if err := g.CloneBare(t.Context(), src, bare); err != nil {
		t.Fatal(err)
	}
	return forge.NewLocal(g, bare, Scenario())
}
