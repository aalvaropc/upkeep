package queue_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/aalvaropc/upkeep/internal/engine"
	"github.com/aalvaropc/upkeep/internal/events"
	"github.com/aalvaropc/upkeep/internal/pull"
)

func TestALearnedFixIsAnnouncedAndNamedInTheReceipt(t *testing.T) {
	p := &scripted{t: t, results: map[int]engine.Result{
		1: {Outcome: pull.Mergeable, Blocker: pull.Build, Summary: "Renames the call.", Learned: &engine.Learned{From: 5, SavedCalls: 4, SavedMicros: 6400}},
		2: {Outcome: pull.Mergeable, Blocker: pull.Clean},
	}}
	r, f, log := newRunner(t, p)
	if _, err := r.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	learned := map[int]json.RawMessage{}
	for _, e := range log.kinds(events.PRDone) {
		var data struct {
			Learned json.RawMessage `json:"learned"`
		}
		if err := json.Unmarshal(e.Data, &data); err != nil {
			t.Fatal(err)
		}
		learned[e.PR] = data.Learned
	}
	if got := string(learned[1]); got != `{"from":5,"saved_calls":4,"saved_micros":6400}` {
		t.Fatalf("pr.done learned = %s", got)
	}
	if got := string(learned[2]); got != "null" {
		t.Fatalf("a pull request without a learned fix announced %s", got)
	}
	c := comments(f, 1, "receipt")
	if len(c) != 1 || !strings.Contains(c[0].Body, "Learned fix: Upkeep reused the verified fix from #5 with 0 model calls; the first time cost $0.0064.") || strings.Contains(c[0].Body, "the checks were enough") {
		t.Fatalf("receipt comment = %+v", c)
	}
}
