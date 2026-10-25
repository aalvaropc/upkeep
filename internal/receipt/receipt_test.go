package receipt

import (
	"sync"
	"testing"
	"time"

	"github.com/aalvaropc/upkeep/internal/model"
)

func TestLinesAndTotals(t *testing.T) {
	r := New()
	u := model.Usage{PromptTokens: 1000, CompletionTokens: 100}
	r.AddCall(model.Nano, "nano", u, model.Charge{Micros: 60, BaselineMicros: 1300, Known: true}, time.Second)
	r.AddCall(model.Super, "super", u, model.Charge{Micros: 390, BaselineMicros: 1300, Known: true}, 2*time.Second)
	r.AddCall(model.Super, "super", u, model.Charge{Micros: 390, BaselineMicros: 1300, Known: true}, 3*time.Second)
	r.AddCall(model.Ultra, "ultra", u, model.Charge{Micros: 1300, BaselineMicros: 1300, Known: true}, 4*time.Second)

	lines := r.Lines()
	if len(lines) != 3 || lines[0].Role != model.Ultra || lines[1].Role != model.Super || lines[2].Role != model.Nano {
		t.Fatalf("lines = %+v, want Ultra, Super, Nano in that order", lines)
	}
	if super := lines[1]; super.Calls != 2 || super.Usage.PromptTokens != 2000 || super.Charge.Micros != 780 || super.Latency != 5*time.Second {
		t.Fatalf("super = %+v", super)
	}
	total := r.Totals()
	if total.Calls != 4 || total.Usage.PromptTokens != 4000 || total.Charge.Micros != 2140 || total.Charge.BaselineMicros != 5200 || total.Latency != 10*time.Second {
		t.Fatalf("totals = %+v", total)
	}
	if total.SavedMicros() != 3060 {
		t.Fatalf("saved = %d", total.SavedMicros())
	}
}

func TestStepsAndAttemptsAreCopies(t *testing.T) {
	r := New()
	r.AddStep(Step{Name: "detect", OK: true})
	r.AddAttempt()
	r.AddAttempt()
	steps := r.Steps()
	steps[0].Name = "changed"
	if r.Steps()[0].Name != "detect" || r.Attempts() != 2 {
		t.Fatalf("steps %v attempts %d", r.Steps(), r.Attempts())
	}
}

func TestMerge(t *testing.T) {
	a, b := New(), New()
	u := model.Usage{PromptTokens: 10}
	a.AddCall(model.Super, "super", u, model.Charge{Micros: 5}, time.Second)
	b.AddCall(model.Super, "super", u, model.Charge{Micros: 7}, time.Second)
	b.AddCall(model.Nano, "nano", u, model.Charge{Micros: 1}, time.Second)
	b.AddStep(Step{Name: "x"})
	b.AddAttempt()
	a.Merge(b)
	if lines := a.Lines(); len(lines) != 2 || lines[0].Charge.Micros != 12 || lines[0].Calls != 2 {
		t.Fatalf("lines = %+v", lines)
	}
	if len(a.Steps()) != 1 || a.Attempts() != 1 {
		t.Fatalf("steps %v attempts %d", a.Steps(), a.Attempts())
	}
}

func TestConcurrentUse(t *testing.T) {
	r := New()
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			for range 100 {
				r.AddCall(model.Super, "super", model.Usage{PromptTokens: 1}, model.Charge{Micros: 1}, time.Millisecond)
				r.AddAttempt()
				_ = r.Totals()
			}
		})
	}
	wg.Wait()
	if total := r.Totals(); total.Calls != 3200 || total.Usage.PromptTokens != 3200 || r.Attempts() != 3200 {
		t.Fatalf("totals = %+v attempts %d", total, r.Attempts())
	}
}
