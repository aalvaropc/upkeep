package eval_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aalvaropc/upkeep/internal/demo"
	"github.com/aalvaropc/upkeep/internal/engine"
	"github.com/aalvaropc/upkeep/internal/eval"
	"github.com/aalvaropc/upkeep/internal/events"
	"github.com/aalvaropc/upkeep/internal/forge/forgetest"
	"github.com/aalvaropc/upkeep/internal/gitx"
	"github.com/aalvaropc/upkeep/internal/learn"
	"github.com/aalvaropc/upkeep/internal/model"
	"github.com/aalvaropc/upkeep/internal/pull"
	"github.com/aalvaropc/upkeep/internal/queue"
	"github.com/aalvaropc/upkeep/internal/receipt"
	"github.com/aalvaropc/upkeep/internal/scenario"
)

type fixed struct{ results map[int]engine.Result }

func (f fixed) Process(_ context.Context, in engine.Input) engine.Result {
	res := f.results[in.PR.Number]
	res.PR = in.PR
	rec := receipt.New()
	rec.AddCall(model.Super, "super", model.Usage{PromptTokens: 1000, CompletionTokens: 100}, model.Charge{Micros: 400, BaselineMicros: 1300, Known: true}, time.Second)
	rec.AddAttempt()
	res.Receipt = rec
	return res
}

func expecting() scenario.Scenario {
	s := forgetest.Scenario()
	expect := map[int]scenario.Expect{
		1: {Blocker: pull.Behind, Outcome: pull.Mergeable, Patch: true},
		2: {Blocker: pull.Build, Outcome: pull.Mergeable, Pushed: true},
		3: {Blocker: pull.ConflictLarge, Outcome: pull.NeedsHuman},
		4: {Blocker: pull.Clean, Outcome: pull.Merged},
	}
	for i := range s.PRs {
		s.PRs[i].Expect = expect[s.PRs[i].Number]
	}
	return s
}

func agreeing() map[int]engine.Result {
	return map[int]engine.Result{
		1: {Blocker: pull.Behind, Outcome: pull.Mergeable, Changed: true, WantsPatch: true, Patch: "diff", Attempts: 0},
		2: {Blocker: pull.Build, Outcome: pull.Mergeable, Changed: true, WantsPush: true, Attempts: 2},
		3: {Blocker: pull.ConflictLarge, Outcome: pull.NeedsHuman, Reason: "too big"},
		4: {Blocker: pull.Clean, Outcome: pull.Mergeable, WantsMerge: true},
	}
}

func manager(t *testing.T, results map[int]engine.Result) *demo.Manager {
	t.Helper()
	template := forgetest.New(t)
	m, err := demo.NewManager(t.Context(), demo.Options{
		Scenario:  expecting(),
		Template:  template.Repo,
		Root:      filepath.Join(t.TempDir(), "sessions"),
		Git:       gitx.Git{},
		Hub:       events.NewHub(100),
		Processor: func(queue.Emitter, learn.Book) (queue.Processor, error) { return fixed{results: results}, nil },
		Limits:    engine.Limits{PRsPerRun: 2, RunsPerServer: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Shutdown)
	return m
}

func TestARunThatMatchesTheScenarioPassesEveryCase(t *testing.T) {
	report, err := eval.Run(t.Context(), manager(t, agreeing()), expecting(), 1, "replay", "v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Cases) != 4 || !report.AllPassed() {
		t.Fatalf("cases = %+v", report.Cases)
	}
	s := report.Summary()
	if s.Passed != 4 || s.Cases != 4 || s.Micros != 1600 || s.BaselineMicros != 5200 {
		t.Fatalf("summary = %+v", s)
	}
	two := report.Cases[1]
	if two.Number != 2 || two.Attempts != 2 || two.Calls != 1 || two.PromptTokens != 1000 || !two.Pushed {
		t.Fatalf("case #2 = %+v", two)
	}
	md := report.Markdown()
	for _, want := range []string{"# Evaluation: tiny", "Mode `replay`", "version `v1.2.3`", "**4 of 4 cases ended as expected.**", "The 4 pull requests cost $0.0016 per run", "3.2 times as much", "not a claim", "| 2 | Change b | 1/1 |"} {
		if !strings.Contains(md, want) {
			t.Errorf("the report lacks %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "Differences") {
		t.Errorf("a clean run should list no differences:\n%s", md)
	}
}

func TestDifferencesFromTheScenarioAreReported(t *testing.T) {
	results := agreeing()
	results[2] = engine.Result{Blocker: pull.Build, Outcome: pull.NeedsHuman, Reason: "3 attempts did not fix it"}
	results[1] = engine.Result{Blocker: pull.Clean, Outcome: pull.Mergeable}
	report, err := eval.Run(t.Context(), manager(t, results), expecting(), 1, "live", "dev")
	if err != nil {
		t.Fatal(err)
	}
	if report.AllPassed() {
		t.Fatal("a run with two differences passed")
	}
	problems := map[int][]string{}
	for _, c := range report.Cases {
		problems[c.Number] = c.Problems
	}
	if got := strings.Join(problems[2], "; "); !strings.Contains(got, `outcome "needs_human", expected "mergeable" (3 attempts did not fix it)`) || !strings.Contains(got, "pushed false, expected true") {
		t.Fatalf("problems for #2 = %v", problems[2])
	}
	if got := strings.Join(problems[1], "; "); !strings.Contains(got, `blocker "clean", expected "behind"`) || !strings.Contains(got, "patch attached false, expected true") {
		t.Fatalf("problems for #1 = %v", problems[1])
	}
	if len(problems[3]) != 0 || len(problems[4]) != 0 {
		t.Fatalf("pull requests that matched were reported: %v", problems)
	}
	md := report.Markdown()
	if !strings.Contains(md, "**2 of 4 cases ended as expected.**") || !strings.Contains(md, "Differences from what the scenario expects") || !strings.Contains(md, "- #2: ") {
		t.Fatalf("report:\n%s", md)
	}
}

func TestSeveralRunsAreAveraged(t *testing.T) {
	report, err := eval.Run(t.Context(), manager(t, agreeing()), expecting(), 3, "replay", "dev")
	if err != nil {
		t.Fatal(err)
	}
	if report.Runs != 3 || len(report.Cases) != 12 {
		t.Fatalf("runs %d, cases %d", report.Runs, len(report.Cases))
	}
	s := report.Summary()
	if len(s.Rows) != 4 || s.Rows[1].Runs != 3 || s.Rows[1].Passed != 3 || s.Rows[1].MeanAttempts != 2 || s.Rows[1].MeanMicros != 400 {
		t.Fatalf("rows = %+v", s.Rows)
	}
	if !strings.Contains(report.Markdown(), "3 run(s)") || !strings.Contains(report.Markdown(), "12 of 12") {
		t.Fatalf("report:\n%s", report.Markdown())
	}
}

func TestTheReportRoundTripsThroughJSON(t *testing.T) {
	report, err := eval.Run(t.Context(), manager(t, agreeing()), expecting(), 1, "replay", "dev")
	if err != nil {
		t.Fatal(err)
	}
	data, err := report.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var back eval.Report
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.Scenario != "tiny" || len(back.Cases) != 4 || back.Cases[0].ExpectBlocker != pull.Behind || !back.AllPassed() {
		t.Fatalf("round trip = %+v", back)
	}
}

func TestAServerFallbackInALiveRunDoesNotCountAsAPass(t *testing.T) {
	template := forgetest.New(t)
	m, err := demo.NewManager(t.Context(), demo.Options{
		Scenario: expecting(),
		Template: template.Repo,
		Root:     filepath.Join(t.TempDir(), "sessions"),
		Git:      gitx.Git{},
		Hub:      events.NewHub(100),
		Processor: func(emit queue.Emitter, _ learn.Book) (queue.Processor, error) {
			emit(events.Notice, 0, map[string]string{"message": "The live token budget for today is used up, so this run replays a recorded one."})
			return fixed{results: agreeing()}, nil
		},
		Limits: engine.Limits{PRsPerRun: 2, RunsPerServer: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Shutdown)
	live, err := eval.Run(t.Context(), m, expecting(), 1, "live", "dev")
	if err != nil {
		t.Fatal(err)
	}
	if live.AllPassed() || !live.Degraded || len(live.Notices) != 1 {
		t.Fatalf("a live evaluation that fell back to a recording passed: %+v", live)
	}
	if md := live.Markdown(); !strings.Contains(md, "Notice from the server: The live token budget") || !strings.Contains(md, "does not count as a pass") {
		t.Fatalf("report:\n%s", md)
	}
	replay, err := eval.Run(t.Context(), m, expecting(), 1, "replay", "dev")
	if err != nil {
		t.Fatal(err)
	}
	if replay.Degraded || !replay.AllPassed() {
		t.Fatalf("a notice during a replay is not a failure: %+v", replay)
	}
}

func TestAnEmptyReportDoesNotPass(t *testing.T) {
	if (eval.Report{}).AllPassed() {
		t.Fatal("a report with no cases should not count as a pass")
	}
}
