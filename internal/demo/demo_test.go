package demo_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aalvaropc/upkeep/internal/demo"
	"github.com/aalvaropc/upkeep/internal/engine"
	"github.com/aalvaropc/upkeep/internal/events"
	"github.com/aalvaropc/upkeep/internal/forecast"
	"github.com/aalvaropc/upkeep/internal/forge/forgetest"
	"github.com/aalvaropc/upkeep/internal/gitx"
	"github.com/aalvaropc/upkeep/internal/learn"
	"github.com/aalvaropc/upkeep/internal/model"
	"github.com/aalvaropc/upkeep/internal/pull"
	"github.com/aalvaropc/upkeep/internal/queue"
	"github.com/aalvaropc/upkeep/internal/receipt"
)

type processor struct {
	gate    chan struct{}
	started chan string
	results map[int]engine.Result
}

func (p *processor) Process(ctx context.Context, in engine.Input) engine.Result {
	if p.started != nil {
		select {
		case p.started <- in.Dir:
		default:
		}
	}
	if p.gate != nil {
		select {
		case <-p.gate:
		case <-ctx.Done():
		}
	}
	res := p.results[in.PR.Number]
	res.PR = in.PR
	if res.Receipt == nil {
		res.Receipt = receipt.New()
		res.Receipt.AddCall(model.Super, "nvidia/super", model.Usage{PromptTokens: 100, CompletionTokens: 20}, model.Charge{Micros: 700, BaselineMicros: 2600, Known: true}, time.Second)
	}
	return res
}

type harness struct {
	t   *testing.T
	m   *demo.Manager
	hub *events.Hub
	p   *processor
	now *time.Time
	mu  sync.Mutex
}

func (h *harness) clock() time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()
	return *h.now
}

func (h *harness) advance(d time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	*h.now = h.now.Add(d)
}

func newHarness(t *testing.T, mutate func(*demo.Options), p *processor) *harness {
	t.Helper()
	template := forgetest.New(t)
	hub := events.NewHub(500)
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	h := &harness{t: t, hub: hub, p: p, now: &now}
	opts := demo.Options{
		Scenario:  forgetest.Scenario(),
		Template:  template.Repo,
		Root:      filepath.Join(t.TempDir(), "sessions"),
		Git:       gitx.Git{},
		Hub:       hub,
		Processor: func(queue.Emitter, learn.Book) (queue.Processor, error) { return p, nil },
		Limits:    engine.Limits{PRsPerRun: 2, RunsPerServer: 2},
		Now:       h.clock,
	}
	if mutate != nil {
		mutate(&opts)
	}
	m, err := demo.NewManager(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Shutdown)
	h.m = m
	return h
}

func (h *harness) session() *demo.Session {
	h.t.Helper()
	s, err := h.m.New(h.t.Context(), "")
	if err != nil {
		h.t.Fatal(err)
	}
	return s
}

func (h *harness) waitStatus(s *demo.Session, want demo.Status) demo.Snapshot {
	h.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if snap := s.Snapshot(h.t.Context()); snap.Status == want {
			return snap
		}
		time.Sleep(5 * time.Millisecond)
	}
	h.t.Fatalf("session %s never reached %s (now %s)", s.ID, want, s.Snapshot(h.t.Context()).Status)
	return demo.Snapshot{}
}

func (h *harness) kinds(s *demo.Session, kind events.Kind) []events.Event {
	ring, ok := h.hub.Get(s.ID)
	if !ok {
		h.t.Fatalf("no ring for %s", s.ID)
	}
	evs, _, _ := ring.Read(0)
	var out []events.Event
	for _, e := range evs {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

func TestARunWorksEveryRequestAndKeepsTheResults(t *testing.T) {
	p := &processor{results: map[int]engine.Result{
		1: {Outcome: pull.Mergeable, Blocker: pull.Behind},
		3: {Outcome: pull.NeedsHuman, Blocker: pull.ConflictLarge, Reason: "too big", Diagnosis: "both rewrote it"},
		4: {Outcome: pull.Mergeable, Blocker: pull.Clean, WantsMerge: true},
	}}
	h := newHarness(t, nil, p)
	s := h.session()
	if snap := s.Snapshot(t.Context()); snap.Status != demo.Idle || len(snap.PRs) != 4 || snap.PRs[0].Number != 1 {
		t.Fatalf("fresh snapshot = %+v", snap)
	}
	if _, err := h.m.Start(s.ID); err != nil {
		t.Fatal(err)
	}
	snap := h.waitStatus(s, demo.Finished)
	if len(h.kinds(s, events.PRDone)) != 4 || len(h.kinds(s, events.RunStarted)) != 1 || len(h.kinds(s, events.RunFinished)) != 1 {
		t.Fatalf("unexpected events for a finished run")
	}
	byNumber := map[int]demo.PRView{}
	for _, v := range snap.PRs {
		byNumber[v.Number] = v
	}
	if byNumber[4].Outcome != pull.Merged || byNumber[4].State != "merged" || byNumber[3].Outcome != pull.NeedsHuman || byNumber[1].Blocker != pull.Behind {
		t.Fatalf("snapshot = %+v", byNumber)
	}
	d, err := s.Detail(t.Context(), 3)
	if err != nil {
		t.Fatal(err)
	}
	if d.Diagnosis != "both rewrote it" || d.Reason != "too big" || d.Receipt == nil || d.Receipt.Micros != 700 || d.Receipt.BaselineMicros != 2600 || d.Receipt.Lines[0].Model != "nvidia/super" {
		t.Fatalf("detail = %+v", d)
	}
	if _, err := s.Detail(t.Context(), 99); !errors.Is(err, demo.ErrNoPull) {
		t.Fatalf("detail of an unknown request: %v", err)
	}
}

func TestTheDetailShowsTheWebSearch(t *testing.T) {
	rec := receipt.New()
	rec.AddSearch(receipt.Search{Provider: "tavily", Query: "q", Results: 4, Kept: 3, Latency: 900 * time.Millisecond, Recorded: true})
	fc := &forecast.Forecast{Module: "m", From: "v1.0.0", To: "v2.0.0", Search: &forecast.SearchTrace{Provider: "tavily", Query: "q", Kept: []string{"u"}, Recorded: true}, Releases: []string{"r"}}
	p := &processor{results: map[int]engine.Result{2: {Outcome: pull.Mergeable, Receipt: rec, Forecast: fc}}}
	h := newHarness(t, nil, p)
	s := h.session()
	if _, err := h.m.Start(s.ID); err != nil {
		t.Fatal(err)
	}
	h.waitStatus(s, demo.Finished)
	d, err := s.Detail(t.Context(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if d.Receipt == nil || len(d.Receipt.Searches) != 1 || d.Receipt.Searches[0] != (demo.ReceiptSearch{Provider: "tavily", Query: "q", Results: 4, Kept: 3, LatencyMS: 900, Recorded: true}) || len(d.Receipt.Lines) != 0 {
		t.Fatalf("receipt = %+v", d.Receipt)
	}
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"searches":[{"provider":"tavily","query":"q","results":4,"kept":3,"latency_ms":900`, `"search":{"provider":"tavily","query":"q","kept":["u"]`, `"recorded":true`, `"releases":["r"]`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the detail does not contain %s:\n%s", want, raw)
		}
	}
}

func TestSessionsDoNotShareARepository(t *testing.T) {
	p := &processor{results: map[int]engine.Result{4: {Outcome: pull.Mergeable, WantsMerge: true}}}
	h := newHarness(t, nil, p)
	a, b := h.session(), h.session()
	if _, err := h.m.Start(a.ID); err != nil {
		t.Fatal(err)
	}
	snapA := h.waitStatus(a, demo.Finished)
	if snapA.PRs[3].State != "merged" {
		t.Fatalf("a's request #4 is %s", snapA.PRs[3].State)
	}
	if snapB := b.Snapshot(t.Context()); snapB.PRs[3].State != "open" || snapB.Status != demo.Idle {
		t.Fatalf("merging in one session changed the other: %+v", snapB.PRs[3])
	}
}

func TestOnlyAFixedNumberOfRunsGoAtOnce(t *testing.T) {
	p := &processor{gate: make(chan struct{}), started: make(chan string, 16)}
	h := newHarness(t, func(o *demo.Options) { o.Limits = engine.Limits{PRsPerRun: 1, RunsPerServer: 1} }, p)
	first, second := h.session(), h.session()
	if pos, err := h.m.Start(first.ID); err != nil || pos != 0 {
		t.Fatalf("first start: position %d, error %v", pos, err)
	}
	h.waitStatus(first, demo.Running)
	pos, err := h.m.Start(second.ID)
	if err != nil || pos != 1 {
		t.Fatalf("second start: position %d, error %v", pos, err)
	}
	if snap := second.Snapshot(t.Context()); snap.Status != demo.Waiting || snap.Position != 1 {
		t.Fatalf("second snapshot = %+v", snap)
	}
	if queued := h.kinds(second, events.RunQueued); len(queued) == 0 || !strings.Contains(string(queued[0].Data), `"position":1`) {
		t.Fatalf("run.queued events = %+v", queued)
	}
	close(p.gate)
	h.waitStatus(first, demo.Finished)
	h.waitStatus(second, demo.Finished)
}

func TestTheManagerReportsTheLimitsItRunsWith(t *testing.T) {
	h := newHarness(t, func(o *demo.Options) { o.Limits = engine.Limits{RunsPerServer: 3} }, &processor{})
	if got := h.m.Limits(); got != (engine.Limits{PRsPerRun: engine.DefaultLimits.PRsPerRun, RunsPerServer: 3}) {
		t.Fatalf("limits = %+v", got)
	}
}

func TestARunCannotBeStartedTwice(t *testing.T) {
	p := &processor{gate: make(chan struct{})}
	h := newHarness(t, nil, p)
	s := h.session()
	if _, err := h.m.Start(s.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.m.Start(s.ID); !errors.Is(err, demo.ErrBusy) {
		t.Fatalf("second start: %v", err)
	}
	close(p.gate)
	h.waitStatus(s, demo.Finished)
	if _, err := h.m.Start(s.ID); !errors.Is(err, demo.ErrBusy) {
		t.Fatalf("start after the run: %v", err)
	}
	if _, err := h.m.Start("nope"); !errors.Is(err, demo.ErrUnknown) {
		t.Fatalf("start of an unknown session: %v", err)
	}
}

func TestTheDemoRefusesNewSessionsWhenEveryoneIsBusy(t *testing.T) {
	p := &processor{gate: make(chan struct{})}
	h := newHarness(t, func(o *demo.Options) { o.MaxSessions = 2 }, p)
	a, b := h.session(), h.session()
	for _, s := range []*demo.Session{a, b} {
		if _, err := h.m.Start(s.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := h.m.New(t.Context(), ""); !errors.Is(err, demo.ErrFull) {
		t.Fatalf("a third session while two are running: %v", err)
	}
	close(p.gate)
	h.waitStatus(a, demo.Finished)
	h.waitStatus(b, demo.Finished)
	h.advance(time.Minute)
	if _, err := h.m.Get(b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.m.New(t.Context(), ""); err != nil {
		t.Fatalf("a session once the oldest was finished: %v", err)
	}
	if _, err := h.m.Get(a.ID); !errors.Is(err, demo.ErrUnknown) {
		t.Fatalf("the least recently used session should have been evicted: %v", err)
	}
	if h.m.Sessions() != 2 {
		t.Fatalf("%d sessions, want 2", h.m.Sessions())
	}
}

func TestIdleSessionsExpire(t *testing.T) {
	h := newHarness(t, func(o *demo.Options) { o.TTL = time.Hour }, &processor{})
	old := h.session()
	h.advance(90 * time.Minute)
	fresh := h.session()
	if _, err := h.m.Get(old.ID); !errors.Is(err, demo.ErrUnknown) {
		t.Fatalf("an idle session past the TTL: %v", err)
	}
	if _, err := h.m.Get(fresh.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := h.hub.Get(old.ID); ok {
		t.Fatal("the expired session still has an event ring")
	}
}

func TestDiscardingAWaitingSessionDoesNotHang(t *testing.T) {
	p := &processor{gate: make(chan struct{})}
	h := newHarness(t, func(o *demo.Options) { o.Limits = engine.Limits{PRsPerRun: 1, RunsPerServer: 1} }, p)
	running, waiting := h.session(), h.session()
	if _, err := h.m.Start(running.ID); err != nil {
		t.Fatal(err)
	}
	h.waitStatus(running, demo.Running)
	if _, err := h.m.Start(waiting.ID); err != nil {
		t.Fatal(err)
	}
	ring, _ := h.hub.Get(waiting.ID)
	done := make(chan error, 1)
	go func() { done <- h.m.Discard(waiting.ID) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("discarding a session that was waiting for a slot hung")
	}
	evs, _, _ := ring.Read(0)
	if last := evs[len(evs)-1]; last.Kind != events.SessionClosed {
		t.Fatalf("the last event of a discarded session is %s", last.Kind)
	}
	if _, err := os.Stat(filepath.Join(h.m.Root(), waiting.ID)); !os.IsNotExist(err) {
		t.Fatalf("the session directory is still there: %v", err)
	}
	close(p.gate)
	h.waitStatus(running, demo.Finished)
}

func TestShutdownStopsARunningRun(t *testing.T) {
	p := &processor{gate: make(chan struct{})}
	h := newHarness(t, nil, p)
	s := h.session()
	if _, err := h.m.Start(s.ID); err != nil {
		t.Fatal(err)
	}
	h.waitStatus(s, demo.Running)
	done := make(chan struct{})
	go func() {
		h.m.Shutdown()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("shutdown did not stop the run")
	}
	if snap := s.Snapshot(t.Context()); snap.Status != demo.Finished {
		t.Fatalf("status after shutdown = %s", snap.Status)
	}
}

func TestAFailingProcessorFactoryEndsTheRunCleanly(t *testing.T) {
	h := newHarness(t, func(o *demo.Options) {
		o.Processor = func(queue.Emitter, learn.Book) (queue.Processor, error) { return nil, context.DeadlineExceeded }
	}, &processor{})
	s := h.session()
	if _, err := h.m.Start(s.ID); err != nil {
		t.Fatal(err)
	}
	h.waitStatus(s, demo.Finished)
	finished := h.kinds(s, events.RunFinished)
	if len(finished) != 1 || !strings.Contains(string(finished[0].Data), "deadline") {
		t.Fatalf("run.finished = %+v", finished)
	}
}
