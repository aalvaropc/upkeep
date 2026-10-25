package receipt

import (
	"slices"
	"sync"
	"time"

	"github.com/aalvaropc/upkeep/internal/model"
)

type Line struct {
	Model   string        `json:"model"`
	Role    model.Role    `json:"role"`
	Calls   int           `json:"calls"`
	Usage   model.Usage   `json:"usage"`
	Charge  model.Charge  `json:"charge"`
	Latency time.Duration `json:"latency"`
}

type Step struct {
	Name     string        `json:"name"`
	Detail   string        `json:"detail,omitempty"`
	Duration time.Duration `json:"duration"`
	OK       bool          `json:"ok"`
}

type Receipt struct {
	mu       sync.Mutex
	lines    map[string]*Line
	steps    []Step
	attempts int
}

func New() *Receipt {
	return &Receipt{lines: make(map[string]*Line)}
}

func (r *Receipt) AddCall(role model.Role, name string, usage model.Usage, charge model.Charge, latency time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	l, ok := r.lines[name]
	if !ok {
		l = &Line{Model: name, Role: role}
		r.lines[name] = l
	}
	l.Calls++
	l.Usage = l.Usage.Add(usage)
	l.Charge = l.Charge.Add(charge)
	l.Latency += latency
}

func (r *Receipt) AddAttempt() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.attempts++
}

func (r *Receipt) AddStep(s Step) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.steps = append(r.steps, s)
}

var roleOrder = map[model.Role]int{model.Ultra: 0, model.Super: 1, model.Nano: 2}

func (r *Receipt) Lines() []Line {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Line, 0, len(r.lines))
	for _, l := range r.lines {
		out = append(out, *l)
	}
	slices.SortFunc(out, func(a, b Line) int {
		if c := roleOrder[a.Role] - roleOrder[b.Role]; c != 0 {
			return c
		}
		if a.Model < b.Model {
			return -1
		}
		if a.Model > b.Model {
			return 1
		}
		return 0
	})
	return out
}

func (r *Receipt) Steps() []Step {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.steps)
}

func (r *Receipt) Attempts() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.attempts
}

type Totals struct {
	Calls   int           `json:"calls"`
	Usage   model.Usage   `json:"usage"`
	Charge  model.Charge  `json:"charge"`
	Latency time.Duration `json:"latency"`
}

func (r *Receipt) Totals() Totals {
	var t Totals
	for _, l := range r.Lines() {
		t.Calls += l.Calls
		t.Usage = t.Usage.Add(l.Usage)
		t.Charge = t.Charge.Add(l.Charge)
		t.Latency += l.Latency
	}
	return t
}

func (t Totals) SavedMicros() int64 { return t.Charge.SavedMicros() }

func (r *Receipt) Merge(other *Receipt) {
	for _, l := range other.Lines() {
		r.mu.Lock()
		mine, ok := r.lines[l.Model]
		if !ok {
			copyOf := l
			r.lines[l.Model] = &copyOf
		} else {
			mine.Calls += l.Calls
			mine.Usage = mine.Usage.Add(l.Usage)
			mine.Charge = mine.Charge.Add(l.Charge)
			mine.Latency += l.Latency
		}
		r.mu.Unlock()
	}
	r.mu.Lock()
	r.steps = append(r.steps, other.Steps()...)
	r.attempts += other.Attempts()
	r.mu.Unlock()
}
