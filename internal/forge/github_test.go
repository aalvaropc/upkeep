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
	mu       sync.Mutex
	calls    []string
	pulls    string
	graph    string
	merge    string
	status   int
	comments string
	edit     int
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
	case strings.HasPrefix(r.URL.Path, "/repos/o/r/issues/comments/"):
		if a.edit != 0 {
			w.WriteHeader(a.edit)
		}
		_, _ = w.Write([]byte(`{}`))
	case strings.HasSuffix(r.URL.Path, "/comments") && r.Method == http.MethodGet:
		if a.comments == "" {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		_, _ = w.Write([]byte(a.comments))
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

func TestBumpFromPull(t *testing.T) {
	const table = "This PR contains the following updates:\n\n| Package | Change | Age | Confidence |\n|---|---|---|---|\n"
	jwt := table + "| [github.com/golang-jwt/jwt/v4](https://redirect.github.com/golang-jwt/jwt) | `v4.5.2` -> `v5.3.1` | ![age](x) | ![confidence](y) |\n"
	cors := table + "| [github.com/rs/cors](https://redirect.github.com/rs/cors) | `v1.11.0` → `v1.11.1` | ![age](x) |\n"
	group := table + "| [golang.org/x/net](https://pkg.go.dev/golang.org/x/net) | `v0.29.0` -> `v0.30.0` |\n| [golang.org/x/text](https://pkg.go.dev/golang.org/x/text) | `v0.18.0` -> `v0.19.0` |\n"
	tests := []struct {
		name, title, branch, body string
		want                      *pull.Bump
	}{
		{"dependabot", "build(deps): bump github.com/rs/cors from 1.11.0 to 1.11.1", "dependabot/go_modules/github.com/rs/cors-1.11.1", "", &pull.Bump{Module: "github.com/rs/cors", From: "v1.11.0", To: "v1.11.1", Level: pull.Patch}},
		{"dependabot ignores the body", "Bump github.com/rs/cors from 1.11.0 to 1.11.1", "dependabot/go_modules/github.com/rs/cors-1.11.1", cors, &pull.Bump{Module: "github.com/rs/cors", From: "v1.11.0", To: "v1.11.1", Level: pull.Patch}},
		{"dependabot outside go modules", "Bump lodash from 4.17.20 to 4.17.21", "dependabot/npm_and_yarn/lodash-4.17.21", "", nil},
		{"renovate module", "fix(deps): update module github.com/rs/cors to v1.11.1", "renovate/github.com-rs-cors-1.x", cors, &pull.Bump{Module: "github.com/rs/cors", From: "v1.11.0", To: "v1.11.1", Level: pull.Patch}},
		{"renovate major with a new path", "Update github.com/golang-jwt/jwt/v4 to v5.3.1", "renovate/github.com-golang-jwt-jwt-v5", jwt, &pull.Bump{Module: "github.com/golang-jwt/jwt/v5", From: "v4.5.2", To: "v5.3.1", Level: pull.Major}},
		{"renovate major named by its major", "chore(deps): update module github.com/golang-jwt/jwt/v4 to v5 (major)", "renovate/major-github.com-golang-jwt-jwt-v4", jwt, &pull.Bump{Module: "github.com/golang-jwt/jwt/v5", From: "v4.5.2", To: "v5.3.1", Level: pull.Major}},
		{"renovate major without a suffix", "Update module github.com/x/y to v2.0.0", "renovate/github.com-x-y-2.x", "| github.com/x/y | `v1.4.0` -> `v2.0.0` |", &pull.Bump{Module: "github.com/x/y", From: "v1.4.0", To: "v2.0.0", Level: pull.Major}},
		{"renovate minor", "Update module golang.org/x/net to v0.30.0 [SECURITY]", "renovate/go-x-net", group, &pull.Bump{Module: "golang.org/x/net", From: "v0.29.0", To: "v0.30.0", Level: pull.Minor}},
		{"renovate without the from version", "Update module github.com/rs/cors to v1.11.1", "renovate/github.com-rs-cors-1.x", "Nothing useful here.", nil},
		{"renovate with a body for another version", "Update module github.com/rs/cors to v1.12.0", "renovate/github.com-rs-cors-1.x", cors, nil},
		{"renovate group", "Update all non-major dependencies", "renovate/all-minor-patch", group, nil},
		{"renovate digest", "Update golang.org/x/exp digest to 7e4ce0a", "renovate/golang.org-x-exp-digest", "| golang.org/x/exp | `9bf2ced` -> `7e4ce0a` |", nil},
		{"renovate action", "Update actions/checkout action to v5", "renovate/actions-checkout-5.x", "| actions/checkout | `v4` -> `v5` |", nil},
		{"renovate npm package", "Update dependency react to v19.0.0", "renovate/react-monorepo", "| react | `18.3.1` -> `19.0.0` |", nil},
		{"renovate title on another branch", "Update module github.com/rs/cors to v1.11.1", "feat/cors", cors, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := forge.BumpFromPull(tt.title, tt.branch, tt.body)
			switch {
			case tt.want == nil && got != nil:
				t.Errorf("got %+v, want none", got)
			case tt.want != nil && (got == nil || *got != *tt.want):
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestARenovatePullRequestIsListedAsADependency(t *testing.T) {
	list := `[{"number":5,"node_id":"PR_5","title":"Update github.com/golang-jwt/jwt/v4 to v5.3.1","body":"| github.com/golang-jwt/jwt/v4 | ` + "`v4.5.2` -> `v5.3.1`" + ` |","user":{"login":"renovate[bot]","type":"Bot"},"head":{"ref":"renovate/github.com-golang-jwt-jwt-v5","sha":"eee","repo":{"full_name":"o/r"}},"base":{"ref":"main"}}]`
	pulls, err := newGitHub(t, &api{pulls: list}).Pulls(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(pulls) != 1 || pulls[0].Kind != pull.Dependency || pulls[0].Author.Kind != pull.Bot || pulls[0].Bump == nil || *pulls[0].Bump != (pull.Bump{Module: "github.com/golang-jwt/jwt/v5", From: "v4.5.2", To: "v5.3.1", Level: pull.Major}) {
		t.Fatalf("pulls = %+v", pulls)
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
	if err := d.Push(t.Context(), t.TempDir(), pr); !errors.Is(err, forge.ErrDryRun) {
		t.Fatalf("push: %v", err)
	}
	if err := d.Comment(t.Context(), pr, forge.Comment{Kind: "receipt"}); !errors.Is(err, forge.ErrDryRun) {
		t.Fatalf("comment: %v", err)
	}
	sha, err := d.Merge(t.Context(), pr)
	if !errors.Is(err, forge.ErrDryRun) || sha != "" {
		t.Fatalf("a dry run reported a merge: sha %q error %v", sha, err)
	}
	if strings.Contains(a.log(), "POST") || strings.Contains(a.log(), "PUT") || strings.Contains(a.log(), "PATCH") {
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

func TestADryRunTakesNotesFromManyWorkersAtOnce(t *testing.T) {
	d := &forge.DryRun{Forge: newGitHub(t, &api{pulls: pullList})}
	var wg sync.WaitGroup
	for n := range 8 {
		wg.Go(func() { _ = d.Comment(t.Context(), pull.Request{Number: n}, forge.Comment{Kind: "receipt"}) })
	}
	wg.Wait()
	if len(d.Notes) != 8 {
		t.Fatalf("notes = %v", d.Notes)
	}
}

func TestACommentIsPostedOnceWithItsMarker(t *testing.T) {
	a := &api{comments: `[{"id":10,"body":"Looks good to me","user":{"login":"alice"}}]`}
	g := newGitHub(t, a)
	if err := g.Comment(t.Context(), pull.Request{Number: 1}, forge.Comment{Kind: "receipt", Body: "Upkeep receipt"}); err != nil {
		t.Fatal(err)
	}
	log := a.log()
	if !strings.Contains(log, "GET /repos/o/r/issues/1/comments") || !strings.Contains(log, `POST /repos/o/r/issues/1/comments {"body":"\u003c!-- upkeep:receipt --\u003e\nUpkeep receipt"}`) || strings.Contains(log, "PATCH") {
		t.Fatalf("calls:\n%s", log)
	}
}

func TestACommentUpdatesTheEarlierOneOfItsKind(t *testing.T) {
	a := &api{comments: `[
  {"id":10,"body":"<!-- upkeep:receipt -->\nUpkeep receipt, first run","user":{"login":"upkeep-bot"}},
  {"id":11,"body":"<!-- upkeep:patch -->\nUpkeep prepared a fix","user":{"login":"upkeep-bot"}},
  {"id":12,"body":"Thanks!","user":{"login":"alice"}},
  {"id":13,"body":"<!-- upkeep:receipt -->\nUpkeep receipt, second run","user":{"login":"upkeep-bot"}}
]`}
	g := newGitHub(t, a)
	if err := g.Comment(t.Context(), pull.Request{Number: 1}, forge.Comment{Kind: "receipt", Body: "Upkeep receipt, third run"}); err != nil {
		t.Fatal(err)
	}
	log := a.log()
	if !strings.Contains(log, "PATCH /repos/o/r/issues/comments/13 ") || !strings.Contains(log, "third run") || strings.Contains(log, "POST") || strings.Contains(log, "comments/10") || strings.Contains(log, "comments/11") {
		t.Fatalf("calls:\n%s", log)
	}
}

func TestAnUnchangedCommentIsLeftAlone(t *testing.T) {
	a := &api{comments: `[{"id":10,"body":"<!-- upkeep:diagnosis -->\nUpkeep could not finish this pull request.","user":{"login":"upkeep-bot"}}]`}
	g := newGitHub(t, a)
	if err := g.Comment(t.Context(), pull.Request{Number: 1}, forge.Comment{Kind: "diagnosis", Body: "Upkeep could not finish this pull request."}); err != nil {
		t.Fatal(err)
	}
	if log := a.log(); strings.Contains(log, "PATCH") || strings.Contains(log, "POST") {
		t.Fatalf("calls:\n%s", log)
	}
}

func TestACommentThatCannotBeEditedIsPostedAgain(t *testing.T) {
	a := &api{edit: http.StatusForbidden, comments: `[{"id":10,"body":"<!-- upkeep:receipt -->\nquoted by someone else","user":{"login":"alice"}}]`}
	g := newGitHub(t, a)
	if err := g.Comment(t.Context(), pull.Request{Number: 1}, forge.Comment{Kind: "receipt", Body: "Upkeep receipt"}); err != nil {
		t.Fatal(err)
	}
	if log := a.log(); !strings.Contains(log, "PATCH /repos/o/r/issues/comments/10") || !strings.Contains(log, "POST /repos/o/r/issues/1/comments") {
		t.Fatalf("calls:\n%s", log)
	}
	b := &api{edit: http.StatusInternalServerError, comments: a.comments}
	if err := newGitHub(t, b).Comment(t.Context(), pull.Request{Number: 1}, forge.Comment{Kind: "receipt", Body: "Upkeep receipt"}); err == nil {
		t.Fatal("a failed edit returned no error")
	}
	if log := b.log(); strings.Contains(log, "POST") {
		t.Fatalf("a failed edit was followed by a new comment:\n%s", log)
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

func TestAForkIsPreparedFromItsPullRefAndNeverPushed(t *testing.T) {
	g := newGitHub(t, &api{pulls: pullList})
	g.AllowForks = true
	pulls, err := g.Pulls(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var fork pull.Request
	for _, p := range pulls {
		if p.Number == 5 {
			fork = p
		}
	}
	if !fork.Fork || fork.Number != 5 {
		t.Fatalf("the fork is listed and marked: %+v", pulls)
	}
	head := forgetest.Git(t, g.CloneURL, "rev-parse", "main")
	forgetest.Git(t, g.CloneURL, "update-ref", "refs/pull/5/head", head)
	dir := filepath.Join(t.TempDir(), "work")
	if err := g.Prepare(t.Context(), fork, dir); err != nil {
		t.Fatal(err)
	}
	if got := forgetest.Git(t, dir, "rev-parse", "--abbrev-ref", "HEAD"); got != "upkeep/pr-5" {
		t.Fatalf("checked out %q", got)
	}
	if err := g.Push(t.Context(), dir, fork); err == nil || !strings.Contains(err.Error(), "fork") {
		t.Fatalf("pushing to a fork must fail, got %v", err)
	}
}
