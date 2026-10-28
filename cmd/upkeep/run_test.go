package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/aalvaropc/upkeep/internal/events"
)

func TestProgressPrintsEveryShapeOfEventData(t *testing.T) {
	var out bytes.Buffer
	emit := progress(&out)
	emit(events.Notice, 0, map[string]string{"message": "A check call to Token Factory failed, so this run replays a recorded one."})
	emit(events.PRBlocker, 4, map[string]any{"blocker": "build", "behind": 2})
	emit(events.PRStep, 4, map[string]any{"step": "detect", "status": "running", "detail": ""})
	emit(events.PRStep, 4, map[string]any{"step": "detect", "status": "done", "detail": "go build failed"})
	emit(events.PRStep, 5, struct {
		Step   string `json:"step"`
		Status string `json:"status"`
		Detail string `json:"detail"`
	}{"verify", "failed", "a test was touched"})
	emit(events.Notice, 0, func() {})
	want := "A check call to Token Factory failed, so this run replays a recorded one.\n#4 blocked by build\n#4 detect done go build failed\n#5 verify failed a test was touched\n"
	if got := out.String(); got != want {
		t.Fatalf("progress printed\n%q\nwant\n%q", got, want)
	}
	if strings.Contains(out.String(), "<nil>") {
		t.Fatalf("progress printed a missing value: %q", out.String())
	}
}
