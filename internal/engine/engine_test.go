package engine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aalvaropc/upkeep/internal/events"
	"github.com/aalvaropc/upkeep/internal/fixer"
	"github.com/aalvaropc/upkeep/internal/gitx"
	"github.com/aalvaropc/upkeep/internal/model"
	"github.com/aalvaropc/upkeep/internal/policy"
	"github.com/aalvaropc/upkeep/internal/pull"
	"github.com/aalvaropc/upkeep/internal/sandbox"
	"github.com/aalvaropc/upkeep/internal/strategy"
)

type step struct {
	calls   []model.ToolCall
	content string
	usage   model.Usage
	err     error
}

type scripted struct {
	mu       sync.Mutex
	steps    []step
	requests []model.Request
}

func (s *scripted) Complete(_ context.Context, req model.Request) (model.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := len(s.requests)
	s.requests = append(s.requests, req)
	if i >= len(s.steps) {
		return model.Response{}, errors.New("the script ran out of steps")
	}
	st := s.steps[i]
	if st.err != nil {
		return model.Response{}, st.err
	}
	usage := st.usage
	if usage == (model.Usage{}) {
		usage = model.Usage{PromptTokens: 1000, CompletionTokens: 50}
	}
	return model.Response{
		Message: model.Message{Role: "assistant", Content: st.content, ToolCalls: st.calls},
		Usage:   usage,
		Latency: time.Second,
	}, nil
}

func (s *scripted) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.requests)
}

func toolCall(id, name string, v any) model.ToolCall {
	arguments, ok := v.(string)
	if !ok {
		data, err := json.Marshal(v)
		if err != nil {
			panic(err)
		}
		arguments = string(data)
	}
	return model.ToolCall{ID: id, Name: name, Arguments: arguments}
}

type world struct {
	t      *testing.T
	git    gitx.Git
	dir    string
	super  *scripted
	ultra  *scripted
	events []events.Kind
	steps  []string
	mu     sync.Mutex
	pol    policy.Policy
}

const (
	limits  = "package m\n\nconst (\n\tMaxTagLength = 32\n)\n"
	service = "package m\n\nfunc Add(a, b int) int { return a + b }\n"
	addTest = "package m\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal(\"wrong\")\n\t}\n}\n"
)

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

func newWorld(t *testing.T, super, ultra []step) *world {
	t.Helper()
	for _, bin := range []string{"git", "go", "gofmt"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s is not installed", bin)
		}
	}
	w := &world{t: t, git: gitx.Git{}, dir: t.TempDir(), super: &scripted{steps: super}, ultra: &scripted{steps: ultra}, pol: policy.Default()}
	if _, err := w.git.Head(t.Context(), w.dir); err == nil {
		t.Fatal("the temporary directory is already a repository")
	}
	cmd := exec.CommandContext(t.Context(), "git", "init", "--quiet", "-b", "main")
	cmd.Dir = w.dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	write(t, w.dir, "go.mod", "module example.test/m\n\ngo 1.25\n")
	write(t, w.dir, "limits.go", limits)
	write(t, w.dir, "service.go", service)
	write(t, w.dir, "service_test.go", addTest)
	w.commit("chore: initial commit")
	return w
}

func (w *world) commit(msg string) {
	w.t.Helper()
	if _, err := w.git.Commit(w.t.Context(), w.dir, msg); err != nil {
		w.t.Fatal(err)
	}
}

func (w *world) branch(name string) {
	w.t.Helper()
	if err := w.git.Checkout(w.t.Context(), w.dir, name, true); err != nil {
		w.t.Fatal(err)
	}
}

func (w *world) checkout(name string) {
	w.t.Helper()
	if err := w.git.Checkout(w.t.Context(), w.dir, name, false); err != nil {
		w.t.Fatal(err)
	}
}

func (w *world) head() string {
	w.t.Helper()
	h, err := w.git.Head(w.t.Context(), w.dir)
	if err != nil {
		w.t.Fatal(err)
	}
	return h
}

func (w *world) process(pr pull.Request) Result {
	w.t.Helper()
	router := model.NewRouter()
	router.Set(model.Ultra, "ultra", w.ultra)
	router.Set(model.Super, "super", w.super)
	book := model.PriceBook{"super": {InputPerMillion: 0.3, OutputPerMillion: 0.9}, "ultra": {InputPerMillion: 1, OutputPerMillion: 3}}
	env := strategy.Env{
		Git:      w.git,
		Runner:   sandbox.GoRunner{Executor: sandbox.Exec{}},
		Executor: sandbox.Exec{},
		Models:   router,
		Meter:    model.Meter{Prices: book, Baseline: "ultra"},
		Policy:   w.pol,
		Emit: func(kind events.Kind, _ int, data any) {
			w.mu.Lock()
			defer w.mu.Unlock()
			w.events = append(w.events, kind)
			if m, ok := data.(map[string]any); ok && kind == events.PRStep {
				w.steps = append(w.steps, m["step"].(string)+":"+m["status"].(string))
			}
		},
	}
	if pr.Number == 0 {
		pr.Number = 7
	}
	if pr.Branch == "" {
		pr.Branch = "feature"
	}
	return Processor{Env: env}.Process(w.t.Context(), Input{PR: pr, Dir: w.dir, Base: "main"})
}

func person(labels ...string) pull.Request {
	return pull.Request{Author: pull.Author{Login: "alice", Kind: pull.Person}, Labels: labels}
}

func (w *world) parents(rev string) int {
	w.t.Helper()
	out, err := exec.CommandContext(w.t.Context(), "git", "-C", w.dir, "rev-list", "--parents", "-n", "1", rev).Output()
	if err != nil {
		w.t.Fatal(err)
	}
	return len(strings.Fields(string(out))) - 1
}

func (w *world) clean() bool {
	w.t.Helper()
	out, err := exec.CommandContext(w.t.Context(), "git", "-C", w.dir, "status", "--porcelain").Output()
	if err != nil {
		w.t.Fatal(err)
	}
	_, mergeErr := os.Stat(filepath.Join(w.dir, ".git", "MERGE_HEAD"))
	return strings.TrimSpace(string(out)) == "" && os.IsNotExist(mergeErr)
}

func TestCleanPullRequestNeedsNothing(t *testing.T) {
	w := newWorld(t, nil, nil)
	w.branch("feature")
	write(t, w.dir, "extra.go", "package m\n\nfunc Extra() int { return 1 }\n")
	w.commit("feat: add extra")
	before := w.head()

	res := w.process(person())
	if res.Outcome != pull.Mergeable || res.Blocker != pull.Clean || res.Changed || res.Err != nil {
		t.Fatalf("result = %+v", res)
	}
	if !res.Checklist.Passed() || res.WantsPush || res.WantsPatch || res.WantsMerge || res.HeadAfter != before {
		t.Fatalf("checklist %v, wants %v %v %v", res.Checklist.Passed(), res.WantsPush, res.WantsPatch, res.WantsMerge)
	}
	if w.super.count()+w.ultra.count() != 0 || res.Receipt.Totals().Calls != 0 {
		t.Fatal("a clean pull request must not call a model")
	}
}

func TestBehindBranchGetsAMergeCommit(t *testing.T) {
	w := newWorld(t, nil, nil)
	w.branch("feature")
	write(t, w.dir, "extra.go", "package m\n\nfunc Extra() int { return 1 }\n")
	w.commit("feat: add extra")
	w.checkout("main")
	write(t, w.dir, "README.md", "# m\n")
	w.commit("docs: add a readme")
	w.checkout("feature")
	before := w.head()

	res := w.process(person("upkeep"))
	if res.Outcome != pull.Mergeable || res.Blocker != pull.Behind || !res.Changed || res.HeadAfter == before {
		t.Fatalf("result = %+v", res)
	}
	if w.parents("HEAD") != 2 || !w.clean() {
		t.Fatal("the branch should end with a clean merge commit")
	}
	if !res.WantsPush || res.WantsPatch {
		t.Fatalf("a labelled pull request is pushed: push %v patch %v", res.WantsPush, res.WantsPatch)
	}
	if res.Patch != "" || len(res.Changes) != 0 {
		t.Fatalf("a plain merge has no fix of its own: %q %v", res.Patch, res.Changes)
	}
}

func TestPushOrPatchFollowsThePolicy(t *testing.T) {
	for name, tc := range map[string]struct {
		pr         pull.Request
		push       bool
		patch      bool
		wantsMerge bool
	}{
		"person without the label": {person(), false, true, false},
		"person with the label":    {person("Upkeep"), true, false, false},
		"bot patch bump": {pull.Request{Author: pull.Author{Login: "dependabot[bot]", Kind: pull.Bot}, Kind: pull.Dependency,
			Bump: &pull.Bump{Module: "example.com/x", From: "v1.0.0", To: "v1.0.1", Level: pull.Patch}}, true, false, true},
		"bot major bump": {pull.Request{Author: pull.Author{Login: "dependabot[bot]", Kind: pull.Bot}, Kind: pull.Dependency,
			Bump: &pull.Bump{Module: "example.com/x", From: "v1.0.0", To: "v2.0.0", Level: pull.Major}}, true, false, false},
	} {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t, nil, nil)
			w.branch("feature")
			write(t, w.dir, "extra.go", "package m\nfunc   Extra()int{return 1}\n")
			w.commit("feat: add extra")
			res := w.process(tc.pr)
			if res.Outcome != pull.Mergeable || res.Blocker != pull.Format || res.WantsPush != tc.push || res.WantsPatch != tc.patch || res.WantsMerge != tc.wantsMerge {
				t.Fatalf("outcome %s blocker %s push %v patch %v merge %v", res.Outcome, res.Blocker, res.WantsPush, res.WantsPatch, res.WantsMerge)
			}
		})
	}
}

func TestUnformattedCodeIsFormatted(t *testing.T) {
	w := newWorld(t, nil, nil)
	w.branch("feature")
	write(t, w.dir, "extra.go", "package m\nfunc   Extra()int{return 1}\n")
	w.commit("feat: add extra")
	res := w.process(person())
	if res.Outcome != pull.Mergeable || !res.Changed || w.super.count()+w.ultra.count() != 0 {
		t.Fatalf("result = %+v", res)
	}
	if read(t, w.dir, "extra.go") != "package m\n\nfunc Extra() int { return 1 }\n" {
		t.Fatalf("extra.go = %q", read(t, w.dir, "extra.go"))
	}
	if !strings.Contains(res.Patch, "+func Extra() int { return 1 }") || len(res.Changes) != 1 || !res.Checklist.Passed() {
		t.Fatalf("patch %q changes %v", res.Patch, res.Changes)
	}
}

func conflictingLimits(w *world) {
	w.branch("feature")
	write(w.t, w.dir, "limits.go", "package m\n\nconst (\n\tMaxTagLength = 32\n\n\tMaxNotesPerOwner = 1000\n)\n")
	w.commit("feat: cap the notes per owner")
	w.checkout("main")
	write(w.t, w.dir, "limits.go", "package m\n\nconst (\n\tMaxTagLength = 32\n\n\tMaxAttachments = 5\n)\n")
	w.commit("feat: cap the attachments")
	w.checkout("feature")
}

func TestSmallConflictIsResolvedByUltra(t *testing.T) {
	answer := `{"files":[{"path":"limits.go","resolutions":["\tMaxNotesPerOwner = 1000\n\tMaxAttachments = 5"]}]}`
	w := newWorld(t, nil, []step{{content: "Here is the resolution:\n" + answer, usage: model.Usage{PromptTokens: 800, CompletionTokens: 60}}})
	conflictingLimits(w)
	res := w.process(person("upkeep"))
	if res.Outcome != pull.Mergeable || res.Blocker != pull.ConflictSmall || res.Err != nil || !res.Changed {
		t.Fatalf("result = %+v", res)
	}
	got := read(t, w.dir, "limits.go")
	if !strings.Contains(got, "MaxNotesPerOwner = 1000") || !strings.Contains(got, "MaxAttachments") || !strings.Contains(got, "= 5") || gitx.HasMarkers(got) {
		t.Fatalf("limits.go:\n%s", got)
	}
	if w.parents("HEAD") != 2 || !w.clean() {
		t.Fatal("the resolution should be committed as a merge")
	}
	lines := res.Receipt.Lines()
	if len(lines) != 1 || lines[0].Role != model.Ultra || lines[0].Calls != 1 || lines[0].Usage.PromptTokens != 800 {
		t.Fatalf("receipt = %+v", lines)
	}
	if w.super.count() != 0 {
		t.Fatal("Super must not be called for a conflict")
	}
	req := w.ultra.requests[0]
	if req.Reasoning == nil || *req.Reasoning || !strings.Contains(req.Messages[1].Content, "feat: cap the attachments") ||
		!strings.Contains(req.Messages[1].Content, "feat: cap the notes per owner") || !strings.Contains(req.Messages[1].Content, "<<<<<<<") {
		t.Fatalf("the prompt lacks the commit messages or the markers: %s", req.Messages[1].Content)
	}
}

func TestUnsafeResolutionsAreEscalatedAndTheMergeIsUndone(t *testing.T) {
	for name, answer := range map[string]string{
		"not json":        "I would keep both constants.",
		"wrong count":     `{"files":[{"path":"limits.go","resolutions":["a","b"]}]}`,
		"unknown file":    `{"files":[{"path":"other.go","resolutions":["a"]}]}`,
		"no files":        `{"files":[]}`,
		"marker left":     `{"files":[{"path":"limits.go","resolutions":["<<<<<<< HEAD\nx"]}]}`,
		"duplicate files": `{"files":[{"path":"limits.go","resolutions":["a"]},{"path":"limits.go","resolutions":["b"]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t, nil, []step{{content: answer}, {content: "Both sides add a limit at the same place; a person should decide the names."}})
			conflictingLimits(w)
			before := w.head()
			res := w.process(person())
			if res.Outcome != pull.NeedsHuman || res.Blocker != pull.ConflictSmall || !strings.Contains(res.Reason, "could not be resolved safely") {
				t.Fatalf("result = %+v", res)
			}
			if !strings.Contains(res.Diagnosis, "a person should decide") || w.ultra.count() != 2 {
				t.Fatalf("diagnosis %q after %d Ultra calls", res.Diagnosis, w.ultra.count())
			}
			if w.head() != before || !w.clean() || gitx.HasMarkers(read(t, w.dir, "limits.go")) {
				t.Fatal("the branch must be left exactly as it was")
			}
		})
	}
}

func TestLargeConflictGetsADiagnosis(t *testing.T) {
	w := newWorld(t, nil, []step{{content: "  Each side rewrote the constants. Split the work.  "}})
	var ours, theirs strings.Builder
	for i := range 30 {
		ours.WriteString("\tOurs" + string(rune('A'+i%26)) + string(rune('a'+i/26)) + " = 1\n")
		theirs.WriteString("\tTheirs" + string(rune('A'+i%26)) + string(rune('a'+i/26)) + " = 2\n")
	}
	w.branch("feature")
	write(t, w.dir, "limits.go", "package m\n\nconst (\n\tMaxTagLength = 32\n\n"+ours.String()+")\n")
	w.commit("feat: many on one side")
	w.checkout("main")
	write(t, w.dir, "limits.go", "package m\n\nconst (\n\tMaxTagLength = 32\n\n"+theirs.String()+")\n")
	w.commit("feat: many on the other")
	w.checkout("feature")
	before := w.head()

	res := w.process(person())
	if res.Outcome != pull.NeedsHuman || res.Blocker != pull.ConflictLarge || res.Diagnosis != "Each side rewrote the constants. Split the work." {
		t.Fatalf("result = %+v", res)
	}
	if res.Changed || w.head() != before || !w.clean() {
		t.Fatal("the branch must be left untouched")
	}
	req := w.ultra.requests[0]
	if req.Reasoning != nil {
		t.Fatalf("a diagnosis is written with the model's own reasoning: %v", *req.Reasoning)
	}
	if !strings.Contains(req.Messages[1].Content, "feat: many on one side") || !strings.Contains(req.Messages[1].Content, "<<<<<<<") {
		t.Fatal("the diagnosis prompt lacks the commits or the conflict")
	}
}

func renameWorld(t *testing.T, super, ultra []step) *world {
	w := newWorld(t, super, ultra)
	w.branch("feature")
	write(t, w.dir, "caller.go", "package m\n\nfunc Total() int { return Add(1, 2) }\n")
	w.commit("feat: add a caller")
	w.checkout("main")
	write(t, w.dir, "service.go", "package m\n\nfunc Sum(a, b int) int { return a + b }\n")
	write(t, w.dir, "service_test.go", strings.ReplaceAll(addTest, "Add(", "Sum("))
	w.commit("refactor: rename Add to Sum")
	w.checkout("feature")
	return w
}

func renameFix() []step {
	return []step{{calls: []model.ToolCall{
		toolCall("1", "apply_edit", fixer.Edit{Path: "caller.go", Old: "Add(1, 2)", New: "Sum(1, 2)"}),
		toolCall("2", "run_checks", "{}"),
	}}}
}

func TestBuildBrokenByMainIsRepairedBySuper(t *testing.T) {
	w := renameWorld(t, renameFix(), nil)
	res := w.process(person("upkeep"))
	if res.Outcome != pull.Mergeable || res.Blocker != pull.Build || res.Err != nil || res.Attempts != 1 || !res.Changed {
		t.Fatalf("result = %+v (%s)", res, res.Reason)
	}
	if read(t, w.dir, "caller.go") != "package m\n\nfunc Total() int { return Sum(1, 2) }\n" {
		t.Fatalf("caller.go = %q", read(t, w.dir, "caller.go"))
	}
	if !strings.Contains(res.Patch, "-func Total() int { return Add(1, 2) }") || !strings.Contains(res.Patch, "+func Total() int { return Sum(1, 2) }") {
		t.Fatalf("patch = %s", res.Patch)
	}
	if len(res.Changes) != 1 || res.Changes[0].Path != "caller.go" || !res.Checklist.Passed() || !res.WantsPush {
		t.Fatalf("changes %v checklist %v", res.Changes, res.Checklist)
	}
	if w.parents("HEAD~1") != 2 || w.parents("HEAD") != 1 || !w.clean() {
		t.Fatal("the history should be the merge followed by the fix")
	}
	subject, _ := w.git.Subjects(t.Context(), w.dir, "HEAD", 1)
	if len(subject) != 1 || subject[0] != "fix: make the checks pass again" {
		t.Fatalf("fix commit = %v", subject)
	}
	lines := res.Receipt.Lines()
	if len(lines) != 1 || lines[0].Role != model.Super || lines[0].Calls != 1 || res.Receipt.Attempts() != 1 || w.ultra.count() != 0 {
		t.Fatalf("receipt = %+v attempts %d", lines, res.Receipt.Attempts())
	}
	totals := res.Receipt.Totals()
	if !totals.Charge.Known || totals.Charge.BaselineMicros <= totals.Charge.Micros || totals.SavedMicros() <= 0 {
		t.Fatalf("totals = %+v", totals)
	}
}

func TestDependencyFixesAreNamedAfterTheModule(t *testing.T) {
	msg := fixMessage(pull.Request{Bump: &pull.Bump{Module: "github.com/golang-jwt/jwt/v5", To: "v5.3.1"}}, []fixer.Edit{{Path: "auth/token.go"}, {Path: "auth/token.go"}, {Path: "api/notes.go"}})
	if msg != "fix: adapt the code to github.com/golang-jwt/jwt/v5 v5.3.1\n\nEdited: auth/token.go, api/notes.go" {
		t.Fatalf("message = %q", msg)
	}
}

func TestFailingTestIsRepairedToo(t *testing.T) {
	w := newWorld(t, []step{{calls: []model.ToolCall{
		toolCall("1", "apply_edit", fixer.Edit{Path: "service.go", Old: "a - b", New: "a + b"}),
		toolCall("2", "run_checks", "{}"),
	}}}, nil)
	w.branch("feature")
	write(t, w.dir, "service.go", "package m\n\nfunc Add(a, b int) int { return a - b }\n")
	w.commit("fix: change the operator")
	res := w.process(person())
	if res.Outcome != pull.Mergeable || res.Blocker != pull.Test || res.Attempts != 1 {
		t.Fatalf("result = %+v (%s)", res, res.Reason)
	}
	if read(t, w.dir, "service.go") != service {
		t.Fatalf("service.go = %q", read(t, w.dir, "service.go"))
	}
}

func TestRepairThatFailsEveryAttemptIsEscalated(t *testing.T) {
	quit := []step{{content: "I give up."}, {content: "Really."}, {content: "I give up."}, {content: "Really."}, {content: "I give up."}, {content: "Really."}}
	w := renameWorld(t, quit, []step{{content: "The caller still uses the old name after the rename on main."}})
	w.pol.MaxAttempts = 3
	res := w.process(person())
	if res.Outcome != pull.NeedsHuman || res.Blocker != pull.Build || res.Attempts != 3 || res.Err != nil {
		t.Fatalf("result = %+v", res)
	}
	if !strings.Contains(res.Reason, "3 attempts did not fix it") || res.Diagnosis != "The caller still uses the old name after the rename on main." {
		t.Fatalf("reason %q diagnosis %q", res.Reason, res.Diagnosis)
	}
	if res.Receipt.Attempts() != 3 || w.super.count() != 6 || w.ultra.count() != 1 {
		t.Fatalf("attempts %d, Super calls %d, Ultra calls %d", res.Receipt.Attempts(), w.super.count(), w.ultra.count())
	}
	if !w.clean() || read(t, w.dir, "caller.go") != "package m\n\nfunc Total() int { return Add(1, 2) }\n" {
		t.Fatal("a failed repair must leave the working copy clean")
	}
	second := w.super.requests[2].Messages[1].Content
	if !strings.Contains(second, "A previous attempt did not work") {
		t.Fatal("the second attempt should know why the first one failed")
	}
}

func TestAFixThatBreaksThePolicyIsRejected(t *testing.T) {
	w := renameWorld(t, renameFix(), nil)
	w.pol.MaxLinesChanged = 1
	before := w.head()
	res := w.process(person())
	if res.Outcome != pull.NeedsHuman || res.Changed || !strings.Contains(res.Reason, "changes at most 1 lines") {
		t.Fatalf("result = %+v", res)
	}
	if res.Checklist.Passed() || res.HeadAfter != res.HeadBefore || w.head() != before || !w.clean() {
		t.Fatal("a rejected fix must be undone")
	}
}

func TestFlakyTestIsOnlyReported(t *testing.T) {
	w := newWorld(t, nil, nil)
	marker := filepath.Join(t.TempDir(), "seen")
	flaky := "package m\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestFlaky(t *testing.T) {\n\tif _, err := os.Stat(\"" + marker + "\"); err != nil {\n\t\tos.WriteFile(\"" + marker + "\", []byte(\"x\"), 0o600)\n\t\tt.Fatal(\"first run fails\")\n\t}\n}\n"
	w.branch("feature")
	write(t, w.dir, "flaky_test.go", flaky)
	w.commit("test: add a flaky test")
	res := w.process(person())
	if res.Outcome != pull.Mergeable || res.Blocker != pull.Flaky || len(res.Flaky) != 1 || res.Flaky[0] != "TestFlaky" || res.Changed {
		t.Fatalf("result = %+v", res)
	}
	if w.super.count()+w.ultra.count() != 0 {
		t.Fatal("a flaky test must not be fixed by a model")
	}
}

func TestEventsDescribeTheWork(t *testing.T) {
	w := renameWorld(t, renameFix(), nil)
	res := w.process(person())
	if res.Outcome != pull.Mergeable {
		t.Fatalf("result = %+v", res)
	}
	for _, want := range []events.Kind{events.PRBlocker, events.PRStep, events.ModelCall, events.ToolCall, events.VerifyResult, events.Receipt} {
		found := false
		for _, k := range w.events {
			found = found || k == want
		}
		if !found {
			t.Errorf("no %s event in %v", want, w.events)
		}
	}
	for _, want := range []string{"detect:running", "detect:done", "merge:done", "repair:running", "repair:done", "verify:done"} {
		found := false
		for _, s := range w.steps {
			found = found || s == want
		}
		if !found {
			t.Errorf("no step %s in %v", want, w.steps)
		}
	}
}

func TestModelFailuresBecomeFailedResults(t *testing.T) {
	boom := errors.New("token factory is down")
	w := renameWorld(t, []step{{err: boom}}, nil)
	res := w.process(person())
	if res.Outcome != pull.Failed || !errors.Is(res.Err, boom) {
		t.Fatalf("result = %+v", res)
	}
}

func TestProcessReportsAnUnreadableWorkingCopy(t *testing.T) {
	w := newWorld(t, nil, nil)
	w.dir = t.TempDir()
	res := w.process(person())
	if res.Outcome != pull.Failed || res.Err == nil {
		t.Fatalf("result = %+v", res)
	}
}

func TestDefaultLimits(t *testing.T) {
	if DefaultLimits.PRsPerRun != 4 || DefaultLimits.RunsPerServer != 2 {
		t.Fatalf("limits = %+v", DefaultLimits)
	}
}
