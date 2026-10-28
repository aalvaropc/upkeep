package runtime

import (
	"testing"

	"github.com/aalvaropc/upkeep/internal/config"
	"github.com/aalvaropc/upkeep/internal/engine"
	"github.com/aalvaropc/upkeep/internal/learn"
)

func TestTheProcessorKeepsTheLearnedFixesOfItsRepository(t *testing.T) {
	o := options(t, config.Replay)
	o.Repo = "acme/notes"
	rt, err := New(t.Context(), o)
	if err != nil {
		t.Fatal(err)
	}
	book := &learn.Memory{}
	proc, err := rt.Processor(nil, book)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := proc.(engine.Processor)
	if !ok || p.Learned != book || p.Repo != "acme/notes" {
		t.Fatalf("processor = %#v", proc)
	}
	none, err := rt.Processor(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if none.(engine.Processor).Learned != nil {
		t.Fatal("a processor without a book still reuses fixes")
	}
}
