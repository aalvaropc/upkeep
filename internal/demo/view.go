package demo

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/aalvaropc/upkeep/internal/forecast"
	"github.com/aalvaropc/upkeep/internal/forge"
	"github.com/aalvaropc/upkeep/internal/gitx"
	"github.com/aalvaropc/upkeep/internal/pull"
	"github.com/aalvaropc/upkeep/internal/queue"
	"github.com/aalvaropc/upkeep/internal/receipt"
	"github.com/aalvaropc/upkeep/internal/verify"
)

var ErrNoPull = errors.New("unknown pull request")

type PRView struct {
	forge.View
	Outcome pull.Outcome   `json:"outcome,omitempty"`
	Blocker pull.Blocker   `json:"blocker,omitempty"`
	Actions []queue.Action `json:"actions,omitempty"`
}

type Snapshot struct {
	ID       string    `json:"id"`
	Status   Status    `json:"status"`
	Position int       `json:"position,omitempty"`
	RunID    string    `json:"run_id,omitempty"`
	Created  time.Time `json:"created"`
	PRs      []PRView  `json:"prs"`
}

type ReceiptLine struct {
	Model            string `json:"model"`
	Role             string `json:"role"`
	Calls            int    `json:"calls"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	ReasoningTokens  int    `json:"reasoning_tokens"`
	LatencyMS        int64  `json:"latency_ms"`
	Micros           int64  `json:"micros"`
	BaselineMicros   int64  `json:"baseline_micros"`
}

type ReceiptStep struct {
	Name       string `json:"name"`
	Detail     string `json:"detail,omitempty"`
	DurationMS int64  `json:"duration_ms"`
	OK         bool   `json:"ok"`
}

type ReceiptView struct {
	Lines          []ReceiptLine `json:"lines"`
	Steps          []ReceiptStep `json:"steps"`
	Calls          int           `json:"calls"`
	Attempts       int           `json:"attempts"`
	Micros         int64         `json:"micros"`
	BaselineMicros int64         `json:"baseline_micros"`
}

type Detail struct {
	PRView
	Reason    string             `json:"reason,omitempty"`
	Diagnosis string             `json:"diagnosis,omitempty"`
	Attempts  int                `json:"attempts"`
	Patch     string             `json:"patch,omitempty"`
	Changes   []gitx.FileChange  `json:"changes,omitempty"`
	Checklist verify.Checklist   `json:"checklist,omitempty"`
	Flaky     []string           `json:"flaky,omitempty"`
	Forecast  *forecast.Forecast `json:"forecast,omitempty"`
	Confirmed []string           `json:"confirmed,omitempty"`
	Missed    []string           `json:"missed,omitempty"`
	Receipt   *ReceiptView       `json:"receipt,omitempty"`
}

func (s *Session) Snapshot(ctx context.Context) Snapshot {
	s.touch()
	s.mu.Lock()
	snap := Snapshot{ID: s.ID, Status: s.status, RunID: s.runID, Created: s.Created}
	items := make(map[int]queue.Item, len(s.items))
	for n, it := range s.items {
		items[n] = it
	}
	s.mu.Unlock()
	if snap.Status == Waiting {
		s.m.mu.Lock()
		snap.Position = slices.Index(s.m.waiting, s) + 1
		s.m.mu.Unlock()
	}
	for _, v := range s.forge.Views(ctx) {
		snap.PRs = append(snap.PRs, prView(v, items[v.Number]))
	}
	return snap
}

func prView(v forge.View, it queue.Item) PRView {
	return PRView{View: v, Outcome: it.Outcome, Blocker: it.Result.Blocker, Actions: it.Actions}
}

func (s *Session) Detail(ctx context.Context, number int) (Detail, error) {
	s.touch()
	var view forge.View
	found := false
	for _, v := range s.forge.Views(ctx) {
		if v.Number == number {
			view, found = v, true
		}
	}
	if !found {
		return Detail{}, ErrNoPull
	}
	s.mu.Lock()
	it := s.items[number]
	s.mu.Unlock()
	res := it.Result
	d := Detail{
		PRView:    prView(view, it),
		Reason:    res.Reason,
		Diagnosis: res.Diagnosis,
		Attempts:  res.Attempts,
		Patch:     res.Patch,
		Changes:   res.Changes,
		Checklist: res.Checklist,
		Flaky:     res.Flaky,
		Forecast:  res.Forecast,
		Confirmed: res.Confirmed,
		Missed:    res.Missed,
	}
	if res.Receipt != nil {
		d.Receipt = receiptView(res.Receipt)
	}
	return d, nil
}

func receiptView(r *receipt.Receipt) *ReceiptView {
	t := r.Totals()
	v := &ReceiptView{Calls: t.Calls, Attempts: r.Attempts(), Micros: t.Charge.Micros, BaselineMicros: t.Charge.BaselineMicros}
	for _, l := range r.Lines() {
		v.Lines = append(v.Lines, ReceiptLine{
			Model: l.Model, Role: string(l.Role), Calls: l.Calls,
			PromptTokens: l.Usage.PromptTokens, CompletionTokens: l.Usage.CompletionTokens, ReasoningTokens: l.Usage.ReasoningTokens,
			LatencyMS: l.Latency.Milliseconds(), Micros: l.Charge.Micros, BaselineMicros: l.Charge.BaselineMicros,
		})
	}
	for _, st := range r.Steps() {
		v.Steps = append(v.Steps, ReceiptStep{Name: st.Name, Detail: st.Detail, DurationMS: st.Duration.Milliseconds(), OK: st.OK})
	}
	return v
}
