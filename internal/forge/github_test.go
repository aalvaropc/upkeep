package forge_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/aalvaropc/upkeep/internal/config"
	"github.com/aalvaropc/upkeep/internal/forge"
	"github.com/aalvaropc/upkeep/internal/forge/forgetest"
	"github.com/aalvaropc/upkeep/internal/github"
	"github.com/aalvaropc/upkeep/internal/gitx"
	"github.com/aalvaropc/upkeep/internal/pull"
)

type api struct {
	mu     sync.Mutex
	calls  []string
	pulls  string
	graph  string
	merge  string
	status int
}

func (a *api) handler(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	a.mu.Lock()
	a.calls = append(a.calls, r.Method+" "+r.URL.Path+" "+string(raw))
	a.mu.Unlock()
	switch {
	case r.URL.Path == "/repos/o/r":
		_, _ = w.Write([]byte(`{"default_branch":"main"}`))
	case r.URL.Path == "/repos/o/r/pulls" && r.Method == http.MethodGet:
		_, _ = w.Write([]byte(a.pulls))
	case strings.HasPrefix(r.URL.Path, "/repos/o/r/pulls/") && strings.HasSuffix(r.URL.Path, "/merge"):
		if a.status != 0 {
			w.WriteHeader(a.status)
		}
		_, _ = w.Write([]byte(a.merge))
	case strings.HasPrefix(r.URL.Path, "/repos/o/r/pulls/"):
		_, _ = w.Write([]byte(`{"number":8,"node_id":"PR_fresh","title":"Bump example.com/x from 1.0.0 to 1.0.1","head":{"ref":"dependabot/go_modules/example.com/x-1.0.1","sha":"head1"}}`))
	case r.URL.Path == "/graphql":
		_, _ = w.Write([]byte(a.graph))
	case strings.HasSuffix(r.URL.Path, "/comments"):
		w.WriteHeader(http.StatusCreated)
	default:
		http.NotFound(w, r)
	}
}

func (a *api) log() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return strings.Join(a.calls, "\n")
}

func newGitHub(t *testing.T, a *api) *forge.GitHub {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(a.handler))
	t.Cleanup(srv.Close)
	local := forgetest.New(t)
	g := forge.NewGitHub(&github.Client{BaseURL: srv.URL, Token: config.Secret("secret-token")}, gitx.Git{}, "o", "r", []string{"renovate[bot]", "ci-helper"})
	g.CloneURL = local.Repo
	return g
}

const pullList = `[
  {"number":1,"node_id":"PR_1","title":"Add a feature","user":{"login":"alice","type":"User"},"labels":[{"name":"upkeep"}],"head":{"ref":"feat/a","sha":"aaa","repo":{"full_name":"o/r"}},"base":{"ref":"main"}},
  {"number":2,"node_id":"PR_2","title":"Bump example.com/x from 1.0.0 to 1.0.1","user":{"login":"dependabot[bot]","type":"Bot"},"head":{"ref":"dependabot/go_modules/example.com/x-1.0.1","sha":"bbb","repo":{"full_name":"o/r"}},"base":{"ref":"main"}},
  {"number":3,"node_id":"PR_3","title":"Update a thing","user":{"login":"ci-helper","type":"User"},"head":{"ref":"feat/b","sha":"ccc","repo":{"full_name":"O/R"}},"base":{"ref":"main"}},
  {"number":4,"title":"Work in progress","draft":true,"user":{"login":"alice","type":"User"},"head":{"ref":"feat/wip","sha":"d","repo":{"full_name":"o/r"}},"base":{"ref":"main"}},
  {"number":5,"title":"From a fork","user":{"login":"mallory","type":"User"},"head":{"ref":"patch-1","sha":"e","repo":{"full_name":"mallory/r"}},"base":{"ref":"main"}},
  {"number":6,"title":"Against another branch","user":{"login":"alice","type":"User"},"head":{"ref":"feat/c","sha":"f","repo":{"full_name":"o/r"}},"base":{"ref":"release"}},
  {"number":7,"title":"Fork deleted","user":{"login":"ghost","type":"User"},"head":{"ref":"x","sha":"g","repo":null},"base":{"ref":"main"}}
]`

func TestOnlyPullRequestsFromTheRepositoryItselfAreListed(t *testing.T) {
	g := newGitHub(t, &api{pulls: pullList})
	pulls, err := g.Pulls(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var numbers []int
	for _, p := range pulls {
		numbers = append(numbers, p.Number)
	}
	if len(numbers) != 3 || numbers[0] != 1 || numbers[1] != 2 || numbers[2] != 3 {
		t.Fatalf("listed %v, want 1, 2 and 3 (not drafts, forks, other bases or deleted forks)", numbers)
	}
	if p := pulls[0]; p.Author.Kind != pull.Person || p.Labels[0] != "upkeep" || p.Kind != pull.Change || p.Base != "main" || p.Branch != "feat/a" || p.HeadSHA != "aaa" {
		t.Fatalf("#1 = %+v", p)
	}
	if p := pulls[1]; p.Author.Kind != pull.Bot || p.Kind != pull.Dependency || p.Bump == nil || p.Bump.Module != "example.com/x" || p.Bump.From != "v1.0.0" || p.Bump.To != "v1.0.1" || p.Bump.Level != pull.Patch {
		t.Fatalf("#2 = %+v", p)
	}
	if pulls[2].Author.Kind != pull.Bot {
		t.Fatalf("a login on the policy's bot list should count as a bot: %+v", pulls[2])
	}
}

func TestBumpFromTitle(t *testing.T) {
	const branch = "dependabot/go_modules/github.com/rs/cors-1.11.1"
	tests := []struct {
		title, branch string
		want          *pull.Bump
	}{
		{"build(deps): bump github.com/rs/cors from 1.11.0 to 1.11.1", branch, &pull.Bump{Module: "github.com/rs/cors", From: "v1.11.0", To: "v1.11.1", Level: pull.Patch}},
		{"Bump github.com/jonboulle/clockwork from 0.4.0 to 0.5.0", "dependabot/go_modules/github.com/jonboulle/clockwork-0.5.0", &pull.Bump{Module: "github.com/jonboulle/clockwork", From: "v0.4.0", To: "v0.5.0", Level: pull.Minor}},
		{"chore(deps): Bump example.com/m from v1.2.3 to v2.0.0.", branch, &pull.Bump{Module: "example.com/m", From: "v1.2.3", To: "v2.0.0", Level: pull.Major}},
		{"build(deps): bump the go-deps group with 2 updates", branch, nil},
		{"build(deps): bump github.com/rs/cors from 1.11.0 to 1.11.1", "feat/something", nil},
		{"build(deps): bump github.com/x/y from latest to next", branch, nil},
		{"Add a feature", branch, nil},
	}
	for _, tt := range tests {
		got := forge.BumpFromTitle(tt.title, tt.branch)
		switch {
		case tt.want == nil && got != nil:
			t.Errorf("%q: got %+v, want none", tt.title, got)
		case tt.want != nil && (got == nil || *got != *tt.want):
			t.Errorf("%q: got %+v, want %+v", tt.title, got, tt.want)
		}
	}
}

func TestAFixIsPreparedPushedAndExplainedOnTheRealBranch(t *testing.T) {
	a := &api{pulls: pullList}
	g := newGitHub(t, a)
	pulls, err := g.Pulls(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "work")
	if err := g.Prepare(t.Context(), pulls[0], dir); err != nil {
		t.Fatal(err)
	}
	if got := forgetest.Git(t, dir, "rev-parse", "--abbrev-ref", "HEAD"); got != "feat/a" {
		t.Fatalf("checked out %q", got)
	}
	forgetest.Write(t, dir, "fix.txt", "fixed\n")
	forgetest.Git(t, dir, "add", "-A")
	forgetest.Git(t, dir, "commit", "--quiet", "-m", "fix")
	if err := g.Push(t.Context(), dir, pulls[0]); err != nil {
		t.Fatal(err)
	}
	if got := forgetest.Git(t, g.CloneURL, "show", "feat/a:fix.txt"); got != "fixed" {
		t.Fatalf("the pushed branch has fix.txt = %q", got)
	}
	if err := g.Comment(t.Context(), pulls[0], forge.Comment{Kind: "patch", Body: "a patch"}); err != nil {
		t.Fatal(err)
	}
	if log := a.log(); !strings.Contains(log, "POST /repos/o/r/issues/1/comments") || strings.Contains(log, "secret-token") {
		t.Fatalf("calls:\n%s", log)
	}
}

func TestMergeArmsAutoMergeSoGitHubWaitsForTheChecks(t *testing.T) {
	a := &api{pulls: pullList, graph: `{"data":{"enablePullRequestAutoMerge":{"pullRequest":{"number":8}}}}`}
	g := newGitHub(t, a)
	sha, err := g.Merge(t.Context(), pull.Request{Number: 8})
	if err != nil || sha != "" {
		t.Fatalf("sha %q error %v", sha, err)
	}
	log := a.log()
	if !strings.Contains(log, "PR_fresh") || strings.Contains(log, "/merge") {
		t.Fatalf("calls:\n%s", log)
	}
}

func TestMergeFallsBackToTheMergeCallWhenThePullRequestIsAlreadyClean(t *testing.T) {
	a := &api{graph: `{"errors":[{"message":"Pull request is in clean status"}]}`, merge: `{"sha":"merged456","merged":true}`}
	g := newGitHub(t, a)
	sha, err := g.Merge(t.Context(), pull.Request{Number: 8})
	if err != nil || sha != "merged456" {
		t.Fatalf("sha %q error %v", sha, err)
	}
	if log := a.log(); !strings.Contains(log, "PUT /repos/o/r/pulls/8/merge") || !strings.Contains(log, `"sha":"head1"`) || !strings.Contains(log, `"commit_title":"Bump example.com/x from 1.0.0 to 1.0.1"`) {
		t.Fatalf("calls:\n%s", log)
	}
}

func TestMergeRefusalsBecomeNotMergeable(t *testing.T) {
	a := &api{graph: `{"errors":[{"message":"Auto merge is not allowed for this repository"}]}`}
	g := newGitHub(t, a)
	if _, err := g.Merge(t.Context(), pull.Request{Number: 8}); !errors.Is(err, forge.ErrNotMergeable) || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("auto-merge refused: %v", err)
	}
	b := &api{graph: `{"errors":[{"message":"Pull request is in clean status"}]}`, status: http.StatusMethodNotAllowed, merge: `{"message":"Required status check is expected"}`}
	if _, err := newGitHub(t, b).Merge(t.Context(), pull.Request{Number: 8}); !errors.Is(err, forge.ErrNotMergeable) || !strings.Contains(err.Error(), "Required status check") {
		t.Fatalf("merge refused: %v", err)
	}
}

func TestADryRunChangesNothingAndSaysWhatItWouldHaveDone(t *testing.T) {
	a := &api{pulls: pullList}
	d := &forge.DryRun{Forge: newGitHub(t, a)}
	pr := pull.Request{Number: 2, Branch: "feat/a"}
	if err := d.Push(t.Context(), t.TempDir(), pr); err != nil {
		t.Fatal(err)
	}
	if err := d.Comment(t.Context(), pr, forge.Comment{Kind: "receipt"}); err != nil {
		t.Fatal(err)
	}
	sha, err := d.Merge(t.Context(), pr)
	if err != nil || !strings.HasPrefix(sha, "dry-run") {
		t.Fatalf("sha %q error %v", sha, err)
	}
	if strings.Contains(a.log(), "POST") || strings.Contains(a.log(), "PUT") {
		t.Fatalf("a dry run reached GitHub:\n%s", a.log())
	}
	want := []string{"would push the fix to feat/a (#2)", "would post a receipt comment on #2", "would merge #2"}
	if strings.Join(d.Notes, "|") != strings.Join(want, "|") {
		t.Fatalf("notes = %v", d.Notes)
	}
	if pulls, err := d.Pulls(t.Context()); err != nil || len(pulls) != 3 {
		t.Fatalf("a dry run still lists the pull requests: %d %v", len(pulls), err)
	}
}

func TestTheTokenReachesGitAsAHeaderNotInTheArguments(t *testing.T) {
	g := gitx.Git{}.WithToken("secret-token")
	joined := strings.Join(g.Env, "\n")
	if !strings.Contains(joined, "GIT_CONFIG_KEY_0=http.https://github.com/.extraheader") || !strings.Contains(joined, "AUTHORIZATION: basic ") || strings.Contains(joined, "secret-token") {
		t.Fatalf("env = %q", joined)
	}
	raw, err := json.Marshal(g.Env)
	if err != nil || strings.Contains(string(raw), "secret-token") {
		t.Fatalf("the token must only be present base64-encoded: %s", raw)
	}
}

func TestOnlyTheSelectedPullRequestsAreListed(t *testing.T) {
	g := newGitHub(t, &api{pulls: pullList})
	only := forge.Only{Forge: g, Numbers: []int{2, 3, 99}}
	pulls, err := only.Pulls(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(pulls) != 2 || pulls[0].Number != 2 || pulls[1].Number != 3 {
		t.Fatalf("listed %+v", pulls)
	}
}
