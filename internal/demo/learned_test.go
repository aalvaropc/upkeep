package demo_test

import (
	"sync"
	"testing"

	"github.com/aalvaropc/upkeep/internal/demo"
	"github.com/aalvaropc/upkeep/internal/engine"
	"github.com/aalvaropc/upkeep/internal/fixer"
	"github.com/aalvaropc/upkeep/internal/learn"
	"github.com/aalvaropc/upkeep/internal/pull"
	"github.com/aalvaropc/upkeep/internal/queue"
)

func TestANewSessionInheritsWhatTheOldOneLearned(t *testing.T) {
	var mu sync.Mutex
	var books []learn.Book
	p := &processor{results: map[int]engine.Result{1: {Outcome: pull.Mergeable, Blocker: pull.Build, Learned: &engine.Learned{From: 5, SavedMicros: 6400}}}}
	h := newHarness(t, func(o *demo.Options) {
		o.Processor = func(_ queue.Emitter, book learn.Book) (queue.Processor, error) {
			mu.Lock()
			books = append(books, book)
			mu.Unlock()
			return p, nil
		}
	}, p)
	run := func(s *demo.Session) learn.Book {
		t.Helper()
		if _, err := h.m.Start(s.ID); err != nil {
			t.Fatal(err)
		}
		h.waitStatus(s, demo.Finished)
		mu.Lock()
		defer mu.Unlock()
		return books[len(books)-1]
	}

	first := h.session()
	learned := run(first)
	if learned == nil {
		t.Fatal("a session runs without a book of learned fixes")
	}
	fix := learn.Fix{Key: "k", PR: 5, Micros: 6400, Edits: []fixer.Edit{{Path: "a.go", Old: "x", New: "y"}}}
	if err := learned.Save(fix); err != nil {
		t.Fatal(err)
	}

	heir, err := h.m.New(t.Context(), first.ID)
	if err != nil {
		t.Fatal(err)
	}
	inherited := run(heir)
	if got, ok := inherited.Find("k"); !ok || got.PR != 5 || got.Micros != 6400 {
		t.Fatalf("the new session did not inherit the fix: %+v, %v", got, ok)
	}
	if err := inherited.Save(learn.Fix{Key: "only-in-the-heir", PR: 6}); err != nil {
		t.Fatal(err)
	}
	if _, ok := learned.Find("only-in-the-heir"); ok {
		t.Fatal("the new session writes into the old session's fixes")
	}

	detail, err := heir.Detail(t.Context(), 1)
	if err != nil || detail.Learned == nil || detail.Learned.From != 5 || detail.Learned.SavedMicros != 6400 {
		t.Fatalf("the detail does not say which fix was reused: %+v, %v", detail.Learned, err)
	}

	if err := h.m.Discard(first.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := inherited.Find("k"); !ok {
		t.Fatal("discarding the old session took the inherited fixes with it")
	}

	for _, inherit := range []string{"", "no-such-session"} {
		fresh, err := h.m.New(t.Context(), inherit)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := run(fresh).Find("k"); ok {
			t.Fatalf("a session created with inherit %q knows fixes it never learned", inherit)
		}
	}
}
