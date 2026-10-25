package model

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aalvaropc/upkeep/internal/fixture"
)

func newStore(t *testing.T) fixture.Store { return fixture.Dir{Path: t.TempDir()} }

type scripted struct {
	mu    sync.Mutex
	calls int
	resp  Response
	err   error
}

func (s *scripted) Complete(context.Context, Request) (Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return s.resp, s.err
}

func request(content string) Request {
	off := false
	return Request{
		Model:     "nvidia/nemotron-3-super-120b-a12b",
		MaxTokens: 600,
		Reasoning: &off,
		Messages:  []Message{{Role: "system", Content: "fix it"}, {Role: "user", Content: content}},
		Tools:     []Tool{{Name: "run_checks", Description: "Run the checks."}},
	}
}

func TestNormalizeRemovesWhatChangesBetweenRuns(t *testing.T) {
	a := "ok  \tgithub.com/x/auth\t0.514s\nok  \tgithub.com/x/api\t(cached)\nfailed in /tmp/run-1000/work-7f3a/auth/token.go:24:6 after 1.5s"
	b := "ok  \tgithub.com/x/auth\t1.022s\nok  \tgithub.com/x/api\t\nfailed in /tmp/other/dir-99/auth/token.go:24:6 after 12ms"
	if Normalize(a) != Normalize(b) {
		t.Fatalf("normalized texts differ:\n%q\n%q", Normalize(a), Normalize(b))
	}
	for _, kept := range []string{"auth/token.go:24:6", "undefined: jwt.StandardClaims", "max_seconds = 180"} {
		if !strings.Contains(Normalize("x "+kept+" y"), kept) {
			t.Errorf("Normalize dropped %q", kept)
		}
	}
	if Normalize("  padded \n") != "padded" {
		t.Error("Normalize should trim")
	}
}

func TestRequestKey(t *testing.T) {
	base, err := RequestKey(request("tests failed in 0.5s at /tmp/a/b"))
	if err != nil {
		t.Fatal(err)
	}
	same, _ := RequestKey(request("tests failed in 9.8s at /tmp/zzz/yyy"))
	if base != same || len(base) != 64 {
		t.Fatalf("keys differ for requests that only differ in volatile text: %s %s", base, same)
	}
	for name, mutate := range map[string]func(*Request){
		"content":    func(r *Request) { r.Messages[1].Content = "another problem" },
		"model":      func(r *Request) { r.Model = "nvidia/Nemotron-3-Ultra-550b-a55b" },
		"max tokens": func(r *Request) { r.MaxTokens = 700 },
		"tool":       func(r *Request) { r.Tools[0].Name = "other" },
		"reasoning":  func(r *Request) { r.Reasoning = nil },
		"extra turn": func(r *Request) { r.Messages = append(r.Messages, Message{Role: "assistant", Content: "ok"}) },
		"tool result": func(r *Request) {
			r.Messages = append(r.Messages, Message{Role: "tool", ToolCallID: "c1", Content: "x"})
		},
	} {
		r := request("tests failed in 0.5s at /tmp/a/b")
		mutate(&r)
		got, _ := RequestKey(r)
		if got == base {
			t.Errorf("changing the %s did not change the key", name)
		}
	}
	withReasoning := request("tests failed in 0.5s at /tmp/a/b")
	withReasoning.Messages[1].Reasoning = "something the model thought"
	if got, _ := RequestKey(withReasoning); got != base {
		t.Error("earlier reasoning must not change the key")
	}
}

func TestRecordThenReplay(t *testing.T) {
	store := newStore(t)
	want := Response{
		Message:      Message{Role: "assistant", Content: "done", Reasoning: "short", ToolCalls: []ToolCall{{ID: "c1", Name: "run_checks", Arguments: "{}"}}},
		Usage:        Usage{PromptTokens: 1200, CompletionTokens: 80, ReasoningTokens: 30, CachedPromptTokens: 400},
		Latency:      1800 * time.Millisecond,
		FinishReason: "tool_calls",
	}
	inner := &scripted{resp: want}
	at := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	rec := &Recorder{Inner: inner, Fixtures: store, Now: func() time.Time { return at }}

	got, err := rec.Complete(t.Context(), request("tests failed in 0.5s"))
	if err != nil {
		t.Fatal(err)
	}
	if inner.calls != 1 || got.Message.Content != "done" {
		t.Fatalf("recorder: %d calls, response %+v", inner.calls, got)
	}

	replayed, err := (&Replayer{Fixtures: store}).Complete(t.Context(), request("tests failed in 7.9s"))
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Message.Content != want.Message.Content || replayed.Message.Reasoning != "short" ||
		len(replayed.Message.ToolCalls) != 1 || replayed.Message.ToolCalls[0] != want.Message.ToolCalls[0] ||
		replayed.Usage != want.Usage || replayed.Latency != want.Latency || replayed.FinishReason != "tool_calls" {
		t.Fatalf("replayed = %+v, want %+v", replayed, want)
	}

	var exchange Exchange
	key, _ := RequestKey(request("tests failed in 0.5s"))
	if err := store.Load(FixtureKind, key, &exchange); err != nil {
		t.Fatal(err)
	}
	if !exchange.RecordedAt.Equal(at) || exchange.Model != "nvidia/nemotron-3-super-120b-a12b" || !strings.Contains(exchange.Request.Messages[1].Content, "<duration>") {
		t.Fatalf("exchange = %+v", exchange)
	}
}

func TestReplayMissIsExplicit(t *testing.T) {
	_, err := (&Replayer{Fixtures: newStore(t)}).Complete(t.Context(), request("never recorded"))
	if !errors.Is(err, fixture.ErrMiss) || !strings.Contains(err.Error(), "nvidia/nemotron-3-super-120b-a12b") {
		t.Fatalf("error = %v, want a miss that names the model", err)
	}
}

func TestRecorderDoesNotSaveFailures(t *testing.T) {
	store := newStore(t)
	boom := errors.New("boom")
	rec := &Recorder{Inner: &scripted{err: boom}, Fixtures: store}
	if _, err := rec.Complete(t.Context(), request("x")); !errors.Is(err, boom) {
		t.Fatalf("error = %v", err)
	}
	if _, err := (&Replayer{Fixtures: store}).Complete(t.Context(), request("x")); !errors.Is(err, fixture.ErrMiss) {
		t.Fatalf("a failed call was recorded: %v", err)
	}
}

func TestMisconfiguration(t *testing.T) {
	if _, err := (&Recorder{}).Complete(t.Context(), request("x")); err == nil {
		t.Error("a recorder with nothing to call returned no error")
	}
	if _, err := (&Replayer{}).Complete(t.Context(), request("x")); err == nil {
		t.Error("a replayer without fixtures returned no error")
	}
}

func TestReplayPacing(t *testing.T) {
	store := newStore(t)
	rec := &Recorder{Inner: &scripted{resp: Response{Message: Message{Role: "assistant", Content: "ok"}, Latency: 4 * time.Second}}, Fixtures: store}
	if _, err := rec.Complete(t.Context(), request("x")); err != nil {
		t.Fatal(err)
	}
	var slept []time.Duration
	rep := &Replayer{Fixtures: store, Pace: 0.25, Sleep: func(_ context.Context, d time.Duration) error {
		slept = append(slept, d)
		return nil
	}}
	if _, err := rep.Complete(t.Context(), request("x")); err != nil {
		t.Fatal(err)
	}
	if len(slept) != 1 || slept[0] != time.Second {
		t.Fatalf("slept %v, want one second at a quarter of the recorded 4s", slept)
	}
	slept = nil
	instant := &Replayer{Fixtures: store, Sleep: func(_ context.Context, d time.Duration) error {
		slept = append(slept, d)
		return nil
	}}
	if _, err := instant.Complete(t.Context(), request("x")); err != nil || len(slept) != 0 {
		t.Fatalf("pace zero should not sleep: %v %v", err, slept)
	}
}

func TestReplayPacingStopsWithTheContext(t *testing.T) {
	store := newStore(t)
	rec := &Recorder{Inner: &scripted{resp: Response{Message: Message{Role: "assistant", Content: "ok"}, Latency: time.Hour}}, Fixtures: store}
	if _, err := rec.Complete(t.Context(), request("x")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := (&Replayer{Fixtures: store, Pace: 1}).Complete(ctx, request("x"))
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 5*time.Second {
		t.Fatalf("error %v after %v", err, time.Since(start))
	}
}

func TestSleepContext(t *testing.T) {
	if err := sleepContext(t.Context(), 0); err != nil {
		t.Fatal(err)
	}
	if err := sleepContext(t.Context(), time.Millisecond); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := sleepContext(ctx, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled context with no wait: %v", err)
	}
}

func TestConcurrentRecording(t *testing.T) {
	store := newStore(t)
	rec := &Recorder{Inner: &scripted{resp: Response{Message: Message{Role: "assistant", Content: "ok"}}}, Fixtures: store}
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Go(func() {
			if _, err := rec.Complete(t.Context(), request(strings.Repeat("x", i+1))); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	for i := range 16 {
		if _, err := (&Replayer{Fixtures: store}).Complete(t.Context(), request(strings.Repeat("x", i+1))); err != nil {
			t.Errorf("request %d: %v", i, err)
		}
	}
}
