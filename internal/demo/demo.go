package demo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/aalvaropc/upkeep/internal/engine"
	"github.com/aalvaropc/upkeep/internal/events"
	"github.com/aalvaropc/upkeep/internal/forge"
	"github.com/aalvaropc/upkeep/internal/gitx"
	"github.com/aalvaropc/upkeep/internal/learn"
	"github.com/aalvaropc/upkeep/internal/queue"
	"github.com/aalvaropc/upkeep/internal/scenario"
)

var (
	ErrUnknown = errors.New("unknown session")
	ErrBusy    = errors.New("this session already has a run")
	ErrFull    = errors.New("the demo is full; try again in a few minutes")
)

type Status string

const (
	Idle     Status = "idle"
	Waiting  Status = "waiting"
	Running  Status = "running"
	Finished Status = "finished"
)

type Factory func(emit queue.Emitter, learned learn.Book) (queue.Processor, error)

type Options struct {
	Scenario    scenario.Scenario
	Template    string
	Root        string
	Git         gitx.Git
	Hub         *events.Hub
	Processor   Factory
	Limits      engine.Limits
	MaxSessions int
	TTL         time.Duration
	Now         func() time.Time
	NewID       func() string
	Logger      *slog.Logger
}

type Manager struct {
	opts     Options
	ctx      context.Context
	stop     context.CancelFunc
	createMu sync.Mutex

	mu       sync.Mutex
	sessions map[string]*Session
	waiting  []*Session
	running  int
}

func randomID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func NewManager(parent context.Context, opts Options) (*Manager, error) {
	switch {
	case opts.Hub == nil:
		return nil, errors.New("demo: a hub is required")
	case opts.Processor == nil:
		return nil, errors.New("demo: a processor factory is required")
	case opts.Template == "" || opts.Root == "":
		return nil, errors.New("demo: the template and root directories are required")
	}
	if opts.Scenario.Base == "" || len(opts.Scenario.PRs) == 0 {
		return nil, errors.New("demo: the scenario needs a base branch and pull requests")
	}
	if opts.Limits.PRsPerRun <= 0 {
		opts.Limits.PRsPerRun = engine.DefaultLimits.PRsPerRun
	}
	if opts.Limits.RunsPerServer <= 0 {
		opts.Limits.RunsPerServer = engine.DefaultLimits.RunsPerServer
	}
	if opts.MaxSessions <= 0 {
		opts.MaxSessions = 50
	}
	if opts.TTL <= 0 {
		opts.TTL = 2 * time.Hour
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.NewID == nil {
		opts.NewID = randomID
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.DiscardHandler)
	}
	if err := os.MkdirAll(opts.Root, 0o750); err != nil {
		return nil, err
	}
	ctx, stop := context.WithCancel(parent)
	return &Manager{opts: opts, ctx: ctx, stop: stop, sessions: map[string]*Session{}}, nil
}

func (m *Manager) Shutdown() {
	m.stop()
	m.mu.Lock()
	all := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		all = append(all, s)
	}
	m.mu.Unlock()
	for _, s := range all {
		s.wait()
	}
}

func (m *Manager) New(ctx context.Context, inherit string) (*Session, error) {
	m.createMu.Lock()
	defer m.createMu.Unlock()

	m.mu.Lock()
	learned := &learn.Memory{}
	if old, ok := m.sessions[inherit]; ok && inherit != "" {
		learned = old.learned.Clone()
	}
	m.sweepLocked()
	if len(m.sessions) >= m.opts.MaxSessions && !m.evictOldestIdleLocked() {
		m.mu.Unlock()
		return nil, ErrFull
	}
	m.mu.Unlock()

	id := m.opts.NewID()
	dir := filepath.Join(m.opts.Root, id)
	f, err := forge.CopyLocal(ctx, m.opts.Git, m.opts.Template, filepath.Join(dir, "forge.git"), m.opts.Scenario)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("creating the session: %w", err)
	}
	s := &Session{
		ID:      id,
		Created: m.opts.Now().UTC(),
		m:       m,
		dir:     dir,
		forge:   f,
		ring:    m.opts.Hub.Open(id),
		status:  Idle,
		touched: m.opts.Now(),
		items:   map[int]queue.Item{},
		learned: learned,
	}
	m.mu.Lock()
	m.sessions[id] = s
	m.mu.Unlock()
	return s, nil
}

func (m *Manager) Get(id string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, ErrUnknown
	}
	s.touch()
	return s, nil
}

func (m *Manager) Discard(id string) error {
	m.mu.Lock()
	s, ok := m.sessions[id]
	if !ok {
		m.mu.Unlock()
		return ErrUnknown
	}
	m.removeLocked(s)
	m.mu.Unlock()
	s.wait()
	_ = os.RemoveAll(s.dir)
	return nil
}

func (m *Manager) Root() string { return m.opts.Root }

func (m *Manager) Sessions() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions)
}

func (m *Manager) removeLocked(s *Session) {
	delete(m.sessions, s.ID)
	if i := slices.Index(m.waiting, s); i >= 0 {
		m.waiting = slices.Delete(m.waiting, i, i+1)
		s.abandon()
	}
	s.emit(events.SessionClosed, 0, map[string]string{"reason": "the session ended"})
	m.opts.Hub.Close(s.ID)
	s.cancelRun()
}

func (m *Manager) sweepLocked() {
	now := m.opts.Now()
	for _, s := range slices.Collect(mapValues(m.sessions)) {
		if s.idleFor(now) > m.opts.TTL && !s.active() {
			m.removeLocked(s)
			go func() { _ = os.RemoveAll(s.dir) }()
		}
	}
}

func (m *Manager) evictOldestIdleLocked() bool {
	var oldest *Session
	for _, s := range m.sessions {
		if s.active() {
			continue
		}
		if oldest == nil || s.lastTouched().Before(oldest.lastTouched()) {
			oldest = s
		}
	}
	if oldest == nil {
		return false
	}
	m.removeLocked(oldest)
	go func() { _ = os.RemoveAll(oldest.dir) }()
	return true
}

func mapValues(m map[string]*Session) func(func(*Session) bool) {
	return func(yield func(*Session) bool) {
		for _, s := range m {
			if !yield(s) {
				return
			}
		}
	}
}

func (m *Manager) Start(id string) (int, error) {
	s, err := m.Get(id)
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	if s.status != Idle {
		s.mu.Unlock()
		return 0, ErrBusy
	}
	s.status = Waiting
	s.runID = m.opts.NewID()
	s.ctx, s.cancel = context.WithCancel(m.ctx)
	s.done = make(chan struct{})
	s.mu.Unlock()

	m.mu.Lock()
	m.waiting = append(m.waiting, s)
	m.dispatchLocked()
	position := slices.Index(m.waiting, s) + 1
	m.mu.Unlock()
	return position, nil
}

func (m *Manager) dispatchLocked() {
	for m.running < m.opts.Limits.RunsPerServer && len(m.waiting) > 0 {
		s := m.waiting[0]
		m.waiting = m.waiting[1:]
		m.running++
		go m.execute(s)
	}
	for i, s := range m.waiting {
		s.emit(events.RunQueued, 0, map[string]int{"position": i + 1})
	}
}

func (m *Manager) execute(s *Session) {
	defer func() {
		m.mu.Lock()
		m.running--
		m.dispatchLocked()
		m.mu.Unlock()
		close(s.done)
	}()
	if err := s.ctx.Err(); err != nil {
		s.emit(events.RunFinished, 0, map[string]any{"error": err.Error()})
		s.setStatus(Finished)
		return
	}
	s.setStatus(Running)
	proc, err := m.opts.Processor(s.emit, s.learned)
	if err != nil {
		m.opts.Logger.Error("preparing a run failed", "session", s.ID, "error", err)
		s.emit(events.RunFinished, 0, map[string]any{"error": err.Error()})
		s.setStatus(Finished)
		return
	}
	runner := queue.Runner{
		Forge:     s.forge,
		Processor: proc,
		Emit:      s.emit,
		Work:      filepath.Join(s.dir, "work"),
		Base:      "origin/" + m.opts.Scenario.Base,
		Parallel:  m.opts.Limits.PRsPerRun,
		Done:      s.record,
	}
	if _, err := runner.Run(s.ctx); err != nil {
		m.opts.Logger.Warn("a run ended early", "session", s.ID, "error", err)
	}
	s.setStatus(Finished)
}

type Session struct {
	ID      string
	Created time.Time
	m       *Manager
	dir     string
	forge   *forge.Local
	ring    *events.Ring
	learned *learn.Memory

	mu      sync.Mutex
	status  Status
	runID   string
	touched time.Time
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	items   map[int]queue.Item
}

func (s *Session) touch() {
	s.mu.Lock()
	s.touched = s.m.opts.Now()
	s.mu.Unlock()
}

func (s *Session) lastTouched() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.touched
}

func (s *Session) idleFor(now time.Time) time.Duration {
	return now.Sub(s.lastTouched())
}

func (s *Session) active() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status == Waiting || s.status == Running
}

func (s *Session) setStatus(st Status) {
	s.mu.Lock()
	s.status = st
	s.mu.Unlock()
}

func (s *Session) abandon() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = Finished
	if s.done != nil {
		close(s.done)
	}
}

func (s *Session) cancelRun() {
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *Session) wait() {
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()
	if done != nil {
		<-done
	}
}

func (s *Session) Wait(ctx context.Context) error {
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()
	if done == nil {
		return errors.New("this session has not started a run")
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Session) Notices() []string {
	evs, _, _ := s.ring.Read(0)
	var out []string
	for _, e := range evs {
		if e.Kind != events.Notice {
			continue
		}
		var data struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(e.Data, &data); err == nil && data.Message != "" && !slices.Contains(out, data.Message) {
			out = append(out, data.Message)
		}
	}
	return out
}

func (s *Session) Items() []queue.Item {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]queue.Item, 0, len(s.items))
	for _, it := range s.items {
		items = append(items, it)
	}
	slices.SortFunc(items, func(a, b queue.Item) int { return a.PR.Number - b.PR.Number })
	return items
}

func (s *Session) record(it queue.Item) {
	s.mu.Lock()
	s.items[it.PR.Number] = it
	s.mu.Unlock()
}

func (s *Session) emit(kind events.Kind, pr int, data any) {
	raw, err := json.Marshal(data)
	if err != nil {
		s.m.opts.Logger.Error("encoding an event failed", "kind", kind, "error", err)
		raw = []byte(`{"error":"the event could not be encoded"}`)
	}
	s.mu.Lock()
	run := s.runID
	s.mu.Unlock()
	s.ring.Append(events.Event{Kind: kind, Run: run, PR: pr, Data: raw})
}
