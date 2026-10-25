package events

import (
	"slices"
	"sync"
	"testing"
	"time"
)

func seqs(evs []Event) []uint64 {
	out := make([]uint64, len(evs))
	for i, e := range evs {
		out[i] = e.Seq
	}
	return out
}

func equal(a, b []uint64) bool { return slices.Equal(a, b) }

func TestAppendAssignsSequenceAndTime(t *testing.T) {
	r := NewRing(4)
	fixed := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	r.now = func() time.Time { return fixed }
	a := r.Append(Event{Kind: RunStarted, Run: "r1"})
	b := r.Append(Event{Kind: PRStep, Run: "r1", PR: 7})
	if a.Seq != 1 || b.Seq != 2 {
		t.Fatalf("seqs = %d, %d; want 1, 2", a.Seq, b.Seq)
	}
	if !a.Time.Equal(fixed) {
		t.Fatalf("time = %v, want %v", a.Time, fixed)
	}
	if r.Last() != 2 {
		t.Fatalf("Last() = %d, want 2", r.Last())
	}
}

func TestReadReturnsOnlyNewerEvents(t *testing.T) {
	r := NewRing(10)
	for range 5 {
		r.Append(Event{Kind: PRStep})
	}
	evs, missed, _ := r.Read(2)
	if missed || !equal(seqs(evs), []uint64{3, 4, 5}) {
		t.Fatalf("Read(2) = %v missed=%v", seqs(evs), missed)
	}
	evs, missed, _ = r.Read(5)
	if missed || len(evs) != 0 {
		t.Fatalf("Read(5) = %v missed=%v", seqs(evs), missed)
	}
}

func TestReadEmptyRing(t *testing.T) {
	r := NewRing(3)
	if evs, missed, _ := r.Read(0); missed || len(evs) != 0 {
		t.Fatalf("Read(0) on empty ring = %v missed=%v", seqs(evs), missed)
	}
	if _, missed, _ := r.Read(9); !missed {
		t.Fatal("Read(9) on an empty ring should report missed events")
	}
}

func TestWrapAroundDropsOldest(t *testing.T) {
	r := NewRing(3)
	for range 5 {
		r.Append(Event{Kind: ToolCall})
	}
	evs, missed, _ := r.Read(0)
	if !missed || !equal(seqs(evs), []uint64{3, 4, 5}) {
		t.Fatalf("Read(0) = %v missed=%v; want [3 4 5] missed", seqs(evs), missed)
	}
	evs, missed, _ = r.Read(2)
	if missed || !equal(seqs(evs), []uint64{3, 4, 5}) {
		t.Fatalf("Read(2) = %v missed=%v; want [3 4 5] not missed", seqs(evs), missed)
	}
	evs, missed, _ = r.Read(1)
	if !missed || !equal(seqs(evs), []uint64{3, 4, 5}) {
		t.Fatalf("Read(1) = %v missed=%v; want [3 4 5] missed", seqs(evs), missed)
	}
}

func TestReadAfterRestartResets(t *testing.T) {
	r := NewRing(5)
	r.Append(Event{Kind: RunStarted})
	evs, missed, _ := r.Read(40)
	if !missed || !equal(seqs(evs), []uint64{1}) {
		t.Fatalf("Read(40) = %v missed=%v; want [1] missed", seqs(evs), missed)
	}
}

func TestChangedClosesOnAppend(t *testing.T) {
	r := NewRing(2)
	_, _, changed := r.Read(0)
	select {
	case <-changed:
		t.Fatal("changed closed before any append")
	default:
	}
	r.Append(Event{Kind: Action})
	select {
	case <-changed:
	case <-time.After(time.Second):
		t.Fatal("changed did not close after append")
	}
	_, _, next := r.Read(1)
	select {
	case <-next:
		t.Fatal("the new changed channel is already closed")
	default:
	}
}

func TestConcurrentWriters(t *testing.T) {
	const writers, perWriter = 8, 1000
	r := NewRing(writers * perWriter)
	var wg sync.WaitGroup
	stop := make(chan struct{})
	var readers sync.WaitGroup
	for range 2 {
		readers.Go(func() {
			var after uint64
			for {
				evs, _, changed := r.Read(after)
				for _, e := range evs {
					if e.Seq != after+1 {
						t.Errorf("reader saw seq %d after %d", e.Seq, after)
						return
					}
					after = e.Seq
				}
				select {
				case <-changed:
				case <-stop:
					return
				}
			}
		})
	}
	for range writers {
		wg.Go(func() {
			for range perWriter {
				r.Append(Event{Kind: ModelCall})
			}
		})
	}
	wg.Wait()
	close(stop)
	readers.Wait()
	evs, missed, _ := r.Read(0)
	if missed || len(evs) != writers*perWriter {
		t.Fatalf("read %d events missed=%v, want %d", len(evs), missed, writers*perWriter)
	}
	seen := make(map[uint64]bool, len(evs))
	for i, e := range evs {
		if e.Seq != uint64(i+1) || seen[e.Seq] {
			t.Fatalf("event %d has seq %d", i, e.Seq)
		}
		seen[e.Seq] = true
	}
	if r.Last() != writers*perWriter {
		t.Fatalf("Last() = %d", r.Last())
	}
}

func TestHub(t *testing.T) {
	h := NewHub(4)
	if _, ok := h.Get("s1"); ok {
		t.Fatal("Get found a ring that was never opened")
	}
	a := h.Open("s1")
	if b := h.Open("s1"); a != b {
		t.Fatal("Open returned a new ring for an open session")
	}
	if got, ok := h.Get("s1"); !ok || got != a {
		t.Fatal("Get did not return the open ring")
	}
	h.Close("s1")
	if _, ok := h.Get("s1"); ok {
		t.Fatal("Get found a closed ring")
	}
}
