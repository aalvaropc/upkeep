package events

import (
	"encoding/json"
	"sync"
	"time"
)

type Kind string

const (
	RunQueued     Kind = "run.queued"
	SessionClosed Kind = "session.closed"
	RunStarted    Kind = "run.started"
	RunPlan       Kind = "run.plan"
	PRBlocker     Kind = "pr.blocker"
	PRStep        Kind = "pr.step"
	ModelCall     Kind = "model.call"
	SearchCall    Kind = "search.call"
	ToolCall      Kind = "tool.call"
	VerifyResult  Kind = "verify.result"
	Action        Kind = "action"
	Receipt       Kind = "receipt"
	PRDone        Kind = "pr.done"
	Notice        Kind = "notice"
	RunFinished   Kind = "run.finished"
)

type Event struct {
	Seq  uint64          `json:"seq"`
	Kind Kind            `json:"kind"`
	Time time.Time       `json:"time"`
	Run  string          `json:"run,omitempty"`
	PR   int             `json:"pr,omitempty"`
	Data json.RawMessage `json:"data,omitempty"`
}

type Ring struct {
	mu      sync.Mutex
	buf     []Event
	size    int
	last    uint64
	changed chan struct{}
	now     func() time.Time
}

func NewRing(capacity int) *Ring {
	if capacity < 1 {
		capacity = 1
	}
	return &Ring{
		buf:     make([]Event, capacity),
		changed: make(chan struct{}),
		now:     time.Now,
	}
}

func (r *Ring) Append(e Event) Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.last++
	e.Seq = r.last
	if e.Time.IsZero() {
		e.Time = r.now().UTC()
	}
	r.buf[(e.Seq-1)%uint64(len(r.buf))] = e
	if r.size < len(r.buf) {
		r.size++
	}
	close(r.changed)
	r.changed = make(chan struct{})
	return e
}

func (r *Ring) Read(after uint64) (events []Event, missed bool, changed <-chan struct{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	changed = r.changed
	if r.size == 0 {
		return nil, after > 0, changed
	}
	first := r.last - uint64(r.size) + 1
	start := after + 1
	switch {
	case after > r.last:
		missed, start = true, first
	case start < first:
		missed, start = true, first
	}
	for seq := start; seq <= r.last; seq++ {
		events = append(events, r.buf[(seq-1)%uint64(len(r.buf))])
	}
	return events, missed, changed
}

func (r *Ring) Last() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.last
}

type Hub struct {
	mu       sync.RWMutex
	rings    map[string]*Ring
	capacity int
}

func NewHub(capacity int) *Hub {
	return &Hub{rings: make(map[string]*Ring), capacity: capacity}
}

func (h *Hub) Open(id string) *Ring {
	h.mu.Lock()
	defer h.mu.Unlock()
	if r, ok := h.rings[id]; ok {
		return r
	}
	r := NewRing(h.capacity)
	h.rings[id] = r
	return r
}

func (h *Hub) Get(id string) (*Ring, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	r, ok := h.rings[id]
	return r, ok
}

func (h *Hub) Close(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.rings, id)
}
