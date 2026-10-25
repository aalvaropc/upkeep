package queue_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aalvaropc/upkeep/internal/engine"
	"github.com/aalvaropc/upkeep/internal/events"
	"github.com/aalvaropc/upkeep/internal/forge"
	"github.com/aalvaropc/upkeep/internal/forge/forgetest"
	"github.com/aalvaropc/upkeep/internal/model"
	"github.com/aalvaropc/upkeep/internal/pull"
	"github.com/aalvaropc/upkeep/internal/queue"
	"github.com/aalvaropc/upkeep/internal/receipt"
)

type scripted struct {
	t       *testing.T
	results map[int]engine.Result
	commit  map[int]string
	gate    func(number int)
	started []int
	mu      sync.Mutex
}

func (s *scripted) Process(_ context.Context, in engine.Input) engine.Result {
	s.mu.Lock()
	s.started = append(s.started, in.PR.Number)
	s.mu.Unlock()
	if s.gate != nil {
		s.gate(in.PR.Number)
	}
	if content, ok := s.commit[in.PR.Number]; ok {
		forgetest.Write(s.t, in.Dir, "fix.txt", content)
		forgetest.Git(s.t, in.Dir, "add", "-A")
		forgetest.Git(s.t, in.Dir, "commit", "--quiet", "-m", "fix")
	}
	res := s.results[in.PR.Number]
	res.PR = in.PR
	if res.Receipt == nil {
		res.Receipt = receipt.New()
	}
	return res
}

type log struct {
	mu     sync.Mutex
	events []events.Event
}

func (l *log) emit(kind events.Kind, pr int, data any) {
	raw, _ := json.Marshal(data)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, events.Event{Kind: kind, PR: pr, Data: raw})
}

func (l *log) kinds(kind events.Kind) []events.Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []events.Event
	for _, e := range l.events {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

func newRunner(t *testing.T, p queue.Processor) (queue.Runner, *forge.Local, *log) {
	t.Helper()
	f := forgetest.New(t)
	l := &log{}
	return queue.Runner{Forge: f, Processor: p, Emit: l.emit, Work: filepath.Join(t.TempDir(), "work"), Base: "origin/main", Parallel: 2}, f, l
}

func item(t *testing.T, s queue.Summary, number int) queue.Item {
	t.Helper()
	for _, it := range s.Items {
		if it.PR.Number == number {
			return it
		}
	}
	t.Fatalf("no item for #%d", number)
	return queue.Item{}
}

func comments(f *forge.Local, number int, kind string) []forge.Comment {
	var out []forge.Comment
	for _, v := range f.Views(context.Background()) {
		if v.Number != number {
			continue
		}
		for _, c := range v.Comments {
			if c.Kind == kind {
				out = append(out, c)
			}
		}
	}
	return out
}

func TestEveryOpenRequestIsWorkedAndAnnounced(t *testing.T) {
	p := &scripted{t: t, results: map[int]engine.Result{
		1: {Outcome: pull.Mergeable}, 2: {Outcome: pull.Mergeable}, 3: {Outcome: pull.NeedsHuman, Reason: "too big", Diagnosis: "both sides rewrote it"}, 4: {Outcome: pull.Failed, Reason: "boom"},
	}}
	r, f, log := newRunner(t, p)
	s, err := r.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Items) != 4 || s.Counts[pull.Mergeable] != 2 || s.Counts[pull.NeedsHuman] != 1 || s.Counts[pull.Failed] != 1 {
		t.Fatalf("summary = %+v", s.Counts)
	}
	if started := log.kinds(events.RunStarted); len(started) != 1 || !strings.Contains(string(started[0].Data), `"number":4`) {
		t.Fatalf("run.started = %+v", started)
	}
	if done := log.kinds(events.PRDone); len(done) != 4 {
		t.Fatalf("%d pr.done events, want 4", len(done))
	}
	finished := log.kinds(events.RunFinished)
	if len(finished) != 1 || !strings.Contains(string(finished[0].Data), `"needs_human":1`) {
		t.Fatalf("run.finished = %+v", finished)
	}
	if c := comments(f, 3, "diagnosis"); len(c) != 1 || !strings.Contains(c[0].Body, "both sides rewrote it") || !strings.Contains(c[0].Body, "too big") {
		t.Fatalf("diagnosis comment = %+v", c)
	}
	if got := item(t, s, 4); got.Outcome != pull.Failed || got.Actions[0].Status != "failed" {
		t.Fatalf("failed item = %+v", got)
	}
}

func TestRequestsStartInNumberOrder(t *testing.T) {
	p := &scripted{t: t, results: map[int]engine.Result{}}
	r, _, _ := newRunner(t, p)
	r.Parallel = 1
	if _, err := r.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := p.started; len(got) != 4 || got[0] != 1 || got[1] != 2 || got[2] != 3 || got[3] != 4 {
		t.Fatalf("started in order %v", got)
	}
}

func TestAtMostParallelRequestsRunAtOnce(t *testing.T) {
	var running, peak atomic.Int32
	p := &scripted{t: t, results: map[int]engine.Result{}, gate: func(int) {
		n := running.Add(1)
		for {
			old := peak.Load()
			if n <= old || peak.CompareAndSwap(old, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		running.Add(-1)
	}}
	r, _, _ := newRunner(t, p)
	r.Parallel = 2
	if _, err := r.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := peak.Load(); got != 2 {
		t.Fatalf("peak concurrency = %d, want 2", got)
	}
}

func TestAFixIsPushedToTheBranchWhenThePolicyAllowsIt(t *testing.T) {
	p := &scripted{t: t, commit: map[int]string{2: "fixed\n"}, results: map[int]engine.Result{2: {Outcome: pull.Mergeable, Changed: true, WantsPush: true, HeadAfter: "0123456789abcdef"}}}
	r, f, log := newRunner(t, p)
	s, err := r.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got := forgetest.Git(t, f.Repo, "show", "feat/b:fix.txt"); got != "fixed" {
		t.Fatalf("the pushed branch has fix.txt = %q", got)
	}
	got := item(t, s, 2)
	if got.Outcome != pull.Mergeable || len(got.Actions) != 1 || got.Actions[0].Kind != "push" || !strings.Contains(got.Actions[0].Detail, "01234567") {
		t.Fatalf("item = %+v", got)
	}
	if actions := log.kinds(events.Action); len(actions) < 1 {
		t.Fatal("no action event was emitted")
	}
}

func TestAFixIsAttachedAsAPatchWhenPushingIsNotAllowed(t *testing.T) {
	p := &scripted{t: t, results: map[int]engine.Result{1: {Outcome: pull.Mergeable, Changed: true, WantsPatch: true, Patch: "diff --git a/x b/x\n+line\n", Flaky: []string{"TestOrder"}}}}
	r, f, _ := newRunner(t, p)
	s, err := r.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	c := comments(f, 1, "patch")
	if len(c) != 1 || !strings.Contains(c[0].Body, "```diff\ndiff --git a/x b/x\n+line\n```") || !strings.Contains(c[0].Body, "TestOrder") {
		t.Fatalf("patch comment = %+v", c)
	}
	if got := item(t, s, 1); got.Actions[0].Kind != "patch" || got.Outcome != pull.Mergeable {
		t.Fatalf("item = %+v", got)
	}
}

func TestABotRequestIsMergedWhenThePolicyAllowsIt(t *testing.T) {
	p := &scripted{t: t, results: map[int]engine.Result{4: {Outcome: pull.Mergeable, WantsMerge: true}}}
	r, f, _ := newRunner(t, p)
	s, err := r.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got := item(t, s, 4); got.Outcome != pull.Merged || got.Actions[0].Kind != "merge" || got.Actions[0].Status != "done" {
		t.Fatalf("item = %+v", got)
	}
	forgetest.Git(t, f.Repo, "cat-file", "-e", "main:c.txt")
	if s.Counts[pull.Merged] != 1 {
		t.Fatalf("counts = %v", s.Counts)
	}
}

func TestARequestThatNoLongerMergesIsHandedToAPerson(t *testing.T) {
	p := &scripted{t: t, results: map[int]engine.Result{
		1: {Outcome: pull.Mergeable, WantsMerge: true},
		3: {Outcome: pull.Mergeable, WantsMerge: true},
	}}
	r, _, _ := newRunner(t, p)
	r.Parallel = 1
	s, err := r.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got := item(t, s, 1); got.Outcome != pull.Merged {
		t.Fatalf("#1 = %+v", got)
	}
	if got := item(t, s, 3); got.Outcome != pull.NeedsHuman || got.Actions[0].Status != "failed" || !strings.Contains(got.Actions[0].Detail, "merge cleanly") {
		t.Fatalf("#3 = %+v", got)
	}
}

func TestARejectedPushStopsTheMerge(t *testing.T) {
	p := &scripted{t: t, commit: map[int]string{4: "x\n"}, results: map[int]engine.Result{4: {Outcome: pull.Mergeable, Changed: true, WantsPush: true, WantsMerge: true}}}
	r, f, _ := newRunner(t, p)
	p.gate = func(number int) {
		if number == 4 {
			dir := filepath.Join(r.Work, "pr-4")
			forgetest.Git(t, dir, "reset", "--quiet", "--hard", "origin/main")
		}
	}
	s, err := r.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	got := item(t, s, 4)
	if got.Outcome != pull.Failed || got.Actions[0].Kind != "push" || got.Actions[0].Status != "failed" {
		t.Fatalf("item = %+v", got)
	}
	for _, v := range f.Views(t.Context()) {
		if v.Number == 4 && v.State != forge.Open {
			t.Fatalf("a request whose push was rejected was merged: %+v", v)
		}
	}
}

func TestAReceiptIsPostedWithEveryModelUsed(t *testing.T) {
	rec := receipt.New()
	rec.AddCall(model.Super, "nvidia/super", model.Usage{PromptTokens: 1000, CompletionTokens: 200}, model.Charge{Micros: 7100, BaselineMicros: 26000, Known: true}, time.Second)
	rec.AddAttempt()
	p := &scripted{t: t, results: map[int]engine.Result{1: {Outcome: pull.Mergeable, Receipt: rec}}}
	r, f, _ := newRunner(t, p)
	s, err := r.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	c := comments(f, 1, "receipt")
	if len(c) != 1 || !strings.Contains(c[0].Body, "nvidia/super") || !strings.Contains(c[0].Body, "$0.0071") || !strings.Contains(c[0].Body, "$0.0260") || !strings.Contains(c[0].Body, "not a claim") {
		t.Fatalf("receipt comment = %+v", c)
	}
	if s.Totals.Charge.Micros != 7100 || s.Totals.Calls != 1 {
		t.Fatalf("totals = %+v", s.Totals)
	}
}

func TestCancellingTheRunMarksTheRestAsFailed(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	p := &scripted{t: t, results: map[int]engine.Result{}, gate: func(number int) {
		if number == 1 {
			cancel()
		}
	}}
	r, _, log := newRunner(t, p)
	r.Parallel = 1
	s, err := r.Run(ctx)
	if err == nil {
		t.Fatal("a cancelled run returned no error")
	}
	if len(s.Items) != 4 || len(log.kinds(events.PRDone)) != 4 {
		t.Fatalf("%d items, %d pr.done events", len(s.Items), len(log.kinds(events.PRDone)))
	}
	if s.Counts[pull.Failed] == 0 {
		t.Fatalf("counts = %v", s.Counts)
	}
}
