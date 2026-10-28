package engine

import (
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/aalvaropc/upkeep/internal/fixer"
	"github.com/aalvaropc/upkeep/internal/learn"
	"github.com/aalvaropc/upkeep/internal/pull"
)

type shelf struct {
	mu    sync.Mutex
	inner *learn.Memory
	saved []learn.Fix
}

func newShelf() *shelf { return &shelf{inner: &learn.Memory{}} }

func (s *shelf) Find(key string) (learn.Fix, bool) { return s.inner.Find(key) }

func (s *shelf) Save(fix learn.Fix) error {
	s.mu.Lock()
	s.saved = append(s.saved, fix)
	s.mu.Unlock()
	return s.inner.Save(fix)
}

func (s *shelf) Covers(scope string) bool { return s.inner.Covers(scope) }

func (s *shelf) all() []learn.Fix {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.saved)
}

const renameNote = "Renames the call to Add so the package builds again."

func learnRename(t *testing.T) (*shelf, Result) {
	t.Helper()
	book := newShelf()
	w := renameWorld(t, renameFix(), nil)
	w.nano = &scripted{steps: []step{{content: renameNote}}}
	w.book = book
	res := w.process(person("upkeep"))
	if res.Outcome != pull.Mergeable || res.Learned != nil {
		t.Fatalf("first result = %+v (%s)", res, res.Reason)
	}
	if !slices.Contains(w.steps, "learn:done") {
		t.Fatalf("steps = %v", w.steps)
	}
	return book, res
}

func (w *world) has(step string) bool { return slices.Contains(w.steps, step) }

func TestAVerifiedRepairIsLearnedAndReusedWithoutAModel(t *testing.T) {
	book, first := learnRename(t)
	saved := book.all()
	if len(saved) != 1 {
		t.Fatalf("%d fixes saved, want 1", len(saved))
	}
	fix := saved[0]
	total := first.Receipt.Totals()
	if fix.PR != 7 || fix.Repo != "example.test/m" || fix.Patch != first.Patch || fix.Note != renameNote || fix.Calls != total.Calls || fix.Micros != total.Charge.Micros || fix.Micros <= 0 {
		t.Fatalf("saved fix = %+v, first receipt %+v", fix, total)
	}
	if len(fix.Edits) != 1 || fix.Edits[0] != (fixer.Edit{Path: "caller.go", Old: "Add(1, 2)", New: "Sum(1, 2)"}) || !strings.Contains(fix.Signature, "caller.go: undefined: Add") || fix.At.IsZero() {
		t.Fatalf("saved fix = %+v", fix)
	}

	w := renameWorld(t, nil, nil)
	w.nano = &scripted{}
	w.book = book
	pr := person("upkeep")
	pr.Number = 8
	res := w.process(pr)
	if res.Outcome != pull.Mergeable || res.Err != nil || !res.Changed || !res.WantsPush {
		t.Fatalf("result = %+v (%s)", res, res.Reason)
	}
	if w.super.count()+w.ultra.count()+w.nano.count() != 0 || res.Receipt.Totals().Calls != 0 {
		t.Fatalf("a learned fix called a model: super %d ultra %d nano %d", w.super.count(), w.ultra.count(), w.nano.count())
	}
	if res.Attempts != 0 || res.Receipt.Attempts() != 0 {
		t.Fatalf("a learned fix counted %d attempts, the receipt %d", res.Attempts, res.Receipt.Attempts())
	}
	if res.Learned == nil || res.Learned.From != 7 || res.Learned.SavedMicros != fix.Micros || res.Learned.SavedCalls != fix.Calls {
		t.Fatalf("learned = %+v", res.Learned)
	}
	if res.Summary != renameNote || res.Patch != first.Patch {
		t.Fatalf("summary %q, patch %q", res.Summary, res.Patch)
	}
	if read(t, w.dir, "caller.go") != "package m\n\nfunc Total() int { return Sum(1, 2) }\n" || !w.clean() || w.parents("HEAD~1") != 2 {
		t.Fatalf("caller.go = %q", read(t, w.dir, "caller.go"))
	}
	subject, _ := w.git.Subjects(t.Context(), w.dir, "HEAD", 1)
	if len(subject) != 1 || subject[0] != "fix: make the checks pass again" {
		t.Fatalf("fix commit = %v", subject)
	}
	if !w.has("learned:running") || !w.has("learned:done") || w.has("repair:running") || w.has("explain:running") || w.has("learn:done") {
		t.Fatalf("steps = %v", w.steps)
	}
	for _, d := range w.detail {
		if d["step"] == "learned" && (d["from"] != 7 || d["saved_micros"] != fix.Micros) {
			t.Fatalf("learned step = %v", d)
		}
	}
	found := false
	for _, s := range res.Receipt.Steps() {
		found = found || (s.Name == "learned" && s.OK && strings.HasPrefix(s.Detail, "learned fix: 0 model calls; the first time cost $0.0"))
	}
	if !found || len(book.all()) != 1 {
		t.Fatalf("receipt steps %+v, %d fixes saved", res.Receipt.Steps(), len(book.all()))
	}
}

func TestAStaleLearnedFixFallsBackToTheRepair(t *testing.T) {
	learned, _ := learnRename(t)
	key := learned.all()[0].Key
	cases := map[string]fixer.Edit{
		"the edit no longer matches": {Path: "caller.go", Old: "Add(9, 9)", New: "Sum(9, 9)"},
		"the checks still fail":      {Path: "caller.go", Old: "Add(1, 2)", New: "Mul(1, 2)"},
	}
	for name, edit := range cases {
		t.Run(name, func(t *testing.T) {
			stale := learned.all()[0]
			stale.Edits = []fixer.Edit{edit}
			book := newShelf()
			if err := book.inner.Save(stale); err != nil {
				t.Fatal(err)
			}
			w := renameWorld(t, renameFix(), nil)
			w.book = book
			res := w.process(person("upkeep"))
			if res.Outcome != pull.Mergeable || res.Learned != nil || res.Attempts != 1 || w.super.count() != 1 {
				t.Fatalf("result = %+v (%s), %d Super calls", res, res.Reason, w.super.count())
			}
			failed := slices.Index(w.steps, "learned:failed")
			repair := slices.Index(w.steps, "repair:running")
			if failed < 0 || repair < failed || w.has("learned:done") {
				t.Fatalf("steps = %v", w.steps)
			}
			if read(t, w.dir, "caller.go") != "package m\n\nfunc Total() int { return Sum(1, 2) }\n" || !w.clean() {
				t.Fatalf("caller.go = %q", read(t, w.dir, "caller.go"))
			}
			again := book.all()
			if len(again) != 1 || again[0].Key != key || again[0].Edits[0].New != "Sum(1, 2)" {
				t.Fatalf("the repair did not replace the stale fix: %+v", again)
			}
			for _, s := range res.Receipt.Steps() {
				if s.Name == "learned" && s.OK {
					t.Fatalf("a stale fix was reported as reused: %+v", s)
				}
			}
		})
	}
}

func TestAFixTheVerifierRejectsIsNotLearned(t *testing.T) {
	w := renameWorld(t, renameFix(), nil)
	w.pol.MaxLinesChanged = 1
	book := newShelf()
	w.book = book
	res := w.process(person())
	if res.Outcome != pull.NeedsHuman || res.Checklist.Passed() {
		t.Fatalf("result = %+v", res)
	}
	if n := len(book.all()); n != 0 || w.has("learn:done") {
		t.Fatalf("%d fixes saved from a rejected change", n)
	}
}

func TestALearnedNoteIsReusedOnlyForTheSamePatch(t *testing.T) {
	learned, _ := learnRename(t)
	fix := learned.all()[0]
	fix.Patch = "diff --git a/other.go b/other.go\n"
	fix.Note = "A note about another change."
	book := newShelf()
	if err := book.inner.Save(fix); err != nil {
		t.Fatal(err)
	}
	w := renameWorld(t, nil, nil)
	w.nano = &scripted{steps: []step{{content: renameNote}}}
	w.book = book
	res := w.process(person("upkeep"))
	if res.Outcome != pull.Mergeable || res.Learned == nil || w.super.count() != 0 {
		t.Fatalf("result = %+v (%s)", res, res.Reason)
	}
	if w.nano.count() != 1 || res.Summary != renameNote {
		t.Fatalf("summary %q after %d Nano calls", res.Summary, w.nano.count())
	}
}

func TestALearnedDependencyFixSkipsTheForecastUnlessItIsStale(t *testing.T) {
	book := newShelf()
	w := renameWorld(t, renameFix(), nil)
	w.fc = newForecast()
	w.nano = &scripted{steps: []step{{content: renameNote}}}
	w.book = book
	first := w.process(dependencyPR())
	if first.Outcome != pull.Mergeable || first.Forecast == nil || w.fc.(*fakeForecaster).calls != 1 || len(book.all()) != 1 {
		t.Fatalf("first result = %+v (%s)", first, first.Reason)
	}
	fix := book.all()[0]
	if fix.Module != "example.com/lib" || fix.From != "v1.0.0" || fix.To != "v2.0.0" || !strings.HasPrefix(fix.Key, learn.Scope("example.test/m", dependencyPR().Bump)) {
		t.Fatalf("saved fix = %+v", fix)
	}

	again := renameWorld(t, nil, nil)
	again.fc = newForecast()
	again.nano = &scripted{}
	again.book = book
	res := again.process(dependencyPR())
	fc := again.fc.(*fakeForecaster)
	if res.Outcome != pull.Mergeable || res.Learned == nil || fc.calls != 0 || res.Forecast != nil || res.Receipt.Totals().Calls != 0 {
		t.Fatalf("a learned dependency fix still forecast: %d forecasts, result %+v (%s)", fc.calls, res, res.Reason)
	}

	stale := fix
	stale.Edits = []fixer.Edit{{Path: "caller.go", Old: "Add(9, 9)", New: "Sum(9, 9)"}}
	staleBook := newShelf()
	if err := staleBook.inner.Save(stale); err != nil {
		t.Fatal(err)
	}
	late := renameWorld(t, renameFix(), nil)
	late.fc = newForecast()
	late.book = staleBook
	res = late.process(dependencyPR())
	fc = late.fc.(*fakeForecaster)
	if res.Outcome != pull.Mergeable || res.Learned != nil || fc.calls != 1 || res.Forecast == nil || len(res.Confirmed) != 1 {
		t.Fatalf("a stale learned fix did not forecast before the repair: %d forecasts, result %+v", fc.calls, res)
	}
	failed, forecast, repair := slices.Index(late.steps, "learned:failed"), slices.Index(late.steps, "forecast:running"), slices.Index(late.steps, "repair:running")
	if failed < 0 || forecast < failed || repair < forecast {
		t.Fatalf("steps = %v", late.steps)
	}
	if !strings.Contains(late.super.requests[0].Messages[1].Content, "Upgrade of example.com/lib from v1.0.0 to v2.0.0.") {
		t.Fatal("the repair after a stale learned fix lost the forecast")
	}
}

func TestLearnedNotes(t *testing.T) {
	var none *Learned
	if _, ok := none.reusedNote("p"); ok {
		t.Fatal("no learned fix has no note")
	}
	l := &Learned{patch: "p", note: "n"}
	if note, ok := l.reusedNote("p"); !ok || note != "n" {
		t.Fatalf("reusedNote = %q, %v", note, ok)
	}
	if _, ok := l.reusedNote("q"); ok {
		t.Fatal("a note was reused for another patch")
	}
	if _, ok := (&Learned{patch: "p"}).reusedNote("p"); ok {
		t.Fatal("an empty note was reused")
	}
}
