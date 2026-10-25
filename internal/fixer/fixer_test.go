package fixer

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/aalvaropc/upkeep/internal/events"
	"github.com/aalvaropc/upkeep/internal/model"
	"github.com/aalvaropc/upkeep/internal/sandbox"
)

func nilContext() context.Context { return context.Background() }

type step struct {
	calls   []model.ToolCall
	content string
	usage   model.Usage
	latency time.Duration
	err     error
	block   bool
}

type scriptedModel struct {
	mu       sync.Mutex
	steps    []step
	requests []model.Request
}

func (s *scriptedModel) Complete(ctx context.Context, req model.Request) (model.Response, error) {
	s.mu.Lock()
	i := len(s.requests)
	s.requests = append(s.requests, req)
	s.mu.Unlock()
	if i >= len(s.steps) {
		return model.Response{}, errors.New("the script ran out of steps")
	}
	st := s.steps[i]
	if st.block {
		<-ctx.Done()
		return model.Response{}, ctx.Err()
	}
	if st.err != nil {
		return model.Response{}, st.err
	}
	return model.Response{
		Message: model.Message{Role: "assistant", Content: st.content, Reasoning: "private thoughts", ToolCalls: st.calls},
		Usage:   st.usage,
		Latency: st.latency,
	}, nil
}

func tc(id, name string, v any) model.ToolCall {
	return model.ToolCall{ID: id, Name: name, Arguments: mustJSON(v)}
}

func mustJSON(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(data)
}

type recorder struct {
	mu     sync.Mutex
	events []events.Kind
}

func (r *recorder) emit(kind events.Kind, _ any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, kind)
}

func (r *recorder) count(kind events.Kind) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, k := range r.events {
		if k == kind {
			n++
		}
	}
	return n
}

const (
	callerBroken = "package m\n\nfunc Total() int { return Add(1, 2) }\n"
	renamedSum   = "package m\n\nfunc Sum(a, b int) int { return a + b }\n"
)

func renameModule(t *testing.T) string {
	return files(t, map[string]string{"service.go": renamedSum, "caller.go": callerBroken})
}

func newFixer(m model.Model, rec *recorder) Fixer {
	book := model.PriceBook{"super": {InputPerMillion: 0.3, OutputPerMillion: 0.9}, "ultra": {InputPerMillion: 1, OutputPerMillion: 3}}
	f := Fixer{
		Model:     m,
		ModelName: "super",
		Meter:     model.Meter{Prices: book, Baseline: "ultra"},
		Runner:    sandbox.GoRunner{Executor: sandbox.Exec{}},
		Executor:  sandbox.Exec{},
	}
	if rec != nil {
		f.Emit = rec.emit
	}
	return f
}

func failureOf(t *testing.T, dir string) string {
	t.Helper()
	report, err := (sandbox.GoRunner{Executor: sandbox.Exec{}}).Check(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	failure, found := report.FirstFailure()
	if !found {
		t.Fatal("the module should not build")
	}
	return failure.Output
}

func TestRunFixesAModuleAndStopsOnPass(t *testing.T) {
	requireGo(t)
	dir := renameModule(t)
	fail := failureOf(t, dir)
	m := &scriptedModel{steps: []step{
		{
			calls: []model.ToolCall{
				tc("1", "read_file", map[string]string{"path": "caller.go"}),
				tc("2", "apply_edit", Edit{Path: "caller.go", Old: "Add(1, 2)", New: "Sum(1, 2)"}),
			},
			usage:   model.Usage{PromptTokens: 2000, CompletionTokens: 100},
			latency: 1500 * time.Millisecond,
		},
		{calls: []model.ToolCall{tc("3", "run_checks", "{}")}, usage: model.Usage{PromptTokens: 2400, CompletionTokens: 40}, latency: 900 * time.Millisecond},
		{content: "must not be reached"},
	}}
	rec := &recorder{}
	a := newFixer(m, rec).Run(t.Context(), Task{Dir: dir, Failure: fail})

	if !a.Success || a.Stop != StopPassed || a.Turns != 2 || a.Checks != 1 || a.Err != nil {
		t.Fatalf("attempt = %+v", a)
	}
	if len(m.requests) != 2 {
		t.Fatalf("the model was called %d times; it must not be called again after PASS", len(m.requests))
	}
	if got := read(t, dir, "caller.go"); got != "package m\n\nfunc Total() int { return Sum(1, 2) }\n" {
		t.Fatalf("caller.go = %q", got)
	}
	if len(a.Edits) != 1 || a.Edits[0].Path != "caller.go" {
		t.Fatalf("edits = %+v", a.Edits)
	}
	if a.Usage.PromptTokens != 4400 || a.Usage.CompletionTokens != 140 || len(a.Calls) != 2 {
		t.Fatalf("usage = %+v calls %d", a.Usage, len(a.Calls))
	}
	if a.Calls[0].Latency != 1500*time.Millisecond || a.Calls[0].Model != "super" {
		t.Fatalf("call = %+v", a.Calls[0])
	}
	wantMicros := int64(4400*0.3 + 140*0.9)
	if !a.Charge.Known || a.Charge.Micros != wantMicros || a.Charge.BaselineMicros != int64(4400*1.0+140*3.0) {
		t.Fatalf("charge = %+v, want %d micro-USD", a.Charge, wantMicros)
	}
	if rec.count(events.ModelCall) != 2 || rec.count(events.ToolCall) != 3 {
		t.Fatalf("events = %v", rec.events)
	}
}

func TestRunSendsTheRightRequests(t *testing.T) {
	requireGo(t)
	dir := renameModule(t)
	m := &scriptedModel{steps: []step{
		{calls: []model.ToolCall{tc("a", "apply_edit", Edit{Path: "caller.go", Old: "Add(1, 2)", New: "Sum(1, 2)"})}},
		{calls: []model.ToolCall{tc("b", "run_checks", "{}")}},
	}}
	a := newFixer(m, nil).Run(t.Context(), Task{Dir: dir, Failure: failureOf(t, dir), Notes: "a note"})
	if !a.Success {
		t.Fatalf("attempt = %+v", a)
	}
	first, second := m.requests[0], m.requests[1]
	if first.Model != "super" || first.Reasoning == nil || *first.Reasoning || len(first.Tools) != 4 || first.MaxTokens <= 0 {
		t.Fatalf("first request = model %q reasoning %v tools %d", first.Model, first.Reasoning, len(first.Tools))
	}
	if first.Messages[0].Role != "system" || first.Messages[0].Content != SystemPrompt || first.Messages[1].Role != "user" {
		t.Fatalf("messages = %+v", first.Messages)
	}
	if len(second.Messages) != 4 {
		t.Fatalf("the second request has %d messages, want system, user, assistant and tool", len(second.Messages))
	}
	assistant, tool := second.Messages[2], second.Messages[3]
	if assistant.Reasoning != "" || len(assistant.ToolCalls) != 1 {
		t.Fatalf("assistant message = %+v; reasoning must not be sent back", assistant)
	}
	if tool.Role != "tool" || tool.ToolCallID != "a" || tool.Content != "ok" {
		t.Fatalf("tool message = %+v", tool)
	}
}

func TestRunRefusedEditsComeBackAsErrors(t *testing.T) {
	requireGo(t)
	dir := renameModule(t)
	m := &scriptedModel{steps: []step{
		{calls: []model.ToolCall{tc("1", "apply_edit", Edit{Path: "caller_test.go", Old: "x", New: "y"})}},
		{calls: []model.ToolCall{tc("2", "apply_edit", Edit{Path: "caller.go", Old: "Add(1, 2)", New: "Sum(1, 2) // fixed"})}},
		{calls: []model.ToolCall{tc("3", "apply_edit", Edit{Path: "caller.go", Old: "Add(1, 2)", New: "Sum(1, 2)"}), tc("4", "run_checks", "{}")}},
	}}
	a := newFixer(m, nil).Run(t.Context(), Task{Dir: dir, Failure: failureOf(t, dir)})
	if !a.Success || a.Turns != 3 {
		t.Fatalf("attempt = %+v", a)
	}
	third := m.requests[2].Messages
	if got := third[len(third)-1].Content; got != "error: this edit adds a comment; the code must have no comments" {
		t.Fatalf("the model was told %q", got)
	}
	if len(a.Edits) != 1 {
		t.Fatalf("only the accepted edit counts: %+v", a.Edits)
	}
}

func TestRunStopsAtTheTurnLimit(t *testing.T) {
	requireGo(t)
	dir := renameModule(t)
	steps := make([]step, 6)
	for i := range steps {
		steps[i] = step{calls: []model.ToolCall{tc("c", "run_checks", "{}")}, usage: model.Usage{PromptTokens: 10, CompletionTokens: 1}}
	}
	f := newFixer(&scriptedModel{steps: steps}, nil)
	f.MaxTurns = 3
	a := f.Run(t.Context(), Task{Dir: dir, Failure: failureOf(t, dir)})
	if a.Success || a.Stop != StopTurns || a.Turns != 3 || a.Checks != 3 || a.Failure == "" {
		t.Fatalf("attempt = %+v", a)
	}
}

func TestRunStopsAtTheTokenBudget(t *testing.T) {
	requireGo(t)
	dir := renameModule(t)
	steps := make([]step, 6)
	for i := range steps {
		steps[i] = step{calls: []model.ToolCall{tc("c", "read_file", map[string]string{"path": "service.go"})}, usage: model.Usage{PromptTokens: 90, CompletionTokens: 10}}
	}
	f := newFixer(&scriptedModel{steps: steps}, nil)
	f.MaxTokens = 250
	a := f.Run(t.Context(), Task{Dir: dir, Failure: failureOf(t, dir)})
	if a.Success || a.Stop != StopTokens || a.Turns != 3 || a.Usage.Total() != 300 {
		t.Fatalf("attempt = %+v", a)
	}
}

func TestRunNudgesOnceThenGivesUp(t *testing.T) {
	requireGo(t)
	dir := renameModule(t)
	m := &scriptedModel{steps: []step{{content: "I think it is fine."}, {content: "Really, it is fine."}, {content: "unreachable"}}}
	a := newFixer(m, nil).Run(t.Context(), Task{Dir: dir, Failure: failureOf(t, dir)})
	if a.Success || a.Stop != StopQuit || a.Turns != 2 || len(m.requests) != 2 {
		t.Fatalf("attempt = %+v, %d requests", a, len(m.requests))
	}
	second := m.requests[1].Messages
	if last := second[len(second)-1]; last.Role != "user" || last.Content == "" {
		t.Fatalf("the second request should end with a nudge: %+v", last)
	}
}

func TestRunNudgeCanLeadToASuccess(t *testing.T) {
	requireGo(t)
	dir := renameModule(t)
	m := &scriptedModel{steps: []step{
		{content: "Let me think."},
		{calls: []model.ToolCall{tc("1", "apply_edit", Edit{Path: "caller.go", Old: "Add(1, 2)", New: "Sum(1, 2)"}), tc("2", "run_checks", "{}")}},
	}}
	if a := newFixer(m, nil).Run(t.Context(), Task{Dir: dir, Failure: failureOf(t, dir)}); !a.Success || a.Turns != 2 {
		t.Fatalf("attempt = %+v", a)
	}
}

func TestRunReportsModelFailures(t *testing.T) {
	requireGo(t)
	dir := renameModule(t)
	boom := errors.New("token factory is down")
	a := newFixer(&scriptedModel{steps: []step{{err: boom}}}, nil).Run(t.Context(), Task{Dir: dir, Failure: "x"})
	if a.Success || a.Stop != StopModel || !errors.Is(a.Err, boom) || a.Turns != 0 {
		t.Fatalf("attempt = %+v", a)
	}
}

func TestRunStopsAtTheDeadline(t *testing.T) {
	requireGo(t)
	dir := renameModule(t)
	f := newFixer(&scriptedModel{steps: []step{{block: true}}}, nil)
	f.Deadline = 100 * time.Millisecond
	start := time.Now()
	a := f.Run(t.Context(), Task{Dir: dir, Failure: "x"})
	if a.Success || a.Stop != StopDeadline || time.Since(start) > 10*time.Second {
		t.Fatalf("attempt = %+v after %v", a, time.Since(start))
	}
}

func TestRunNeedsItsParts(t *testing.T) {
	if a := (Fixer{}).Run(t.Context(), Task{Dir: t.TempDir()}); a.Err == nil || a.Success {
		t.Fatalf("attempt = %+v", a)
	}
	f := newFixer(&scriptedModel{}, nil)
	if a := f.Run(t.Context(), Task{Dir: "/does/not/exist"}); a.Err == nil {
		t.Fatalf("a missing directory returned no error: %+v", a)
	}
}

func TestSuccessNeedsTheChecksAfterTheLastEdit(t *testing.T) {
	requireGo(t)
	dir := renameModule(t)
	m := &scriptedModel{steps: []step{
		{calls: []model.ToolCall{tc("1", "apply_edit", Edit{Path: "caller.go", Old: "Add(1, 2)", New: "Sum(1, 2)"}), tc("2", "run_checks", "{}"), tc("3", "apply_edit", Edit{Path: "caller.go", Old: "Sum(1, 2)", New: "Sum(2, 3)"})}},
		{content: "done"},
		{content: "done again"},
	}}
	a := newFixer(m, nil).Run(t.Context(), Task{Dir: dir, Failure: failureOf(t, dir)})
	if a.Success || a.Stop != StopQuit || len(a.Edits) != 2 || a.Turns != 3 {
		t.Fatalf("a pass followed by another edit must not count: %+v", a)
	}
}
