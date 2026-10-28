package engine

import (
	"strings"
	"sync"
	"testing"

	"github.com/aalvaropc/upkeep/internal/events"
	"github.com/aalvaropc/upkeep/internal/fixer"
	"github.com/aalvaropc/upkeep/internal/model"
	"github.com/aalvaropc/upkeep/internal/policy"
	"github.com/aalvaropc/upkeep/internal/pull"
	"github.com/aalvaropc/upkeep/internal/sandbox"
	"github.com/aalvaropc/upkeep/internal/strategy"
)

type traced struct {
	kind events.Kind
	data map[string]any
}

type trail struct {
	mu     sync.Mutex
	events []traced
}

func (tr *trail) of(kind events.Kind, step string) []map[string]any {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	var out []map[string]any
	for _, e := range tr.events {
		if e.kind == kind && (step == "" || e.data["step"] == step) {
			out = append(out, e.data)
		}
	}
	return out
}

func (w *world) trace(pr pull.Request) (Result, *trail) {
	w.t.Helper()
	router := model.NewRouter()
	router.Set(model.Ultra, "ultra", w.ultra)
	router.Set(model.Super, "super", w.super)
	if w.nano != nil {
		router.Set(model.Nano, "nano", w.nano)
	}
	book := model.PriceBook{"super": {InputPerMillion: 0.3, OutputPerMillion: 0.9}, "ultra": {InputPerMillion: 1, OutputPerMillion: 3}, "nano": {InputPerMillion: 0.06, OutputPerMillion: 0.24}}
	tr := &trail{}
	env := strategy.Env{
		Forecaster: w.fc,
		Git:        w.git,
		Runner:     sandbox.GoRunner{Executor: sandbox.Exec{}},
		Executor:   sandbox.Exec{},
		Models:     router,
		Meter:      model.Meter{Prices: book, Baseline: "ultra"},
		Policy:     w.pol,
		Emit: func(kind events.Kind, _ int, data any) {
			m, _ := data.(map[string]any)
			tr.mu.Lock()
			defer tr.mu.Unlock()
			tr.events = append(tr.events, traced{kind: kind, data: m})
		},
	}
	pr.Number, pr.Branch = 7, "feature"
	return Processor{Env: env}.Process(w.t.Context(), Input{PR: pr, Dir: w.dir, Base: "main"}), tr
}

func TestThePlanComesRightAfterTheBlocker(t *testing.T) {
	w := renameWorld(t, renameFix(), nil)
	res, tr := w.trace(person("upkeep"))
	if res.Outcome != pull.Mergeable {
		t.Fatalf("result = %+v", res)
	}
	plans := tr.of(events.PRStep, "plan")
	if len(plans) != 1 || plans[0]["status"] != "done" || plans[0]["detail"] != "build fails: Super repairs it with 4 tools, up to 3 attempts, then 9 checks" {
		t.Fatalf("plan = %+v", plans)
	}
	for i, e := range tr.events {
		if e.kind == events.PRBlocker {
			if next := tr.events[i+1]; next.kind != events.PRStep || next.data["step"] != "plan" {
				t.Fatalf("the event after the blocker is %s %+v", next.kind, next.data)
			}
			return
		}
	}
	t.Fatal("no blocker event")
}

func TestThePlanFollowsThePolicy(t *testing.T) {
	pol := policy.Default()
	pol.MaxAttempts = 1
	for blocker, want := range map[pull.Blocker]string{
		pull.Build:         "build fails: Super repairs it with 4 tools, up to 1 attempt, then 9 checks",
		pull.Test:          "tests fail: Super repairs it with 4 tools, up to 1 attempt, then 9 checks",
		pull.ConflictSmall: "a small conflict: Ultra resolves it in one call, then 9 checks",
		pull.ConflictLarge: "a large conflict: Ultra writes a diagnosis for a person",
		pull.Behind:        "behind main: merge main mechanically, then 9 checks",
		pull.Format:        "unformatted files: gofmt them, then 9 checks",
		pull.Flaky:         "a flaky test: report it and change nothing, then 9 checks",
		pull.Clean:         "nothing blocks it: run the 9 checks",
	} {
		if got := approach(blocker, pol, "main"); got != want {
			t.Errorf("%s: %q, want %q", blocker, got, want)
		}
	}
}

func TestRepairStepsCarryEachAttempt(t *testing.T) {
	failing := []step{
		{content: "Let me check.", calls: []model.ToolCall{toolCall("1", "run_checks", "{}")}},
		{content: "I give up."},
		{content: "Really."},
	}
	w := renameWorld(t, append(failing, renameFix()...), nil)
	w.pol.MaxAttempts = 2
	res, tr := w.trace(person("upkeep"))
	if res.Outcome != pull.Mergeable || res.Attempts != 2 {
		t.Fatalf("result = %+v", res)
	}
	repairs := tr.of(events.PRStep, "repair")
	if len(repairs) != 4 {
		t.Fatalf("repair steps = %+v", repairs)
	}
	if r := repairs[0]; r["status"] != "running" || r["attempt"] != 1 || r["of"] != 2 || r["detail"] != "attempt 1 of 2" {
		t.Fatalf("first running step = %+v", r)
	}
	first := repairs[1]
	if first["status"] != "failed" || first["attempt"] != 1 || first["stop"] != string(fixer.StopQuit) || first["turns"] != 3 || first["edits"] != 0 || first["files"] != 0 {
		t.Fatalf("first attempt = %+v", first)
	}
	if failure, _ := first["failure"].(string); !strings.HasPrefix(failure, "caller.go:3:") || !strings.Contains(failure, "undefined: Add") {
		t.Fatalf("first attempt failure = %q", first["failure"])
	}
	if detail, _ := first["detail"].(string); !strings.HasPrefix(detail, "attempt 1: "+string(fixer.StopQuit)+" after 3 turns, 0 edits in 0 files") {
		t.Fatalf("first attempt detail = %q", detail)
	}
	second := repairs[3]
	if second["status"] != "done" || second["attempt"] != 2 || second["stop"] != string(fixer.StopPassed) || second["edits"] != 1 || second["files"] != 1 {
		t.Fatalf("second attempt = %+v", second)
	}
	if _, ok := second["failure"]; ok {
		t.Fatalf("a passing attempt has no failure: %+v", second)
	}
	calls := tr.of(events.ModelCall, "repair")
	if len(calls) != 4 || calls[0]["attempt"] != 1 || calls[2]["turn"] != 3 || calls[3]["attempt"] != 2 || calls[3]["turn"] != 1 {
		t.Fatalf("model calls = %+v", calls)
	}
	for _, tool := range tr.of(events.ToolCall, "") {
		if tool["attempt"] != 1 && tool["attempt"] != 2 {
			t.Fatalf("tool call without its attempt: %+v", tool)
		}
	}
}

func TestEveryModelCallNamesItsStep(t *testing.T) {
	good := `{"files":[{"path":"limits.go","resolutions":["\tMaxNotesPerOwner = 1000\n\tMaxAttachments = 5"]}]}`
	bad := "{\"files\":[{\"path\":\"limits.go\",\"resolutions\":[\"a\",\"b\"]}]}"
	w := newWorld(t, nil, []step{{content: bad}, {content: good}})
	conflictingLimits(w)
	if res, tr := w.trace(person("upkeep")); res.Outcome != pull.Mergeable {
		t.Fatalf("result = %+v", res)
	} else if calls := tr.of(events.ModelCall, "conflict"); len(calls) != 2 || calls[0]["purpose"] != "resolve" || calls[1]["purpose"] != "resolve_retry" {
		t.Fatalf("conflict calls = %+v", calls)
	}

	w = renameWorld(t, renameFix(), nil)
	w.fc = newForecast()
	w.nano = &scripted{steps: []step{{content: "Renames the call to Add so the package builds again."}}}
	res, tr := w.trace(dependencyPR())
	if res.Outcome != pull.Mergeable {
		t.Fatalf("result = %+v", res)
	}
	for name, purpose := range map[string]string{"forecast": "forecast", "repair": "fix", "explain": "explain"} {
		calls := tr.of(events.ModelCall, name)
		if len(calls) != 1 || calls[0]["purpose"] != purpose {
			t.Errorf("%s calls = %+v", name, calls)
		}
	}

	quit := []step{{content: "I give up."}, {content: "Really."}}
	w = renameWorld(t, quit, []step{{content: "The caller still uses the old name."}})
	w.pol.MaxAttempts = 1
	res, tr = w.trace(person())
	if res.Outcome != pull.NeedsHuman {
		t.Fatalf("result = %+v", res)
	}
	if calls := tr.of(events.ModelCall, "diagnose"); len(calls) != 1 || calls[0]["purpose"] != "diagnose" || calls[0]["role"] != "ultra" {
		t.Fatalf("diagnose calls = %+v", calls)
	}
}
